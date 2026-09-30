import { useId, useState, type SubmitEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import AuthForm from '@/src/components/AuthForm'
import { cognitoErrorMessage, confirmSignUp } from '@/src/lib/cognito'

function emailFromState(state: unknown): string | null {
  if (!state || typeof state !== 'object' || !('email' in state)) return null
  const email = state.email
  if (typeof email !== 'string') return null
  const trimmed = email.trim()
  return trimmed || null
}

function ConfirmSignUpPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const emailId = useId()
  const codeId = useId()
  // Pre-filled from the post-signup redirect's navigation state when
  // present (#202) — but always editable and required, since state
  // doesn't survive the "Have a confirmation code? Enter it" link, a
  // fresh tab, or a bookmark, and erroring out with no way to recover in
  // those cases is the bug this fixes.
  const [email, setEmail] = useState(() => emailFromState(location.state) ?? '')
  const [code, setCode] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault()
    const trimmedEmail = email.trim()
    const trimmedCode = code.trim()
    if (!trimmedEmail || !trimmedCode || submitting) {
      if (!trimmedEmail || !trimmedCode) setError('Enter your email and the confirmation code from it.')
      return
    }
    setError(null)
    setSubmitting(true)
    try {
      await confirmSignUp(trimmedEmail, trimmedCode)
      navigate('/sign-in')
    } catch (err) {
      setError(cognitoErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthForm
      title="Confirm your email"
      description="Enter your email and the confirmation code sent to it."
      errorTitle="Couldn't confirm your email"
      error={error}
      submitLabel={submitting ? 'Confirming…' : 'Confirm'}
      submitDisabled={submitting}
      onSubmit={handleSubmit}
      footer={
        <div className="flex flex-col gap-1 text-center text-muted-foreground">
          <p>
            Don&apos;t have an account yet?{' '}
            <Link to="/sign-up" className="font-medium text-foreground underline-offset-4 hover:underline">
              Sign up
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
        <Label htmlFor={emailId}>Email</Label>
        <Input
          id={emailId}
          type="email"
          name="email"
          autoComplete="email"
          placeholder="you@example.com"
          value={email}
          onChange={(event) => {
            setEmail(event.target.value)
            setError(null)
          }}
          autoFocus={!email}
          disabled={submitting}
          aria-invalid={Boolean(error) && !email.trim()}
        />
      </div>
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
          autoFocus={Boolean(email)}
          disabled={submitting}
          aria-invalid={Boolean(error) && !code.trim()}
        />
      </div>
    </AuthForm>
  )
}

export default ConfirmSignUpPage
