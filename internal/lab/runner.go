// Package lab contains the autonomous coding-laboratory runtime.
//
// runner.go provides the process-execution layer used by the Coding Lab.
// It deliberately does not decide whether a command is permitted; command
// policy belongs to the policy layer. The runner's responsibility is to
// execute a permitted command inside a validated workspace and return a
// bounded, structured result.
package lab

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var (
	ErrCommandEmpty = errors.New("lab: command is empty")
	ErrOutputLimit  = errors.New("lab: command output exceeded configured limit")
	ErrCommandTimedOut = errors.New("lab: command timed out")
)

// Command describes one process execution requested by the Coding Lab.
type Command struct {
	// Command is interpreted by the platform shell.
	Command string `json:"command"`

	// WorkingDir is relative to the workspace root. Empty means the workspace
	// root itself.
	WorkingDir string `json:"workingDir,omitempty"`

	// Environment contains additional environment variables in KEY=VALUE form.
	// Values are added on top of the current process environment.
	Environment []string `json:"environment,omitempty"`

	// Timeout overrides the runner default for this command. A zero value uses
	// the runner's configured default.
	Timeout time.Duration `json:"timeout,omitempty"`

	// MaxOutputBytes limits combined stdout/stderr captured from the process.
	// A zero value uses the runner default.
	MaxOutputBytes int64 `json:"maxOutputBytes,omitempty"`
}

// CommandResult contains the complete structured result of one execution.
type CommandResult struct {
	Command      string        `json:"command"`
	WorkingDir   string        `json:"workingDir"`
	Stdout       string        `json:"stdout"`
	Stderr       string        `json:"stderr"`
	Output       string        `json:"output"`
	ExitCode     int           `json:"exitCode"`
	Duration     time.Duration `json:"duration"`
	StartedAt    time.Time     `json:"startedAt"`
	FinishedAt   time.Time     `json:"finishedAt"`
	TimedOut     bool          `json:"timedOut"`
	Canceled     bool          `json:"canceled"`
	OutputLimit  bool          `json:"outputLimit"`
	Success      bool          `json:"success"`
}

// Runner executes commands inside Coding Lab workspaces.
type Runner struct {
	DefaultTimeout      time.Duration
	DefaultMaxOutputBytes int64
}

// NewRunner creates a process runner with bounded defaults.
//
// A timeout of <= 0 uses 5 minutes.
// An output limit of <= 0 uses 2 MiB of combined stdout/stderr.
func NewRunner(timeout time.Duration, maxOutputBytes int64) *Runner {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	if maxOutputBytes <= 0 {
		maxOutputBytes = 2 * 1024 * 1024
	}

	return &Runner{
		DefaultTimeout:       timeout,
		DefaultMaxOutputBytes: maxOutputBytes,
	}
}

// Run executes one command inside the given workspace.
func (r *Runner) Run(ctx context.Context, workspace *Workspace, command Command) (CommandResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if workspace == nil || workspace.Path == "" {
		return CommandResult{}, ErrInvalidWorkspace
	}

	command.Command = strings.TrimSpace(command.Command)
	if command.Command == "" {
		return CommandResult{}, ErrCommandEmpty
	}

	workingDir, err := workspace.PathFor(command.WorkingDir)
	if err != nil {
		return CommandResult{}, fmt.Errorf("lab: resolve command working directory: %w", err)
	}

	if info, statErr := os.Stat(workingDir); statErr != nil {
		return CommandResult{}, fmt.Errorf("lab: stat command working directory: %w", statErr)
	} else if !info.IsDir() {
		return CommandResult{}, fmt.Errorf("lab: command working directory is not a directory: %s", workingDir)
	}

	timeout := command.Timeout
	if timeout <= 0 {
		timeout = r.defaultTimeout()
	}

	maxOutput := command.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = r.defaultMaxOutputBytes()
	}

	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	cmd := buildShellCommand(runCtx, command.Command)

	cmd.Dir = workingDir

	env := os.Environ()
	if len(command.Environment) > 0 {
		env = mergeEnvironment(env, command.Environment)
	}
	cmd.Env = env

	// Make command execution deterministic for tooling that checks terminal
	// behavior. stdin is intentionally disconnected: autonomous runs should
	// not hang waiting for interactive input.
	cmd.Stdin = nil

	stdoutBuffer := &boundedBuffer{
		Limit: maxOutput,
	}
	stderrBuffer := &boundedBuffer{
		Limit: maxOutput,
	}

	cmd.Stdout = stdoutBuffer
	cmd.Stderr = stderrBuffer

	started := time.Now().UTC()

	startErr := cmd.Start()
	if startErr != nil {
		finished := time.Now().UTC()
		result := buildCommandResult(
			command,
			workingDir,
			stdoutBuffer.String(),
			stderrBuffer.String(),
			stdoutBuffer.Truncated || stderrBuffer.Truncated,
			started,
			finished,
			startErr,
			false,
			false,
		)
		return result, fmt.Errorf("lab: start command: %w", startErr)
	}

	waitErr := cmd.Wait()
	finished := time.Now().UTC()

	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	canceled := !timedOut && errors.Is(runCtx.Err(), context.Canceled)

	outputLimited := stdoutBuffer.Truncated || stderrBuffer.Truncated

	result := buildCommandResult(
		command,
		workingDir,
		stdoutBuffer.String(),
		stderrBuffer.String(),
		outputLimited,
		started,
		finished,
		waitErr,
		timedOut,
		canceled,
	)

	if timedOut {
		if outputLimited {
			return result, errors.Join(ErrCommandTimedOut, ErrOutputLimit)
		}
		return result, ErrCommandTimedOut
	}

	if canceled {
		if outputLimited {
			return result, errors.Join(context.Canceled, ErrOutputLimit)
		}
		return result, context.Canceled
	}

	if outputLimited {
		if waitErr != nil {
			return result, errors.Join(waitErr, ErrOutputLimit)
		}
		return result, ErrOutputLimit
	}

	if waitErr != nil {
		return result, fmt.Errorf("lab: command failed: %w", waitErr)
	}

	return result, nil
}

func buildShellCommand(ctx context.Context, command string) *exec.Cmd {
	switch runtime.GOOS {
	case "windows":
		// cmd.exe is deliberately used instead of PowerShell so the command
		// syntax stays predictable for common build/test tooling and matches
		// the user's normal Windows terminal environment.
		return exec.CommandContext(
			ctx,
			"cmd.exe",
			"/d",
			"/s",
			"/c",
			command,
		)
	default:
		// `sh -lc` provides shell pipelines, redirection, &&, environment
		// expansion, and other standard Unix build-tool behavior.
		return exec.CommandContext(
			ctx,
			"/bin/sh",
			"-lc",
			command,
		)
	}
}

func mergeEnvironment(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}

	index := make(map[string]int, len(base))

	for i, item := range base {
		key := envKey(item)
		if key != "" {
			index[key] = i
		}
	}

	result := append([]string(nil), base...)

	for _, item := range extra {
		key := envKey(item)
		if key == "" {
			continue
		}

		if i, exists := index[key]; exists {
			result[i] = item
			continue
		}

		index[key] = len(result)
		result = append(result, item)
	}

	return result
}

func envKey(value string) string {
	index := strings.IndexByte(value, '=')
	if index <= 0 {
		return ""
	}

	return strings.ToUpper(value[:index])
}

func buildCommandResult(
	command Command,
	workingDir string,
	stdout string,
	stderr string,
	outputLimited bool,
	started time.Time,
	finished time.Time,
	runErr error,
	timedOut bool,
	canceled bool,
) CommandResult {
	exitCode := 0
	success := runErr == nil && !timedOut && !canceled && !outputLimited

	if runErr != nil {
		exitCode = exitCodeFromError(runErr)
	}

	output := combineOutput(stdout, stderr)

	return CommandResult{
		Command:     command.Command,
		WorkingDir:  workingDir,
		Stdout:      stdout,
		Stderr:      stderr,
		Output:      output,
		ExitCode:    exitCode,
		Duration:    finished.Sub(started),
		StartedAt:   started,
		FinishedAt:  finished,
		TimedOut:    timedOut,
		Canceled:    canceled,
		OutputLimit: outputLimited,
		Success:     success,
	}
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	return -1
}

func combineOutput(stdout, stderr string) string {
	switch {
	case stdout == "":
		return stderr
	case stderr == "":
		return stdout
	default:
		var b strings.Builder
		b.Grow(len(stdout) + len(stderr) + 16)
		b.WriteString("[stdout]\n")
		b.WriteString(stdout)
		b.WriteString("\n[stderr]\n")
		b.WriteString(stderr)
		return b.String()
	}
}

// boundedBuffer captures at most Limit bytes.
//
// Once the limit is reached, additional bytes are discarded and Truncated is
// set. The implementation deliberately preserves the beginning of output,
// because compiler/test diagnostics usually explain the root cause near the
// start of the stream.
type boundedBuffer struct {
	Limit     int64
	Written   int64
	Truncated bool
	buffer    bytes.Buffer
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}

	if b.Limit <= 0 {
		b.Truncated = true
		return len(p), nil
	}

	remaining := b.Limit - b.Written

	if remaining <= 0 {
		b.Truncated = true
		return len(p), nil
	}

	if int64(len(p)) <= remaining {
		n, err := b.buffer.Write(p)
		b.Written += int64(n)
		return n, err
	}

	n, err := b.buffer.Write(p[:remaining])
	b.Written += int64(n)
	b.Truncated = true

	if err != nil {
		return n, err
	}

	// Returning len(p) tells os/exec that the writer intentionally consumed
	// the input even though only the bounded prefix was retained.
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	if b == nil {
		return ""
	}

	value := b.buffer.String()

	if !b.Truncated {
		return value
	}

	const marker = "\n\n[LAB OUTPUT TRUNCATED]\n"
	return value + marker
}

func (r *Runner) defaultTimeout() time.Duration {
	if r == nil || r.DefaultTimeout <= 0 {
		return 5 * time.Minute
	}
	return r.DefaultTimeout
}

func (r *Runner) defaultMaxOutputBytes() int64 {
	if r == nil || r.DefaultMaxOutputBytes <= 0 {
		return 2 * 1024 * 1024
	}
	return r.DefaultMaxOutputBytes
}
