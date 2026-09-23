import { useId, useState, type SubmitEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import AuthForm from '@/src/components/AuthForm'
import PasswordField from '@/src/components/PasswordField'
import { cognitoErrorMessage, signIn } from '@/src/lib/cognito'

function SignInPage() {
  const navigate = useNavigate()
  const emailId = useId()
  const passwordId = useId()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault()
    const trimmedEmail = email.trim()
    if (!trimmedEmail || !password || submitting) {
      if (!trimmedEmail || !password) setError('Enter your email and password.')
      return
    }
    setError(null)
    setSubmitting(true)
    try {
      await signIn(trimmedEmail, password)
      navigate('/projects')
    } catch (err) {
      setError(cognitoErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthForm
      title="Sign in"
      description="Use the email and password for your Nudge account."
      errorTitle="Couldn't sign in"
      error={error}
      submitLabel={submitting ? 'Signing in…' : 'Sign in'}
      submitDisabled={submitting}
      onSubmit={handleSubmit}
      footer={
        <div className="flex flex-col gap-1 text-center text-muted-foreground">
          <p>
            Don&apos;t have an account?{' '}
            <Link to="/sign-up" className="font-medium text-foreground underline-offset-4 hover:underline">
              Sign up
            </Link>
          </p>
          <p>
            Forgot your password?{' '}
            <Link
              to="/forgot-password"
              className="font-medium text-foreground underline-offset-4 hover:underline"
            >
              Reset it
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
          autoFocus
          disabled={submitting}
          aria-invalid={Boolean(error) && !email.trim()}
        />
      </div>
      <PasswordField
        id={passwordId}
        label="Password"
        name="password"
        autoComplete="current-password"
        value={password}
        onChange={(event) => {
          setPassword(event.target.value)
          setError(null)
        }}
        disabled={submitting}
        aria-invalid={Boolean(error) && !password}
      />
    </AuthForm>
  )
}

export default SignInPage
