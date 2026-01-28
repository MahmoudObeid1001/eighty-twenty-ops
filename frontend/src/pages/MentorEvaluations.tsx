import { useEffect, useState } from 'react'
import { api } from '../api/client'

interface MentorKPI {
  id: string
  name: string
  email: string
  assignedClassCount: number
  kpis: {
    sessionQuality: number
    trelloCompliance: number
    whatsappManagement: number
    studentsFeedback: number
  }
  attendance: {
    sessionsTotal: number
    statuses: string[]
    onTimePercent: number
  }
}

function getStatusColor(status: string): string {
  switch (status) {
    case 'on-time':
      return '#28a745' // green
    case 'late':
      return '#ffc107' // yellow
    case 'absent':
      return '#dc3545' // red
    case 'unknown':
    default:
      return '#6c757d' // gray
  }
}

function getStatusLabel(status: string): string {
  switch (status) {
    case 'on-time':
      return 'On time'
    case 'late':
      return 'Late'
    case 'absent':
      return 'Absent'
    case 'unknown':
    default:
      return 'Unknown'
  }
}

export default function MentorEvaluations() {
  const [mentors, setMentors] = useState<MentorKPI[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [editingMentor, setEditingMentor] = useState<MentorKPI | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    loadMentors()
  }, [])

  async function loadMentors() {
    try {
      setLoading(true)
      setError(null)
      const data = await api.getMentorEvaluations()
      setMentors(data.mentors)
    } catch (err) {
      if (err instanceof Error) {
        if (err.message.includes('401') || err.message.includes('403')) {
          setError('No access')
        } else {
          setError('Failed to load evaluations')
        }
      } else {
        setError('Failed to load evaluations')
      }
    } finally {
      setLoading(false)
    }
  }

  if (loading) {
    return (
      <div style={{ padding: '24px' }}>
        <h1 style={{ marginBottom: '24px', fontSize: '24px', fontWeight: 600 }}>Mentor Evaluations</h1>
        <p>Loading...</p>
      </div>
    )
  }

  if (error) {
    return (
      <div style={{ padding: '24px' }}>
        <h1 style={{ marginBottom: '24px', fontSize: '24px', fontWeight: 600 }}>Mentor Evaluations</h1>
        <div style={{ padding: '16px', background: '#f8d7da', color: '#721c24', borderRadius: '4px', border: '1px solid #f5c6cb' }}>
          {error}
        </div>
      </div>
    )
  }

  return (
    <div style={{ padding: '24px' }}>
      <h1 style={{ marginBottom: '24px', fontSize: '24px', fontWeight: 600 }}>Mentor Evaluations</h1>

      {mentors.length === 0 ? (
        <p style={{ color: '#666' }}>No mentors assigned to classes.</p>
      ) : (
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(400px, 1fr))',
            gap: '20px',
          }}
        >
          {mentors.map((mentor) => (
            <div
              key={mentor.id}
              style={{
                background: 'white',
                border: '1px solid #dee2e6',
                borderRadius: '8px',
                padding: '20px',
                boxShadow: '0 2px 4px rgba(0,0,0,0.1)',
              }}
            >
              {/* Header */}
              <div style={{ marginBottom: '16px', borderBottom: '1px solid #dee2e6', paddingBottom: '12px' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                  <div style={{ flex: 1 }}>
                    <h2 style={{ margin: 0, fontSize: '18px', fontWeight: 600, marginBottom: '4px' }}>
                      {mentor.name}
                    </h2>
                    <p style={{ margin: 0, fontSize: '14px', color: '#666' }}>{mentor.email}</p>
                    <span
                      style={{
                        display: 'inline-block',
                        marginTop: '8px',
                        padding: '4px 8px',
                        background: '#d4edda',
                        color: '#155724',
                        borderRadius: '4px',
                        fontSize: '12px',
                        fontWeight: 600,
                      }}
                    >
                      {mentor.assignedClassCount} {mentor.assignedClassCount === 1 ? 'class' : 'classes'}
                    </span>
                  </div>
                  <button
                    onClick={() => setEditingMentor(mentor)}
                    style={{
                      padding: '6px 12px',
                      background: '#007bff',
                      color: 'white',
                      border: 'none',
                      borderRadius: '4px',
                      cursor: 'pointer',
                      fontSize: '13px',
                      fontWeight: 500,
                    }}
                  >
                    Edit
                  </button>
                </div>
              </div>

              {/* KPI Metrics */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {/* Session Quality */}
                <div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px' }}>
                    <span style={{ fontSize: '14px', fontWeight: 500 }}>Session Quality</span>
                    <span style={{ fontSize: '14px', fontWeight: 600 }}>{mentor.kpis.sessionQuality}%</span>
                  </div>
                  <div
                    style={{
                      width: '100%',
                      height: '8px',
                      background: '#e9ecef',
                      borderRadius: '4px',
                      overflow: 'hidden',
                    }}
                  >
                    <div
                      style={{
                        width: `${mentor.kpis.sessionQuality}%`,
                        height: '100%',
                        background: mentor.kpis.sessionQuality >= 80 ? '#28a745' : mentor.kpis.sessionQuality >= 60 ? '#ffc107' : '#dc3545',
                        transition: 'width 0.3s',
                      }}
                    />
                  </div>
                </div>

                {/* Trello Compliance */}
                <div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px' }}>
                    <span style={{ fontSize: '14px', fontWeight: 500 }}>Trello Compliance</span>
                    <span style={{ fontSize: '14px', fontWeight: 600 }}>{mentor.kpis.trelloCompliance}%</span>
                  </div>
                  <div
                    style={{
                      width: '100%',
                      height: '8px',
                      background: '#e9ecef',
                      borderRadius: '4px',
                      overflow: 'hidden',
                    }}
                  >
                    <div
                      style={{
                        width: `${mentor.kpis.trelloCompliance}%`,
                        height: '100%',
                        background: mentor.kpis.trelloCompliance >= 80 ? '#28a745' : mentor.kpis.trelloCompliance >= 60 ? '#ffc107' : '#dc3545',
                        transition: 'width 0.3s',
                      }}
                    />
                  </div>
                </div>

                {/* WhatsApp Management */}
                <div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px' }}>
                    <span style={{ fontSize: '14px', fontWeight: 500 }}>WhatsApp Groups Management</span>
                    <span style={{ fontSize: '14px', fontWeight: 600 }}>{mentor.kpis.whatsappManagement}%</span>
                  </div>
                  <div
                    style={{
                      width: '100%',
                      height: '8px',
                      background: '#e9ecef',
                      borderRadius: '4px',
                      overflow: 'hidden',
                    }}
                  >
                    <div
                      style={{
                        width: `${mentor.kpis.whatsappManagement}%`,
                        height: '100%',
                        background: mentor.kpis.whatsappManagement >= 80 ? '#28a745' : mentor.kpis.whatsappManagement >= 60 ? '#ffc107' : '#dc3545',
                        transition: 'width 0.3s',
                      }}
                    />
                  </div>
                </div>

                {/* Attendance Punctuality */}
                <div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '8px' }}>
                    <span style={{ fontSize: '14px', fontWeight: 500 }}>Attendance Punctuality</span>
                    <span style={{ fontSize: '14px', fontWeight: 600 }}>{mentor.attendance.onTimePercent}%</span>
                  </div>
                  <div style={{ display: 'flex', gap: '6px', marginBottom: '4px', flexWrap: 'wrap' }}>
                    {mentor.attendance.statuses.map((status, index) => (
                      <div
                        key={index}
                        title={`Session ${index + 1}: ${getStatusLabel(status)}`}
                        style={{
                          width: '32px',
                          height: '32px',
                          borderRadius: '50%',
                          background: getStatusColor(status),
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontSize: '11px',
                          fontWeight: 600,
                          color: 'white',
                          cursor: 'pointer',
                          border: '2px solid white',
                          boxShadow: '0 1px 2px rgba(0,0,0,0.1)',
                        }}
                      >
                        {index + 1}
                      </div>
                    ))}
                  </div>
                  <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
                    Overall: {mentor.attendance.onTimePercent}% on time
                  </div>
                </div>

                {/* Students Feedback */}
                <div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px' }}>
                    <span style={{ fontSize: '14px', fontWeight: 500 }}>Students Feedback</span>
                    <span style={{ fontSize: '14px', fontWeight: 600 }}>{mentor.kpis.studentsFeedback}%</span>
                  </div>
                  <div
                    style={{
                      width: '100%',
                      height: '8px',
                      background: '#e9ecef',
                      borderRadius: '4px',
                      overflow: 'hidden',
                    }}
                  >
                    <div
                      style={{
                        width: `${mentor.kpis.studentsFeedback}%`,
                        height: '100%',
                        background: mentor.kpis.studentsFeedback >= 80 ? '#28a745' : mentor.kpis.studentsFeedback >= 60 ? '#ffc107' : '#dc3545',
                        transition: 'width 0.3s',
                      }}
                    />
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Edit Modal */}
      {editingMentor && (
        <EditMentorModal
          mentor={editingMentor}
          onClose={() => {
            setEditingMentor(null)
            setSaveError(null)
          }}
          onSave={async (updated) => {
            try {
              setSaving(true)
              setSaveError(null)
              await api.updateMentorEvaluation(editingMentor.id, {
                kpis: updated.kpis,
                attendance: { statuses: updated.attendance.statuses },
              })
              // Update local state
              setMentors((prev) =>
                prev.map((m) =>
                  m.id === editingMentor.id
                    ? {
                        ...m,
                        kpis: updated.kpis,
                        attendance: {
                          ...m.attendance,
                          statuses: updated.attendance.statuses,
                          onTimePercent: updated.attendance.onTimePercent,
                        },
                      }
                    : m
                )
              )
              setEditingMentor(null)
            } catch (err) {
              if (err instanceof Error) {
                if (err.message.includes('403')) {
                  setSaveError('No access')
                } else {
                  setSaveError(err.message || 'Failed to save evaluation')
                }
              } else {
                setSaveError('Failed to save evaluation')
              }
            } finally {
              setSaving(false)
            }
          }}
          saving={saving}
          error={saveError}
        />
      )}
    </div>
  )
}

interface EditMentorModalProps {
  mentor: MentorKPI
  onClose: () => void
  onSave: (updated: MentorKPI) => void
  saving: boolean
  error: string | null
}

function EditMentorModal({ mentor, onClose, onSave, saving, error }: EditMentorModalProps) {
  const [kpis, setKPIs] = useState(mentor.kpis)
  const [statuses, setStatuses] = useState([...mentor.attendance.statuses])

  // Compute on-time percent
  const onTimeCount = statuses.filter((s) => s === 'on-time').length
  const onTimePercent = (onTimeCount * 100) / 8

  function handleSave() {
    onSave({
      ...mentor,
      kpis,
      attendance: {
        ...mentor.attendance,
        statuses,
        onTimePercent,
      },
    })
  }

  return (
    <>
      {/* Backdrop */}
      <div
        onClick={onClose}
        style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'rgba(0, 0, 0, 0.5)',
          zIndex: 1000,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        {/* Modal */}
        <div
          onClick={(e) => e.stopPropagation()}
          style={{
            background: 'white',
            borderRadius: '8px',
            width: '90%',
            maxWidth: '600px',
            maxHeight: '90vh',
            overflow: 'auto',
            padding: '24px',
            boxShadow: '0 4px 6px rgba(0, 0, 0, 0.1)',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
            <h2 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>Edit Evaluation: {mentor.name}</h2>
            <button
              onClick={onClose}
              style={{
                background: 'none',
                border: 'none',
                fontSize: '24px',
                cursor: 'pointer',
                color: '#666',
                padding: 0,
                width: '32px',
                height: '32px',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              ×
            </button>
          </div>

          {error && (
            <div style={{ padding: '12px', background: '#f8d7da', color: '#721c24', borderRadius: '4px', marginBottom: '16px' }}>
              {error}
            </div>
          )}

          {/* KPI Fields */}
          <div style={{ marginBottom: '24px' }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, marginBottom: '12px' }}>KPIs (0-100)</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              {/* Session Quality */}
              <div>
                <label style={{ display: 'block', marginBottom: '6px', fontSize: '14px', fontWeight: 500 }}>
                  Session Quality: {kpis.sessionQuality}%
                </label>
                <input
                  type="range"
                  min="0"
                  max="100"
                  value={kpis.sessionQuality}
                  onChange={(e) => setKPIs({ ...kpis, sessionQuality: parseInt(e.target.value) })}
                  style={{ width: '100%' }}
                />
              </div>

              {/* Trello Compliance */}
              <div>
                <label style={{ display: 'block', marginBottom: '6px', fontSize: '14px', fontWeight: 500 }}>
                  Trello Compliance: {kpis.trelloCompliance}%
                </label>
                <input
                  type="range"
                  min="0"
                  max="100"
                  value={kpis.trelloCompliance}
                  onChange={(e) => setKPIs({ ...kpis, trelloCompliance: parseInt(e.target.value) })}
                  style={{ width: '100%' }}
                />
              </div>

              {/* WhatsApp Management */}
              <div>
                <label style={{ display: 'block', marginBottom: '6px', fontSize: '14px', fontWeight: 500 }}>
                  WhatsApp Groups Management: {kpis.whatsappManagement}%
                </label>
                <input
                  type="range"
                  min="0"
                  max="100"
                  value={kpis.whatsappManagement}
                  onChange={(e) => setKPIs({ ...kpis, whatsappManagement: parseInt(e.target.value) })}
                  style={{ width: '100%' }}
                />
              </div>

              {/* Students Feedback */}
              <div>
                <label style={{ display: 'block', marginBottom: '6px', fontSize: '14px', fontWeight: 500 }}>
                  Students Feedback: {kpis.studentsFeedback}%
                </label>
                <input
                  type="range"
                  min="0"
                  max="100"
                  value={kpis.studentsFeedback}
                  onChange={(e) => setKPIs({ ...kpis, studentsFeedback: parseInt(e.target.value) })}
                  style={{ width: '100%' }}
                />
              </div>
            </div>
          </div>

          {/* Attendance */}
          <div style={{ marginBottom: '24px' }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, marginBottom: '12px' }}>
              Attendance Punctuality: {onTimePercent}% on time
            </h3>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px' }}>
              {statuses.map((status, index) => (
                <div key={index}>
                  <label style={{ display: 'block', marginBottom: '4px', fontSize: '13px', fontWeight: 500 }}>
                    Session {index + 1}
                  </label>
                  <select
                    value={status}
                    onChange={(e) => {
                      const newStatuses = [...statuses]
                      newStatuses[index] = e.target.value
                      setStatuses(newStatuses)
                    }}
                    style={{
                      width: '100%',
                      padding: '6px',
                      border: '1px solid #ddd',
                      borderRadius: '4px',
                      fontSize: '13px',
                    }}
                  >
                    <option value="unknown">Unknown</option>
                    <option value="on-time">On time</option>
                    <option value="late">Late</option>
                    <option value="absent">Absent</option>
                  </select>
                </div>
              ))}
            </div>
          </div>

          {/* Actions */}
          <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
            <button
              onClick={onClose}
              disabled={saving}
              style={{
                padding: '8px 16px',
                background: '#6c757d',
                color: 'white',
                border: 'none',
                borderRadius: '4px',
                cursor: saving ? 'not-allowed' : 'pointer',
                fontSize: '14px',
                opacity: saving ? 0.6 : 1,
              }}
            >
              Cancel
            </button>
            <button
              onClick={handleSave}
              disabled={saving}
              style={{
                padding: '8px 16px',
                background: saving ? '#6c757d' : '#007bff',
                color: 'white',
                border: 'none',
                borderRadius: '4px',
                cursor: saving ? 'not-allowed' : 'pointer',
                fontSize: '14px',
                fontWeight: 500,
              }}
            >
              {saving ? 'Saving...' : 'Save'}
            </button>
          </div>
        </div>
      </div>
    </>
  )
}
