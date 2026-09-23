import { useId, useState, type SubmitEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import AuthForm from '@/src/components/AuthForm'
import { cognitoErrorMessage, forgotPassword } from '@/src/lib/cognito'

function ForgotPasswordPage() {
  const navigate = useNavigate()
  const emailId = useId()
  const [email, setEmail] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault()
    const trimmedEmail = email.trim()
    if (!trimmedEmail || submitting) {
      if (!trimmedEmail) setError('Enter your email.')
      return
    }
    setError(null)
    setSubmitting(true)
    try {
      await forgotPassword(trimmedEmail)
      navigate('/forgot-password/reset', { state: { email: trimmedEmail } })
    } catch (err) {
      setError(cognitoErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthForm
      title="Reset your password"
      description="Enter your account email and we'll send you a code to reset your password."
      errorTitle="Couldn't send reset code"
      error={error}
      submitLabel={submitting ? 'Sending code…' : 'Send reset code'}
      submitDisabled={submitting}
      onSubmit={handleSubmit}
      footer={
        <p className="text-center text-muted-foreground">
          Remembered it?{' '}
          <Link to="/sign-in" className="font-medium text-foreground underline-offset-4 hover:underline">
            Sign in
          </Link>
        </p>
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
    </AuthForm>
  )
}

export default ForgotPasswordPage
