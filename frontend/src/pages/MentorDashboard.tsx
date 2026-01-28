import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, User, Class } from '../api/client'

export default function MentorDashboard() {
  const [user, setUser] = useState<User | null>(null)
  const [classes, setClasses] = useState<Class[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()

  useEffect(() => {
    loadData()
  }, [])

  async function loadData() {
    try {
      setLoading(true)
      const [userData, classesData] = await Promise.all([
        api.getMe(),
        api.getMentorClasses(),
      ])
      setUser(userData)
      setClasses(classesData)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load data')
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
      <div style={{ padding: '40px' }}>
        <div style={{ background: '#fee', padding: '16px', borderRadius: '8px', color: '#c33' }}>
          <strong>Error:</strong> {error}
        </div>
      </div>
    )
  }

  return (
    <div>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <h1>Welcome, {user?.email || 'Mentor'}</h1>
      </div>

      {classes.length === 0 ? (
        <div style={{ padding: '40px', textAlign: 'center', background: 'white', borderRadius: '8px' }}>
          <p style={{ color: '#666' }}>No classes assigned yet.</p>
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: '20px' }}>
          {classes.map((cls) => (
            <div
              key={cls.class_key}
              style={{
                background: 'white',
                padding: '20px',
                borderRadius: '8px',
                border: '1px solid #ddd',
                boxShadow: '0 2px 4px rgba(0,0,0,0.1)',
              }}
            >
              <div style={{ marginBottom: '12px' }}>
                <h3 style={{ fontSize: '18px', marginBottom: '8px' }}>
                  Level {cls.level} · Class {cls.class_number}
                </h3>
                <p style={{ color: '#666', fontSize: '14px', marginBottom: '4px' }}>
                  {cls.days} · {cls.time}
                </p>
                <p style={{ color: '#666', fontSize: '14px' }}>
                  {cls.student_count} student{cls.student_count !== 1 ? 's' : ''}
                </p>
              </div>
              <button
                onClick={() => navigate(`/mentor/class?class_key=${encodeURIComponent(cls.class_key)}`)}
                style={{
                  width: '100%',
                  padding: '10px',
                  background: '#007bff',
                  color: 'white',
                  border: 'none',
                  borderRadius: '6px',
                  cursor: 'pointer',
                  fontSize: '14px',
                }}
              >
                Open Class
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
