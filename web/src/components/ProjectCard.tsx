import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { Project } from '@/src/api/projects'

function ProjectCard({ project }: { project: Project }) {
  return (
    <Link to={`/projects/${project.id}`}>
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
  )
}

export default ProjectCard
