package lab

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTaskInvalid       = errors.New("lab: invalid task")
	ErrTaskNotRunnable   = errors.New("lab: task is not runnable")
	ErrTaskAlreadyClosed = errors.New("lab: task is already closed")
)

// TaskStatus represents the lifecycle state of one Coding Lab task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskSucceeded TaskStatus = "succeeded"
	TaskFailed    TaskStatus = "failed"
	TaskCanceled  TaskStatus = "canceled"
	TaskBlocked   TaskStatus = "blocked"
)

// Task describes one autonomous coding operation.
//
// A task owns a disposable workspace and a sequence of command executions.
// The LLM may generate the task description and commands, but this structure
// is deliberately independent from any specific model or orchestrator.
type Task struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      TaskStatus        `json:"status"`
	Workspace   *Workspace        `json:"workspace,omitempty"`
	Commands    []Command         `json:"commands,omitempty"`
	Results     []CommandResult   `json:"results,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`

	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// TaskManager coordinates task lifecycle, workspace creation, policy, and
// command execution.
//
// It is intentionally small at this stage. Verification, repair loops,
// research, and orchestration will be layered on top later.
type TaskManager struct {
	Workspaces *WorkspaceManager
	Runner     *Runner
	Policy     Policy

	// KeepWorkspaces controls whether completed task workspaces are retained.
	// Retention is useful while debugging autonomous behavior.
	KeepWorkspaces bool
}

// NewTaskManager constructs a Coding Lab task manager.
func NewTaskManager(
	workspaces *WorkspaceManager,
	runner *Runner,
	policy Policy,
	keepWorkspaces bool,
) (*TaskManager, error) {
	if workspaces == nil {
		return nil, ErrInvalidWorkspace
	}

	if runner == nil {
		return nil, errors.New("lab: runner is nil")
	}

	return &TaskManager{
		Workspaces:      workspaces,
		Runner:          runner,
		Policy:          policy,
		KeepWorkspaces:  keepWorkspaces,
	}, nil
}

// NewTask creates a pending task.
//
// The source repository is copied only when Start is called. This keeps task
// construction cheap and lets callers validate or enrich the task first.
func (m *TaskManager) NewTask(title, description string) *Task {
	now := time.Now().UTC()

	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)

	return &Task{
		ID:          newTaskID(now),
		Title:       title,
		Description: description,
		Status:      TaskPending,
		Commands:    make([]Command, 0),
		Results:     make([]CommandResult, 0),
		Metadata:    make(map[string]string),
		CreatedAt:   now,
	}
}

// Start creates an isolated workspace for the task.
//
// source must be a local directory containing the project that the task is
// expected to modify or test.
func (m *TaskManager) Start(
	ctx context.Context,
	task *Task,
	source string,
) error {
	if err := m.validateTask(task); err != nil {
		return err
	}

	if task.Status != TaskPending {
		return fmt.Errorf("%w: current status=%s", ErrTaskNotRunnable, task.Status)
	}

	if strings.TrimSpace(source) == "" {
		return ErrInvalidSource
	}

	if ctx == nil {
		ctx = context.Background()
	}

	workspace, err := m.Workspaces.Create(ctx, source)
	if err != nil {
		task.Status = TaskFailed
		task.Error = err.Error()
		return err
	}

	now := time.Now().UTC()

	task.Workspace = workspace
	task.StartedAt = &now
	task.Status = TaskRunning
	task.Error = ""

	return nil
}

// RunCommand validates and executes one command belonging to the task.
//
// Commands are appended to task.Commands and their corresponding results are
// appended to task.Results in the same order.
func (m *TaskManager) RunCommand(
	ctx context.Context,
	task *Task,
	command Command,
) (CommandResult, error) {
	if err := m.validateTask(task); err != nil {
		return CommandResult{}, err
	}

	if task.Status != TaskRunning {
		return CommandResult{}, fmt.Errorf(
			"%w: current status=%s",
			ErrTaskNotRunnable,
			task.Status,
		)
	}

	if task.Workspace == nil {
		return CommandResult{}, ErrInvalidWorkspace
	}

	if err := m.Policy.EvaluateForWorkspace(
		command.Command,
		command.WorkingDir,
	); err != nil {
		return CommandResult{}, err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	result, err := m.Runner.Run(ctx, task.Workspace, command)

	task.Commands = append(task.Commands, command)
	task.Results = append(task.Results, result)

	if err != nil {
		// Cancellation and timeout are reflected in the command result first.
		// The task remains explicitly running so callers can decide whether to
		// retry, repair, or terminate the task.
		return result, err
	}

	return result, nil
}

// Finish marks a running task as successfully completed.
func (m *TaskManager) Finish(task *Task) error {
	if err := m.validateTask(task); err != nil {
		return err
	}

	if task.Status != TaskRunning {
		return fmt.Errorf(
			"%w: current status=%s",
			ErrTaskNotRunnable,
			task.Status,
		)
	}

	now := time.Now().UTC()
	task.FinishedAt = &now
	task.Status = TaskSucceeded
	task.Error = ""

	return m.cleanupTaskWorkspace(task)
}

// Fail marks a running task as failed.
//
// The provided cause is recorded in the task for later diagnostics.
func (m *TaskManager) Fail(task *Task, cause error) error {
	if err := m.validateTask(task); err != nil {
		return err
	}

	if task.Status != TaskRunning && task.Status != TaskPending {
		return fmt.Errorf(
			"%w: current status=%s",
			ErrTaskNotRunnable,
			task.Status,
		)
	}

	now := time.Now().UTC()
	task.FinishedAt = &now
	task.Status = TaskFailed

	if cause != nil {
		task.Error = cause.Error()
	} else {
		task.Error = "task failed"
	}

	return m.cleanupTaskWorkspace(task)
}

// Cancel marks a running task as canceled.
func (m *TaskManager) Cancel(task *Task) error {
	if err := m.validateTask(task); err != nil {
		return err
	}

	if task.Status != TaskRunning && task.Status != TaskPending {
		return fmt.Errorf(
			"%w: current status=%s",
			ErrTaskNotRunnable,
			task.Status,
		)
	}

	now := time.Now().UTC()
	task.FinishedAt = &now
	task.Status = TaskCanceled
	task.Error = "task canceled"

	return m.cleanupTaskWorkspace(task)
}

// Block marks a task as blocked.
//
// This is useful when policy, required tooling, missing dependencies, or an
// external approval prevents safe execution.
func (m *TaskManager) Block(task *Task, reason string) error {
	if err := m.validateTask(task); err != nil {
		return err
	}

	if task.Status != TaskPending && task.Status != TaskRunning {
		return fmt.Errorf(
			"%w: current status=%s",
			ErrTaskNotRunnable,
			task.Status,
		)
	}

	now := time.Now().UTC()
	task.FinishedAt = &now
	task.Status = TaskBlocked

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "task blocked"
	}

	task.Error = reason

	return m.cleanupTaskWorkspace(task)
}

// Close cleans up a task workspace without changing a terminal task status.
//
// This is primarily useful for callers that need deterministic resource
// cleanup after inspecting a completed task.
func (m *TaskManager) Close(task *Task) error {
	if err := m.validateTask(task); err != nil {
		return err
	}

	if task.Workspace == nil {
		return ErrInvalidWorkspace
	}

	if m.KeepWorkspaces {
		return nil
	}

	if err := m.Workspaces.Remove(task.Workspace); err != nil {
		return err
	}

	task.Workspace = nil

	return nil
}

// LastResult returns the most recent command result.
func (t *Task) LastResult() *CommandResult {
	if t == nil || len(t.Results) == 0 {
		return nil
	}

	return &t.Results[len(t.Results)-1]
}

// SuccessfulCommands returns the number of commands that completed
// successfully.
func (t *Task) SuccessfulCommands() int {
	if t == nil {
		return 0
	}

	count := 0

	for _, result := range t.Results {
		if result.Success {
			count++
		}
	}

	return count
}

// FailedCommands returns the number of commands that did not complete
// successfully.
func (t *Task) FailedCommands() int {
	if t == nil {
		return 0
	}

	count := 0

	for _, result := range t.Results {
		if !result.Success {
			count++
		}
	}

	return count
}

func (m *TaskManager) validateTask(task *Task) error {
	if m == nil {
		return ErrTaskInvalid
	}

	if task == nil {
		return ErrTaskInvalid
	}

	if strings.TrimSpace(task.ID) == "" {
		return ErrTaskInvalid
	}

	return nil
}

func (m *TaskManager) cleanupTaskWorkspace(task *Task) error {
	if task == nil || task.Workspace == nil {
		return nil
	}

	if m.KeepWorkspaces {
		return nil
	}

	if err := m.Workspaces.Remove(task.Workspace); err != nil {
		return err
	}

	task.Workspace = nil

	return nil
}

func newTaskID(now time.Time) string {
	return fmt.Sprintf(
		"task-%s",
		now.UTC().Format("20060102-150405.000000000"),
	)
}
