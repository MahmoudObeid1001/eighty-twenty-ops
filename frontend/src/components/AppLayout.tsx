import { ReactNode, useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { api, User } from '../api/client'

interface AppLayoutProps {
  children: ReactNode
}

export default function AppLayout({ children }: AppLayoutProps) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const location = useLocation()

  useEffect(() => {
    loadUser()
  }, [])

  async function loadUser() {
    try {
      const userData = await api.getMe()
      setUser(userData)
    } catch (err) {
      console.error('Failed to load user:', err)
    } finally {
      setLoading(false)
    }
  }

  if (loading) {
    return (
      <div style={{ padding: '40px', textAlign: 'center' }}>
        <p>Loading...</p>
      </div>
    )
  }

  const role = user?.role || ''
  const isActive = (path: string) => location.pathname.startsWith(path)

  // Determine Learning link based on role (relative to /app basename)
  const getLearningLink = () => {
    if (role === 'mentor') return '/mentor'
    if (role === 'mentor_head') return '/mentor-head'
    if (role === 'community_officer') return '/community-officer'
    if (role === 'hr') return '/hr/mentors'
    if (role === 'student_success') return '/student-success'
    return '/mentor'
  }

  // Check if we're on a class page (for active state)
  const isClassPage = location.pathname.includes('/class')

  return (
    <div className="container">
      <aside className="sidebar">
        <div className="brand-block">
          <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo-sidebar" />
          <span className="brand-name">Eighty Twenty</span>
        </div>
        <nav style={{ flex: 1 }}>
          <ul>
            {(role === 'mentor_head' || role === 'mentor' || role === 'community_officer' || role === 'hr' || role === 'student_success') && (
              <li>
                <Link
                  to={getLearningLink()}
                  className={
                    (isActive('/mentor') ||
                      (isActive('/mentor-head') && !isActive('/mentor-head/evaluations')) ||
                      isActive('/community-officer') ||
                      isActive('/hr') ||
                      isActive('/student-success') ||
                      isClassPage)
                      ? 'active'
                      : ''
                  }
                >
                  Learning
                </Link>
              </li>
            )}
            {role === 'mentor_head' && (
              <li>
                <Link to="/mentor-head/evaluations" className={isActive('/mentor-head/evaluations') ? 'active' : ''}>
                  Mentor Evaluations
                </Link>
              </li>
            )}
            {role === 'admin' && (
              <>
                <li>
                  <a href="/pre-enrolment">Pre-Enrolment</a>
                </li>
                <li>
                  <a href="/classes">Classes</a>
                </li>
                <li>
                  <a href="/finance">Finance</a>
                </li>
              </>
            )}
            {role === 'moderator' && (
              <li>
                <a href="/pre-enrolment">Pre-Enrolment</a>
              </li>
            )}
          </ul>
        </nav>
        <div style={{ padding: '20px', borderTop: '1px solid #8C8C8C', marginTop: 'auto' }}>
          <a href="/logout" className="btn btn-secondary" style={{ width: '100%', display: 'block', textAlign: 'center', backgroundColor: '#6c757d', color: 'white', textDecoration: 'none', padding: '12px' }}>
            Logout
          </a>
        </div>
      </aside>
      <main className="main-content">
        {children}
      </main>
    </div>
  )
}
