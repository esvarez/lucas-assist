import { useId, useState, type FormEvent, type ReactElement } from 'react'
import { SparklesIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

const DEFAULT_TRIGGER = (
  <Button variant="outline" size="sm">
    <SparklesIcon data-icon="inline-start" />
    Break down
  </Button>
)

// DecomposeTaskDialog only collects an arbitrary task's title/description
// — the user has one task in mind and types it in. It has no idea what
// happens after submit: the propose -> poll -> review -> accept loop
// (#169) runs inline in the Tasks section instead of inside this dialog,
// shared with every other decompose_task entry point via useDecomposeRun,
// so this dialog closes itself right after handing off to its caller.
function DecomposeTaskDialog({
  onSubmit,
  trigger = DEFAULT_TRIGGER,
}: {
  // Called with the collected title/description — the caller owns
  // dispatching decompose_task and rendering whatever comes back.
  onSubmit: (title: string, description: string) => void
  // Lets callers place this dialog behind a differently styled/labeled
  // entry point (e.g. the Tasks section header vs. the mobile FAB),
  // including a disabled Button while a proposal is already pending for
  // this project (#169: only one at a time) — a disabled trigger just
  // never opens the dialog, no extra prop needed here for that.
  trigger?: ReactElement
}) {
  const titleId = useId()
  const descriptionId = useId()

  const [open, setOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    onSubmit(title.trim(), description.trim())
    setOpen(false)
    setTitle('')
    setDescription('')
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next: boolean) => {
        setOpen(next)
        if (!next) {
          setTitle('')
          setDescription('')
        }
      }}
    >
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Decompose a task</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor={titleId}>Task title</Label>
            <Input
              id={titleId}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="e.g. Add sync command to the CLI"
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor={descriptionId}>Description</Label>
            <Textarea
              id={descriptionId}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What does done look like for this task?"
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!title.trim()}>
              Propose subtasks
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export default DecomposeTaskDialog
