import { Navigate, Route, Routes } from 'react-router-dom'
import AuthLayout from '@/src/layout/AuthLayout'
import Layout from '@/src/layout/Layout'
import RequireAuth from '@/src/layout/RequireAuth'
import ConfirmSignUpPage from '@/src/routes/ConfirmSignUpPage'
import ProjectDetailPage from '@/src/routes/ProjectDetailPage'
import ProjectsPage from '@/src/routes/ProjectsPage'
import SignInPage from '@/src/routes/SignInPage'
import SignUpPage from '@/src/routes/SignUpPage'

function App() {
  return (
    <Routes>
      <Route element={<AuthLayout />}>
        <Route path="/sign-in" element={<SignInPage />} />
        <Route path="/sign-up" element={<SignUpPage />} />
        <Route path="/sign-up/confirm" element={<ConfirmSignUpPage />} />
      </Route>
      <Route element={<RequireAuth />}>
        <Route element={<Layout />}>
          <Route index element={<Navigate to="/projects" replace />} />
          <Route path="/projects" element={<ProjectsPage />} />
          <Route path="/projects/:id" element={<ProjectDetailPage />} />
        </Route>
      </Route>
    </Routes>
  )
}

export default App
