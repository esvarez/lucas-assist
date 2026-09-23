import { useId, type FormEvent } from 'react'
import { InfoIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DialogFooter } from '@/components/ui/dialog'
import { Item, ItemContent, ItemDescription, ItemGroup, ItemTitle } from '@/components/ui/item'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import type { Changeset } from '@/src/api/skills'

// Presentational pieces shared by every decompose_task entry point
// (DecomposeTaskDialog's manual form, BreakIntoTasksButton's background
// flow, #166) for the 'working' / 'clarify' / 'review' / 'error' states a
// useDecomposeRun can be in — kept out of both so the two don't drift.

export function WorkingStep({ label }: { label: string }) {
  return (
    <div className="flex flex-col items-center gap-3 py-8 text-sm text-muted-foreground">
      <Spinner className="size-6" />
      <p>{label}</p>
    </div>
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
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" disabled={answers.some((a) => !a.trim())}>
          Continue
        </Button>
      </DialogFooter>
    </form>
  )
}

export function ReviewStep({
  changeset,
  onAccept,
  onCancel,
}: {
  changeset: Changeset
  onAccept: () => void
  onCancel: () => void
}) {
  const proposedTasks = changeset.proposed_tasks ?? []
  const assumptions = changeset.assumptions ?? []
  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-muted-foreground">
        Review the proposed subtasks below. Nothing is saved until you accept.
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
      <ItemGroup className="max-h-[50vh] overflow-y-auto">
        {proposedTasks.map((task, index) => (
          <Item key={index} variant="outline" size="sm">
            <ItemContent>
              <ItemTitle>{task.title}</ItemTitle>
              {task.description && <ItemDescription>{task.description}</ItemDescription>}
              {task.acceptance_criteria.length > 0 && (
                <ul className="list-inside list-disc text-xs text-muted-foreground">
                  {task.acceptance_criteria.map((criterion, i) => (
                    <li key={i}>{criterion}</li>
                  ))}
                </ul>
              )}
            </ItemContent>
          </Item>
        ))}
      </ItemGroup>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="button" onClick={onAccept}>
          Accept {proposedTasks.length} subtask{proposedTasks.length === 1 ? '' : 's'}
        </Button>
      </DialogFooter>
    </div>
  )
}

export function ErrorStep({
  message,
  onClose,
  onRetry,
}: {
  message: string
  onClose: () => void
  onRetry: () => void
}) {
  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-destructive">{message}</p>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          Close
        </Button>
        <Button type="button" onClick={onRetry}>
          Try again
        </Button>
      </DialogFooter>
    </div>
  )
}
