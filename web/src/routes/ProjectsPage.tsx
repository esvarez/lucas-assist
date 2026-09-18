import { useState } from 'react'
import NewProjectDialog from '../components/NewProjectDialog'
import type { Project } from '../lib/api'

function ProjectsPage() {
  const [projects, setProjects] = useState<Project[]>([])

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-bold">Projects</h1>
          <p className="text-xs text-muted-foreground">
            {projects.length} project{projects.length === 1 ? '' : 's'}
          </p>
        </div>
        <NewProjectDialog onCreated={(project) => setProjects((prev) => [project, ...prev])} />
      </div>

      {projects.length > 0 && (
        <ul className="flex flex-col gap-2">
          {projects.map((project) => (
            <li
              key={project.id}
              className="rounded-md border border-border bg-card px-3 py-2 text-sm text-card-foreground"
            >
              {project.name}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export default ProjectsPage
