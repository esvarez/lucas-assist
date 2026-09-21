import { useId, useState, type FormEvent } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import AuthForm from '@/src/components/AuthForm'

function emailFromState(state: unknown): string | null {
  if (!state || typeof state !== 'object' || !('email' in state)) return null
  const email = state.email
  if (typeof email !== 'string') return null
  const trimmed = email.trim()
  return trimmed || null
}

function ConfirmSignUpPage() {
  const location = useLocation()
  const email = emailFromState(location.state)
  const codeId = useId()
  const [code, setCode] = useState('')
  const [error, setError] = useState<string | null>(null)

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    if (!code.trim()) {
      setError('Enter the confirmation code from your email.')
      return
    }
    setError(null)
    // ConfirmSignUp is not wired on this screen yet.
  }

  return (
    <AuthForm
      title="Confirm your email"
      description={
        email
          ? `Enter the confirmation code sent to ${email}.`
          : 'Enter the confirmation code sent to your email.'
      }
      errorTitle="Couldn't confirm your email"
      error={error}
      submitLabel="Confirm"
      onSubmit={handleSubmit}
      footer={
        <div className="flex flex-col gap-1 text-center text-muted-foreground">
          <p>
            Wrong email?{' '}
            <Link to="/sign-up" className="font-medium text-foreground underline-offset-4 hover:underline">
              Start over
            </Link>
          </p>
          <p>
            Already confirmed?{' '}
            <Link to="/sign-in" className="font-medium text-foreground underline-offset-4 hover:underline">
              Sign in
            </Link>
          </p>
        </div>
      }
    >
      <div className="flex flex-col gap-2">
        <Label htmlFor={codeId}>Confirmation code</Label>
        <Input
          id={codeId}
          name="code"
          inputMode="numeric"
          autoComplete="one-time-code"
          placeholder="123456"
          value={code}
          onChange={(event) => {
            setCode(event.target.value)
            setError(null)
          }}
          autoFocus
          aria-invalid={Boolean(error) && !code.trim()}
        />
      </div>
    </AuthForm>
  )
}

export default ConfirmSignUpPage
