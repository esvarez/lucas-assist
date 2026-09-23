import { CognitoUser, CognitoUserPool } from 'amazon-cognito-identity-js'

const userPool = new CognitoUserPool({
  UserPoolId: import.meta.env.VITE_COGNITO_USER_POOL_ID,
  ClientId: import.meta.env.VITE_COGNITO_CLIENT_ID,
})

export function cognitoErrorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message
  return 'Something went wrong. Try again.'
}

// PASSWORD_MIN_LENGTH mirrors NudgeUserPool's password policy
// (template.yaml sets no explicit PasswordPolicy, so the User Pool runs
// Cognito's default: minimum length 8, plus at least one uppercase,
// lowercase, number, and symbol). Update this alongside template.yaml if
// an explicit PasswordPolicy block is ever added there.
const PASSWORD_MIN_LENGTH = 8

// passwordPolicyError checks password against that same policy and
// returns a human-readable reason it fails, or null if it satisfies every
// rule. Checked before calling signUp so a violation is caught locally
// instead of round-tripping to Cognito's InvalidPasswordException.
export function passwordPolicyError(password: string): string | null {
  if (password.length < PASSWORD_MIN_LENGTH) {
    return `Password must be at least ${PASSWORD_MIN_LENGTH} characters.`
  }
  if (!/[A-Z]/.test(password)) return 'Password must contain an uppercase letter.'
  if (!/[a-z]/.test(password)) return 'Password must contain a lowercase letter.'
  if (!/[0-9]/.test(password)) return 'Password must contain a number.'
  if (!/[^A-Za-z0-9]/.test(password)) return 'Password must contain a symbol.'
  return null
}

export function signUp(email: string, password: string): Promise<void> {
  return new Promise((resolve, reject) => {
    userPool.signUp(email, password, [], [], (err) => {
      if (err) {
        reject(err)
        return
      }
      resolve()
    })
  })
}

export function confirmSignUp(email: string, code: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const user = new CognitoUser({ Username: email, Pool: userPool })
    user.confirmRegistration(code, true, (err) => {
      if (err) {
        reject(err)
        return
      }
      resolve()
    })
  })
}
