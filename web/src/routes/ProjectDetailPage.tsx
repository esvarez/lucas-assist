import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { AlertCircleIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { getProject, type Project } from '../api/projects'
import { statusBadgeClassName } from '../lib/project-status'

function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [project, setProject] = useState<Project | null>(null)
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
        <Link to="/projects" className="text-sm text-primary underline">
          Back to projects
        </Link>
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
      <Link to="/projects" className="text-xs text-muted-foreground underline">
        Back to projects
      </Link>

      <div className="flex items-center justify-between">
        <h1 className="text-lg font-bold">{project.name}</h1>
        <Badge variant="outline" className={statusBadgeClassName(project.status)}>
          {project.status}
        </Badge>
      </div>

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
    </div>
  )
}

export default ProjectDetailPage
