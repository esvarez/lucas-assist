import { useEffect, useState } from 'react'
import { AlertCircleIcon, PlusIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import NewProjectDialog from '@/src/components/NewProjectDialog'
import ProjectCard from '@/src/components/ProjectCard'
import { listProjects, type Project } from '@/src/api/projects'

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
        <div>
          <h1 className="text-lg font-bold">Projects</h1>
          {projects && projects.length > 0 && (
            <p className="text-xs text-muted-foreground">
              {projects.length} project{projects.length === 1 ? '' : 's'}
            </p>
          )}
        </div>
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
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Skeleton className="h-28 w-full" />
          <Skeleton className="h-28 w-full" />
          <Skeleton className="h-28 w-full" />
        </div>
      )}

      {!error && projects !== null && projects.length === 0 && (
        <Card className="items-center gap-4 py-10 text-center">
          <CardHeader className="items-center">
            <CardTitle>No projects yet</CardTitle>
            <CardDescription>Create one to get started.</CardDescription>
          </CardHeader>
          <NewProjectDialog
            trigger={
              <Button size="lg">
                <PlusIcon /> Create your first project
              </Button>
            }
          />
        </Card>
      )}

      {!error && projects && projects.length > 0 && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((project) => (
            <ProjectCard key={project.id} project={project} />
          ))}
        </div>
      )}
    </div>
  )
}

export default ProjectsPage
