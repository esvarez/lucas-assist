import { SparklesIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

// BreakIntoTasksButton is a plain trigger — the caller owns the
// useDecomposeRun instance and renders whatever comes back inline in the
// Tasks section (#169), so this component has nothing to do beyond
// starting that run and reflecting whether one is already in flight.
function BreakIntoTasksButton({
  onStart,
  working,
  disabled,
}: {
  onStart: () => void
  // Shows a spinner instead of the sparkles icon while this project's
  // decompose_task run is dispatching/polling.
  working: boolean
  // True whenever any proposal is pending for this project (working,
  // needing clarification, or awaiting review) — #169 allows only one at
  // a time, so the trigger disables rather than letting a second dispatch
  // race the first.
  disabled: boolean
}) {
  return (
    <Button onClick={onStart} disabled={disabled}>
      {working ? <Spinner data-icon="inline-start" className="size-4" /> : <SparklesIcon data-icon="inline-start" />}
      Break into tasks
    </Button>
  )
}

export default BreakIntoTasksButton
