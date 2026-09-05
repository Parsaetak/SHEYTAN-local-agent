// Package contextplan implements SHEYTAN's explicit long-context budget
// (v1.1.3Z). Every inference request assembles its prompt from named
// sections — system instructions, tool schemas, recalled memory, attachment
// chunks, recent history, and reserved output space — and every section is
// measured, prioritized, and accounted against the model's real context
// capacity BEFORE the request is sent.
//
// The plan is pure math over token estimates: no I/O, no goroutines. The
// orchestrator calls it once per turn, windowes history to the computed
// history budget, and reports the result to the UI (provenance: the UI can
// show WHERE the prompt budget went without exposing raw prompts).
package contextplan

import (
	"fmt"
	"strings"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/chunking"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
)

// Section names, stable for the UI contract.
const (
	SectionSystem      = "system"
	SectionTools       = "tools"
	SectionRecall      = "recall"
	SectionAttachments = "attachments"
	SectionHistory     = "history"
	SectionReserve     = "output-reserve"
)

// Priority orders sections when pressure forces cuts. Lower wins.
const (
	PriorityReserve     = 0 // output space is never spent on prompt
	PrioritySystem      = 1 // the agent briefing is indispensable
	PriorityTools       = 2 // tool schemas enable the tool loop
	PriorityHistory     = 3 // the current turn lives here
	PriorityRecall      = 4 // past-exchange digests
	PriorityAttachments = 5 // retrieved attachment chunks
)

// Budget is the explicit context budget for one request.
type Budget struct {
	// Total is the model's usable context window (num_ctx).
	Total int

	// ReserveOutput is tokens held back for the model's reply.
	ReserveOutput int

	// Usable is Total - ReserveOutput; the prompt must never exceed it.
	Usable int
}

// NewBudget computes the budget from the engine context size. It clamps
// insane values and always reserves at least 512 tokens for output.
func NewBudget(numCtx, maxTokens int) Budget {
	if numCtx < 1024 {
		numCtx = 1024
	}

	if maxTokens <= 0 {
		maxTokens = 1024
	}

	if maxTokens > numCtx/2 {
		maxTokens = numCtx / 2
	}

	reserve := maxTokens
	if reserve < 512 {
		reserve = 512
	}

	return Budget{
		Total:         numCtx,
		ReserveOutput: reserve,
		Usable:        numCtx - reserve,
	}
}

// Section is the measured cost of one prompt section after assembly.
type Section struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
	Tokens   int    `json:"tokens"`
	Budget   int    `json:"budget"`
	Included bool   `json:"included"`
	Note     string `json:"note,omitempty"`
}

// Plan is the full accounting of one assembled request.
type Plan struct {
	Budget   Budget    `json:"budget"`
	Sections []Section `json:"sections"`

	// HistoryBudget is the token budget handed to the history windower.
	HistoryBudget int `json:"historyBudget"`

	// Elided counts history messages compacted by the windower.
	Elided int `json:"elided,omitempty"`

	// Recalled counts injected recall digests.
	Recalled int `json:"recalled,omitempty"`

	// Attachments counts attachments represented in the prompt.
	Attachments int `json:"attachments,omitempty"`
}

// SetSectionTokens overrides the measured token count of one section
// (used by the orchestrator after history windowing measures the actual
// windowed messages).
func (p *Plan) SetSectionTokens(name string, tokens int) {
	for i := range p.Sections {
		if p.Sections[i].Name == name {
			p.Sections[i].Tokens = tokens
			return
		}
	}
}

// TotalTokens sums the measured sections.
func (p Plan) TotalTokens() int {
	total := 0
	for _, s := range p.Sections {
		total += s.Tokens
	}
	return total
}

// Pressure returns used/usable as 0..1+.
func (p Plan) Pressure() float64 {
	if p.Budget.Usable <= 0 {
		return 0
	}
	return float64(p.TotalTokens()) / float64(p.Budget.Usable)
}

// Summary renders the compact provenance line for the UI, e.g.
// "context 3.2k/8.0k tok — system 0.4k · tools 1.1k · history 1.2k · recall 0.3k".
func (p Plan) Summary() string {
	var b strings.Builder

	fmt.Fprintf(&b, "context %s/%s tok",
		formatK(p.TotalTokens()), formatK(p.Budget.Usable))

	first := true

	for _, s := range p.Sections {
		if !s.Included || s.Tokens <= 0 || s.Name == SectionReserve {
			continue
		}

		if first {
			b.WriteString(" — ")
			first = false
		} else {
			b.WriteString(" · ")
		}

		fmt.Fprintf(&b, "%s %s", s.Name, formatK(s.Tokens))

		if s.Name == SectionHistory && p.Elided > 0 {
			fmt.Fprintf(&b, " (%d compacted)", p.Elided)
		}
	}

	return b.String()
}

func formatK(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// Input carries the measured pieces the orchestrator already has.
type Input struct {
	SystemTokens     int // AI-context briefing (+ thinking nudge)
	ToolTokens       int // serialized tool schemas
	RecallTokens     int // recall block already composed
	AttachmentTokens int // attachment representation already composed
	History          []llm.Message
	NumCtx           int
	MaxOutputTokens  int
	MinHistoryTokens int // floor so a huge current turn still gets room
}

// Assemble computes the plan: allocates budgets per section with priority
// fallbacks, and returns the history budget to window the conversation
// with. History always receives at least MinHistoryTokens (default 2048)
// even under pressure — dropping the user's current turn is never
// acceptable; instead the plan reports the overflow.
func Assemble(in Input) Plan {
	budget := NewBudget(in.NumCtx, in.MaxOutputTokens)

	minHistory := in.MinHistoryTokens
	if minHistory <= 0 {
		minHistory = 2048
	}

	used := 0

	sections := make([]Section, 0, 6)

	// 1. output reserve (always accounted, never spent).
	sections = append(sections, Section{
		Name:     SectionReserve,
		Priority: PriorityReserve,
		Tokens:   budget.ReserveOutput,
		Budget:   budget.ReserveOutput,
		Included: true,
		Note:     "reserved for the model's reply",
	})

	// 2. system instructions — always included (clamped if pathological).
	sysOK := true
	sysNote := ""
	if in.SystemTokens > budget.Usable/2 {
		sysOK = false
		sysNote = "system briefing exceeds half the window — engine context too small"
	} else {
		used += in.SystemTokens
	}

	sections = append(sections, Section{
		Name:     SectionSystem,
		Priority: PrioritySystem,
		Tokens:   in.SystemTokens,
		Budget:   budget.Usable,
		Included: sysOK,
		Note:     sysNote,
	})

	// 3. tool schemas — always included (they enable the tool loop).
	used += in.ToolTokens

	sections = append(sections, Section{
		Name:     SectionTools,
		Priority: PriorityTools,
		Tokens:   in.ToolTokens,
		Budget:   budget.Usable,
		Included: true,
	})

	// 4. history: whatever remains after fixed sections, bounded below by
	// the floor. Optional blocks (recall, attachments) do NOT inflate this
	// floor — under pressure they are dropped, not force-fitted.
	historyBudget := budget.Usable - used

	if historyBudget < minHistory {
		historyBudget = minHistory
	}

	sections = append(sections, Section{
		Name:     SectionHistory,
		Priority: PriorityHistory,
		Budget:   historyBudget,
		Included: true,
	})

	// 5. recall — included when composed and it fits alongside the history
	// floor. Under pressure it is dropped first (its facts remain available
	// through the memory tool).
	recallOK := false
	recallNote := ""

	if in.RecallTokens > 0 {
		if in.RecallTokens <= historyBudget-minHistory {
			recallOK = true
		} else {
			recallNote = "dropped under context pressure — history keeps the budget"
		}
	}

	sections = append(sections, Section{
		Name:     SectionRecall,
		Priority: PriorityRecall,
		Tokens:   in.RecallTokens,
		Budget:   historyBudget,
		Included: recallOK,
		Note:     recallNote,
	})

	// 6. attachments — same treatment as recall.
	attOK := false
	attNote := ""

	if in.AttachmentTokens > 0 {
		avail := historyBudget - minHistory
		if recallOK {
			avail -= in.RecallTokens
		}

		if in.AttachmentTokens <= avail {
			attOK = true
		} else {
			attNote = "dropped under context pressure — retrieve fewer chunks"
		}
	}

	sections = append(sections, Section{
		Name:     SectionAttachments,
		Priority: PriorityAttachments,
		Tokens:   in.AttachmentTokens,
		Budget:   historyBudget,
		Included: attOK,
		Note:     attNote,
	})

	plan := Plan{
		Budget:        budget,
		Sections:      sections,
		HistoryBudget: historyBudget,
	}

	if recallOK || attOK {
		// Fixed sections + injected blocks must leave the floor for history;
		// the windower works within what actually remains.
		remaining := budget.Usable - in.SystemTokens - in.ToolTokens
		if recallOK {
			remaining -= in.RecallTokens
		}
		if attOK {
			remaining -= in.AttachmentTokens
		}
		if remaining < minHistory {
			remaining = minHistory
		}
		plan.HistoryBudget = remaining
	}

	return plan
}

// EstimateTokens delegates to the shared estimator (single source of
// truth for token math).
func EstimateTokens(s string) int {
	return chunking.EstimateTokens(s)
}
