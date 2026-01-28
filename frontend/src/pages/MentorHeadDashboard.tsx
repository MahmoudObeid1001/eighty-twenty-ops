import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type MentorHeadDashboard as MentorHeadDashboardData, MentorHeadClass } from '../api/client'

export default function MentorHeadDashboard() {
  const [dashboard, setDashboard] = useState<MentorHeadDashboardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [assigning, setAssigning] = useState<string | null>(null)
  const [actioning, setActioning] = useState<string | null>(null)
  const [cardError, setCardError] = useState<Record<string, string>>({}) // per-class_key error (e.g. 409)
  const navigate = useNavigate()

  useEffect(() => {
    loadData()
  }, [])

  async function loadData() {
    try {
      setLoading(true)
      setError(null)
      const data = await api.getMentorHeadDashboard()
      setDashboard(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load data')
    } finally {
      setLoading(false)
    }
  }

  function clearCardError(classKey: string) {
    setCardError((prev) => {
      const next = { ...prev }
      delete next[classKey]
      return next
    })
  }

  async function handleAssignMentor(classKey: string, mentorEmail: string) {
    try {
      setAssigning(classKey)
      setMessage(null)
      clearCardError(classKey)
      await api.assignMentor(classKey, mentorEmail)
      setMessage({ type: 'success', text: 'Mentor assigned successfully' })
      await loadData()
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to assign mentor'
      setCardError((prev) => ({ ...prev, [classKey]: msg }))
    } finally {
      setAssigning(null)
    }
  }

  async function handleUnassign(classKey: string) {
    try {
      setActioning(`${classKey}:unassign`)
      setMessage(null)
      clearCardError(classKey)
      await api.unassignMentor(classKey)
      setMessage({ type: 'success', text: 'Mentor unassigned' })
      await loadData()
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to unassign' })
    } finally {
      setActioning(null)
    }
  }

  async function handleStartRound(classKey: string) {
    try {
      setActioning(`${classKey}:start`)
      setMessage(null)
      await api.startRound(classKey)
      setMessage({ type: 'success', text: 'Round started successfully' })
      await loadData()
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to start round' })
    } finally {
      setActioning(null)
    }
  }

  async function handleCloseRound(classKey: string) {
    try {
      setActioning(`${classKey}:close`)
      setMessage(null)
      await api.closeRound(classKey)
      setMessage({ type: 'success', text: 'Round closed successfully' })
      await loadData()
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to close round' })
    } finally {
      setActioning(null)
    }
  }

  async function handleReturnClass(classKey: string) {
    try {
      setActioning(`${classKey}:return`)
      setMessage(null)
      await api.returnToOps(classKey)
      setMessage({ type: 'success', text: 'Class returned to Operations' })
      await loadData()
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to return class' })
    } finally {
      setActioning(null)
    }
  }

  // Group classes by mentor (same logic as SSR)
  function groupClassesByMentor(classes: MentorHeadClass[]): MentorGroup[] {
    const mentorMap = new Map<string, MentorGroup>()
    const unassigned: MentorGroup = { classes: [] }

    for (const cls of classes) {
      if (cls.mentor_user_id && cls.mentor_email) {
        if (!mentorMap.has(cls.mentor_user_id)) {
          mentorMap.set(cls.mentor_user_id, {
            mentor_id: cls.mentor_user_id,
            mentor_email: cls.mentor_email,
            classes: [],
          })
        }
        mentorMap.get(cls.mentor_user_id)!.classes.push(cls)
      } else {
        unassigned.classes.push(cls)
      }
    }

    const groups: MentorGroup[] = []
    if (unassigned.classes.length > 0) {
      groups.push(unassigned)
    }
    mentorMap.forEach((group) => groups.push(group))
    return groups
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

  if (!dashboard) {
    return (
      <div style={{ padding: '40px', textAlign: 'center' }}>
        <p>No data available.</p>
      </div>
    )
  }

  const groups = groupClassesByMentor(dashboard.classes)

  return (
    <div>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <h1>Mentor Head Dashboard</h1>
      </div>

      {message && (
        <div
          style={{
            marginBottom: '20px',
            padding: '12px 16px',
            borderRadius: '6px',
            background: message.type === 'success' ? '#d4edda' : '#f8d7da',
            color: message.type === 'success' ? '#155724' : '#721c24',
            border: `1px solid ${message.type === 'success' ? '#c3e6cb' : '#f5c6cb'}`,
          }}
        >
          {message.text}
        </div>
      )}

      {groups.length === 0 ? (
        <div style={{ padding: '40px', textAlign: 'center', background: 'white', borderRadius: '8px' }}>
          <p style={{ color: '#666' }}>No classes available.</p>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '32px' }}>
          {groups.map((group, idx) => (
            <div key={idx} style={{ background: 'white', padding: '24px', borderRadius: '8px', border: '1px solid #ddd' }}>
              <h2 style={{ fontSize: '20px', marginBottom: '16px', color: '#333' }}>
                {group.mentor_email || 'Unassigned'}
              </h2>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: '16px' }}>
                {group.classes.map((cls) => (
                  <div
                    key={cls.class_key}
                    style={{
                      padding: '16px',
                      borderRadius: '6px',
                      border: '1px solid #eee',
                      background: '#f9f9f9',
                    }}
                  >
                    <div style={{ marginBottom: '12px' }}>
                      <h3 style={{ fontSize: '16px', marginBottom: '6px' }}>
                        Level {cls.level} · Class {cls.class_number}
                      </h3>
                      <p style={{ color: '#666', fontSize: '13px', marginBottom: '4px' }}>
                        {cls.days} · {cls.time}
                      </p>
                      <p style={{ color: '#666', fontSize: '13px', marginBottom: '4px' }}>
                        {cls.student_count} student{cls.student_count !== 1 ? 's' : ''} · {cls.readiness}
                      </p>
                    </div>

                    {/* Per-card error (e.g. 409) */}
                    {cardError[cls.class_key] && (
                      <div
                        style={{
                          marginBottom: '12px',
                          padding: '8px',
                          background: '#f8d7da',
                          color: '#721c24',
                          borderRadius: '4px',
                          fontSize: '13px',
                        }}
                      >
                        {cardError[cls.class_key]}
                      </div>
                    )}

                    {/* Mentor assignment */}
                    {!cls.mentor_user_id ? (
                      <div style={{ marginBottom: '12px' }}>
                        <select
                          id={`mentor-select-${cls.class_key}`}
                          style={{
                            width: '100%',
                            padding: '6px',
                            border: '1px solid #ddd',
                            borderRadius: '4px',
                            fontSize: '13px',
                            marginBottom: '6px',
                          }}
                        >
                          <option value="">Select mentor...</option>
                          {dashboard.mentors.map((m) => (
                            <option key={m.id} value={m.email}>
                              {m.email}
                            </option>
                          ))}
                        </select>
                        <button
                          onClick={() => {
                            const select = document.getElementById(`mentor-select-${cls.class_key}`) as HTMLSelectElement
                            const mentorEmail = select.value
                            if (mentorEmail) {
                              handleAssignMentor(cls.class_key, mentorEmail)
                            }
                          }}
                          disabled={assigning === cls.class_key}
                          style={{
                            width: '100%',
                            padding: '6px',
                            background: assigning === cls.class_key ? '#ccc' : '#007bff',
                            color: 'white',
                            border: 'none',
                            borderRadius: '4px',
                            cursor: assigning === cls.class_key ? 'not-allowed' : 'pointer',
                            fontSize: '13px',
                          }}
                        >
                          {assigning === cls.class_key ? 'Assigning...' : 'Assign Mentor'}
                        </button>
                      </div>
                    ) : (
                      <div style={{ marginBottom: '12px' }}>
                        <p style={{ margin: '0 0 6px', fontSize: '13px', color: '#155724' }}>
                          Assigned to {cls.mentor_email}
                        </p>
                        <button
                          onClick={() => handleUnassign(cls.class_key)}
                          disabled={actioning === `${cls.class_key}:unassign`}
                          style={{
                            width: '100%',
                            padding: '6px',
                            background: actioning === `${cls.class_key}:unassign` ? '#ccc' : '#6c757d',
                            color: 'white',
                            border: 'none',
                            borderRadius: '4px',
                            cursor: actioning === `${cls.class_key}:unassign` ? 'not-allowed' : 'pointer',
                            fontSize: '13px',
                          }}
                        >
                          {actioning === `${cls.class_key}:unassign` ? 'Unassigning...' : 'Unassign'}
                        </button>
                      </div>
                    )}

                    {/* Actions */}
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                      <button
                        onClick={() => navigate(`/mentor-head/class?class_key=${encodeURIComponent(cls.class_key)}`)}
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
                      <button
                        onClick={() => handleStartRound(cls.class_key)}
                        disabled={actioning === `${cls.class_key}:start`}
                        style={{
                          width: '100%',
                          padding: '6px',
                          background: actioning === `${cls.class_key}:start` ? '#ccc' : '#28a745',
                          color: 'white',
                          border: 'none',
                          borderRadius: '4px',
                          cursor: actioning === `${cls.class_key}:start` ? 'not-allowed' : 'pointer',
                          fontSize: '12px',
                        }}
                      >
                        {actioning === `${cls.class_key}:start` ? 'Starting...' : 'Start Round'}
                      </button>
                      <button
                        onClick={() => handleCloseRound(cls.class_key)}
                        disabled={actioning === `${cls.class_key}:close`}
                        style={{
                          width: '100%',
                          padding: '6px',
                          background: actioning === `${cls.class_key}:close` ? '#ccc' : '#ffc107',
                          color: '#333',
                          border: 'none',
                          borderRadius: '4px',
                          cursor: actioning === `${cls.class_key}:close` ? 'not-allowed' : 'pointer',
                          fontSize: '12px',
                        }}
                      >
                        {actioning === `${cls.class_key}:close` ? 'Closing...' : 'Close Round'}
                      </button>
                      <button
                        onClick={() => handleReturnClass(cls.class_key)}
                        disabled={actioning === `${cls.class_key}:return`}
                        style={{
                          width: '100%',
                          padding: '6px',
                          background: actioning === `${cls.class_key}:return` ? '#ccc' : '#dc3545',
                          color: 'white',
                          border: 'none',
                          borderRadius: '4px',
                          cursor: actioning === `${cls.class_key}:return` ? 'not-allowed' : 'pointer',
                          fontSize: '12px',
                        }}
                      >
                        {actioning === `${cls.class_key}:return` ? 'Returning...' : 'Return to Operations'}
                      </button>
                    </div>
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

interface MentorGroup {
  mentor_id?: string
  mentor_email?: string
  classes: MentorHeadClass[]
}
