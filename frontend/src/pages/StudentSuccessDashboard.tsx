import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type StudentSuccessClass } from '../api/client'

interface Group {
  mentor_id?: string
  mentor_email?: string
  mentor_name?: string
  classes: StudentSuccessClass[]
}

export default function StudentSuccessDashboard() {
  const [classes, setClasses] = useState<StudentSuccessClass[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()

  useEffect(() => {
    loadData()
  }, [])

  async function loadData() {
    try {
      setLoading(true)
      setError(null)
      const me = await api.getMe()
      if (me.role !== 'student_success') {
        setError('No access. Student Success only.')
        setLoading(false)
        return
      }
      const data = await api.getStudentSuccessClasses()
      setClasses(data.classes)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load classes')
    } finally {
      setLoading(false)
    }
  }

  function groupByMentor(list: StudentSuccessClass[]): Group[] {
    const mentorMap = new Map<string, Group>()
    const unassigned: Group = { classes: [] }

    for (const c of list) {
      if (c.mentor_user_id && (c.mentor_email || c.mentor_name)) {
        const key = c.mentor_user_id
        if (!mentorMap.has(key)) {
          mentorMap.set(key, {
            mentor_id: key,
            mentor_email: c.mentor_email,
            mentor_name: c.mentor_name,
            classes: [],
          })
        }
        mentorMap.get(key)!.classes.push(c)
      } else {
        unassigned.classes.push(c)
      }
    }

    const out: Group[] = []
    if (unassigned.classes.length > 0) out.push(unassigned)
    mentorMap.forEach((g) => out.push(g))
    return out
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
        <div style={{ background: '#f8d7da', padding: '16px', borderRadius: '8px', color: '#721c24' }}>
          <strong>Error:</strong> {error}
        </div>
      </div>
    )
  }

  const groups = groupByMentor(classes)

  return (
    <div>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <h1>Student Success Dashboard</h1>
      </div>

      <p style={{ marginTop: '8px', marginBottom: '24px', color: '#666' }}>
        Active classes only (round started). Grouped by mentor.
      </p>

      {groups.length === 0 ? (
        <div style={{ padding: '24px', background: '#f9f9f9', borderRadius: '8px', textAlign: 'center' }}>
          <p>No active classes.</p>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
          {groups.map((grp) => (
            <div key={grp.mentor_id ?? 'unassigned'}>
              <h2 style={{ fontSize: '16px', marginBottom: '12px', color: '#333' }}>
                {grp.mentor_email ?? grp.mentor_name ?? 'Unassigned'}
              </h2>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: '16px' }}>
                {grp.classes.map((cls) => (
                  <div
                    key={cls.class_key}
                    style={{
                      padding: '16px',
                      borderRadius: '6px',
                      border: '1px solid #eee',
                      background: '#fff',
                    }}
                  >
                    <div style={{ marginBottom: '12px' }}>
                      <h3 style={{ fontSize: '16px', marginBottom: '6px' }}>
                        Level {cls.level} · Class {cls.class_number}
                      </h3>
                      <p style={{ color: '#666', fontSize: '13px', marginBottom: '4px' }}>
                        {cls.days} · {cls.time}
                      </p>
                      <p style={{ color: '#666', fontSize: '13px', marginBottom: '8px' }}>
                        {cls.student_count} student{cls.student_count !== 1 ? 's' : ''}
                      </p>
                      <span
                        style={{
                          display: 'inline-block',
                          padding: '4px 10px',
                          borderRadius: '12px',
                          fontSize: '12px',
                          fontWeight: 600,
                          background: '#d4edda',
                          color: '#155724',
                        }}
                      >
                        ACTIVE
                      </span>
                    </div>
                    <button
                      onClick={() => navigate(`/student-success/class?class_key=${encodeURIComponent(cls.class_key)}`)}
                      style={{
                        width: '100%',
                        padding: '8px',
                        background: '#007bff',
                        color: 'white',
                        border: 'none',
                        borderRadius: '4px',
                        cursor: 'pointer',
                        fontSize: '13px',
                      }}
                    >
                      Open Class
                    </button>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
