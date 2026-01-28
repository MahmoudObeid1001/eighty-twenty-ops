import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, ClassDetail, Student } from '../api/client'
import StudentModal from '../components/StudentModal'

export default function ClassWorkspace() {
  const [searchParams] = useSearchParams()
  const classKey = searchParams.get('class_key') || ''
  const [classData, setClassData] = useState<ClassDetail | null>(null)
  const [selectedStudent, setSelectedStudent] = useState<Student | null>(null)
  const [selectedSessionNumber, setSelectedSessionNumber] = useState<number>(1)
  const [loading, setLoading] = useState(true)
  const [updating, setUpdating] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (classKey) {
      loadClass()
    } else {
      setError('class_key is required')
      setLoading(false)
    }
  }, [classKey])

  async function loadClass(silent = false) {
    try {
      if (!silent) setLoading(true)
      setError(null)
      const data = await api.getClassWorkspace(classKey)
      setClassData(data)

      // Set initial selected session to the first scheduled one, or last one
      if (!silent) {
        const nextNotCompleted = data.sessions.find((s) => s.status === 'scheduled')
        if (nextNotCompleted) {
          setSelectedSessionNumber(nextNotCompleted.session_number)
        } else if (data.sessions.length > 0) {
          setSelectedSessionNumber(data.sessions[data.sessions.length - 1].session_number)
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load class')
    } finally {
      if (!silent) setLoading(false)
    }
  }

  async function handleMarkAttendance(sessionId: string, leadId: string, status: string) {
    try {
      setUpdating(`${leadId}-${sessionId}`)
      await api.markAttendance(sessionId, leadId, status, classKey)
      await loadClass(true)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to mark attendance')
    } finally {
      setUpdating(null)
    }
  }

  async function handleCompleteSession(sessionId: string) {
    if (!confirm('Are you sure you want to mark this session as completed?')) return
    try {
      setLoading(true)
      await api.completeSession(sessionId, classKey)
      await loadClass(true)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to complete session')
    } finally {
      setLoading(false)
    }
  }

  if (loading && !classData) {
    return (
      <div style={{ padding: '40px', textAlign: 'center' }}>
        <p>Loading...</p>
      </div>
    )
  }

  if (error || !classData) {
    return (
      <div style={{ padding: '40px' }}>
        <div style={{ background: '#fee', padding: '16px', borderRadius: '8px', color: '#c33' }}>
          <strong>Error:</strong> {error || 'Class not found'}
        </div>
      </div>
    )
  }

  const selectedSession = classData.sessions.find((s) => s.session_number === selectedSessionNumber)

  return (
    <>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <h1>
          Level {classData.class.level} · {classData.class.days} · {classData.class.time} · Class {classData.class.class_number}
        </h1>
      </div>

      <div style={{ marginBottom: '24px' }}>
        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', background: '#f8f9fa', padding: '12px', borderRadius: '12px' }}>
          {classData.sessions.map((s) => {
            const isSelected = s.session_number === selectedSessionNumber
            const statusColor = s.status === 'completed' ? '#28a745' : s.status === 'scheduled' ? '#007bff' : '#6c757d'
            return (
              <button
                key={s.id}
                onClick={() => setSelectedSessionNumber(s.session_number)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  padding: '8px 16px',
                  borderRadius: '8px',
                  border: isSelected ? 'none' : `2px solid ${statusColor}`,
                  background: isSelected ? statusColor : 'white',
                  color: isSelected ? 'white' : statusColor,
                  fontWeight: 600,
                  cursor: 'pointer',
                  transition: 'all 0.2s',
                  boxShadow: isSelected ? '0 2px 4px rgba(0,0,0,0.1)' : 'none',
                }}
              >
                S{s.session_number}
                <span style={{ fontSize: '10px', textTransform: 'uppercase', opacity: 0.8 }}>{s.status}</span>
              </button>
            )
          })}
        </div>
      </div>

      {selectedSession && selectedSession.status === 'scheduled' && (
        <div style={{ marginBottom: '24px' }}>
          <button
            onClick={() => handleCompleteSession(selectedSession.id)}
            style={{
              padding: '10px 20px',
              background: '#28a745',
              color: 'white',
              border: 'none',
              borderRadius: '8px',
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            ✓ Complete Session {selectedSession.session_number}
          </button>
        </div>
      )}

      <div style={{ display: 'flex', gap: '20px', position: 'relative' }}>
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: '18px', marginBottom: '16px' }}>Students</h2>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: '16px' }}>
            {classData.students.map((student) => {
              const status = selectedSession ? student.attendance?.[selectedSession.id] : undefined
              const isUpdating = updating === `${student.lead_id}-${selectedSession?.id}`

              return (
                <div
                  key={student.lead_id}
                  style={{
                    background: 'white',
                    padding: '20px',
                    borderRadius: '12px',
                    border: '2px solid #dee2e6',
                    transition: 'all 0.2s',
                    boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start', marginBottom: '12px' }}>
                    <div onClick={() => setSelectedStudent(student)} style={{ cursor: 'pointer', flex: 1 }}>
                      <h3 style={{ fontSize: '17px', marginBottom: '4px', color: '#333', fontWeight: 600 }}>
                        {student.full_name}
                      </h3>
                      <p style={{ fontSize: '12px', color: '#666', marginBottom: '0' }}>
                        {student.phone}
                      </p>
                    </div>
                    {student.missed_count !== undefined && (
                      <span
                        style={{
                          padding: '4px 8px',
                          background: student.missed_count === 0 ? '#d4edda' : student.missed_count <= 2 ? '#fff3cd' : '#f8d7da',
                          color: student.missed_count === 0 ? '#155724' : student.missed_count <= 2 ? '#856404' : '#721c24',
                          borderRadius: '12px',
                          fontSize: '11px',
                          fontWeight: 600,
                        }}
                      >
                        {student.missed_count} missed
                      </span>
                    )}
                  </div>

                  {selectedSession ? (
                    <div style={{ background: '#f8f9fa', padding: '12px', borderRadius: '8px', opacity: isUpdating ? 0.6 : 1 }}>
                      <div style={{ fontSize: '12px', color: '#666', marginBottom: '8px' }}>
                        Session {selectedSession.session_number} Attendance
                      </div>
                      <div style={{ display: 'flex', gap: '8px' }}>
                        <button
                          disabled={isUpdating}
                          onClick={() => handleMarkAttendance(selectedSession.id, student.lead_id, 'PRESENT')}
                          style={{
                            flex: 1,
                            padding: '8px',
                            borderRadius: '6px',
                            border: 'none',
                            background: status === 'PRESENT' ? '#28a745' : '#e9ecef',
                            color: status === 'PRESENT' ? 'white' : '#666',
                            fontWeight: 600,
                            cursor: 'pointer',
                            fontSize: '13px',
                          }}
                        >
                          Present
                        </button>
                        <button
                          disabled={isUpdating}
                          onClick={() => handleMarkAttendance(selectedSession.id, student.lead_id, 'ABSENT')}
                          style={{
                            flex: 1,
                            padding: '8px',
                            borderRadius: '6px',
                            border: 'none',
                            background: status === 'ABSENT' ? '#dc3545' : '#e9ecef',
                            color: status === 'ABSENT' ? 'white' : '#666',
                            fontWeight: 600,
                            cursor: 'pointer',
                            fontSize: '13px',
                          }}
                        >
                          Absent
                        </button>
                      </div>
                    </div>
                  ) : (
                    <div style={{ background: '#fff3cd', padding: '12px', borderRadius: '8px', fontSize: '12px', color: '#856404' }}>
                      No session selected or available.
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </div>
      </div>

      {selectedStudent && (
        <StudentModal
          student={selectedStudent}
          classKey={classKey}
          sessionsCount={classData.sessionsCount}
          onClose={() => setSelectedStudent(null)}
        />
      )}
    </>
  )
}

