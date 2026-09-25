import { useId, useState, type FormEvent } from 'react'
import { CheckIcon, InfoIcon, XIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Item, ItemActions, ItemContent, ItemGroup } from '@/components/ui/item'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import type { AcceptTaskSelection, Changeset, ProposedTask } from '@/src/api/skills'

// Presentational pieces shared by every decompose_task entry point for the
// 'working' / 'clarify' / 'review' / 'error' states a useDecomposeRun can
// be in — rendered inline in the Tasks section (#169), not behind a
// dialog, so a pending proposal is just part of the page rather than a
// modal blocking it. A plain Card stands in for what DialogContent used
// to wrap these in.

const actionsRowClassName = 'flex flex-col-reverse gap-2 sm:flex-row sm:justify-end'

export function WorkingStep({ label }: { label: string }) {
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-3 py-8 text-sm text-muted-foreground">
        <Spinner className="size-6" />
        <p>{label}</p>
      </CardContent>
    </Card>
  )
}

export function ClarifyStep({
  questions,
  answers,
  onAnswerChange,
  onSubmit,
  onCancel,
}: {
  questions: string[]
  answers: string[]
  onAnswerChange: (index: number, value: string) => void
  onSubmit: (e: FormEvent) => void
  onCancel: () => void
}) {
  const baseId = useId()
  return (
    <Card>
      <CardContent>
        <form onSubmit={onSubmit} className="flex flex-col gap-4">
          <p className="text-xs text-muted-foreground">
            A couple of details would change which subtasks make sense.
          </p>
          {questions.map((question, index) => (
            <div key={index} className="flex flex-col gap-2">
              <Label htmlFor={`${baseId}-${index}`}>{question}</Label>
              <Textarea
                id={`${baseId}-${index}`}
                value={answers[index] ?? ''}
                onChange={(e) => onAnswerChange(index, e.target.value)}
                placeholder="Your answer (or “no preference” / “out of scope”)"
                required
              />
            </div>
          ))}
          <div className={actionsRowClassName}>
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
            <Button type="submit" disabled={answers.some((a) => !a.trim())}>
              Continue
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

// ReviewStep owns its own editable copy of changeset.proposed_tasks
// (#169): the user can retitle, redescribe, remove, or accept each
// subtask independently — the model's proposal is a starting point, not a
// mandate (ADR 002 still requires this explicit acceptance either way).
// Accepting or removing a row calls out immediately (persisted by the
// caller); editing a row is purely local until that row is accepted.
// acceptance_criteria is shown but not editable — editing a whole
// criteria list is more than this pass is worth, and removing the task it
// belongs to is still one click away.
export function ReviewStep({
  changeset,
  onAcceptSelections,
  onRemoveTasks,
}: {
  changeset: Changeset
  // Commits exactly these (index + edited content) — one row's Accept
  // sends a single-element array, "Accept all" sends every row.
  onAcceptSelections: (selections: AcceptTaskSelection[]) => void
  // Persists whatever should remain after a removal — one row's Remove
  // sends everything except that row, "Reject all" sends [].
  onRemoveTasks: (remaining: ProposedTask[]) => void
}) {
  const [tasks, setTasks] = useState<ProposedTask[]>(() => changeset.proposed_tasks ?? [])
  const assumptions = changeset.assumptions ?? []

  function updateTask(index: number, patch: Partial<ProposedTask>) {
    setTasks((prev) => prev.map((task, i) => (i === index ? { ...task, ...patch } : task)))
  }

  // removeTask drops `remaining` from the on-screen list immediately, in
  // addition to persisting it via onRemoveTasks (#173) — the row otherwise
  // stayed visible after a successful removal, since nothing else here
  // ever wrote back to `tasks` (the state this component actually
  // renders): onRemoveTasks's result only reaches this component as a new
  // `changeset` prop, and ReviewStep isn't remounted when that changes, so
  // the `useState` initializer below never re-runs to pick it up.
  function removeTask(remaining: ProposedTask[]) {
    setTasks(remaining)
    onRemoveTasks(remaining)
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4">
        <p className="text-xs text-muted-foreground">
          Nudge proposed these subtasks — edit, remove, or accept them individually or all at once. Nothing is
          saved until you accept it.
        </p>
        {assumptions.length > 0 && (
          <Alert>
            <InfoIcon />
            <AlertTitle>Assumptions made on your behalf</AlertTitle>
            <AlertDescription>
              <ul className="list-inside list-disc">
                {assumptions.map((assumption, index) => (
                  <li key={index}>{assumption}</li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        )}
        <ItemGroup>
          {tasks.map((task, index) => (
            <Item key={index} variant="outline" size="sm">
              <ItemContent className="gap-2">
                <Input
                  value={task.title}
                  onChange={(e) => updateTask(index, { title: e.target.value })}
                  placeholder="Task title"
                  aria-label={`Title for subtask ${index + 1}`}
                />
                <Textarea
                  value={task.description}
                  onChange={(e) => updateTask(index, { description: e.target.value })}
                  placeholder="What does done look like for this task?"
                  aria-label={`Description for subtask ${index + 1}`}
                  className="min-h-16 text-xs"
                />
                {task.acceptance_criteria.length > 0 && (
                  <ul className="list-inside list-disc text-xs text-muted-foreground">
                    {task.acceptance_criteria.map((criterion, i) => (
                      <li key={i}>{criterion}</li>
                    ))}
                  </ul>
                )}
              </ItemContent>
              <ItemActions>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => onAcceptSelections([{ index, ...task }])}
                  disabled={!task.title.trim()}
                  aria-label={`Accept subtask ${index + 1}`}
                >
                  <CheckIcon />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => removeTask(tasks.filter((_, i) => i !== index))}
                  aria-label={`Remove subtask ${index + 1}`}
                >
                  <XIcon />
                </Button>
              </ItemActions>
            </Item>
          ))}
        </ItemGroup>
        <div className={actionsRowClassName}>
          <Button type="button" variant="outline" onClick={() => removeTask([])}>
            Reject all
          </Button>
          <Button
            type="button"
            onClick={() => onAcceptSelections(tasks.map((task, index) => ({ index, ...task })))}
            disabled={tasks.some((t) => !t.title.trim())}
          >
            Accept all {tasks.length}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

export function ErrorStep({
  message,
  onClose,
  onRetry,
}: {
  message: string
  onClose: () => void
  // Omitted when the caller has no title/description to redispatch with
  // — e.g. a proposal rediscovered after a page refresh (#169), which
  // carries no memory of what originally created it. Only Dismiss shows
  // in that case, rather than retrying with a blank task.
  onRetry?: () => void
}) {
  return (
    <Card>
      <CardContent className="flex flex-col gap-4">
        <p className="text-sm text-destructive">{message}</p>
        <div className={actionsRowClassName}>
          <Button type="button" variant="outline" onClick={onClose}>
            Dismiss
          </Button>
          {onRetry && (
            <Button type="button" onClick={onRetry}>
              Try again
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
