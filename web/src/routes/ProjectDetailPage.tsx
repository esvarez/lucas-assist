import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { AlertCircleIcon, ChevronLeftIcon } from 'lucide-react'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { listTasks, type Task } from '@/src/api/tasks'
import DeleteProjectDialog from '@/src/components/DeleteProjectDialog'
import EditProjectDialog from '@/src/components/EditProjectDialog'
import WhatsNextCard from '@/src/components/WhatsNextCard'
import { statusBadgeClassName } from '@/src/lib/project-status'
import { taskStatusClassName, taskStatusLabel } from '@/src/lib/task-status'

function ProjectDetailHeader({
  project,
  onUpdated,
}: {
  project: Project
  onUpdated: (project: Project) => void
}) {
  return (
    <div className="flex items-center justify-between gap-2">
      <div className="flex items-center gap-2">
        <Button variant="ghost" className="self-start" render={<Link to="/projects" />}>
          <ChevronLeftIcon data-icon="inline-start" />
        </Button>
        <h1 className="text-lg font-bold">{project.name}</h1>
        <Badge className={statusBadgeClassName(project.status)}>{project.status}</Badge>
      </div>
      <EditProjectDialog project={project} onUpdated={onUpdated} />
      <DeleteProjectDialog project={project} />
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

function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [project, setProject] = useState<Project | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [error, setError] = useState<string | null>(null)

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
        // Tasks endpoint is mock-only — hide the list rather than fail the page.
        if (!ignore) setTasks([])
      })
    return () => {
      ignore = true
    }
  }, [id])

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
    <div className="flex flex-col gap-4 p-4">
      <ProjectDetailHeader project={project} onUpdated={setProject} />

      <WhatsNextCard projectId={project.id} />

      {project.goal && <p className="text-sm text-muted-foreground">{project.goal}</p>}

      {project.deadline && (
        <p className="text-xs text-muted-foreground">
          Deadline: {new Date(project.deadline).toLocaleDateString()}
        </p>
      )}

      {project.constraints.length > 0 && (
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium text-muted-foreground">Constraints</span>
          <ul className="list-inside list-disc text-sm">
            {project.constraints.map((constraint) => (
              <li key={constraint}>{constraint}</li>
            ))}
          </ul>
        </div>
      )}

      <h2 className="text-lg font-bold">Tasks</h2>

      <ItemGroup>
        {tasks.map((task) => (
          <TaskItem key={task.id} task={task} />
        ))}
      </ItemGroup>

    </div>
  )
}

export default ProjectDetailPage
