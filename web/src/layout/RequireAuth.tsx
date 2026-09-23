import { useEffect, useState } from 'react'
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { getAccessToken } from '@/src/lib/cognito'

type AuthStatus = 'checking' | 'signed-in' | 'signed-out'

function RequireAuth() {
  const { pathname } = useLocation()
  const [status, setStatus] = useState<AuthStatus>('checking')

  // Re-check on every navigation so a session that expires mid-visit is
  // treated the same as never having signed in.
  useEffect(() => {
    let cancelled = false
    getAccessToken().then(
      (token) => {
        if (!cancelled) setStatus(token ? 'signed-in' : 'signed-out')
      },
      () => {
        if (!cancelled) setStatus('signed-out')
      },
    )
    return () => {
      cancelled = true
    }
  }, [pathname])

  // Render nothing until the first check resolves, so protected content
  // never flashes before a redirect.
  if (status === 'checking') return null
  if (status === 'signed-out') return <Navigate to="/sign-in" replace />
  return <Outlet />
}

export default RequireAuth
