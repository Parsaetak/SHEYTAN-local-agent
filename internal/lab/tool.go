package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sheytan/local-agent/internal/config"
)

// Tool exposes the Coding Lab through the orchestrator's generic tool system.
//
// It intentionally provides a single stable tool surface instead of exposing
// filesystem/process primitives independently to the model. The Lab itself
// remains responsible for workspace isolation, policy enforcement, execution,
// and verification.
type Tool struct {
	cfg      *config.Config
	tasks    *TaskManager
	verifier *Verifier
}

// NewTool creates the Coding Lab orchestrator tool.
func NewTool(cfg *config.Config) (*Tool, error) {
	if cfg == nil {
		return nil, errors.New("lab: config is nil")
	}

	if !cfg.LabEnabled {
		return nil, errors.New("lab: Coding Lab is disabled")
	}

	timeout := cfg.EffectiveLabCommandTimeout()
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	workspaceRoot := strings.TrimSpace(cfg.LabWorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = cfg.LabDir() + string(pathSeparator())
	}

	workspaces, err := NewWorkspaceManager(workspaceRoot)
	if err != nil {
		return nil, err
	}

	runner := NewRunner(
		timeout,
		2*1024*1024,
	)

	policy := DefaultPolicy()
	policy.AllowNetwork = cfg.LabAllowNetwork

	tasks, err := NewTaskManager(
		workspaces,
		runner,
		policy,
		cfg.LabKeepWorkspaces,
	)
	if err != nil {
		return nil, err
	}

	verifier, err := NewVerifier(tasks)
	if err != nil {
		return nil, err
	}

	return &Tool{
		cfg:      cfg,
		tasks:    tasks,
		verifier: verifier,
	}, nil
}

// Name implements the agent.Tool interface.
func (t *Tool) Name() string {
	return "coding_lab"
}

// Description implements the agent.Tool interface.
func (t *Tool) Description() string {
	return `Use the isolated Coding Lab to work on a local code project.

The Lab creates a disposable workspace, executes permitted commands, and can
run objective verification checks. Use it for inspecting, building, testing,
diagnosing, and fixing code.

Available actions:
- start_task: create an isolated workspace from a local source directory
- run: execute one permitted command inside the task workspace
- verify: run objective build/test/check commands
- finish: mark the task successful after verification
- fail: mark the task failed
- cancel: cancel the task
- block: mark the task blocked
- close: release the task workspace

Important:
- The Lab does not assume that a successful command means the project works.
- Verification should be used before finish.
- Network access is disabled by default unless explicitly enabled by policy.
- Interactive and dangerous commands are blocked by policy.`
}

// Parameters implements the agent.Tool interface.
func (t *Tool) Parameters() any {
	return codingLabParameters{}
}

// Run implements the agent.Tool interface.
func (t *Tool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	if t == nil || t.tasks == nil || t.verifier == nil {
		return "", errors.New("lab: tool is not initialized")
	}

	var request codingLabRequest

	if len(args) == 0 {
		return "", errors.New("lab: tool arguments are empty")
	}

	if err := json.Unmarshal(args, &request); err != nil {
		return "", fmt.Errorf("lab: invalid tool arguments: %w", err)
	}

	request.Action = strings.ToLower(strings.TrimSpace(request.Action))

	switch request.Action {
	case "start_task":
		return t.startTask(ctx, request)
	case "run":
		return t.runCommand(ctx, request)
	case "verify":
		return t.verifyTask(ctx, request)
	case "finish":
		return t.finishTask(request)
	case "fail":
		return t.failTask(request)
	case "cancel":
		return t.cancelTask(request)
	case "block":
		return t.blockTask(request)
	case "close":
		return t.closeTask(request)
	default:
		return "", fmt.Errorf(
			"lab: unknown action %q; expected start_task, run, verify, finish, fail, cancel, block, or close",
			request.Action,
		)
	}
}

type codingLabRequest struct {
	Action string `json:"action"`

	TaskID string `json:"taskId,omitempty"`

	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`

	Command      string        `json:"command,omitempty"`
	WorkingDir  string        `json:"workingDir,omitempty"`
	Environment []string      `json:"environment,omitempty"`
	TimeoutSec  int           `json:"timeoutSec,omitempty"`
	MaxOutputMB float64       `json:"maxOutputMB,omitempty"`

	BuildCommand string `json:"buildCommand,omitempty"`
	TestCommand  string `json:"testCommand,omitempty"`

	Checks []codingLabCheck `json:"checks,omitempty"`

	Error  string `json:"error,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type codingLabCheck struct {
	Name         string `json:"name,omitempty"`
	Command      string `json:"command"`
	WorkingDir   string `json:"workingDir,omitempty"`
	Required     bool   `json:"required"`
	AllowFailure bool   `json:"allowFailure,omitempty"`
	TimeoutSec   int    `json:"timeoutSec,omitempty"`
}

type codingLabParameters struct {
	Type       string                    `json:"type"`
	Properties map[string]labToolSchema  `json:"properties"`
	Required   []string                  `json:"required"`
}

type labToolSchema struct {
	Type        string                    `json:"type,omitempty"`
	Description string                    `json:"description,omitempty"`
	Enum        []string                  `json:"enum,omitempty"`
	Items       *labToolSchema            `json:"items,omitempty"`
}

func (t *Tool) startTask(
	ctx context.Context,
	request codingLabRequest,
) (string, error) {
	title := strings.TrimSpace(request.Title)
	description := strings.TrimSpace(request.Description)
	source := strings.TrimSpace(request.Source)

	if source == "" {
		return "", ErrInvalidSource
	}

	task := t.tasks.NewTask(title, description)

	if err := t.tasks.Start(ctx, task, source); err != nil {
		return encodeLabResponse(
			codingLabResponse{
				OK:     false,
				Action: "start_task",
				Task:   task,
				Error:  err.Error(),
			},
		), err
	}

	return encodeLabResponse(
		codingLabResponse{
			OK:     true,
			Action: "start_task",
			Task:   task,
			Message: "Coding Lab task started in an isolated workspace.",
		},
	)
}

func (t *Tool) runCommand(
	ctx context.Context,
	request codingLabRequest,
) (string, error) {
	task, err := t.lookupTask(request.TaskID)
	if err != nil {
		return "", err
	}

	command := Command{
		Command:      strings.TrimSpace(request.Command),
		WorkingDir:   strings.TrimSpace(request.WorkingDir),
		Environment: append([]string(nil), request.Environment...),
	}

	if request.TimeoutSec > 0 {
		command.Timeout = time.Duration(request.TimeoutSec) * time.Second
	}

	if request.MaxOutputMB > 0 {
		command.MaxOutputBytes = int64(request.MaxOutputMB * 1024 * 1024)
	}

	result, err := t.tasks.RunCommand(ctx, task, command)

	response := codingLabResponse{
		OK:      err == nil && result.Success,
		Action:  "run",
		Task:    task,
		Result:  &result,
	}

	if err != nil {
		response.Error = err.Error()
	}

	encoded, encodeErr := encodeLabResponse(response)
	if encodeErr != nil {
		return "", encodeErr
	}

	return encoded, err
}

func (t *Tool) verifyTask(
	ctx context.Context,
	request codingLabRequest,
) (string, error) {
	task, err := t.lookupTask(request.TaskID)
	if err != nil {
		return "", err
	}

	var summary VerificationSummary

	switch {
	case len(request.Checks) > 0:
		checks := make([]VerificationCheck, 0, len(request.Checks))

		for index, check := range request.Checks {
			command := strings.TrimSpace(check.Command)
			if command == "" {
				return "", fmt.Errorf("lab: verification check %d has an empty command", index+1)
			}

			verificationCommand := Command{
				Command:    command,
				WorkingDir: strings.TrimSpace(check.WorkingDir),
			}

			if check.TimeoutSec > 0 {
				verificationCommand.Timeout =
					time.Duration(check.TimeoutSec) * time.Second
			}

			checks = append(checks, VerificationCheck{
				Name:         strings.TrimSpace(check.Name),
				Command:      verificationCommand,
				Required:     check.Required,
				AllowFailure: check.AllowFailure,
			})
		}

		summary, err = t.verifier.Verify(
			ctx,
			task,
			checks,
		)

	case strings.TrimSpace(request.BuildCommand) != "" ||
		strings.TrimSpace(request.TestCommand) != "":
		summary, err = t.verifier.VerifyStandard(
			ctx,
			task,
			request.BuildCommand,
			request.TestCommand,
		)

	default:
		return "", ErrVerificationEmpty
	}

	response := codingLabResponse{
		OK:         err == nil && summary.Passed,
		Action:     "verify",
		Task:       task,
		Verification: &summary,
	}

	if err != nil {
		response.Error = err.Error()
	}

	encoded, encodeErr := encodeLabResponse(response)
	if encodeErr != nil {
		return "", encodeErr
	}

	return encoded, err
}

func (t *Tool) finishTask(
	request codingLabRequest,
) (string, error) {
	task, err := t.lookupTask(request.TaskID)
	if err != nil {
		return "", err
	}

	if err := t.tasks.Finish(task); err != nil {
		return "", err
	}

	return encodeLabResponse(
		codingLabResponse{
			OK:      true,
			Action:  "finish",
			Task:    task,
			Message: "Coding Lab task marked successful.",
		},
	)
}

func (t *Tool) failTask(
	request codingLabRequest,
) (string, error) {
	task, err := t.lookupTask(request.TaskID)
	if err != nil {
		return "", err
	}

	var cause error
	if reason := strings.TrimSpace(request.Error); reason != "" {
		cause = errors.New(reason)
	} else {
		cause = errors.New("task failed by agent request")
	}

	if err := t.tasks.Fail(task, cause); err != nil {
		return "", err
	}

	return encodeLabResponse(
		codingLabResponse{
			OK:      true,
			Action:  "fail",
			Task:    task,
			Message: "Coding Lab task marked failed.",
		},
	)
}

func (t *Tool) cancelTask(
	request codingLabRequest,
) (string, error) {
	task, err := t.lookupTask(request.TaskID)
	if err != nil {
		return "", err
	}

	if err := t.tasks.Cancel(task); err != nil {
		return "", err
	}

	return encodeLabResponse(
		codingLabResponse{
			OK:      true,
			Action:  "cancel",
			Task:    task,
			Message: "Coding Lab task canceled.",
		},
	)
}

func (t *Tool) blockTask(
	request codingLabRequest,
) (string, error) {
	task, err := t.lookupTask(request.TaskID)
	if err != nil {
		return "", err
	}

	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		reason = "task blocked by agent"
	}

	if err := t.tasks.Block(task, reason); err != nil {
		return "", err
	}

	return encodeLabResponse(
		codingLabResponse{
			OK:      true,
			Action:  "block",
			Task:    task,
			Message: "Coding Lab task blocked.",
		},
	)
}

func (t *Tool) closeTask(
	request codingLabRequest,
) (string, error) {
	task, err := t.lookupTask(request.TaskID)
	if err != nil {
		return "", err
	}

	if err := t.tasks.Close(task); err != nil {
		return "", err
	}

	return encodeLabResponse(
		codingLabResponse{
			OK:      true,
			Action:  "close",
			Task:    task,
			Message: "Coding Lab workspace released.",
		},
	)
}

func (t *Tool) lookupTask(id string) (*Task, error) {
	// The first implementation keeps active tasks in memory. Persistent task
	// storage will be introduced with the Lab session layer, where task state
	// can be recovered across application restarts.
	//
	// This method deliberately exists as the single lookup boundary so that
	// persistence can replace the in-memory implementation without changing
	// the tool API.
	id = strings.TrimSpace(id)

	if id == "" {
		return nil, errors.New("lab: taskId is required")
	}

	return nil, fmt.Errorf(
		"lab: task %q is not available yet; task persistence/registry is the next Lab layer",
		id,
	)
}

type codingLabResponse struct {
	OK           bool                  `json:"ok"`
	Action       string                `json:"action"`
	Task         *Task                 `json:"task,omitempty"`
	Result       *CommandResult        `json:"result,omitempty"`
	Verification *VerificationSummary  `json:"verification,omitempty"`
	Message      string                `json:"message,omitempty"`
	Error        string                `json:"error,omitempty"`
}

func encodeLabResponse(value codingLabResponse) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("lab: encode response: %w", err)
	}

	return string(data), nil
}

func pathSeparator() rune {
	return '/'
}
