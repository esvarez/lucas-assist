import { CognitoUser, CognitoUserPool, type CognitoUserSession } from 'amazon-cognito-identity-js'

const userPool = new CognitoUserPool({
  UserPoolId: import.meta.env.VITE_COGNITO_USER_POOL_ID,
  ClientId: import.meta.env.VITE_COGNITO_CLIENT_ID,
})

export function cognitoErrorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message
  return 'Something went wrong. Try again.'
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
