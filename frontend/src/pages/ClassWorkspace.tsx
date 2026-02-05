import { useEffect, useMemo, useState } from 'react'
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
  const [actionError, setActionError] = useState<string | null>(null)
  const [confirmSessionId, setConfirmSessionId] = useState<string | null>(null)
  const [confirmSessionNumber, setConfirmSessionNumber] = useState<number | null>(null)

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
      setActionError(null)
      await api.markAttendance(sessionId, leadId, status, classKey)
      await loadClass(true)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to mark attendance')
    } finally {
      setUpdating(null)
    }
  }

  function handleCompleteSession(sessionId: string, sessionNumber: number) {
    setConfirmSessionId(sessionId)
    setConfirmSessionNumber(sessionNumber)
  }

  async function confirmCompleteSession() {
    if (!confirmSessionId) return
    try {
      setLoading(true)
      setActionError(null)
      await api.completeSession(confirmSessionId, classKey)
      await loadClass(true)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to complete session')
    } finally {
      setLoading(false)
      setConfirmSessionId(null)
      setConfirmSessionNumber(null)
    }
  }

  const overdueSessions = useMemo(() => {
    if (!classData) return []
    const now = new Date()
    return classData.sessions.filter((s) => {
      if (s.status === 'completed') return false
      if (!s.scheduled_time) return false

      const sessionDateTime = new Date(`${s.scheduled_date}T${s.scheduled_time}`)
      sessionDateTime.setHours(sessionDateTime.getHours() + 2)
      const diffHours = (now.getTime() - sessionDateTime.getTime()) / (1000 * 60 * 60)

      if (diffHours > 24) {
        return classData.students.some((student) => !student.attendance?.[s.id])
      }
      return false
    })
  }, [classData])

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
          {classData.class.round_status === 'closed' && (
            <span
              style={{
                marginLeft: '12px',
                padding: '4px 8px',
                background: '#6c757d',
                color: 'white',
                borderRadius: '4px',
                fontSize: '12px',
                verticalAlign: 'middle',
                textTransform: 'uppercase',
                fontWeight: 600,
              }}
            >
              Archived
            </span>
          )}
        </h1>
      </div>

      {actionError && (
        <div
          style={{
            background: '#f8d7da',
            color: '#721c24',
            padding: '12px 20px',
            borderRadius: '8px',
            marginBottom: '20px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: '12px',
            boxShadow: '0 2px 4px rgba(0,0,0,0.1)',
          }}
        >
          <span>{actionError}</span>
          <button onClick={() => setActionError(null)} style={{ background: 'none', border: 'none', fontSize: '20px', cursor: 'pointer', color: '#721c24' }}>
            ×
          </button>
        </div>
      )}

      {overdueSessions.length > 0 && (
        <div
          style={{
            background: '#dc3545',
            color: 'white',
            padding: '12px 20px',
            borderRadius: '8px',
            marginBottom: '20px',
            display: 'flex',
            alignItems: 'center',
            gap: '12px',
            boxShadow: '0 2px 4px rgba(0,0,0,0.1)',
            fontWeight: 600,
          }}
        >
          <span style={{ fontSize: '20px' }}>⚠️</span>
          <span>
            Attention: Attendance is missing for {overdueSessions.length} session(s) that took place more than 24 hours ago. Please mark attendance for:{' '}
            {overdueSessions.map((s) => `S${s.session_number}`).join(', ')}.
          </span>
        </div>
      )}

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

      {selectedSession && selectedSession.status === 'scheduled' && classData.class.round_status !== 'closed' && (
        <div style={{ marginBottom: '24px' }}>
          <button
            onClick={() => handleCompleteSession(selectedSession.id, selectedSession.session_number)}
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
                      <h3 style={{ fontSize: '17px', marginBottom: '4px', color: '#333', fontWeight: 600, display: 'flex', alignItems: 'center', gap: '8px' }}>
                        {student.full_name}
                        {student.joined_at_session_number && (
                          <span
                            style={{
                              background: '#6c5ce7',
                              color: 'white',
                              padding: '2px 6px',
                              borderRadius: '4px',
                              fontSize: '10px',
                            }}
                          >
                            Late Join (S{student.joined_at_session_number})
                          </span>
                        )}
                      </h3>
                      <p style={{ fontSize: '12px', color: '#666', marginBottom: '0' }}>{student.phone}</p>
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
                      <div style={{ fontSize: '12px', color: '#666', marginBottom: '8px' }}>Session {selectedSession.session_number} Attendance</div>
                      <div style={{ display: 'flex', gap: '8px' }}>
                        <button
                          disabled={isUpdating || classData.class.round_status === 'closed'}
                          onClick={() => handleMarkAttendance(selectedSession.id, student.lead_id, 'PRESENT')}
                          style={{
                            flex: 1,
                            padding: '8px',
                            borderRadius: '6px',
                            border: 'none',
                            background: status === 'PRESENT' ? '#28a745' : '#e9ecef',
                            color: status === 'PRESENT' ? 'white' : '#666',
                            fontWeight: 600,
                            cursor: classData.class.round_status === 'closed' ? 'not-allowed' : 'pointer',
                            fontSize: '13px',
                            opacity: classData.class.round_status === 'closed' ? 0.7 : 1,
                          }}
                        >
                          Present
                        </button>
                        <button
                          disabled={isUpdating || classData.class.round_status === 'closed'}
                          onClick={() => handleMarkAttendance(selectedSession.id, student.lead_id, 'ABSENT')}
                          style={{
                            flex: 1,
                            padding: '8px',
                            borderRadius: '6px',
                            border: 'none',
                            background: status === 'ABSENT' ? '#dc3545' : '#e9ecef',
                            color: status === 'ABSENT' ? 'white' : '#666',
                            fontWeight: 600,
                            cursor: classData.class.round_status === 'closed' ? 'not-allowed' : 'pointer',
                            fontSize: '13px',
                            opacity: classData.class.round_status === 'closed' ? 0.7 : 1,
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

      {confirmSessionId && (
        <div
          onClick={() => {
            if (loading) return
            setConfirmSessionId(null)
            setConfirmSessionNumber(null)
          }}
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(0,0,0,0.45)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 3000,
            padding: '16px',
          }}
        >
          <div
            onClick={(e) => e.stopPropagation()}
            style={{
              background: 'white',
              borderRadius: '12px',
              padding: '24px',
              width: '420px',
              maxWidth: '100%',
              boxShadow: '0 12px 30px rgba(0,0,0,0.2)',
            }}
          >
            <h3 style={{ marginTop: 0, marginBottom: '8px' }}>Complete session?</h3>
            <p style={{ marginTop: 0, color: '#555', marginBottom: '20px' }}>
              You are about to mark session {confirmSessionNumber ?? ''} as completed. This cannot be undone.
            </p>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px' }}>
              <button
                onClick={() => {
                  setConfirmSessionId(null)
                  setConfirmSessionNumber(null)
                }}
                disabled={loading}
                style={{
                  padding: '8px 14px',
                  borderRadius: '8px',
                  border: '1px solid #ccc',
                  background: '#fff',
                  cursor: loading ? 'not-allowed' : 'pointer',
                }}
              >
                Cancel
              </button>
              <button
                onClick={confirmCompleteSession}
                disabled={loading}
                style={{
                  padding: '8px 14px',
                  borderRadius: '8px',
                  border: 'none',
                  background: '#28a745',
                  color: 'white',
                  fontWeight: 600,
                  cursor: loading ? 'not-allowed' : 'pointer',
                }}
              >
                Yes, complete
              </button>
            </div>
          </div>
        </div>
      )}

      {selectedStudent && (
        <StudentModal
          student={selectedStudent}
          classKey={classKey}
          sessionsCount={classData.sessionsCount}
          totalSessions={classData.totalSessions}
          attendedCount={
            selectedStudent.attendance
              ? Object.values(selectedStudent.attendance).filter(
                  (status) => status === 'PRESENT' || status === 'LATE' || status === 'on-time' || status === 'late' || status === 'present'
                ).length
              : 0
          }
          onClose={() => setSelectedStudent(null)}
        />
      )}
    </>
  )
}
