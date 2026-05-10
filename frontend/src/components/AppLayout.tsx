import { ReactNode, useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { api, User } from '../api/client'
import LateJoinerBanner from './LateJoinerBanner'
import OpsNotificationBanner from './OpsNotificationBanner'

interface AppLayoutProps {
  children: ReactNode
}

export default function AppLayout({ children }: AppLayoutProps) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false)
  const location = useLocation()

  useEffect(() => {
    setIsMobileMenuOpen(false)
  }, [location.pathname])

  useEffect(() => {
    loadUser()
  }, [])

  async function loadUser() {
    try {
      const userData = await api.getMe()
      if (userData.must_change_password && location.pathname !== '/setup-password') {
        window.location.href = '/app/setup-password'
        return
      }
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
    return (
      <div style={{ padding: '40px', margin: '40px auto', maxWidth: '600px', background: 'white', borderRadius: '8px', border: '1px solid #ddd', textAlign: 'center' }}>
        <h2 style={{ color: '#dc3545', marginBottom: '1rem' }}>Authentication Error</h2>
        <p style={{ color: '#333', marginBottom: '1.5rem' }}>{error}</p>
        <a
          href="/login"
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
    if (role === 'manager') return '/mentor-head'
    return '/mentor'
  }

  // Check if we're on a class page (for active state)
  const isClassPage = location.pathname.includes('/class')

  return (
    <div className="container">
      {/* Mobile Header */}
      <div className="mobile-header">
        <div className="brand-block">
          <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo-sidebar" />
          <span className="brand-name">Eighty Twenty</span>
        </div>
        <button className="hamburger-btn" onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)} aria-label="Toggle Navigation">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="3" y1="12" x2="21" y2="12"></line><line x1="3" y1="6" x2="21" y2="6"></line><line x1="3" y1="18" x2="21" y2="18"></line></svg>
        </button>
      </div>

      <div 
        className={`sidebar-overlay ${isMobileMenuOpen ? 'active' : ''}`}
        onClick={() => setIsMobileMenuOpen(false)}
      ></div>

      <aside className={`sidebar ${isMobileMenuOpen ? 'sidebar-open' : ''}`}>
        <div className="brand-block">
          <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo-sidebar" />
          <span className="brand-name">Eighty Twenty</span>
        </div>


        <nav className="app-nav" style={{ flex: 1 }}>
          <ul className="app-nav-list">
            {(role === 'mentor_head' || role === 'mentor' || role === 'hr' || role === 'student_success' || role === 'manager') && (
              <li>
                <Link
                  to={getLearningLink()}
                  className={
                    (isActive('/mentor') ||
                      (isActive('/mentor-head') && !isActive('/mentor-head/evaluations')) ||
                      isActive('/hr') ||
                      isActive('/student-success') ||
                      isClassPage)
                      && !isActive('/mentors')
                      ? 'active'
                      : ''
                  }
                >
                  Learning
                </Link>
              </li>
            )}
            {role === 'student_success' && (
              <li>
                <a href="/classes" className={typeof window !== 'undefined' && window.location.pathname === '/classes' ? 'active' : ''}>
                  Classes
                </a>
              </li>
            )}
            {role === 'manager' && (
              <li>
                <Link to="/manager-dashboard" className={isActive('/manager-dashboard') ? 'active' : ''}>
                  Manager Dashboard
                </Link>
              </li>
            )}
            {(role === 'mentor_head' || role === 'manager') && (
              <li>
                <Link to="/mentor-head/evaluations" className={isActive('/mentor-head/evaluations') ? 'active' : ''}>
                  Mentor Evaluations
                </Link>
              </li>
            )}
            {(role === 'mentor_head' || role === 'mentor' || role === 'hr' || role === 'student_success' || role === 'admin' || role === 'moderator' || role === 'manager') && (
              <li>
                <Link to="/students" className={isActive('/students') ? 'active' : ''}>
                  Students
                </Link>
              </li>
            )}
            {(role === 'student_success' || role === 'mentor_head' || role === 'admin' || role === 'manager') && (
              <li>
                <Link
                  to="/reports"
                  className={isActive('/reports') ? 'active' : ''}
                  style={{ display: 'flex', alignItems: 'center', gap: '8px' }}
                >
                  <BarChartIcon />
                  Reports
                </Link>
              </li>
            )}
            {(role === 'mentor_head' || role === 'admin' || role === 'manager') && (
              <li>
                <Link to="/mentors" className={isActive('/mentors') ? 'active' : ''}>
                  Mentors
                </Link>
              </li>
            )}
            {(role === 'admin' || role === 'manager') && (
              <>
                {role === 'manager' && (
                  <li>
                    <Link to="/staff" className={isActive('/staff') ? 'active' : ''}>
                      Staff Management
                    </Link>
                  </li>
                )}
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
        <div className="app-sidebar-footer" style={{ padding: '20px', borderTop: '1px solid #8C8C8C', marginTop: 'auto' }}>
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
        <OpsNotificationBanner userRole={role} />
        {children}
      </main>
    </div>
  )
}

function BarChartIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M4 20V10" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M10 20V4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M16 20V13" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M22 20V7" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  )
}
