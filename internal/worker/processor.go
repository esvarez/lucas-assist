// Package worker implements the Agent Worker's core processing loop
// (architecture.md §1/§10/§11): lease a queued AgentRun, run its skill,
// save the result as a Changeset, and mark the run completed or failed.
// It knows nothing about SQS — cmd/worker unmarshals the queue message and
// calls Processor.ProcessRun with the identifiers it needs.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/agent/skills"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

// DefaultLeaseDuration is how long a worker holds a run before another
// attempt may reclaim it (architecture.md §15). Kept comfortably under
// AgentJobsQueue's 360s VisibilityTimeout (template.yaml) so a lease
// doesn't outlive the point at which SQS would redeliver the message
// anyway.
const DefaultLeaseDuration = 5 * time.Minute

// RunRepository is the slice of store.Repository the worker actually
// calls — same narrowing convention as internal/api's ProjectRepository
// and internal/skillsapi's AgentRunStore.
type RunRepository interface {
	GetAgentRun(ctx context.Context, userID, projectID, runID string) (domain.AgentRun, error)
	LeaseAgentRun(ctx context.Context, userID, projectID, runID, workerID string, leaseUntil time.Time) (domain.AgentRun, error)
	CompleteAgentRun(ctx context.Context, userID, projectID, runID, changesetID string) (domain.AgentRun, error)
	FailAgentRun(ctx context.Context, userID, projectID, runID, errMsg string) (domain.AgentRun, error)
	GetProject(ctx context.Context, userID, id string) (domain.Project, error)
	CreateChangeset(ctx context.Context, c domain.Changeset) (domain.Changeset, error)
	GetChangeset(ctx context.Context, userID, projectID, changesetID string) (domain.Changeset, error)
}

// runSkillFunc matches agent.Run's signature — a seam so tests can stub
// the model call instead of hitting OpenAI, same testability principle as
// agent.Run's own newChatCompletion seam and internal/queue's
// sendMessageAPI.
type runSkillFunc func(ctx context.Context, s agent.Skill, rawInput json.RawMessage) (any, error)

// Processor leases and processes one AgentRun at a time.
type Processor struct {
	Repo     RunRepository
	Registry *agent.Registry
	WorkerID string

	// LeaseFor defaults to DefaultLeaseDuration when zero.
	LeaseFor time.Duration

	// runSkill defaults to agent.Run; tests override it.
	runSkill runSkillFunc
}

// NewProcessor builds a Processor wired to call the real agent.Run.
func NewProcessor(repo RunRepository, registry *agent.Registry, workerID string) *Processor {
	return &Processor{
		Repo:     repo,
		Registry: registry,
		WorkerID: workerID,
		LeaseFor: DefaultLeaseDuration,
		runSkill: agent.Run,
	}
}

func (p *Processor) leaseFor() time.Duration {
	if p.LeaseFor > 0 {
		return p.LeaseFor
	}
	return DefaultLeaseDuration
}

func (p *Processor) doRunSkill() runSkillFunc {
	if p.runSkill != nil {
		return p.runSkill
	}
	return agent.Run
}

// isTerminal reports whether status is one a worker should never touch
// again.
func isTerminal(status domain.AgentRunStatus) bool {
	switch status {
	case domain.AgentRunCompleted, domain.AgentRunFailed, domain.AgentRunCancelled:
		return true
	default:
		return false
	}
}

// ProcessRun handles one AgentRun end to end, tolerating SQS's
// at-least-once delivery (architecture.md §15):
//
//   - Already terminal (completed/failed/cancelled): a duplicate delivery
//     of a finished job — no-op, never calls the model twice.
//   - Not leasable (another attempt holds a live lease): also a no-op —
//     let SQS's visibility timeout handle redelivery rather than racing
//     another worker.
//   - Otherwise: lease it, run the skill, and save the outcome.
//
// Past the point of leasing, failures split into two kinds:
//
//   - Permanent (unknown skill, or a model result with nothing usable in
//     it — needs_clarification, or status "ok" with an empty payload):
//     retrying with the same input won't help, so these mark the run
//     failed and return nil — the run's Error field is where that belongs
//     (GET /agent-runs/{id}, #100), not a Lambda error that would just
//     trigger a pointless redelivery.
//   - Transient (the model call itself, or a DynamoDB read/write): these
//     are returned as real errors instead, leaving the run "running" so
//     AgentJobsQueue's RedrivePolicy (architecture.md §15, up to
//     maxReceiveCount attempts) actually gets a chance to retry before
//     landing in the DLQ. Marking these failed immediately would burn the
//     job's one shot on a blip and silently defeat the DLQ's purpose.
func (p *Processor) ProcessRun(ctx context.Context, userID, projectID, runID string) error {
	run, err := p.Repo.GetAgentRun(ctx, userID, projectID, runID)
	if err != nil {
		return fmt.Errorf("get agent run %q: %w", runID, err)
	}

	if isTerminal(run.Status) {
		return nil
	}

	leaseUntil := time.Now().Add(p.leaseFor())
	leased, err := p.Repo.LeaseAgentRun(ctx, userID, projectID, runID, p.WorkerID, leaseUntil)
	if err != nil {
		if errors.Is(err, store.ErrRunLeased) {
			return nil
		}
		return fmt.Errorf("lease agent run %q: %w", runID, err)
	}

	skill, err := p.Registry.Get(leased.Skill)
	if err != nil {
		// Permanent: retrying won't change which skill this run names.
		return p.fail(ctx, leased, err)
	}

	result, err := p.doRunSkill()(ctx, skill, leased.Input)
	if err != nil {
		// Transient: an OpenAI call failure (rate limit, 5xx, timeout) may
		// well succeed on a later attempt.
		return fmt.Errorf("run skill %q for agent run %q: %w", leased.Skill, runID, err)
	}

	changeset, err := changesetFromResult(leased, result)
	if err != nil {
		// Permanent: the model's own output gives us nothing to act on.
		return p.fail(ctx, leased, err)
	}

	// BaseVersion is the project's version at the moment of proposing, not
	// at the moment the run was created — a decompose_task run can sit
	// queued for a while, during which the project's version could move.
	// Reading it fresh here, right before saving the changeset, is what
	// makes the later accept-time version check (#77) meaningful; a
	// create_project changeset has no project yet, so there's nothing to
	// read.
	if changeset.ProjectID != "" {
		project, err := p.Repo.GetProject(ctx, userID, changeset.ProjectID)
		if err != nil {
			// Transient: a DynamoDB read failure, not a reason to discard
			// a model result we already paid for.
			return fmt.Errorf("load project for changeset base version, agent run %q: %w", runID, err)
		}
		changeset.BaseVersion = project.Version
	}

	// changeset.ID is deterministic (the run's own ID, which nothing else
	// keys off of) rather than freshly generated, so a retry after
	// CreateChangeset succeeds but CompleteAgentRun doesn't — a crash
	// between the two calls, or CompleteAgentRun itself erroring — reuses
	// the changeset a prior attempt already saved instead of writing a
	// second, duplicate proposal. CreateChangeset's own attribute_not_exists
	// guard is what makes this safe to just retry.
	changeset.ID = leased.ID
	created, err := p.Repo.CreateChangeset(ctx, changeset)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateID) {
			created, err = p.Repo.GetChangeset(ctx, userID, changeset.ProjectID, changeset.ID)
		}
		if err != nil {
			// Transient: either the original write or this recovery read
			// hit a DynamoDB error, not a permanent reason to give up.
			return fmt.Errorf("save changeset for agent run %q: %w", runID, err)
		}
	}

	if _, err := p.Repo.CompleteAgentRun(ctx, userID, projectID, runID, created.ID); err != nil {
		// Transient, and safe to retry from the top: the changeset above
		// is now saved under a deterministic ID, so a later attempt finds
		// it via ErrDuplicateID instead of duplicating it.
		return fmt.Errorf("complete agent run %q: %w", runID, err)
	}
	return nil
}

// fail marks run failed with cause's message and reports whether that
// itself succeeded — mirroring skillsapi's enqueue-failure handling
// (internal/skillsapi/handler.go), so a run never silently stays "running"
// forever just because FailAgentRun also errored.
func (p *Processor) fail(ctx context.Context, run domain.AgentRun, cause error) error {
	if _, err := p.Repo.FailAgentRun(ctx, run.UserID, run.ProjectID, run.ID, cause.Error()); err != nil {
		return fmt.Errorf("%w (and failed to mark run %q failed: %v)", cause, run.ID, err)
	}
	return nil
}

// changesetFromResult converts a skill's Parse output into the Changeset
// shape to save. Exactly one of decompose_task's or create_project's
// result types is expected, since the registry only holds those two
// skills today (cmd/skills/main.go, cmd/local/main.go).
//
// create_project's result can still carry status "needs_clarification",
// which has no representation as a Changeset, so it's treated as a
// failure here rather than silently dropped or half-saved.
// decompose_task no longer has this branch at all (#42) — it always
// decomposes and discloses unspecified choices as Assumptions instead.
func changesetFromResult(run domain.AgentRun, result any) (domain.Changeset, error) {
	base := domain.Changeset{
		ProjectID: run.ProjectID,
		UserID:    run.UserID,
		Skill:     run.Skill,
		Status:    domain.ChangesetProposed,
	}

	switch r := result.(type) {
	case skills.DecomposeResult:
		// Defensive: strict mode's schema can't express "Subtasks is
		// non-empty" as a hard constraint, so a malformed response could
		// otherwise complete the run with an empty, useless changeset.
		if len(r.Subtasks) == 0 {
			return domain.Changeset{}, fmt.Errorf("%s: no subtasks in the model's response", run.Skill)
		}
		base.ProposedTasks = r.Subtasks
		base.Assumptions = r.Assumptions
	case skills.CreateProjectResult:
		if r.Status != "ok" {
			return domain.Changeset{}, fmt.Errorf("%s: needs clarification, cannot save a changeset yet (see #42): %v", run.Skill, r.Questions)
		}
		// Same defensive reasoning as Subtasks above — Project is a
		// nullable field of the status union, so strict mode doesn't
		// forbid status "ok" with Project null.
		if r.Project == nil {
			return domain.Changeset{}, fmt.Errorf("%s: status \"ok\" but no project in the model's response", run.Skill)
		}
		base.ProposedProject = r.Project
	default:
		return domain.Changeset{}, fmt.Errorf("%s: unrecognized skill result type %T", run.Skill, result)
	}

	return base, nil
}
