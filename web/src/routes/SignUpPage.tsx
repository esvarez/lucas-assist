import { useId, useState, type SubmitEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import AuthForm from '@/src/components/AuthForm'
import PasswordField from '@/src/components/PasswordField'
import { cognitoErrorMessage, passwordPolicyError, signUp } from '@/src/lib/cognito'

function SignUpPage() {
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
      if (!trimmedEmail || !password) setError('Enter your email and a password.')
      return
    }
    const policyError = passwordPolicyError(password)
    if (policyError) {
      setError(policyError)
      return
    }
    setError(null)
    setSubmitting(true)
    try {
      await signUp(trimmedEmail, password)
      navigate('/sign-up/confirm', { state: { email: trimmedEmail } })
    } catch (err) {
      setError(cognitoErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthForm
      title="Create account"
      description="Sign up with your email. You'll confirm it on the next screen."
      errorTitle="Couldn't create account"
      error={error}
      submitLabel={submitting ? 'Creating account…' : 'Create account'}
      submitDisabled={submitting}
      onSubmit={handleSubmit}
      footer={
        <div className="flex flex-col gap-1 text-center text-muted-foreground">
          <p>
            Already have an account?{' '}
            <Link to="/sign-in" className="font-medium text-foreground underline-offset-4 hover:underline">
              Sign in
            </Link>
          </p>
          <p>
            Have a confirmation code?{' '}
            <Link
              to="/sign-up/confirm"
              className="font-medium text-foreground underline-offset-4 hover:underline"
            >
              Enter it
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
        autoComplete="new-password"
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

export default SignUpPage
