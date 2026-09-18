import { Outlet } from 'react-router-dom'
import { Button } from '@/components/ui/button'

function Layout() {
  return (
    <div className="app-shell">
      <header className="app-header">
        <nav>Nudge</nav>
        <h1 className="text-3xl font-bold underline">
    Hello world!
  </h1>
  <Button>Click me</Button>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}

export default Layout
