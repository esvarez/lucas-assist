import { useId, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import AuthForm from '@/src/components/AuthForm'
import PasswordField from '@/src/components/PasswordField'

function SignInPage() {
  const emailId = useId()
  const passwordId = useId()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    if (!email.trim() || !password) {
      setError('Enter your email and password.')
      return
    }
    setError(null)
    // Cognito InitiateAuth (SRP) is not wired on this screen yet.
  }

  return (
    <AuthForm
      title="Sign in"
      description="Use the email and password for your Nudge account."
      errorTitle="Couldn't sign in"
      error={error}
      submitLabel="Sign in"
      onSubmit={handleSubmit}
      footer={
        <p className="text-center text-muted-foreground">
          Don&apos;t have an account?{' '}
          <Link to="/sign-up" className="font-medium text-foreground underline-offset-4 hover:underline">
            Sign up
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
        aria-invalid={Boolean(error) && !password}
      />
    </AuthForm>
  )
}

export default SignInPage
