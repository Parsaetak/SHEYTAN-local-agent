package lab

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrVerificationFailed   = errors.New("lab: verification failed")
	ErrVerificationEmpty    = errors.New("lab: no verification checks configured")
	ErrVerificationCanceled = errors.New("lab: verification canceled")
)

// VerificationStatus describes the outcome of one verification check.
type VerificationStatus string

const (
	VerificationPassed   VerificationStatus = "passed"
	VerificationFailed   VerificationStatus = "failed"
	VerificationCanceled VerificationStatus = "canceled"
	VerificationSkipped  VerificationStatus = "skipped"
)

// VerificationCheck describes one command that determines whether the task
// actually works.
//
// A check is intentionally separate from a normal Task command. Commands may
// modify the workspace; verification checks are expected to observe its state.
type VerificationCheck struct {
	Name         string        `json:"name"`
	Command      Command       `json:"command"`
	Required     bool          `json:"required"`
	AllowFailure bool          `json:"allowFailure,omitempty"`
}

// VerificationResult records the result of one verification check.
type VerificationResult struct {
	Name       string             `json:"name"`
	Status     VerificationStatus `json:"status"`
	Result     CommandResult      `json:"result"`
	Error      string             `json:"error,omitempty"`
	StartedAt  time.Time          `json:"startedAt"`
	FinishedAt time.Time          `json:"finishedAt"`
}

// VerificationSummary is the aggregate judgment for a verification run.
type VerificationSummary struct {
	Passed          bool                 `json:"passed"`
	RequiredTotal   int                  `json:"requiredTotal"`
	RequiredPassed  int                  `json:"requiredPassed"`
	RequiredFailed  int                  `json:"requiredFailed"`
	OptionalTotal   int                  `json:"optionalTotal"`
	OptionalPassed  int                  `json:"optionalPassed"`
	OptionalFailed  int                  `json:"optionalFailed"`
	Results         []VerificationResult `json:"results"`
	Duration        time.Duration        `json:"duration"`
	StartedAt       time.Time            `json:"startedAt"`
	FinishedAt      time.Time            `json:"finishedAt"`
	Error           string               `json:"error,omitempty"`
}

// Verifier executes objective checks against a running task.
//
// The verifier deliberately reuses TaskManager so policy and workspace
// boundaries remain identical between normal task commands and verification.
type Verifier struct {
	Tasks *TaskManager
}

// NewVerifier creates a verifier backed by a TaskManager.
func NewVerifier(tasks *TaskManager) (*Verifier, error) {
	if tasks == nil {
		return nil, errors.New("lab: task manager is nil")
	}

	return &Verifier{
		Tasks: tasks,
	}, nil
}

// Verify executes all configured checks.
//
// Required checks must pass. Optional checks are recorded but do not determine
// the aggregate Passed field unless AllowFailure is false and the check fails.
//
// Verification stops when the context is canceled.
func (v *Verifier) Verify(
	ctx context.Context,
	task *Task,
	checks []VerificationCheck,
) (VerificationSummary, error) {
	started := time.Now().UTC()

	summary := VerificationSummary{
		Results:  make([]VerificationResult, 0, len(checks)),
		StartedAt: started,
	}

	if v == nil || v.Tasks == nil {
		summary.FinishedAt = time.Now().UTC()
		summary.Duration = summary.FinishedAt.Sub(started)
		summary.Error = "lab: verifier is not configured"
		return summary, errors.New("lab: verifier is not configured")
	}

	if task == nil {
		summary.FinishedAt = time.Now().UTC()
		summary.Duration = summary.FinishedAt.Sub(started)
		summary.Error = ErrTaskInvalid.Error()
		return summary, ErrTaskInvalid
	}

	if task.Status != TaskRunning {
		summary.FinishedAt = time.Now().UTC()
		summary.Duration = summary.FinishedAt.Sub(started)
		summary.Error = fmt.Sprintf(
			"%s: current status=%s",
			ErrTaskNotRunnable,
			task.Status,
		)
		return summary, fmt.Errorf(
			"%w: current status=%s",
			ErrTaskNotRunnable,
			task.Status,
		)
	}

	if len(checks) == 0 {
		summary.FinishedAt = time.Now().UTC()
		summary.Duration = summary.FinishedAt.Sub(started)
		summary.Error = ErrVerificationEmpty.Error()
		return summary, ErrVerificationEmpty
	}

	if ctx == nil {
		ctx = context.Background()
	}

	for index, check := range checks {
		if err := ctx.Err(); err != nil {
			summary.FinishedAt = time.Now().UTC()
			summary.Duration = summary.FinishedAt.Sub(started)
			summary.Error = ErrVerificationCanceled.Error()

			return summary, ErrVerificationCanceled
		}

		normalized, err := normalizeCheck(check, index)
		if err != nil {
			result := VerificationResult{
				Name:       check.Name,
				Status:     VerificationFailed,
				Error:      err.Error(),
				StartedAt:  time.Now().UTC(),
				FinishedAt: time.Now().UTC(),
			}

			summary.Results = append(summary.Results, result)

			if check.Required && !check.AllowFailure {
				summary.RequiredTotal++
				summary.RequiredFailed++
			} else {
				summary.OptionalTotal++
				summary.OptionalFailed++
			}

			summary.FinishedAt = time.Now().UTC()
			summary.Duration = summary.FinishedAt.Sub(started)
			summary.Passed = false
			summary.Error = ErrVerificationFailed.Error()

			return summary, errors.Join(ErrVerificationFailed, err)
		}

		if normalized.Required {
			summary.RequiredTotal++
		} else {
			summary.OptionalTotal++
		}

		checkStarted := time.Now().UTC()

		if err := v.Tasks.Policy.EvaluateForWorkspace(
			normalized.Command.Command,
			normalized.Command.WorkingDir,
		); err != nil {
			checkFinished := time.Now().UTC()

			result := VerificationResult{
				Name:       normalized.Name,
				Status:     VerificationFailed,
				Error:      err.Error(),
				StartedAt:  checkStarted,
				FinishedAt: checkFinished,
			}

			summary.Results = append(summary.Results, result)

			if normalized.Required && !normalized.AllowFailure {
				summary.RequiredFailed++
			} else {
				summary.OptionalFailed++
			}

			if normalized.Required && !normalized.AllowFailure {
				summary.FinishedAt = checkFinished
				summary.Duration = summary.FinishedAt.Sub(started)
				summary.Passed = false
				summary.Error = ErrVerificationFailed.Error()

				return summary, errors.Join(ErrVerificationFailed, err)
			}

			continue
		}

		commandResult, commandErr := v.Tasks.RunCommand(
			ctx,
			task,
			normalized.Command,
		)

		checkFinished := time.Now().UTC()

		verificationResult := VerificationResult{
			Name:       normalized.Name,
			Result:     commandResult,
			StartedAt:  checkStarted,
			FinishedAt: checkFinished,
		}

		switch {
		case commandErr == nil && commandResult.Success:
			verificationResult.Status = VerificationPassed

			if normalized.Required {
				summary.RequiredPassed++
			} else {
				summary.OptionalPassed++
			}

		case errors.Is(commandErr, context.Canceled),
			errors.Is(commandErr, ErrCommandTimedOut) &&
				ctx.Err() != nil:

			verificationResult.Status = VerificationCanceled

			if normalized.Required {
				summary.RequiredFailed++
			} else {
				summary.OptionalFailed++
			}

			verificationResult.Error = commandErrorString(commandErr)

			summary.Results = append(summary.Results, verificationResult)
			summary.FinishedAt = checkFinished
			summary.Duration = summary.FinishedAt.Sub(started)
			summary.Error = ErrVerificationCanceled.Error()

			return summary, ErrVerificationCanceled

		default:
			verificationResult.Status = VerificationFailed
			verificationResult.Error = commandErrorString(commandErr)

			if normalized.Required && !normalized.AllowFailure {
				summary.RequiredFailed++
			} else {
				summary.OptionalFailed++
			}
		}

		summary.Results = append(summary.Results, verificationResult)

		if verificationResult.Status == VerificationFailed &&
			normalized.Required &&
			!normalized.AllowFailure {
			summary.FinishedAt = checkFinished
			summary.Duration = summary.FinishedAt.Sub(started)
			summary.Passed = false
			summary.Error = ErrVerificationFailed.Error()

			return summary, ErrVerificationFailed
		}
	}

	finished := time.Now().UTC()

	summary.FinishedAt = finished
	summary.Duration = finished.Sub(started)
	summary.Passed =
		summary.RequiredTotal > 0 &&
		summary.RequiredFailed == 0 &&
		summary.RequiredPassed == summary.RequiredTotal

	if !summary.Passed {
		summary.Error = ErrVerificationFailed.Error()
		return summary, ErrVerificationFailed
	}

	return summary, nil
}

// VerifyStandard provides a simple build/test verification profile.
//
// Empty command strings are ignored, so callers can select only the checks
// relevant to a particular repository.
func (v *Verifier) VerifyStandard(
	ctx context.Context,
	task *Task,
	buildCommand string,
	testCommand string,
) (VerificationSummary, error) {
	checks := make([]VerificationCheck, 0, 2)

	buildCommand = strings.TrimSpace(buildCommand)
	testCommand = strings.TrimSpace(testCommand)

	if buildCommand != "" {
		checks = append(checks, VerificationCheck{
			Name:     "build",
			Required: true,
			Command: Command{
				Command: buildCommand,
			},
		})
	}

	if testCommand != "" {
		checks = append(checks, VerificationCheck{
			Name:     "tests",
			Required: true,
			Command: Command{
				Command: testCommand,
			},
		})
	}

	return v.Verify(ctx, task, checks)
}

// VerifyCommands verifies an arbitrary ordered list of commands.
//
// Every supplied check is treated as required.
func (v *Verifier) VerifyCommands(
	ctx context.Context,
	task *Task,
	commands []string,
) (VerificationSummary, error) {
	checks := make([]VerificationCheck, 0, len(commands))

	for index, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}

		checks = append(checks, VerificationCheck{
			Name: fmt.Sprintf("check-%d", index+1),
			Required: true,
			Command: Command{
				Command: command,
			},
		})
	}

	return v.Verify(ctx, task, checks)
}

func normalizeCheck(check VerificationCheck, index int) (VerificationCheck, error) {
	name := strings.TrimSpace(check.Name)
	if name == "" {
		name = fmt.Sprintf("check-%d", index+1)
	}

	command := strings.TrimSpace(check.Command.Command)
	if command == "" {
		return VerificationCheck{}, ErrCommandEmpty
	}

	check.Name = name
	check.Command.Command = command

	return check, nil
}

func commandErrorString(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}

// Required checks can be explicitly marked AllowFailure, which is useful for
// advisory diagnostics that are structurally important but should not block
// the autonomous repair loop.
func isBlockingVerificationFailure(check VerificationCheck) bool {
	return check.Required && !check.AllowFailure
}

// Keep this helper local for future policy/reporting extensions.
var _ = isBlockingVerificationFailure
