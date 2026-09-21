import { useId, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import AuthForm from '@/src/components/AuthForm'
import PasswordField from '@/src/components/PasswordField'

function SignUpPage() {
  const navigate = useNavigate()
  const emailId = useId()
  const passwordId = useId()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    const trimmedEmail = email.trim()
    if (!trimmedEmail || !password) {
      setError('Enter your email and a password.')
      return
    }
    setError(null)
    // Account creation is not wired yet. This only advances to the confirmation screen.
    navigate('/sign-up/confirm', { state: { email: trimmedEmail } })
  }

  return (
    <AuthForm
      title="Create account"
      description="Sign up with your email. You'll confirm it on the next screen."
      errorTitle="Couldn't create account"
      error={error}
      submitLabel="Create account"
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
        aria-invalid={Boolean(error) && !password}
      />
    </AuthForm>
  )
}

export default SignUpPage
