import { Outlet } from 'react-router-dom'
import ThemeToggle from '@/src/components/ThemeToggle'

function Layout() {
  return (
    <div className="min-h-screen">
      <header className="flex items-center justify-between border-b border-border px-4 py-3">
        <nav className="text-sm font-bold">Nudge</nav>
        <ThemeToggle />
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}

export default Layout
