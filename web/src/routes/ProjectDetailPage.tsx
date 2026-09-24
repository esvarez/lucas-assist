import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { AlertCircleIcon, ChevronLeftIcon, EllipsisIcon, PlusIcon, SparklesIcon } from 'lucide-react'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import {
  Item,
  ItemContent,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from '@/components/ui/item'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { getProject, type Project } from '@/src/api/projects'
import { listAgentRuns, listChangesets } from '@/src/api/skills'
import { flattenTasks, listTasks, type Task } from '@/src/api/tasks'
import BreakIntoTasksButton from '@/src/components/BreakIntoTasksButton'
import DecomposeRunPanel from '@/src/components/DecomposeRunPanel'
import DecomposeTaskDialog from '@/src/components/DecomposeTaskDialog'
import DeleteProjectDialog from '@/src/components/DeleteProjectDialog'
import EditProjectDialog from '@/src/components/EditProjectDialog'
import { useDecomposeRun } from '@/src/hooks/useDecomposeRun'
import { statusBadgeClassName } from '@/src/lib/project-status'
import { taskStatusClassName, taskStatusLabel } from '@/src/lib/task-status'
import { cn } from '@/lib/utils'

// Deadlines are stored as UTC midnight for a calendar day (see
// EditProjectDialog's toUTCMidnightISO/parseDeadline comments) — reading
// them back with UTC getters keeps the same calendar day regardless of the
// viewer's timezone, instead of shifting a day in either direction.
function formatDeadlineMeta(deadlineIso: string) {
  const deadline = new Date(deadlineIso)
  const todayUTC = Date.now()
  const deadlineUTC = Date.UTC(deadline.getUTCFullYear(), deadline.getUTCMonth(), deadline.getUTCDate())
  const startOfTodayUTC = Date.UTC(
    new Date(todayUTC).getUTCFullYear(),
    new Date(todayUTC).getUTCMonth(),
    new Date(todayUTC).getUTCDate()
  )
  const days = Math.round((deadlineUTC - startOfTodayUTC) / (24 * 60 * 60 * 1000))

  const dateLabel = deadline.toLocaleDateString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    timeZone: 'UTC',
  })

  if (days < 0) {
    return {
      dateLabel,
      pillLabel: `${-days}d overdue`,
      pillClassName: 'bg-destructive/10 text-destructive',
    }
  }
  if (days === 0) {
    return { dateLabel, pillLabel: 'today', pillClassName: 'bg-amber-500/15 text-amber-600 dark:text-amber-400' }
  }
  if (days <= 7) {
    return {
      dateLabel,
      pillLabel: `in ${days}d`,
      pillClassName: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
    }
  }
  return { dateLabel, pillLabel: `in ${days}d`, pillClassName: 'bg-muted text-muted-foreground' }
}

// Collapses long constraint lists behind a "+N more" toggle (mockup frame
// 1b) — applied at every width rather than only on mobile, so the sidebar
// card on wide viewports doesn't grow unbounded either.
const CONSTRAINTS_COLLAPSE_AT = 2

function ConstraintsList({ constraints }: { constraints: string[] }) {
  const [expanded, setExpanded] = useState(false)
  const visible = expanded ? constraints : constraints.slice(0, CONSTRAINTS_COLLAPSE_AT)

  return (
    <div className="flex flex-col gap-2">
      <ul className="flex flex-col gap-1.5 text-sm">
        {visible.map((constraint) => (
          <li key={constraint} className="flex items-baseline gap-2">
            <span className="size-1 shrink-0 -translate-y-0.5 rounded-full bg-foreground" />
            {constraint}
          </li>
        ))}
      </ul>
      {constraints.length > CONSTRAINTS_COLLAPSE_AT && (
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="self-start text-xs text-muted-foreground hover:text-foreground"
        >
          {expanded ? 'Show less' : `+${constraints.length - CONSTRAINTS_COLLAPSE_AT} more`}
        </button>
      )}
    </div>
  )
}

// Combines the mockup's separate desktop "Deadline" and "Constraints"
// sidebar cards into one card with two sections — mobile already renders
// them this way (frames 1a/1b), and reusing one component for both widths
// avoids keeping two near-identical implementations in sync.
function ProjectInfoCard({ project }: { project: Project }) {
  if (!project.deadline && project.constraints.length === 0) return null

  const deadlineMeta = project.deadline ? formatDeadlineMeta(project.deadline) : null

  return (
    <Card>
      {deadlineMeta && (
        <CardContent className="flex items-center justify-between gap-3">
          <span className="text-xs text-muted-foreground">Deadline</span>
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{deadlineMeta.dateLabel}</span>
            <Badge className={deadlineMeta.pillClassName}>{deadlineMeta.pillLabel}</Badge>
          </div>
        </CardContent>
      )}
      {project.constraints.length > 0 && (
        <CardContent className={cn('flex flex-col gap-2', deadlineMeta && 'border-t border-border pt-4')}>
          <span className="text-xs text-muted-foreground">Constraints</span>
          <ConstraintsList constraints={project.constraints} />
        </CardContent>
      )}
    </Card>
  )
}

function ProjectDetailHeader({
  project,
  onUpdated,
}: {
  project: Project
  onUpdated: (project: Project) => void
}) {
  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  return (
    <div className="flex flex-col gap-3">
      <Breadcrumb className="hidden sm:block">
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink render={<Link to="/projects" />}>Projects</BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{project.name}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
      <Link
        to="/projects"
        className="flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground sm:hidden"
      >
        <ChevronLeftIcon className="size-4" />
        Projects
      </Link>

      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-2">
          <Badge className={statusBadgeClassName(project.status)}>{project.status}</Badge>
          <h1 className="text-2xl font-bold tracking-tight text-balance">{project.name}</h1>
          {project.goal && <p className="text-sm text-muted-foreground text-balance">{project.goal}</p>}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button variant="outline" className="hidden sm:inline-flex" onClick={() => setEditOpen(true)}>
            Edit
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Project actions"
                  className="sm:border-border sm:hover:bg-input/50 sm:dark:bg-input/30"
                />
              }
            >
              <EllipsisIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem className="sm:hidden" onClick={() => setEditOpen(true)}>
                Edit project
              </DropdownMenuItem>
              {/* Duplicate/Archive have no backend endpoint yet (#161) — shown
                  disabled rather than silently implying functionality that
                  doesn't exist. */}
              <DropdownMenuItem disabled className="hidden sm:flex">
                Duplicate
              </DropdownMenuItem>
              <DropdownMenuItem disabled className="hidden sm:flex">
                Archive
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onClick={() => setDeleteOpen(true)}>
                Delete project
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <EditProjectDialog project={project} open={editOpen} onOpenChange={setEditOpen} onUpdated={onUpdated} />
      <DeleteProjectDialog project={project} open={deleteOpen} onOpenChange={setDeleteOpen} />
    </div>
  )
}

function TaskCheckRow({
  task,
  onToggleDone,
  variant = 'default',
}: {
  task: Task
  onToggleDone: (done: boolean) => void
  variant?: 'default' | 'outline'
}) {
  return (
    <Item variant={variant} size="sm">
      <ItemMedia>
        <Checkbox
          checked={task.status === 'done'}
          onCheckedChange={(value) => onToggleDone(value === true)}
        />
      </ItemMedia>
      <ItemContent className="flex-row items-center justify-between">
        <ItemTitle className={task.status === 'done' ? 'text-muted-foreground line-through' : undefined}>
          {task.title}
        </ItemTitle>
        <ItemTitle className={taskStatusClassName(task.status)}>{taskStatusLabel(task.status)}</ItemTitle>
      </ItemContent>
    </Item>
  )
}

// TaskDetails renders a task's description and acceptance criteria — the
// two fields a task carries beyond its title/status, shown inside a task's
// accordion content (both LeafTask's own and TaskAccordion's) rather than
// in the always-visible row, since most projects have many tasks and this
// content would otherwise dominate the list.
function TaskDetails({ task }: { task: Task }) {
  if (!task.description && task.acceptance_criteria.length === 0) return null

  return (
    <div className="flex flex-col gap-3">
      {task.description && <p className="text-muted-foreground">{task.description}</p>}
      {task.acceptance_criteria.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <span className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
            Acceptance criteria
          </span>
          <ul className="flex flex-col gap-1.5">
            {task.acceptance_criteria.map((criterion) => (
              <li key={criterion} className="flex items-baseline gap-2">
                <span className="size-1 shrink-0 -translate-y-0.5 rounded-full bg-foreground" />
                {criterion}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

// AddSubtaskRow is every task's way to add a subtask by hand, or to hand
// the task (further, if breakLabel says "more") to decompose_task —
// shared by LeafTask and TaskAccordion so both offer it identically.
// Adding a subtask manually, and decomposing a task (further), both need
// a backend way to attach new tasks under this task's id —
// decompose_task has no such input yet and there's no manual
// task-creation endpoint (#161), so this stays disabled rather than
// silently doing nothing.
function AddSubtaskRow({ breakLabel }: { breakLabel: string }) {
  return (
    <div className="flex items-center gap-2">
      <Input placeholder="Add a subtask" disabled className="h-8" />
      <Button variant="outline" size="sm" disabled>
        <SparklesIcon data-icon="inline-start" />
        {breakLabel}
      </Button>
    </div>
  )
}

// Every task can be expanded now — even one with no description,
// acceptance criteria, or subtasks yet still has AddSubtaskRow to show —
// so LeafTask always renders the accordion rather than short-circuiting
// to a plain checkbox row.
function LeafTask({ task: initial }: { task: Task }) {
  const [task, setTask] = useState(initial)
  const toggleDone = (done: boolean) => setTask({ ...task, status: done ? 'done' : 'todo' })

  return (
    <Accordion>
      <AccordionItem value={task.id}>
        {/* The checkbox sits beside AccordionTrigger, not inside it — Trigger
            renders a <button>, and nesting the Checkbox's own button inside
            would both be invalid HTML and make every done-toggle also
            open/close the accordion. */}
        <div className="flex items-center gap-2 p-2">
          <Checkbox checked={task.status === 'done'} onCheckedChange={(value) => toggleDone(value === true)} />
          <AccordionTrigger className="border-none p-0 hover:no-underline">
            <span className="flex flex-1 items-center justify-between gap-2">
              <span className={task.status === 'done' ? 'text-muted-foreground line-through' : undefined}>
                {task.title}
              </span>
              <span className={taskStatusClassName(task.status)}>{taskStatusLabel(task.status)}</span>
            </span>
          </AccordionTrigger>
        </div>
        <AccordionContent>
          <div className="flex flex-col gap-4 pt-2">
            <TaskDetails task={task} />
            <AddSubtaskRow breakLabel="Break into subtasks" />
          </div>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
  )
}

function TaskAccordion({ task }: { task: Task }) {
  const [subtasks, setSubtasks] = useState(task.subtasks)
  const done = subtasks.filter((subtask) => subtask.status === 'done').length

  return (
    <Accordion>
      <AccordionItem value={task.id}>
        <AccordionTrigger>
          <span className="flex flex-1 items-center justify-between gap-2">
            <span>{task.title}</span>
            <span>
              {done}/{subtasks.length}{' '}
              <span className={taskStatusClassName(task.status)}>{taskStatusLabel(task.status)}</span>
            </span>
          </span>
        </AccordionTrigger>
        <div>
          <Progress
            value={(done / subtasks.length) * 100}
            className="[&_[data-slot=progress-track]]:rounded-none!"
          />
        </div>
        <AccordionContent>
          <div className="flex flex-col gap-4 pt-2">
            <TaskDetails task={task} />
            <ul className="flex flex-col gap-2">
              {subtasks.map((subtask) => (
                <li key={subtask.id}>
                  <TaskCheckRow
                    task={subtask}
                    onToggleDone={(done) =>
                      setSubtasks((current) =>
                        current.map((item) =>
                          item.id === subtask.id ? { ...item, status: done ? 'done' : 'todo' } : item,
                        ),
                      )
                    }
                  />
                </li>
              ))}
            </ul>
            <AddSubtaskRow breakLabel="Break down more" />
          </div>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
  )
}

function TaskItem({ task }: { task: Task }) {
  if (task.subtasks.length === 0) {
    return <LeafTask task={task} />
  }
  return <TaskAccordion task={task} />
}

// projectSeed derives decompose_task's task_title/task_description from
// the project's own card (#163) — the "Break into tasks" CTA breaks down
// the whole project, not one task the user has in mind, so there's
// nothing to ask for that isn't already on the card.
function projectSeed(project: Project): { title: string; description: string } {
  const description =
    project.constraints.length > 0
      ? `${project.goal}\n\nConstraints: ${project.constraints.join('; ')}`
      : project.goal
  return { title: project.name, description }
}

function TasksSection({
  project,
  tasks,
  onAccepted,
}: {
  project: Project
  tasks: Task[]
  onAccepted: () => void
}) {
  const projectId = project.id
  const flat = flattenTasks(tasks)
  const done = flat.filter((task) => task.status === 'done').length

  const run = useDecomposeRun(projectId, onAccepted)
  // Remembered across the run's lifecycle (a clarify resubmit, an error
  // retry) — whatever form collected it, if any, has already closed by
  // the time those happen.
  const [runSeed, setRunSeed] = useState({ title: '', description: '' })

  // Rediscovers a pending proposal, or failing that a still-running
  // dispatch, on mount — so both survive a page refresh (#169) instead of
  // looking idle and letting the trigger below start a second, racing
  // decompose_task run. Nothing else exposes a changeset's or an
  // AgentRun's id without already having it from the request that created
  // it, so this is the only way a fresh page load can find either.
  useEffect(() => {
    let ignore = false

    async function rediscoverPending() {
      const { changesets } = await listChangesets(projectId, 'proposed')
      const pendingChangeset = changesets.find((c) => c.skill === 'decompose_task')
      if (pendingChangeset) {
        if (!ignore) run.resumeReview(pendingChangeset)
        return
      }

      const { agent_runs: agentRuns } = await listAgentRuns(projectId)
      const inFlightRun = agentRuns.find(
        (r) => r.skill === 'decompose_task' && (r.status === 'queued' || r.status === 'running')
      )
      if (inFlightRun && !ignore) {
        // Recovered so a later clarify resubmit or error retry has
        // something real to redispatch with, instead of a blank task.
        setRunSeed({
          title: inFlightRun.input?.task_title ?? '',
          description: inFlightRun.input?.task_description ?? '',
        })
        run.resumePoll(inFlightRun.id)
      }
    }

    rediscoverPending().catch(() => {
      // Best-effort: a fresh page still works without a rediscovered
      // proposal or run, just as if neither existed.
    })

    return () => {
      ignore = true
    }
    // Runs once per project id — run's own identity isn't a dependency
    // this effect needs to react to.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId])

  // Only one proposal in flight per project at a time (#169) — starting
  // another while this one is working, needs an answer, or awaits review
  // would race it, so every trigger below disables while true.
  const pending = run.state.name !== 'idle'

  function startBreakIntoTasks() {
    const seed = projectSeed(project)
    setRunSeed(seed)
    void run.start(seed.title, seed.description)
  }

  function startDecomposeTask(title: string, description: string) {
    setRunSeed({ title, description })
    void run.start(title, description)
  }

  if (tasks.length === 0 && !pending) {
    return (
      <Empty className="border">
        <EmptyHeader>
          <EmptyTitle>No tasks yet</EmptyTitle>
          <EmptyDescription>Let Nudge break this project into small steps.</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <BreakIntoTasksButton onStart={startBreakIntoTasks} working={run.state.name === 'working'} disabled={pending} />
          {/* Manual single-task creation has no backend endpoint yet (#161)
              — shown disabled rather than silently implying it works. */}
          <Button variant="ghost" size="sm" disabled>
            Add a task manually
          </Button>
        </EmptyContent>
      </Empty>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-baseline gap-3">
          <h2 className="text-lg font-bold">Tasks</h2>
          {flat.length > 0 && (
            <span className="text-sm text-muted-foreground">
              {done} of {flat.length} done
            </span>
          )}
        </div>
        {/* Desktop only (frame 2a) — mobile gets the same two actions in the
            fixed footer below instead, so they don't render twice at once. */}
        <div className="hidden items-center gap-2 lg:flex">
          {/* Manual single-task creation has no backend endpoint yet (#161)
              — shown disabled rather than silently implying it works. */}
          <Button size="sm" disabled>
            <PlusIcon data-icon="inline-start" />
            Add task
          </Button>
          <DecomposeTaskDialog
            onSubmit={startDecomposeTask}
            trigger={
              <Button variant="outline" size="sm" disabled={pending}>
                <SparklesIcon data-icon="inline-start" />
                Break down
              </Button>
            }
          />
        </div>
      </div>
      {flat.length > 0 && <Progress value={(done / flat.length) * 100} />}
      <DecomposeRunPanel run={run} title={runSeed.title} description={runSeed.description} />
      {tasks.length > 0 && (
        <ItemGroup>
          {tasks.map((task) => (
            <TaskItem key={task.id} task={task} />
          ))}
        </ItemGroup>
      )}
      {/* Mobile/tablet only (frame 1b) — the desktop header above carries
          the same two actions, hidden here to avoid showing both at once. */}
      <div className="fixed inset-x-0 bottom-0 flex items-center gap-2 border-t border-border bg-background p-3 lg:hidden">
        {/* Manual single-task creation has no backend endpoint yet (#161)
            — shown disabled rather than silently implying it works. */}
        <Button className="flex-1" disabled>
          <PlusIcon data-icon="inline-start" />
          Add task
        </Button>
        <DecomposeTaskDialog
          onSubmit={startDecomposeTask}
          trigger={
            <Button variant="outline" disabled={pending}>
              <SparklesIcon data-icon="inline-start" />
              Break down
            </Button>
          }
        />
      </div>
    </div>
  )
}

function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [project, setProject] = useState<Project | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [error, setError] = useState<string | null>(null)
  // Bumped after a decompose_task changeset is accepted, so the task list
  // is refetched and picks up the newly committed tasks.
  const [taskRefreshKey, setTaskRefreshKey] = useState(0)

  useEffect(() => {
    if (!id) return
    let ignore = false
    getProject(id)
      .then((p) => {
        if (!ignore) setProject(p)
      })
      .catch((err) => {
        if (!ignore) setError(err instanceof Error ? err.message : 'Failed to load project')
      })
    listTasks(id)
      .then((list) => {
        if (!ignore) setTasks(list)
      })
      .catch(() => {
        // A failed task fetch shouldn't fail the whole page — hide the list instead.
        if (!ignore) setTasks([])
      })
    return () => {
      ignore = true
    }
  }, [id, taskRefreshKey])

  if (error) {
    return (
      <div className="flex flex-col gap-3 p-4">
        <Alert variant="destructive">
          <AlertCircleIcon />
          <AlertTitle>Couldn't load project</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
        <Button variant="ghost" className="self-start" render={<Link to="/projects" />}>
          <ChevronLeftIcon data-icon="inline-start" />
          Back to projects
        </Button>
      </div>
    )
  }

  if (!project) {
    return (
      <div className="flex flex-col gap-4 p-4">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-16 w-full" />
      </div>
    )
  }

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 p-4 pb-24 lg:pb-4">
      <ProjectDetailHeader project={project} onUpdated={setProject} />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start lg:gap-10">
        <div className="order-2 lg:order-1">
          <TasksSection
            project={project}
            tasks={tasks}
            onAccepted={() => setTaskRefreshKey((k) => k + 1)}
          />
        </div>
        <div className="order-1 lg:order-2">
          <ProjectInfoCard project={project} />
        </div>
      </div>
    </div>
  )
}

export default ProjectDetailPage
