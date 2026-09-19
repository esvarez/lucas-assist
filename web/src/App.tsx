import { Navigate, Route, Routes } from 'react-router-dom'
import Layout from '@/src/layout/Layout'
import ProjectsPage from '@/src/routes/ProjectsPage'
import ProjectDetailPage from '@/src/routes/ProjectDetailPage'

function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Navigate to="/projects" replace />} />
        <Route path="/projects" element={<ProjectsPage />} />
        <Route path="/projects/:id" element={<ProjectDetailPage />} />
      </Route>
    </Routes>
  )
}

export default App
