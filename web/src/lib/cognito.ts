import {
  AuthenticationDetails,
  CognitoUser,
  CognitoUserPool,
  type CognitoUserSession,
} from 'amazon-cognito-identity-js'

const userPool = new CognitoUserPool({
  UserPoolId: import.meta.env.VITE_COGNITO_USER_POOL_ID,
  ClientId: import.meta.env.VITE_COGNITO_CLIENT_ID,
})

export function cognitoErrorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message
  return 'Something went wrong. Try again.'
}

const PASSWORD_MIN_LENGTH = 8

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

export function forgotPassword(email: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const user = new CognitoUser({ Username: email, Pool: userPool })
    user.forgotPassword({
      onSuccess: () => resolve(),
      onFailure: (err) => {
        // NudgeUserPoolClient doesn't set PreventUserExistenceErrors:
        // ENABLED (changing that is out of scope for #146), so Cognito's
        // raw response for an unknown email is UserNotFoundException —
        // which reveals whether an account exists. Mask that one case by
        // treating it as success here, at the app layer, matching what
        // PreventUserExistenceErrors would do at the Cognito layer.
        if (err instanceof Error && err.name === 'UserNotFoundException') {
          resolve()
          return
        }
        reject(err)
      },
    })
  })
}

export function confirmForgotPassword(email: string, code: string, newPassword: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const user = new CognitoUser({ Username: email, Pool: userPool })
    user.confirmPassword(code, newPassword, {
      onSuccess: () => resolve(),
      onFailure: (err) => reject(err),
    })
  })
}

// authenticateUser defaults to USER_SRP_AUTH — the password itself is never
// sent to Cognito, per ADR 017. Do not switch this to USER_PASSWORD_AUTH.
export function signIn(email: string, password: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const user = new CognitoUser({ Username: email, Pool: userPool })
    const authDetails = new AuthenticationDetails({ Username: email, Password: password })
    user.authenticateUser(authDetails, {
      onSuccess: () => resolve(),
      onFailure: (err) => reject(err),
    })
  })
}

// Resolves the signed-in user's access token, or null when nobody is signed in
// or the session can't be restored. getSession refreshes an expired access
// token from the stored refresh token before resolving.
export function getAccessToken(): Promise<string | null> {
  const user = userPool.getCurrentUser()
  if (!user) return Promise.resolve(null)
  return new Promise((resolve) => {
    user.getSession((err: Error | null, session: CognitoUserSession | null) => {
      if (err || !session || !session.isValid()) {
        resolve(null)
        return
      }
      resolve(session.getAccessToken().getJwtToken())
    })
  })
}
