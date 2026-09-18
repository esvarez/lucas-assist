import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertCircleIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
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
        if (!ignore) setError(err instanceof Error ? err.message : 'Failed to load projects')
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

      {error && (
        <Alert variant="destructive">
          <AlertCircleIcon />
          <AlertTitle>Couldn't load projects</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {!error && projects === null && (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      )}

      {!error && projects !== null && projects.length === 0 && (
        <Card className="items-center py-8 text-center">
          <CardHeader className="items-center">
            <CardTitle>No projects yet</CardTitle>
            <CardDescription>Create one to get started.</CardDescription>
          </CardHeader>
        </Card>
      )}

      {!error && projects && projects.length > 0 && (
        <div className="flex flex-col gap-2">
          {projects.map((project) => (
            <Link key={project.id} to={`/projects/${project.id}`}>
              <Card className="transition-colors hover:bg-accent">
                <CardHeader>
                  <div className="flex items-center justify-between gap-2">
                    <CardTitle>{project.name}</CardTitle>
                    <Badge variant="secondary">{project.status}</Badge>
                  </div>
                  {project.goal && (
                    <CardDescription className="line-clamp-2">{project.goal}</CardDescription>
                  )}
                </CardHeader>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}

export default ProjectsPage
