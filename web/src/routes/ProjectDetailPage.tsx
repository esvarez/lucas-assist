import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { getProject, type Project } from '../api/projects'

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
        <p className="text-sm text-destructive">{error}</p>
        <Link to="/projects" className="text-sm text-primary underline">
          Back to projects
        </Link>
      </div>
    )
  }

  if (!project) {
    return <div className="p-4 text-sm text-muted-foreground">Loading…</div>
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <Link to="/projects" className="text-xs text-muted-foreground underline">
        Back to projects
      </Link>

      <div className="flex items-center justify-between">
        <h1 className="text-lg font-bold">{project.name}</h1>
        <span className="rounded-full bg-secondary px-2 py-0.5 text-xs font-medium text-secondary-foreground">
          {project.status}
        </span>
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
