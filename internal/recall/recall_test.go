package recall

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sessions"
)

func TestIndexSearchDedup(t *testing.T) {
	dir := t.TempDir()
	e := New(dir)

	if err := e.IndexTurn("s1", "Go", "how to read a file in go", "use os.ReadFile", nil); err != nil {
		t.Fatal(err)
	}
	if err := e.IndexTurn("s1", "Go", "how to read a file in go", "use os.ReadFile", nil); err != nil {
		t.Fatal(err)
	}
	if got := e.Count(); got != 1 {
		t.Fatalf("dedup failed: %d capsules", got)
	}

	hits := e.Search("read file go", 2)
	if len(hits) != 1 || hits[0].SessionID != "s1" {
		t.Fatalf("search miss: %+v", hits)
	}
}

func TestBackfillOnce(t *testing.T) {
	dir := t.TempDir()
	store := sessions.New(filepath.Join(dir, "sessions"))
	s := store.Create()
	s.Title = "recipes"
	s.Messages = []llm.Message{
		{Role: "user", Content: "best pizza dough"},
		{Role: "assistant", Content: "flour water yeast salt, 24h cold rise"},
	}
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}

	e := New(dir)
	if err := e.Backfill(store); err != nil {
		t.Fatal(err)
	}
	if got := e.Count(); got != 1 {
		t.Fatalf("backfill count = %d, want 1", got)
	}
	// Second run is a no-op.
	if err := e.Backfill(store); err != nil {
		t.Fatal(err)
	}
	if got := e.Count(); got != 1 {
		t.Fatalf("backfill ran twice: %d", got)
	}
}

func TestRelevantBlockBounded(t *testing.T) {
	dir := t.TempDir()
	e := New(dir)
	long := strings.Repeat("deep learning gradient descent optimizer adam ", 30)
	if err := e.IndexTurn("s1", "ml", "explain "+long, strings.Repeat("answer ", 100), nil); err != nil {
		t.Fatal(err)
	}
	block := e.RelevantBlock("gradient descent", 1, 120) // tiny budget
	if block != "" && len(block) > 120*8 {               // generous byte ceiling
		t.Errorf("block not bounded: %d bytes", len(block))
	}
	// With a sane budget the capsule appears.
	block = e.RelevantBlock("gradient descent", 1, 600)
	if !strings.Contains(block, "RELEVANT PAST CONTEXT") {
		t.Errorf("block missing header: %q", block)
	}
}

func TestTokenize(t *testing.T) {
	terms := Tokenize("The Quick brown fox, and the lazy DOG!")
	want := []string{"quick", "brown", "fox", "lazy", "dog"}
	if len(terms) != len(want) {
		t.Fatalf("tokenize = %v, want %v", terms, want)
	}
	for i := range want {
		if terms[i] != want[i] {
			t.Errorf("term %d = %s, want %s", i, terms[i], want[i])
		}
	}
}
