package lab

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type scriptedRepairAgent struct {
	decisions []RepairDecision
	calls     int
}

func (a *scriptedRepairAgent) Repair(
	_ context.Context,
	_ *Task,
	_ VerificationSummary,
	_ []RepairIteration,
) (RepairDecision, error) {
	a.calls++

	if len(a.decisions) == 0 {
		return RepairDecision{}, ErrRepairNoAction
	}

	index := a.calls - 1
	if index >= len(a.decisions) {
		index = len(a.decisions) - 1
	}

	return a.decisions[index], nil
}

func TestRepairLoopStopsAtMaxIterations(t *testing.T) {
	manager, task, verifier := newRepairTestFixture(t)

	controller, err := NewRepairController(
		manager,
		verifier,
		3,
	)
	if err != nil {
		t.Fatal(err)
	}

	agent := &scriptedRepairAgent{
		decisions: []RepairDecision{
			{
				Command: Command{
					Command: repairFailCommand(),
				},
			},
		},
	}

	summary, err := controller.Run(
		context.Background(),
		task,
		agent,
	)
	if err == nil {
		t.Fatal("expected repair loop to stop with an error")
	}

	if !errors.Is(err, ErrRepairMaxIterations) {
		t.Fatalf(
			"expected ErrRepairMaxIterations, got %v",
			err,
		)
	}

	if summary.Iterations != 3 {
		t.Fatalf(
			"iterations = %d, want 3",
			summary.Iterations,
		)
	}

	if agent.calls != 3 {
		t.Fatalf(
			"agent calls = %d, want 3",
			agent.calls,
		)
	}
}

func TestRepairLoopDoesNotRetryIdenticalCommandForever(t *testing.T) {
	manager, task, verifier := newRepairTestFixture(t)

	controller, err := NewRepairController(
		manager,
		verifier,
		100,
	)
	if err != nil {
		t.Fatal(err)
	}

	agent := &scriptedRepairAgent{
		decisions: []RepairDecision{
			{
				Command: Command{
					Command: repairFailCommand(),
				},
			},
		},
	}

	summary, err := controller.Run(
		context.Background(),
		task,
		agent,
	)
	if err == nil {
		t.Fatal("expected repeated-command protection to stop the loop")
	}

	if !errors.Is(err, ErrRepairRepeatedCommand) {
		t.Fatalf(
			"expected ErrRepairRepeatedCommand, got %v",
			err,
		)
	}

	if summary.Iterations != 3 {
		t.Fatalf(
			"iterations = %d, want 3",
			summary.Iterations,
		)
	}

	if agent.calls != 3 {
		t.Fatalf(
			"agent calls = %d, want 3",
			agent.calls,
		}
	}

	if len(summary.History) != 3 {
		t.Fatalf(
			"history length = %d, want 3",
			len(summary.History),
		)
	}
}

func newRepairTestFixture(
	t *testing.T,
) (*TaskManager, *Task, *Verifier) {
	t.Helper()

	root := t.TempDir()

	source := filepath.Join(root, "source")
	workspaces := filepath.Join(root, "workspaces")

	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}

	// Ensure native Go discovery has a meaningful project to verify.
	goMod := []byte("module example.com/repair-test\n\ngo 1.22\n")
	goFile := []byte("package main\n\nfunc main() {}\n")

	if err := os.WriteFile(
		filepath.Join(source, "go.mod"),
		goMod,
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(source, "main.go"),
		goFile,
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	workspaceManager, err := NewWorkspaceManager(workspaces)
	if err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(
		5*time.Second,
		2*1024*1024,
	)

	tasks, err := NewTaskManager(
		workspaceManager,
		runner,
		DefaultPolicy(),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := NewVerifier(tasks)
	if err != nil {
		t.Fatal(err)
	}

	task := tasks.NewTask(
		"repair test",
		"exercise bounded repair",
	)

	if err := tasks.Start(
		context.Background(),
		task,
		source,
	); err != nil {
		t.Fatal(err)
	}

	return tasks, task, verifier
}

func repairFailCommand() string {
	if os.PathSeparator == '\\' {
		return "cmd /c exit 1"
	}

	return "sh -c 'exit 1'"
}
