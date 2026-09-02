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
// A check is intentionally separate from a normal Task command. Verification
// commands may invoke compilers, test runners, linters, or other diagnostics,
// but their final purpose is to establish objective evidence about the current
// workspace state.
type VerificationCheck struct {
	Name         string  `json:"name"`
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
	Passed         bool                 `json:"passed"`
	RequiredTotal  int                  `json:"requiredTotal"`
	RequiredPassed int                  `json:"requiredPassed"`
	RequiredFailed int                  `json:"requiredFailed"`
	OptionalTotal  int                  `json:"optionalTotal"`
	OptionalPassed int                  `json:"optionalPassed"`
	OptionalFailed int                  `json:"optionalFailed"`
	Results        []VerificationResult `json:"results"`
	Duration       time.Duration        `json:"duration"`
	StartedAt      time.Time            `json:"startedAt"`
	FinishedAt     time.Time            `json:"finishedAt"`
	Error          string               `json:"error,omitempty"`
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

// Verify executes all configured checks and records the resulting verification
// state on the task.
//
// The task is considered verified only after the complete verification run has
// finished and the resulting summary has been recorded through
// TaskManager.RecordVerification().
func (v *Verifier) Verify(
	ctx context.Context,
	task *Task,
	checks []VerificationCheck,
) (VerificationSummary, error) {
	started := time.Now().UTC()

	summary := VerificationSummary{
		Results:   make([]VerificationResult, 0, len(checks)),
		StartedAt: started,
	}

	if v == nil || v.Tasks == nil {
		return finishVerification(
			summary,
			errors.New("lab: verifier is not configured"),
		)
	}

	if task == nil {
		return finishVerification(summary, ErrTaskInvalid)
	}

	if task.Status != TaskRunning {
		err := fmt.Errorf(
			"%w: current status=%s",
			ErrTaskNotRunnable,
			task.Status,
		)
		return finishVerification(summary, err)
	}

	if len(checks) == 0 {
		return finishVerification(summary, ErrVerificationEmpty)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	for index, check := range checks {
		// Check cancellation before starting another verification command.
		if err := ctx.Err(); err != nil {
			summary.Error = ErrVerificationCanceled.Error()
			summary.Passed = false

			finished := time.Now().UTC()
			summary.FinishedAt = finished
			summary.Duration = finished.Sub(started)

			_ = v.Tasks.RecordVerification(task, summary)

			return summary, ErrVerificationCanceled
		}

		normalized, err := normalizeCheck(check, index)
		if err != nil {
			finished := time.Now().UTC()

			verificationResult := VerificationResult{
				Name:       check.Name,
				Status:     VerificationFailed,
				Error:      err.Error(),
				StartedAt:  finished,
				FinishedAt: finished,
			}

			summary.Results = append(
				summary.Results,
				verificationResult,
			)

			if check.Required && !check.AllowFailure {
				summary.RequiredTotal++
				summary.RequiredFailed++
			} else {
				summary.OptionalTotal++
				summary.OptionalFailed++
			}

			summary.Passed = false
			summary.Error = ErrVerificationFailed.Error()
			summary.FinishedAt = finished
			summary.Duration = finished.Sub(started)

			recordErr := v.Tasks.RecordVerification(task, summary)

			if recordErr != nil {
				return summary, errors.Join(
					ErrVerificationFailed,
					err,
					recordErr,
				)
			}

			return summary, errors.Join(
				ErrVerificationFailed,
				err,
			)
		}

		if normalized.Required {
			summary.RequiredTotal++
		} else {
			summary.OptionalTotal++
		}

		checkStarted := time.Now().UTC()

		// Policy is checked again here even though RunCommand also checks it.
		// This keeps verification policy failures represented explicitly as
		// verification results rather than as an unexplained execution failure.
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
				summary.Passed = false
				summary.Error = ErrVerificationFailed.Error()
				summary.FinishedAt = checkFinished
				summary.Duration = checkFinished.Sub(started)

				recordErr := v.Tasks.RecordVerification(task, summary)

				if recordErr != nil {
					return summary, errors.Join(
						ErrVerificationFailed,
						err,
						recordErr,
					)
				}

				return summary, errors.Join(
					ErrVerificationFailed,
					err,
				)
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

		case errors.Is(commandErr, context.Canceled):
			verificationResult.Status = VerificationCanceled
			verificationResult.Error = commandErrorString(commandErr)

			if normalized.Required {
				summary.RequiredFailed++
			} else {
				summary.OptionalFailed++
			}

			summary.Results = append(
				summary.Results,
				verificationResult,
			)

			summary.Passed = false
			summary.Error = ErrVerificationCanceled.Error()
			summary.FinishedAt = checkFinished
			summary.Duration = checkFinished.Sub(started)

			_ = v.Tasks.RecordVerification(task, summary)

			return summary, ErrVerificationCanceled

		case errors.Is(commandErr, ErrCommandTimedOut):
			verificationResult.Status = VerificationFailed
			verificationResult.Error = commandErrorString(commandErr)

			if normalized.Required && !normalized.AllowFailure {
				summary.RequiredFailed++
			} else {
				summary.OptionalFailed++
			}

		default:
			verificationResult.Status = VerificationFailed
			verificationResult.Error = commandErrorString(commandErr)

			if normalized.Required && !normalized.AllowFailure {
				summary.RequiredFailed++
			} else {
				summary.OptionalFailed++
			}
		}

		summary.Results = append(
			summary.Results,
			verificationResult,
		)

		// Required failures stop the verification run immediately. The failed
		// summary is still persisted to the Task so Finish() remains blocked.
		if verificationResult.Status == VerificationFailed &&
			normalized.Required &&
			!normalized.AllowFailure {
			summary.Passed = false
			summary.Error = ErrVerificationFailed.Error()
			summary.FinishedAt = checkFinished
			summary.Duration = checkFinished.Sub(started)

			recordErr := v.Tasks.RecordVerification(task, summary)

			if recordErr != nil {
				return summary, errors.Join(
					ErrVerificationFailed,
					recordErr,
				)
			}

			return summary, ErrVerificationFailed
		}
	}

	finished := time.Now().UTC()

	summary.FinishedAt = finished
	summary.Duration = finished.Sub(started)

	// At least one required check must exist. This prevents an accidentally
	// empty required set from becoming an automatic success.
	summary.Passed =
		summary.RequiredTotal > 0 &&
		summary.RequiredFailed == 0 &&
		summary.RequiredPassed == summary.RequiredTotal

	if !summary.Passed {
		summary.Error = ErrVerificationFailed.Error()
	}

	// Record the result AFTER all commands are complete. RunCommand invalidates
	// verification before every command, so recording earlier would immediately
	// be wiped out by the next check.
	recordErr := v.Tasks.RecordVerification(task, summary)
	if recordErr != nil {
		if summary.Passed {
			summary.Passed = false
			summary.Error = ErrVerificationFailed.Error()
		}

		return summary, recordErr
	}

	if !summary.Passed {
		return summary, ErrVerificationFailed
	}

	return summary, nil
}

// VerifyStandard provides a simple build/test verification profile.
//
// Empty command strings are ignored. At least one of the supplied commands
// must therefore be present for verification to succeed.
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
// Every supplied command is treated as a required verification check.
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
			Name:     fmt.Sprintf("check-%d", index+1),
			Required: true,
			Command: Command{
				Command: command,
			},
		})
	}

	return v.Verify(ctx, task, checks)
}

func normalizeCheck(
	check VerificationCheck,
	index int,
) (VerificationCheck, error) {
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

func finishVerification(
	summary VerificationSummary,
	err error,
) (VerificationSummary, error) {
	finished := time.Now().UTC()

	summary.FinishedAt = finished
	summary.Duration = finished.Sub(summary.StartedAt)
	summary.Passed = false

	if err != nil {
		summary.Error = err.Error()
	}

	return summary, err
}
