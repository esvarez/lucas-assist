import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { flattenTasks, listTasks, type Task } from '@/src/api/tasks'
import { LightRays } from '@/components/ui/light-rays'

export function pickNextTask(tasks: Task[]): Task | null {
  const sorted = [...tasks].sort((a, b) => a.order - b.order)
  return sorted.find((task) => task.status === 'in-progress') ?? sorted.find((task) => task.status === 'todo') ?? null
}

function WhatsNextCard({ projectId }: { projectId: string }) {
  const [task, setTask] = useState<Task | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let ignore = false
    setLoading(true)
    listTasks(projectId)
      .then((tasks) => {
        if (!ignore) setTask(pickNextTask(flattenTasks(tasks)))
      })
      .catch(() => {
        if (!ignore) setTask(null)
      })
      .finally(() => {
        if (!ignore) setLoading(false)
      })
    return () => {
      ignore = true
    }
  }, [projectId])

  if (loading) {
    return <Skeleton className="h-36 w-full rounded-2xl" />
  }

  if (!task) {
    return null
  }

  const alreadyInProgress = task.status === 'in-progress'

  return (
    <Card className="relative overflow-hidden border border-primary ring-0">
      <LightRays color="rgba(0, 120, 111, 0.2)"/>
      <CardHeader>
        <div className="flex items-center gap-2">
          <span className="size-1.5 rounded-full bg-primary" aria-hidden />
          <span className="text-[11px] font-medium tracking-wider text-muted-foreground uppercase">
            What's next
          </span>
        </div>
        <CardTitle className="text-base font-semibold">{task.title}</CardTitle>
        {task.description && (
          <CardDescription className="text-sm">{task.description}</CardDescription>
        )}
      </CardHeader>
      <CardFooter className="gap-2">
        <Button
          type="button"
          disabled={alreadyInProgress}
          onClick={() => setTask({ ...task, status: 'in-progress' })}
        >
          Mark in progress
        </Button>
        <Button type="button" variant="outline">
          Ask Nudge
        </Button>
      </CardFooter>
      
    </Card>
  )
}

export default WhatsNextCard
