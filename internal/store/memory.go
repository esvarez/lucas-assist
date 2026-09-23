package store

import (
	"context"
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

// idempotencyRecord is MemoryRepository's cache entry for one accept-
// changeset idempotency key — see AcceptChangeset and
// AcceptCreateProjectChangeset. Result holds whichever of
// AcceptChangesetResult/AcceptCreateProjectResult that call produced —
// an "any" rather than a second map, since both share one per-user
// idempotency-key namespace (architecture.md §15).
type idempotencyRecord struct {
	RequestHash string
	Result      any
}

// taskRecord pairs a Task with the userID it was created under. domain.Task
// itself carries no UserID (only ProjectID) — see store.Repository's doc
// comment — so the owning user has to be tracked alongside it here instead.
type taskRecord struct {
	domain.Task
	UserID string
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

	// Mirrors DynamoRepository's read-time default (projectItem.toDomain):
	// a project created without a Domain is General, not empty.
	if p.Domain == "" {
		p.Domain = domain.ProjectDomainGeneral
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
	run.Questions = nil
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

func (r *MemoryRepository) CompleteAgentRun(ctx context.Context, userID, projectID, runID, changesetID string) (domain.AgentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, ok := r.agentRuns[runID]
	if !ok || run.UserID != userID || run.ProjectID != projectID {
		return domain.AgentRun{}, ErrNotFound
	}

	run.Status = domain.AgentRunCompleted
	run.Error = ""
	run.ChangesetID = changesetID
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

func (r *MemoryRepository) NeedsInputAgentRun(ctx context.Context, userID, projectID, runID string, questions []string) (domain.AgentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, ok := r.agentRuns[runID]
	if !ok || run.UserID != userID || run.ProjectID != projectID {
		return domain.AgentRun{}, ErrNotFound
	}

	run.Status = domain.AgentRunNeedsInput
	run.Questions = questions
	run.UpdatedAt = time.Now().UTC()

	r.agentRuns[runID] = run
	return run, nil
}

// AcceptChangeset holds r.mu for the whole operation, which is what makes
// it atomic here — there's no separate transaction primitive to reach for
// in a single in-memory map, unlike DynamoRepository's TransactWriteItems.
func (r *MemoryRepository) AcceptChangeset(ctx context.Context, p domain.Project, c domain.Changeset, idempotencyKey string) (AcceptChangesetResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hash := requestHash(c.UserID, c.ProjectID, c.ID)
	idemKey := c.UserID + "|" + idempotencyKey
	if rec, ok := r.idempotency[idemKey]; ok {
		if rec.RequestHash != hash {
			return AcceptChangesetResult{}, ErrIdempotencyKeyReused
		}
		return rec.Result.(AcceptChangesetResult), nil
	}

	existingProject, ok := r.projects[p.ID]
	if !ok || existingProject.UserID != c.UserID {
		return AcceptChangesetResult{}, ErrNotFound
	}

	// This check has to run after the idempotency lookup above, not
	// before: a replay of an already-applied changeset (same key) must
	// still return the cached result, even though the changeset's current
	// status is no longer "proposed". A *different* key against a
	// non-proposed changeset (already applied/rejected/expired/conflict)
	// is a genuine conflict — left as-is, not flipped to "conflict", since
	// that status may already correctly describe something else (e.g.
	// "rejected").
	if c.Status != domain.ChangesetProposed {
		return AcceptChangesetResult{}, ErrConflict
	}

	if existingProject.Version != c.BaseVersion {
		c.Status = domain.ChangesetConflict
		r.changesets[c.ID] = c
		return AcceptChangesetResult{}, ErrConflict
	}

	now := time.Now().UTC()

	tasks := make([]domain.Task, 0, len(c.ProposedTasks))
	for i, pt := range c.ProposedTasks {
		t := domain.Task{
			ID:          domain.NewID(),
			ProjectID:   c.ProjectID,
			Title:       pt.Title,
			Description: pt.Description,
			// "todo", not some other unstarted value: it's the status the
			// rest of the app (task-status.ts, the "what's next" candidate
			// selection) already treats as eligible and unstarted — a
			// mismatched value here would make a freshly accepted task
			// invisible to both.
			Status:             "todo",
			Order:              i,
			AcceptanceCriteria: pt.AcceptanceCriteria,
		}
		r.tasks[t.ID] = taskRecord{Task: t, UserID: c.UserID}
		tasks = append(tasks, t)
	}

	existingProject.Version++
	existingProject.UpdatedAt = now
	r.projects[existingProject.ID] = existingProject

	event := domain.Event{
		ID:          domain.NewID(),
		ProjectID:   c.ProjectID,
		UserID:      c.UserID,
		Type:        domain.EventChangesetAccepted,
		ChangesetID: c.ID,
		ActorID:     c.UserID,
		BaseVersion: c.BaseVersion,
		CreatedAt:   now,
	}
	r.events[event.ID] = event

	c.Status = domain.ChangesetApplied
	r.changesets[c.ID] = c

	result := AcceptChangesetResult{Project: existingProject, Tasks: tasks, Event: event}
	r.idempotency[idemKey] = idempotencyRecord{RequestHash: hash, Result: result}

	return result, nil
}

// AcceptCreateProjectChangeset holds r.mu for the whole operation, same
// reasoning as AcceptChangeset.
func (r *MemoryRepository) AcceptCreateProjectChangeset(ctx context.Context, c domain.Changeset, idempotencyKey string) (AcceptCreateProjectResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hash := requestHash(c.UserID, c.ProjectID, c.ID)
	idemKey := c.UserID + "|" + idempotencyKey
	if rec, ok := r.idempotency[idemKey]; ok {
		if rec.RequestHash != hash {
			return AcceptCreateProjectResult{}, ErrIdempotencyKeyReused
		}
		return rec.Result.(AcceptCreateProjectResult), nil
	}

	// Same ordering as AcceptChangeset: this check has to run after the
	// idempotency lookup above, so a replay of an already-applied
	// changeset still returns the cached result.
	if c.Status != domain.ChangesetProposed {
		return AcceptCreateProjectResult{}, ErrConflict
	}

	now := time.Now().UTC()

	project := domain.Project{
		UserID:    c.UserID,
		ID:        domain.NewID(),
		Name:      c.ProposedProject.Name,
		Goal:      c.ProposedProject.Goal,
		Deadline:  c.ProposedProject.Deadline,
		CreatedAt: now,
		UpdatedAt: now,
		Version:   1,
	}
	if c.ProposedProject.Constraints != nil {
		project.Constraints = c.ProposedProject.Constraints
	} else {
		project.Constraints = []string{}
	}
	r.projects[project.ID] = project

	event := domain.Event{
		ID:          domain.NewID(),
		ProjectID:   project.ID,
		UserID:      c.UserID,
		Type:        domain.EventProjectCreated,
		ChangesetID: c.ID,
		ActorID:     c.UserID,
		CreatedAt:   now,
	}
	r.events[event.ID] = event

	c.Status = domain.ChangesetApplied
	r.changesets[c.ID] = c

	result := AcceptCreateProjectResult{Project: project}
	r.idempotency[idemKey] = idempotencyRecord{RequestHash: hash, Result: result}

	return result, nil
}
