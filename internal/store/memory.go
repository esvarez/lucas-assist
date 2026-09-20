package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/esvarez/lucas-assist/internal/domain"
)

// MemoryRepository is an in-memory Repository for local dev and unit
// tests. State lives only as long as the process — nothing is durable.
type MemoryRepository struct {
	mu          sync.Mutex
	projects    map[string]domain.Project
	tasks       map[string]taskRecord
	changesets  map[string]domain.Changeset
	agentRuns   map[string]domain.AgentRun
	events      map[string]domain.Event
	idempotency map[string]idempotencyRecord
}

// taskRecord pairs a Task with the userID it was created under. domain.Task
// itself carries no UserID (only ProjectID) — see store.Repository's doc
// comment — so the owning user has to be tracked alongside it here instead.
type taskRecord struct {
	domain.Task
	UserID string
}

// idempotencyRecord is AcceptChangeset's idempotency-key bookkeeping
// (architecture.md §15): "the request hash and final result reference."
// The result reference here is just the created Task IDs — everything
// else needed to rebuild the same AcceptChangesetResult (the Project) is
// already independently readable.
type idempotencyRecord struct {
	UserID      string
	RequestHash string
	TaskIDs     []string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		projects:    make(map[string]domain.Project),
		tasks:       make(map[string]taskRecord),
		changesets:  make(map[string]domain.Changeset),
		agentRuns:   make(map[string]domain.AgentRun),
		events:      make(map[string]domain.Event),
		idempotency: make(map[string]idempotencyRecord),
	}
}

func (r *MemoryRepository) CreateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p.ID == "" {
		p.ID = domain.NewID()
	} else if _, exists := r.projects[p.ID]; exists {
		return domain.Project{}, ErrDuplicateID
	}

	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.Version = 1

	r.projects[p.ID] = p
	return p, nil
}

func (r *MemoryRepository) GetProject(ctx context.Context, userID, id string) (domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.projects[id]
	if !ok || p.UserID != userID {
		return domain.Project{}, ErrNotFound
	}
	return p, nil
}

func (r *MemoryRepository) DeleteProject(ctx context.Context, userID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.projects[id]
	if !ok || p.UserID != userID {
		return ErrNotFound
	}

	delete(r.projects, id)
	return nil
}

func (r *MemoryRepository) UpdateProject(ctx context.Context, userID string, p domain.Project) (domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.projects[p.ID]
	if !ok || existing.UserID != userID {
		return domain.Project{}, ErrNotFound
	}
	if existing.Version != p.Version {
		return domain.Project{}, ErrConflict
	}

	existing.Goal = p.Goal
	existing.Deadline = p.Deadline
	existing.Constraints = p.Constraints
	existing.Status = p.Status
	existing.Version++
	existing.UpdatedAt = time.Now().UTC()

	r.projects[existing.ID] = existing
	return existing, nil
}

func (r *MemoryRepository) ListProjects(ctx context.Context, userID string) ([]domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	projects := make([]domain.Project, 0)
	for _, p := range r.projects {
		if p.UserID == userID {
			projects = append(projects, p)
		}
	}
	return projects, nil
}

func (r *MemoryRepository) CreateTask(ctx context.Context, userID string, t domain.Task) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if t.ID == "" {
		t.ID = domain.NewID()
	} else if _, exists := r.tasks[t.ID]; exists {
		return domain.Task{}, ErrDuplicateID
	}

	r.tasks[t.ID] = taskRecord{Task: t, UserID: userID}
	return t, nil
}

func (r *MemoryRepository) GetTask(ctx context.Context, userID, projectID, taskID string) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.tasks[taskID]
	if !ok || rec.UserID != userID || rec.ProjectID != projectID {
		return domain.Task{}, ErrNotFound
	}
	return rec.Task, nil
}

func (r *MemoryRepository) ListTasks(ctx context.Context, userID, projectID string) ([]domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tasks := make([]domain.Task, 0)
	for _, rec := range r.tasks {
		if rec.UserID == userID && rec.ProjectID == projectID {
			tasks = append(tasks, rec.Task)
		}
	}
	return tasks, nil
}

func (r *MemoryRepository) CreateChangeset(ctx context.Context, c domain.Changeset) (domain.Changeset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if c.ID == "" {
		c.ID = domain.NewID()
	} else if _, exists := r.changesets[c.ID]; exists {
		return domain.Changeset{}, ErrDuplicateID
	}

	c.CreatedAt = time.Now().UTC()

	r.changesets[c.ID] = c
	return c, nil
}

func (r *MemoryRepository) GetChangeset(ctx context.Context, userID, projectID, changesetID string) (domain.Changeset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.changesets[changesetID]
	if !ok || c.UserID != userID || c.ProjectID != projectID {
		return domain.Changeset{}, ErrNotFound
	}
	return c, nil
}

func (r *MemoryRepository) UpdateChangesetStatus(ctx context.Context, userID, projectID, changesetID string, status domain.ChangesetStatus) (domain.Changeset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.changesets[changesetID]
	if !ok || c.UserID != userID || c.ProjectID != projectID {
		return domain.Changeset{}, ErrNotFound
	}

	c.Status = status
	r.changesets[changesetID] = c
	return c, nil
}

func (r *MemoryRepository) CreateAgentRun(ctx context.Context, run domain.AgentRun) (domain.AgentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if run.ID == "" {
		run.ID = domain.NewRunID()
	} else if _, exists := r.agentRuns[run.ID]; exists {
		return domain.AgentRun{}, ErrDuplicateID
	}

	now := time.Now().UTC()
	run.Status = domain.AgentRunQueued
	run.Attempt = 0
	run.WorkerID = ""
	run.LeaseUntil = nil
	run.Error = ""
	run.CreatedAt = now
	run.UpdatedAt = now

	r.agentRuns[run.ID] = run
	return run, nil
}

func (r *MemoryRepository) GetAgentRun(ctx context.Context, userID, projectID, runID string) (domain.AgentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, ok := r.agentRuns[runID]
	if !ok || run.UserID != userID || run.ProjectID != projectID {
		return domain.AgentRun{}, ErrNotFound
	}
	return run, nil
}

// LeaseAgentRun is leasable when the run is still queued, or when it's
// running but its previous lease has expired (reclaiming a crashed or
// timed-out worker's attempt).
func (r *MemoryRepository) LeaseAgentRun(ctx context.Context, userID, projectID, runID, workerID string, leaseUntil time.Time) (domain.AgentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, ok := r.agentRuns[runID]
	if !ok || run.UserID != userID || run.ProjectID != projectID {
		return domain.AgentRun{}, ErrNotFound
	}

	now := time.Now().UTC()
	expiredLease := run.Status == domain.AgentRunRunning && run.LeaseUntil != nil && run.LeaseUntil.Before(now)
	if run.Status != domain.AgentRunQueued && !expiredLease {
		return domain.AgentRun{}, ErrRunLeased
	}

	run.Status = domain.AgentRunRunning
	run.WorkerID = workerID
	lease := leaseUntil
	run.LeaseUntil = &lease
	run.Attempt++
	run.UpdatedAt = now

	r.agentRuns[runID] = run
	return run, nil
}

func (r *MemoryRepository) CompleteAgentRun(ctx context.Context, userID, projectID, runID string) (domain.AgentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, ok := r.agentRuns[runID]
	if !ok || run.UserID != userID || run.ProjectID != projectID {
		return domain.AgentRun{}, ErrNotFound
	}

	run.Status = domain.AgentRunCompleted
	run.Error = ""
	run.UpdatedAt = time.Now().UTC()

	r.agentRuns[runID] = run
	return run, nil
}

func (r *MemoryRepository) FailAgentRun(ctx context.Context, userID, projectID, runID, errMsg string) (domain.AgentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, ok := r.agentRuns[runID]
	if !ok || run.UserID != userID || run.ProjectID != projectID {
		return domain.AgentRun{}, ErrNotFound
	}

	run.Status = domain.AgentRunFailed
	run.Error = errMsg
	run.UpdatedAt = time.Now().UTC()

	r.agentRuns[runID] = run
	return run, nil
}

// AcceptChangeset commits a proposed changeset's Task mutations, bumps the
// project version, appends an Event, and records the idempotency key — all
// under the same lock, so there's no partial-application window to guard
// against the way the DynamoDB implementation needs a real transaction for.
func (r *MemoryRepository) AcceptChangeset(ctx context.Context, in AcceptChangesetInput) (AcceptChangesetResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if rec, ok := r.idempotency[in.IdempotencyKey]; ok {
		if rec.UserID != in.UserID || rec.RequestHash != in.RequestHash {
			return AcceptChangesetResult{}, ErrIdempotencyKeyMismatch
		}
		return r.loadAcceptResultLocked(in.UserID, in.ProjectID, rec.TaskIDs)
	}

	changeset, ok := r.changesets[in.ChangesetID]
	if !ok || changeset.UserID != in.UserID || changeset.ProjectID != in.ProjectID {
		return AcceptChangesetResult{}, ErrNotFound
	}
	if changeset.Status != domain.ChangesetProposed {
		return AcceptChangesetResult{}, ErrChangesetNotProposed
	}
	if len(changeset.ProposedTasks) > maxChangesetMutations {
		return AcceptChangesetResult{}, ErrChangesetTooLarge
	}

	project, ok := r.projects[in.ProjectID]
	if !ok || project.UserID != in.UserID {
		return AcceptChangesetResult{}, ErrNotFound
	}
	if project.Version != changeset.BaseVersion {
		changeset.Status = domain.ChangesetConflict
		r.changesets[in.ChangesetID] = changeset
		return AcceptChangesetResult{}, ErrConflict
	}

	tasks := make([]domain.Task, 0, len(changeset.ProposedTasks))
	taskIDs := make([]string, 0, len(changeset.ProposedTasks))
	for i, pt := range changeset.ProposedTasks {
		task := domain.Task{
			ID:                 domain.NewID(),
			ProjectID:          in.ProjectID,
			Title:              pt.Title,
			Description:        pt.Description,
			AcceptanceCriteria: pt.AcceptanceCriteria,
			Status:             "pending",
			Order:              i,
		}
		r.tasks[task.ID] = taskRecord{Task: task, UserID: in.UserID}
		tasks = append(tasks, task)
		taskIDs = append(taskIDs, task.ID)
	}

	project.Version++
	project.UpdatedAt = time.Now().UTC()
	r.projects[in.ProjectID] = project

	changeset.Status = domain.ChangesetApplied
	r.changesets[in.ChangesetID] = changeset

	event := domain.Event{
		ID:          domain.NewEventID(),
		ProjectID:   in.ProjectID,
		UserID:      in.UserID,
		Type:        domain.EventChangesetApplied,
		ChangesetID: in.ChangesetID,
		ActorID:     in.UserID,
		BaseVersion: changeset.BaseVersion,
		CreatedAt:   time.Now().UTC(),
	}
	r.events[event.ID] = event

	r.idempotency[in.IdempotencyKey] = idempotencyRecord{
		UserID:      in.UserID,
		RequestHash: in.RequestHash,
		TaskIDs:     taskIDs,
	}

	return AcceptChangesetResult{Project: project, Tasks: tasks}, nil
}

// loadAcceptResultLocked rebuilds an AcceptChangesetResult from an
// idempotency record's stored task IDs, for a replayed accept request. The
// caller must already hold r.mu.
func (r *MemoryRepository) loadAcceptResultLocked(userID, projectID string, taskIDs []string) (AcceptChangesetResult, error) {
	project, ok := r.projects[projectID]
	if !ok || project.UserID != userID {
		return AcceptChangesetResult{}, ErrNotFound
	}

	tasks := make([]domain.Task, 0, len(taskIDs))
	for _, id := range taskIDs {
		rec, ok := r.tasks[id]
		if !ok {
			return AcceptChangesetResult{}, fmt.Errorf("replay accept changeset: task %q from idempotency record not found", id)
		}
		tasks = append(tasks, rec.Task)
	}

	return AcceptChangesetResult{Project: project, Tasks: tasks}, nil
}
