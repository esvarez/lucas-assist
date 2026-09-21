import { Outlet } from 'react-router-dom'
import ThemeToggle from '@/src/components/ThemeToggle'

function AuthLayout() {
  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex items-center justify-between border-b border-border px-4 py-3">
        <span className="text-sm font-bold">Nudge</span>
        <ThemeToggle />
      </header>
      <main className="flex flex-1 items-center justify-center p-4">
        <div className="w-full max-w-sm">
          <Outlet />
        </div>
      </main>
    </div>
  )
}

export default AuthLayout
