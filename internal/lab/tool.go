package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sheytan/local-agent/internal/config"
)

// Tool exposes the Coding Lab through the orchestrator's generic tool system.
//
// The tool is deliberately a single controlled interface. The model does not
// receive direct filesystem or process primitives; all operations flow through
// the Lab session, task, workspace, policy, runner, and verifier layers.
type Tool struct {
	cfg      *config.Config
	tasks    *TaskManager
	verifier *Verifier
	sessions *SessionRegistry
}

// NewTool creates a fully initialized Coding Lab tool.
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
		workspaceRoot = filepath.Join(cfg.LabDir(), "workspaces")
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
		sessions: NewSessionRegistry(),
	}, nil
}

// Name implements the agent.Tool interface.
func (t *Tool) Name() string {
	return "coding_lab"
}

// Description implements the agent.Tool interface.
func (t *Tool) Description() string {
	return `Use the isolated Coding Lab to work on a local code project.

The Lab creates a disposable workspace, executes policy-controlled commands,
and runs objective verification checks.

Use this tool when the task requires inspecting, modifying, building, testing,
debugging, or verifying a local project.

Lifecycle:
1. start_task
2. run one or more commands
3. verify the project
4. finish only after successful verification
5. close when retained workspace cleanup is required

Available actions:
- start_task
- run
- verify
- finish
- fail
- cancel
- block
- close

Important:
- Command success does not mean the project is correct.
- Use verify before declaring a coding task complete.
- Network access is disabled by default.
- Dangerous, interactive, and workspace-escaping commands are blocked by policy.`
}

// Parameters implements the agent.Tool interface.
func (t *Tool) Parameters() any {
	return codingLabParameters{
		Type: "object",
		Properties: map[string]labToolSchema{
			"action": {
				Type:        "string",
				Description: "Coding Lab lifecycle operation to perform.",
				Enum: []string{
					"start_task",
					"run",
					"verify",
					"finish",
					"fail",
					"cancel",
					"block",
					"close",
				},
			},
			"taskId": {
				Type:        "string",
				Description: "Task ID returned by start_task.",
			},
			"title": {
				Type:        "string",
				Description: "Human-readable task title used by start_task.",
			},
			"description": {
				Type:        "string",
				Description: "Detailed coding objective used by start_task.",
			},
			"source": {
				Type:        "string",
				Description: "Absolute or relative local project directory copied into the isolated workspace.",
			},
			"command": {
				Type:        "string",
				Description: "Shell command executed inside the task workspace.",
			},
			"workingDir": {
				Type:        "string",
				Description: "Workspace-relative directory used as the command working directory.",
			},
			"environment": {
				Type:        "array",
				Description: "Additional KEY=VALUE environment variables for the command.",
				Items: &labToolSchema{
					Type: "string",
				},
			},
			"timeoutSec": {
				Type:        "integer",
				Description: "Optional command timeout in seconds.",
			},
			"maxOutputMB": {
				Type:        "number",
				Description: "Optional maximum command output retained in megabytes.",
			},
			"buildCommand": {
				Type:        "string",
				Description: "Required build command used by standard verification.",
			},
			"testCommand": {
				Type:        "string",
				Description: "Required test command used by standard verification.",
			},
			"checks": {
				Type:        "array",
				Description: "Explicit verification checks.",
				Items: &labToolSchema{
					Type: "object",
				},
			},
			"error": {
				Type:        "string",
				Description: "Failure explanation used by the fail action.",
			},
			"reason": {
				Type:        "string",
				Description: "Reason used by the block action.",
			},
		},
		Required: []string{
			"action",
		},
	}
}

// Run implements the agent.Tool interface.
func (t *Tool) Run(
	ctx context.Context,
	args json.RawMessage,
) (string, error) {
	if t == nil ||
		t.tasks == nil ||
		t.verifier == nil ||
		t.sessions == nil {
		return "", errors.New("lab: tool is not initialized")
	}

	if len(args) == 0 {
		return "", errors.New("lab: tool arguments are empty")
	}

	var request codingLabRequest

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

	Command     string   `json:"command,omitempty"`
	WorkingDir string   `json:"workingDir,omitempty"`
	Environment []string `json:"environment,omitempty"`

	TimeoutSec  int     `json:"timeoutSec,omitempty"`
	MaxOutputMB float64 `json:"maxOutputMB,omitempty"`

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
	Type       string                   `json:"type"`
	Properties map[string]labToolSchema `json:"properties"`
	Required   []string                 `json:"required"`
}

type labToolSchema struct {
	Type        string         `json:"type,omitempty"`
	Description string         `json:"description,omitempty"`
	Enum        []string       `json:"enum,omitempty"`
	Items       *labToolSchema `json:"items,omitempty"`
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

	if _, err := t.sessions.Create(task); err != nil {
		// The task has successfully created a workspace, but the runtime
		// session could not be registered. Clean it up rather than leaving
		// an orphaned workspace behind.
		cleanupErr := t.tasks.Fail(task, err)

		if cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}

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
			OK:      true,
			Action:  "start_task",
			Task:    task,
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

	_ = t.sessions.Touch(task.ID)

	response := codingLabResponse{
		OK:     err == nil && result.Success,
		Action: "run",
		Task:   task,
		Result: &result,
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
				return "", fmt.Errorf(
					"lab: verification check %d has an empty command",
					index+1,
				)
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

	_ = t.sessions.Touch(task.ID)

	response := codingLabResponse{
		OK:           err == nil && summary.Passed,
		Action:       "verify",
		Task:         task,
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

	response, encodeErr := encodeLabResponse(
		codingLabResponse{
			OK:      true,
			Action:  "finish",
			Task:    task,
			Message: "Coding Lab task marked successful.",
		},
	)

	removeErr := t.sessions.RemoveCompleted(task.ID)

	if encodeErr != nil {
		return "", encodeErr
	}

	if removeErr != nil {
		return response, removeErr
	}

	return response, nil
}

func (t *Tool) failTask(
	request codingLabRequest,
) (string, error) {
	task, err := t.lookupTask(request.TaskID)
	if err != nil {
		return "", err
	}

	reason := strings.TrimSpace(request.Error)

	var cause error

	if reason != "" {
		cause = errors.New(reason)
	} else {
		cause = errors.New("task failed by agent request")
	}

	if err := t.tasks.Fail(task, cause); err != nil {
		return "", err
	}

	response, encodeErr := encodeLabResponse(
		codingLabResponse{
			OK:      true,
			Action:  "fail",
			Task:    task,
			Message: "Coding Lab task marked failed.",
		},
	)

	removeErr := t.sessions.RemoveCompleted(task.ID)

	if encodeErr != nil {
		return "", encodeErr
	}

	if removeErr != nil {
		return response, removeErr
	}

	return response, nil
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

	response, encodeErr := encodeLabResponse(
		codingLabResponse{
			OK:      true,
			Action:  "cancel",
			Task:    task,
			Message: "Coding Lab task canceled.",
		},
	)

	removeErr := t.sessions.RemoveCompleted(task.ID)

	if encodeErr != nil {
		return "", encodeErr
	}

	if removeErr != nil {
		return response, removeErr
	}

	return response, nil
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

	response, encodeErr := encodeLabResponse(
		codingLabResponse{
			OK:      true,
			Action:  "block",
			Task:    task,
			Message: "Coding Lab task blocked.",
		},
	)

	removeErr := t.sessions.RemoveCompleted(task.ID)

	if encodeErr != nil {
		return "", encodeErr
	}

	if removeErr != nil {
		return response, removeErr
	}

	return response, nil
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

	if err := t.sessions.Delete(task.ID); err != nil {
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
	id = strings.TrimSpace(id)

	if id == "" {
		return nil, errors.New("lab: taskId is required")
	}

	return t.sessions.GetTask(id)
}

type codingLabResponse struct {
	OK           bool                 `json:"ok"`
	Action       string               `json:"action"`
	Task         *Task                `json:"task,omitempty"`
	Result       *CommandResult       `json:"result,omitempty"`
	Verification *VerificationSummary `json:"verification,omitempty"`
	Message      string               `json:"message,omitempty"`
	Error        string               `json:"error,omitempty"`
}

func encodeLabResponse(value codingLabResponse) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("lab: encode response: %w", err)
	}

	return string(data), nil
}
