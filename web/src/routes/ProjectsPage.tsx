import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import NewProjectDialog from '../components/NewProjectDialog'
import { listProjects, type Project } from '../api/projects'

function ProjectsPage() {
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let ignore = false
    listProjects()
      .then((p) => {
        if (!ignore) setProjects(p)
      })
      .catch((err) => {
        if (ignore) return
        setError(err instanceof Error ? err.message : 'Failed to load projects')
      })
    return () => {
      ignore = true
    }
  }, [])

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-bold">Projects</h1>
        <NewProjectDialog />
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}

      {!error && projects === null && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {!error && projects !== null && projects.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No projects yet — create one to get started.
        </p>
      )}

      {!error && projects && projects.length > 0 && (
        <ul className="flex flex-col gap-2">
          {projects.map((project) => (
            <li key={project.id}>
              <Link
                to={`/projects/${project.id}`}
                className="flex flex-col gap-1 rounded-md border border-border bg-card px-3 py-2 text-card-foreground transition-colors hover:bg-accent"
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="text-sm font-medium">{project.name}</span>
                  <span className="rounded-full bg-secondary px-2 py-0.5 text-xs font-medium text-secondary-foreground">
                    {project.status}
                  </span>
                </div>
                {project.goal && (
                  <p className="line-clamp-2 text-xs text-muted-foreground">{project.goal}</p>
                )}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export default ProjectsPage
