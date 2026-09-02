package lab

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrSessionNotFound = errors.New("lab: task session not found")
	ErrSessionExists   = errors.New("lab: task session already exists")
)

// Session owns one active Coding Lab task.
//
// A Session is the runtime handle used by the tool layer. The Task itself
// remains the serializable state object; Session supplies synchronized access
// to that state.
type Session struct {
	ID        string
	Task      *Task
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SessionRegistry stores active Coding Lab sessions.
//
// The registry is intentionally independent from persistence. This provides a
// clean boundary for a future persistent task database without changing the
// agent tool contract.
type SessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionRegistry creates an empty active-task registry.
func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{
		sessions: make(map[string]*Session),
	}
}

// Create registers a task as an active session.
func (r *SessionRegistry) Create(task *Task) (*Session, error) {
	if r == nil {
		return nil, errors.New("lab: session registry is nil")
	}

	if task == nil || task.ID == "" {
		return nil, ErrTaskInvalid
	}

	now := time.Now().UTC()

	session := &Session{
		ID:        task.ID,
		Task:      task,
		CreatedAt: now,
		UpdatedAt: now,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[task.ID]; exists {
		return nil, fmt.Errorf("%w: %s", ErrSessionExists, task.ID)
	}

	r.sessions[task.ID] = session

	return session, nil
}

// Get returns an active session by task ID.
func (r *SessionRegistry) Get(taskID string) (*Session, error) {
	if r == nil {
		return nil, errors.New("lab: session registry is nil")
	}

	if taskID == "" {
		return nil, ErrSessionNotFound
	}

	r.mu.RLock()
	session, ok := r.sessions[taskID]
	r.mu.RUnlock()

	if !ok || session == nil || session.Task == nil {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, taskID)
	}

	return session, nil
}

// GetTask returns the active Task associated with a task ID.
func (r *SessionRegistry) GetTask(taskID string) (*Task, error) {
	session, err := r.Get(taskID)
	if err != nil {
		return nil, err
	}

	return session.Task, nil
}

// Touch updates the session's activity timestamp.
func (r *SessionRegistry) Touch(taskID string) error {
	if r == nil {
		return errors.New("lab: session registry is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.sessions[taskID]
	if !ok || session == nil {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, taskID)
	}

	session.UpdatedAt = time.Now().UTC()

	return nil
}

// Delete removes a task session from the active registry.
//
// It does not delete the task's workspace. Workspace cleanup remains the
// responsibility of TaskManager, which guarantees that filesystem boundaries
// are enforced in one place.
func (r *SessionRegistry) Delete(taskID string) error {
	if r == nil {
		return errors.New("lab: session registry is nil")
	}

	if taskID == "" {
		return ErrSessionNotFound
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.sessions[taskID]; !ok {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, taskID)
	}

	delete(r.sessions, taskID)

	return nil
}

// List returns a snapshot of all active sessions.
//
// The returned slice and session pointers are detached from the registry map,
// preventing callers from mutating the map itself. Task contents are still
// shared and should be treated as runtime state owned by the session.
func (r *SessionRegistry) List() []*Session {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Session, 0, len(r.sessions))

	for _, session := range r.sessions {
		if session == nil {
			continue
		}

		copySession := *session
		result = append(result, &copySession)
	}

	return result
}

// Count returns the number of active sessions.
func (r *SessionRegistry) Count() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.sessions)
}

// Active reports whether a task currently has an active session.
func (r *SessionRegistry) Active(taskID string) bool {
	if r == nil || taskID == "" {
		return false
	}

	r.mu.RLock()
	_, ok := r.sessions[taskID]
	r.mu.RUnlock()

	return ok
}

// RemoveCompleted removes terminal tasks from the active registry.
//
// This is useful after Finish, Fail, Cancel, or Block when the tool no longer
// needs the task in the active runtime map. The Task object itself can still
// be retained by the caller for reporting.
func (r *SessionRegistry) RemoveCompleted(taskID string) error {
	session, err := r.Get(taskID)
	if err != nil {
		return err
	}

	if session.Task == nil {
		return fmt.Errorf("%w: %s", ErrTaskNotFoundForCleanup(), taskID)
	}

	switch session.Task.Status {
	case TaskSucceeded, TaskFailed, TaskCanceled, TaskBlocked:
		return r.Delete(taskID)
	default:
		return fmt.Errorf(
			"lab: task %q is still active with status %s",
			taskID,
			session.Task.Status,
		)
	}
}

// SnapshotTasks returns the active tasks as a stable slice.
func (r *SessionRegistry) SnapshotTasks() []*Task {
	sessions := r.List()

	result := make([]*Task, 0, len(sessions))

	for _, session := range sessions {
		if session == nil || session.Task == nil {
			continue
		}

		result = append(result, session.Task)
	}

	return result
}

func ErrTaskNotFoundForCleanup() error {
	return ErrSessionNotFound
}
