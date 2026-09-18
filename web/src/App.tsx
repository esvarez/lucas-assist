import { Navigate, Route, Routes } from 'react-router-dom'
import Layout from './layout/Layout'
import ProjectsPage from './routes/ProjectsPage'
import ProjectDetailPage from './routes/ProjectDetailPage'

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
