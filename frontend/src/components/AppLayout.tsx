import { ReactNode, useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { api, User } from '../api/client'
import LateJoinerBanner from './LateJoinerBanner'

interface AppLayoutProps {
  children: ReactNode
}

export default function AppLayout({ children }: AppLayoutProps) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
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
      if (err instanceof Error && (err.message.includes('401') || err.message.includes('403'))) {
        setError('Your session has expired. Please log in again.')
      } else {
        setError(err instanceof Error ? err.message : 'An unknown error occurred while trying to authenticate.')
      }
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

  if (error) {
    const backendOrigin = window.location.origin.includes(':3000')
      ? window.location.origin.replace(':3000', ':3001')
      : window.location.origin
    return (
      <div style={{ padding: '40px', margin: '40px auto', maxWidth: '600px', background: 'white', borderRadius: '8px', border: '1px solid #ddd', textAlign: 'center' }}>
        <h2 style={{ color: '#dc3545', marginBottom: '1rem' }}>Authentication Error</h2>
        <p style={{ color: '#333', marginBottom: '1.5rem' }}>{error}</p>
        <a
          href={`${backendOrigin}/login`}
          style={{
            display: 'inline-block',
            padding: '12px 24px',
            background: '#007bff',
            color: 'white',
            textDecoration: 'none',
            borderRadius: '6px',
            fontWeight: 600,
          }}
        >
          Go to Login
        </a>
      </div>
    )
  }

  const role = user?.role || ''
  const isActive = (path: string) => location.pathname.startsWith(path)

  // Determine Learning link based on role (relative to /app basename)
  const getLearningLink = () => {
    if (role === 'mentor') return '/mentor'
    if (role === 'mentor_head') return '/mentor-head'
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
            {(role === 'mentor_head' || role === 'mentor' || role === 'hr' || role === 'student_success') && (
              <li>
                <Link
                  to={getLearningLink()}
                  className={
                    (isActive('/mentor') ||
                      (isActive('/mentor-head') && !isActive('/mentor-head/evaluations')) ||
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
            {(role === 'mentor_head' || role === 'mentor' || role === 'hr' || role === 'student_success' || role === 'admin' || role === 'moderator') && (
              <li>
                <Link to="/students" className={isActive('/students') ? 'active' : ''}>
                  Students
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
          <a
            href="/logout"
            className="btn btn-secondary"
            style={{
              width: '100%',
              display: 'block',
              textAlign: 'center',
              backgroundColor: '#6c757d',
              color: 'white',
              textDecoration: 'none',
              padding: '12px',
            }}
          >
            Logout
          </a>
        </div>
      </aside>
      <main className="main-content">
        <LateJoinerBanner userRole={role} />
        {children}
      </main>
    </div>
  )
}
