import { Outlet, useNavigate } from 'react-router-dom'
import { LogOutIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import ThemeToggle from '@/src/components/ThemeToggle'
import { signOut } from '@/src/lib/cognito'

function Layout() {
  const navigate = useNavigate()

  function handleSignOut() {
    signOut()
    navigate('/sign-in')
  }

  return (
    <div className="min-h-screen">
      <header className="flex items-center justify-between border-b border-border px-4 py-3">
        <nav className="text-sm font-bold">Nudge</nav>
        <div className="flex items-center gap-1">
          <Button variant="ghost" onClick={handleSignOut}>
            <LogOutIcon /> Sign out
          </Button>
          <ThemeToggle />
        </div>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}

export default Layout
