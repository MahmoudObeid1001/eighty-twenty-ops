import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import AppLayout from './components/AppLayout'
import MentorDashboard from './pages/MentorDashboard'
import MentorHeadDashboard from './pages/MentorHeadDashboard'
import MentorEvaluations from './pages/MentorEvaluations'
import ClassWorkspace from './pages/ClassWorkspace'
import StudentSuccessDashboard from './pages/StudentSuccessDashboard'
import StudentSuccessClass from './pages/StudentSuccessClass'

function App() {
  const location = useLocation()
  const classKey = new URLSearchParams(location.search).get('class_key')

  return (
    <AppLayout>
      <Routes>
        <Route path="/mentor" element={<MentorDashboard />} />
        <Route path="/mentor-head" element={<MentorHeadDashboard />} />
        <Route path="/mentor-head/evaluations" element={<MentorEvaluations />} />
        <Route path="/mentor/class" element={<ClassWorkspace />} />
        <Route path="/mentor-head/class" element={<ClassWorkspace />} />
        <Route path="/student-success" element={<StudentSuccessDashboard />} />
        <Route path="/student-success/class" element={<StudentSuccessClass />} />
        <Route path="/" element={<Navigate to={classKey ? `/student-success/class?class_key=${classKey}` : "/mentor"} replace />} />
      </Routes>
    </AppLayout>
  )
}

export default App
