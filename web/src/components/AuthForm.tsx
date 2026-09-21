import type { FormEvent, ReactNode } from 'react'
import { AlertCircleIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader } from '@/components/ui/card'

function AuthForm({
  title,
  description,
  errorTitle,
  error,
  submitLabel,
  onSubmit,
  footer,
  children,
}: {
  title: string
  description: string
  errorTitle: string
  error: string | null
  submitLabel: string
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
  footer?: ReactNode
  children: ReactNode
}) {
  return (
    <Card>
      <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
        <CardHeader>
          <h1 className="font-heading text-sm font-medium">{title}</h1>
          <CardDescription>{description}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {error && (
            <Alert variant="destructive">
              <AlertCircleIcon />
              <AlertTitle>{errorTitle}</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          {children}
        </CardContent>
        <CardFooter className="flex-col items-stretch gap-3">
          <Button type="submit" size="lg" className="w-full">
            {submitLabel}
          </Button>
          {footer}
        </CardFooter>
      </form>
    </Card>
  )
}

export default AuthForm
