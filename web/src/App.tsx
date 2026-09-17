import { Navigate, Route, Routes } from 'react-router-dom'
import Layout from './layout/Layout'
import ProjectsPage from './routes/ProjectsPage'

function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Navigate to="/projects" replace />} />
        <Route path="/projects" element={<ProjectsPage />} />
      </Route>
    </Routes>
  )
}

export default App
