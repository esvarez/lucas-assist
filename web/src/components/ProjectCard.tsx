import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { formatDistanceToNow } from 'date-fns'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { flattenTasks, listTasks } from '@/src/api/tasks'
import type { Project } from '@/src/api/projects'
import { statusBadgeClassName } from '@/src/lib/project-status'

function ProjectCard({ project }: { project: Project }) {
  // Task counts come from a mock-only endpoint (web/mock-server.mjs) —
  // there's no real one yet, see src/api/tasks.ts. Failing to load it
  // (e.g. running against the real backend, which 404s) just hides the
  // progress section rather than breaking the card.
  const [taskCounts, setTaskCounts] = useState<{ done: number; total: number } | null>(null)

  useEffect(() => {
    let ignore = false
    listTasks(project.id)
      .then((tasks) => {
        if (ignore) return
        const all = flattenTasks(tasks)
        setTaskCounts({ done: all.filter((t) => t.status === 'done').length, total: all.length })
      })
      .catch(() => {
        // Supplementary info — silently unavailable until the real
        // endpoint exists.
      })
    return () => {
      ignore = true
    }
  }, [project.id])

  return (
    <Link to={`/projects/${project.id}`}>
      <Card className="h-full transition-colors hover:bg-accent">
        <CardHeader>
          <div className="flex items-start justify-between gap-2">
            <CardTitle>{project.name}</CardTitle>
            {project.status && (
              <Badge className={`${statusBadgeClassName(project.status)}`}>
                {project.status}
              </Badge>
            )}
          </div>
        </CardHeader>
        {(project.goal || taskCounts) && (
          <CardContent className="flex flex-col gap-3">
            {project.goal && <CardDescription className="line-clamp-2">{project.goal}</CardDescription>}
            {taskCounts && (
              <div className="flex flex-col gap-1.5">
                <Progress
                  value={taskCounts.total ? (taskCounts.done / taskCounts.total) * 100 : 0}
                />
                <span className="text-xs text-muted-foreground">
                  {taskCounts.done}/{taskCounts.total} tasks
                </span>
              </div>
            )}
          </CardContent>
        )}
        <CardFooter>
          <span className="font-mono text-[11px] text-muted-foreground">
            Active {formatDistanceToNow(new Date(project.updated_at), { addSuffix: true })}
          </span>
        </CardFooter>
      </Card>
    </Link>
  )
}

export default ProjectCard
