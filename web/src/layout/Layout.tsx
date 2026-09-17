import { Outlet } from 'react-router-dom'

function Layout() {
  return (
    <div className="app-shell">
      <header className="app-header">
        <nav>Nudge</nav>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}

export default Layout
