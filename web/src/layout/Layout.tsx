import { Link, Outlet } from 'react-router-dom'
import { LogOutIcon } from 'lucide-react'
import { buttonVariants } from '@/components/ui/button'
import ThemeToggle from '@/src/components/ThemeToggle'

function Layout() {
  return (
    <div className="min-h-screen">
      <header className="flex items-center justify-between border-b border-border px-4 py-3">
        <nav className="text-sm font-bold">Nudge</nav>
        <div className="flex items-center gap-1">
          {/* Session clearing is not wired yet — this only opens the sign-in screen. */}
          <Link to="/sign-in" className={buttonVariants({ variant: 'ghost' })}>
            <LogOutIcon /> Sign out
          </Link>
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
