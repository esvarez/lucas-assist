import { useId, useState, type SubmitEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import AuthForm from '@/src/components/AuthForm'
import PasswordField from '@/src/components/PasswordField'
import { cognitoErrorMessage, confirmForgotPassword, passwordPolicyError } from '@/src/lib/cognito'

function emailFromState(state: unknown): string | null {
  if (!state || typeof state !== 'object' || !('email' in state)) return null
  const email = state.email
  if (typeof email !== 'string') return null
  const trimmed = email.trim()
  return trimmed || null
}

function ResetPasswordPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const email = emailFromState(location.state)
  const codeId = useId()
  const passwordId = useId()
  const [code, setCode] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault()
    const trimmedCode = code.trim()
    if (!trimmedCode || !newPassword || submitting) {
      if (!trimmedCode || !newPassword) setError('Enter the code and a new password.')
      return
    }
    if (!email) {
      setError('Missing email — start over from the forgot password form.')
      return
    }
    const policyError = passwordPolicyError(newPassword)
    if (policyError) {
      setError(policyError)
      return
    }
    setError(null)
    setSubmitting(true)
    try {
      await confirmForgotPassword(email, trimmedCode, newPassword)
      navigate('/sign-in')
    } catch (err) {
      setError(cognitoErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthForm
      title="Choose a new password"
      description={
        email
          ? `Enter the code sent to ${email} and choose a new password.`
          : 'Enter the code sent to your email and choose a new password.'
      }
      errorTitle="Couldn't reset password"
      error={error}
      submitLabel={submitting ? 'Resetting…' : 'Reset password'}
      submitDisabled={submitting}
      onSubmit={handleSubmit}
      footer={
        <p className="text-center text-muted-foreground">
          Didn&apos;t get a code?{' '}
          <Link to="/forgot-password" className="font-medium text-foreground underline-offset-4 hover:underline">
            Start over
          </Link>
        </p>
      }
    >
      <div className="flex flex-col gap-2">
        <Label htmlFor={codeId}>Reset code</Label>
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
          disabled={submitting}
          aria-invalid={Boolean(error) && !code.trim()}
        />
      </div>
      <PasswordField
        id={passwordId}
        label="New password"
        name="newPassword"
        autoComplete="new-password"
        value={newPassword}
        onChange={(event) => {
          setNewPassword(event.target.value)
          setError(null)
        }}
        disabled={submitting}
        aria-invalid={Boolean(error) && !newPassword}
      />
    </AuthForm>
  )
}

export default ResetPasswordPage
