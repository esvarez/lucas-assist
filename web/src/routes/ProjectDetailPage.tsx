import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { AlertCircleIcon, ChevronLeftIcon, EllipsisIcon, SparklesIcon } from 'lucide-react'
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
import { flattenTasks, listTasks, type Task } from '@/src/api/tasks'
import BreakIntoTasksButton from '@/src/components/BreakIntoTasksButton'
import DecomposeTaskDialog from '@/src/components/DecomposeTaskDialog'
import DeleteProjectDialog from '@/src/components/DeleteProjectDialog'
import EditProjectDialog from '@/src/components/EditProjectDialog'
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
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">Constraints</span>
            <span className="text-xs text-muted-foreground">{project.constraints.length}</span>
          </div>
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

function LeafTask({ task: initial }: { task: Task }) {
  const [task, setTask] = useState(initial)

  return (
    <TaskCheckRow
      task={task}
      variant="outline"
      onToggleDone={(done) => setTask({ ...task, status: done ? 'done' : 'todo' })}
    />
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
          <ul className="flex flex-col gap-2 pt-2">
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

function TasksSection({
  project,
  projectId,
  tasks,
  onAccepted,
}: {
  project: Project
  projectId: string
  tasks: Task[]
  onAccepted: () => void
}) {
  const flat = flattenTasks(tasks)
  const done = flat.filter((task) => task.status === 'done').length

  if (tasks.length === 0) {
    return (
      <Empty className="border">
        <EmptyHeader>
          <EmptyTitle>No tasks yet</EmptyTitle>
          <EmptyDescription>Let Nudge break this project into small steps.</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <BreakIntoTasksButton project={project} onAccepted={onAccepted} />
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
          <span className="text-sm text-muted-foreground">
            {done} of {flat.length} done
          </span>
        </div>
        <DecomposeTaskDialog projectId={projectId} onAccepted={onAccepted} />
      </div>
      <Progress value={(done / flat.length) * 100} />
      <ItemGroup>
        {tasks.map((task) => (
          <TaskItem key={task.id} task={task} />
        ))}
      </ItemGroup>
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
            projectId={project.id}
            tasks={tasks}
            onAccepted={() => setTaskRefreshKey((k) => k + 1)}
          />
        </div>
        <div className="order-1 lg:order-2">
          <ProjectInfoCard project={project} />
        </div>
      </div>

      {tasks.length > 0 && (
        <div className="fixed inset-x-0 bottom-0 border-t border-border bg-background p-3 lg:hidden">
          <DecomposeTaskDialog
            projectId={project.id}
            onAccepted={() => setTaskRefreshKey((k) => k + 1)}
            trigger={
              <Button className="w-full">
                <SparklesIcon data-icon="inline-start" />
                Break down
              </Button>
            }
          />
        </div>
      )}
    </div>
  )
}

export default ProjectDetailPage
