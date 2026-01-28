import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, type StudentSuccessClassDetail } from '../api/client'

type Tab = 'students' | 'absence' | 'followups' | 'feedback'

type StudentRow = StudentSuccessClassDetail['students'][number]


function FeedbackCheckpoint({ classKey, students, onUpdate }: { classKey: string, students: any[], onUpdate: () => void }) {
  const [selected, setSelected] = useState<{ lead_id: string; full_name: string; session_number: number } | null>(null)
  const [viewFeedback, setViewFeedback] = useState<{ student_name: string; session: number; text: string } | null>(null)
  const [feedbackText, setFeedbackText] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  async function handleSubmit() {
    if (!selected || !feedbackText) return
    setIsSubmitting(true)
    try {
      await api.submitFeedback({
        lead_id: selected.lead_id,
        class_key: classKey,
        session_number: selected.session_number,
        feedback_text: feedbackText,
        follow_up_required: false,
      })
      setSelected(null)
      setFeedbackText('')
      onUpdate()
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to submit feedback')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div style={{ background: 'white', borderRadius: '8px', border: '1px solid #dee2e6' }}>
      <div style={{ padding: '16px', borderBottom: '1px solid #eee' }}>
        <h2 style={{ fontSize: '18px', margin: 0 }}>Feedback Checkpoints (Session 4 & 8)</h2>
      </div>
      <div style={{ overflowX: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '14px' }}>
          <thead>
            <tr style={{ textAlign: 'left', background: '#f8f9fa' }}>
              <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Student</th>
              <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Mid-Round (S4)</th>
              <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>End-of-Round (S8)</th>
            </tr>
          </thead>
          <tbody>
            {students.map((s) => (
              <tr key={s.lead_id} style={{ borderBottom: '1px solid #eee' }}>
                <td style={{ padding: '12px' }}>
                  <div style={{ fontWeight: 600 }}>{s.full_name}</div>
                </td>
                <td style={{ padding: '12px' }}>
                  {s.s4 ? (
                    <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                      <span style={{ color: '#155724', background: '#d4edda', padding: '4px 8px', borderRadius: '4px', fontSize: '12px', fontWeight: 600 }}>
                        ✓ SENT
                      </span>
                      <button
                        onClick={() => setViewFeedback({ student_name: s.full_name, session: 4, text: s.s4.feedback_text })}
                        style={{ padding: '4px 8px', borderRadius: '4px', border: '1px solid #6c757d', background: '#fff', color: '#6c757d', fontSize: '11px', cursor: 'pointer' }}
                      >
                        View
                      </button>
                    </div>
                  ) : (
                    <button
                      onClick={() => setSelected({ lead_id: s.lead_id, full_name: s.full_name, session_number: 4 })}
                      style={{ padding: '4px 8px', borderRadius: '4px', border: '1px solid #007bff', background: '#fff', color: '#007bff', fontSize: '12px', cursor: 'pointer' }}
                    >
                      Send S4 Feedback
                    </button>
                  )}
                </td>
                <td style={{ padding: '12px' }}>
                  {s.s8 ? (
                    <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                      <span style={{ color: '#155724', background: '#d4edda', padding: '4px 8px', borderRadius: '4px', fontSize: '12px', fontWeight: 600 }}>
                        ✓ SENT
                      </span>
                      <button
                        onClick={() => setViewFeedback({ student_name: s.full_name, session: 8, text: s.s8.feedback_text })}
                        style={{ padding: '4px 8px', borderRadius: '4px', border: '1px solid #6c757d', background: '#fff', color: '#6c757d', fontSize: '11px', cursor: 'pointer' }}
                      >
                        View
                      </button>
                    </div>
                  ) : (
                    <button
                      onClick={() => setSelected({ lead_id: s.lead_id, full_name: s.full_name, session_number: 8 })}
                      style={{ padding: '4px 8px', borderRadius: '4px', border: '1px solid #007bff', background: '#fff', color: '#007bff', fontSize: '12px', cursor: 'pointer' }}
                    >
                      Send S8 Feedback
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Send Feedback Modal */}
      {selected && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 3000 }}>
          <div style={{ background: 'white', padding: '24px', borderRadius: '12px', width: '400px', maxWidth: '90%' }}>
            <h3 style={{ marginBottom: '16px' }}>Send Session {selected.session_number} Feedback</h3>
            <p style={{ fontSize: '14px', color: '#666', marginBottom: '16px' }}>Student: <strong>{selected.full_name}</strong></p>
            <textarea
              value={feedbackText}
              onChange={(e) => setFeedbackText(e.target.value)}
              placeholder="Enter feedback details..."
              style={{ width: '100%', height: '120px', padding: '12px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px', marginBottom: '16px' }}
            />
            <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
              <button
                onClick={() => { setSelected(null); setFeedbackText(''); }}
                style={{ padding: '8px 16px', borderRadius: '6px', border: '1px solid #ddd', background: '#fff', cursor: 'pointer' }}
              >
                Cancel
              </button>
              <button
                disabled={isSubmitting || !feedbackText}
                onClick={handleSubmit}
                style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: '#007bff', color: '#fff', cursor: 'pointer', opacity: (isSubmitting || !feedbackText) ? 0.6 : 1 }}
              >
                {isSubmitting ? 'Sending...' : 'Send Feedback'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* View Feedback Modal */}
      {viewFeedback && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 3000 }}>
          <div style={{ background: 'white', padding: '24px', borderRadius: '12px', width: '400px', maxWidth: '90%' }}>
            <h3 style={{ marginBottom: '16px' }}>Session {viewFeedback.session} Feedback</h3>
            <p style={{ fontSize: '14px', color: '#666', marginBottom: '16px' }}>Student: <strong>{viewFeedback.student_name}</strong></p>
            <div style={{ background: '#f8f9fa', padding: '16px', borderRadius: '8px', marginBottom: '16px', fontSize: '14px', lineHeight: '1.6', whiteSpace: 'pre-wrap' }}>
              {viewFeedback.text}
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button
                onClick={() => setViewFeedback(null)}
                style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: '#007bff', color: '#fff', cursor: 'pointer' }}
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default function StudentSuccessClass() {
  const [searchParams] = useSearchParams()
  const classKey = searchParams.get('class_key') || ''
  const [data, setData] = useState<StudentSuccessClassDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>('students')
  const [selectedStudent, setSelectedStudent] = useState<StudentRow | null>(null)

  useEffect(() => {
    if (classKey) loadClass()
    else {
      setError('class_key is required')
      setLoading(false)
    }
  }, [classKey])

  async function loadClass() {
    try {
      setLoading(true)
      setError(null)
      const me = await api.getMe()
      if (me.role !== 'student_success') {
        setError('No access. Student Success only.')
        setLoading(false)
        return
      }
      const res = await api.getStudentSuccessClass(classKey)
      setData(res)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load class')
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

  if (error || !data) {
    return (
      <div style={{ padding: '40px' }}>
        <div style={{ background: '#f8d7da', padding: '16px', borderRadius: '8px', color: '#721c24' }}>
          <strong>Error:</strong> {error || 'Class not found'}
        </div>
      </div>
    )
  }

  const c = data.class
  const tabs: { id: Tab; label: string }[] = [
    { id: 'students', label: 'Students' },
    { id: 'absence', label: 'Absence Feed' },
    { id: 'followups', label: 'Follow-ups' },
    { id: 'feedback', label: 'Feedback Checkpoints (Session 4 & 8)' },
  ]

  return (
    <>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <h1>
          Level {c.level} · {c.days} · {c.time} · Class {c.class_number}
        </h1>
      </div>

      <div style={{ display: 'flex', gap: '12px', alignItems: 'center', marginTop: '8px', marginBottom: '16px' }}>
        <span
          style={{
            padding: '4px 12px',
            background: '#d4edda',
            color: '#155724',
            borderRadius: '12px',
            fontSize: '12px',
            fontWeight: 600,
          }}
        >
          ACTIVE · Current Session: {data.completedSessionsCount + 1} · Total: {data.totalSessions}
        </span>
      </div>

      {data.milestones.midRound.reached && !data.milestones.midRound.complete && (
        <div style={{
          background: '#fff3cd',
          border: '1px solid #ffeeba',
          color: '#856404',
          padding: '16px',
          borderRadius: '8px',
          marginBottom: '24px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center'
        }}>
          <div>
            <strong style={{ display: 'block', fontSize: '16px' }}>Mid-Round Feedback Required!</strong>
            <span style={{ fontSize: '14px' }}>Session 4 reached. Please send feedback to all students.</span>
          </div>
          <button
            onClick={() => setTab('feedback')}
            style={{
              background: '#856404',
              color: 'white',
              border: 'none',
              padding: '8px 16px',
              borderRadius: '6px',
              cursor: 'pointer',
              fontWeight: 600
            }}
          >
            Go to Feedbacks
          </button>
        </div>
      )}

      {data.milestones.endRound.reached && !data.milestones.endRound.complete && (
        <div style={{
          background: '#fff3cd',
          border: '1px solid #ffeeba',
          color: '#856404',
          padding: '16px',
          borderRadius: '8px',
          marginBottom: '24px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center'
        }}>
          <div>
            <strong style={{ display: 'block', fontSize: '16px' }}>End-of-Round Feedback Required!</strong>
            <span style={{ fontSize: '14px' }}>Session 8 reached. Please send final feedback to all students.</span>
          </div>
          <button
            onClick={() => setTab('feedback')}
            style={{
              background: '#856404',
              color: 'white',
              border: 'none',
              padding: '8px 16px',
              borderRadius: '6px',
              cursor: 'pointer',
              fontWeight: 600
            }}
          >
            Go to Feedbacks
          </button>
        </div>
      )}

      <div style={{ display: 'flex', gap: '8px', marginBottom: '24px', flexWrap: 'wrap' }}>
        {tabs.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            style={{
              padding: '8px 16px',
              border: `1px solid ${tab === t.id ? '#007bff' : '#dee2e6'}`,
              background: tab === t.id ? '#007bff' : '#fff',
              color: tab === t.id ? '#fff' : '#333',
              borderRadius: '6px',
              cursor: 'pointer',
              fontSize: '14px',
            }}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'students' && (
        <div style={{ display: 'flex', gap: '20px', position: 'relative' }}>
          <div style={{ flex: 1 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
              <h2 style={{ fontSize: '18px', margin: 0 }}>Students</h2>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: '16px' }}>
              {data.students.map((s) => (
                <div
                  key={s.lead_id}
                  onClick={() => setSelectedStudent(s)}
                  style={{
                    background: 'white',
                    padding: '20px',
                    borderRadius: '8px',
                    border: '2px solid #dee2e6',
                    cursor: 'pointer',
                    transition: 'all 0.2s',
                    boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.boxShadow = '0 4px 12px rgba(0,0,0,0.15)'
                    e.currentTarget.style.borderColor = '#007bff'
                    e.currentTarget.style.transform = 'translateY(-2px)'
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.boxShadow = '0 1px 3px rgba(0,0,0,0.1)'
                    e.currentTarget.style.borderColor = '#dee2e6'
                    e.currentTarget.style.transform = 'translateY(0)'
                  }}
                >
                  <h3 style={{ fontSize: '18px', marginBottom: '6px', color: '#333', fontWeight: 600 }}>
                    {s.full_name}
                  </h3>
                  <p style={{ fontSize: '12px', color: '#999', marginBottom: '8px', fontFamily: 'monospace' }}>
                    ID: {s.lead_id.substring(0, 8)}...
                  </p>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                    <span
                      style={{
                        display: 'inline-block',
                        width: 'fit-content',
                        padding: '4px 8px',
                        background: s.missed_count === 0 ? '#d4edda' : s.missed_count <= 2 ? '#fff3cd' : '#f8d7da',
                        color: s.missed_count === 0 ? '#155724' : s.missed_count <= 2 ? '#856404' : '#721c24',
                        borderRadius: '4px',
                        fontSize: '11px',
                        fontWeight: 600,
                      }}
                    >
                      {s.missed_count} missed
                    </span>
                    {s.missed_sessions && s.missed_sessions.length > 0 && (
                      <div style={{ fontSize: '10px', color: '#666' }}>
                        Missed Session {s.missed_sessions.join(', ')}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {tab === 'absence' && (
        <AbsenceFeed classKey={classKey} />
      )}

      {tab === 'followups' && (
        <div style={{ padding: '24px', background: '#f9f9f9', borderRadius: '8px' }}>
          <h2 style={{ fontSize: '18px', marginBottom: '12px' }}>Follow-ups</h2>
          <p style={{ color: '#666' }}>Placeholder. Data fetching coming later.</p>
        </div>
      )}

      {tab === 'feedback' && (
        <FeedbackCheckpoint classKey={classKey} students={data.feedback} onUpdate={loadClass} />
      )}


      {selectedStudent && (
        <StudentSuccessStudentModal
          student={selectedStudent}
          onClose={() => setSelectedStudent(null)}
        />
      )}
    </>
  )
}

function AbsenceFeed({ classKey }: { classKey: string }) {
  const [items, setItems] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState<'all' | 'unresolved' | 'absent' | 'late'>('all')
  const [search, setSearch] = useState('')
  const [followUpModal, setFollowUpModal] = useState<{ open: boolean; item: any | null }>({
    open: false,
    item: null,
  })

  useEffect(() => {
    loadFeed()
  }, [classKey, filter, search])

  async function loadFeed() {
    try {
      setLoading(true)
      const res = await api.getAbsenceFeed(classKey, filter, search)
      setItems(res || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load feed')
    } finally {
      setLoading(false)
    }
  }

  async function handleMarkResolved(id: string) {
    if (!confirm('Mark this absence as resolved?')) return
    try {
      if (id) {
        await api.resolveFollowUp(id)
      }
      await loadFeed()
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to resolve')
    }
  }

  const filteredItems = items // Already filtered by backend

  // Group items by session number
  const groupedItems = filteredItems.reduce((acc: Record<number, any[]>, item) => {
    if (!acc[item.sessionNumber]) {
      acc[item.sessionNumber] = []
    }
    acc[item.sessionNumber].push(item)
    return acc
  }, {})

  const sessionNumbers = Object.keys(groupedItems)
    .map(Number)
    .sort((a, b) => b - a)

  if (loading) return <p style={{ padding: '20px' }}>Loading absence feed...</p>
  if (error) return <p style={{ color: 'red', padding: '20px' }}>{error}</p>

  return (
    <div style={{ background: 'white', borderRadius: '8px', border: '1px solid #dee2e6', overflow: 'hidden' }}>
      <div style={{ padding: '16px', borderBottom: '1px solid #eee', display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '12px' }}>
        <div style={{ display: 'flex', gap: '8px' }}>
          {(['all', 'unresolved', 'absent', 'late'] as const).map((f) => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              style={{
                padding: '4px 12px',
                borderRadius: '4px',
                border: '1px solid #dee2e6',
                background: filter === f ? '#007bff' : '#fff',
                color: filter === f ? '#fff' : '#666',
                fontSize: '12px',
                cursor: 'pointer',
                textTransform: 'capitalize',
              }}
            >
              {f}
            </button>
          ))}
        </div>
        <input
          type="text"
          placeholder="Search name or phone..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{
            padding: '6px 12px',
            borderRadius: '4px',
            border: '1px solid #dee2e6',
            fontSize: '14px',
            width: '200px',
          }}
        />
      </div>

      <div style={{ overflowX: 'auto' }}>
        {sessionNumbers.length === 0 ? (
          <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>
            No absences found matching filters.
          </div>
        ) : (
          sessionNumbers.map((sn) => (
            <div key={sn} style={{ borderBottom: '4px solid #f8f9fa' }}>
              <div style={{ background: '#f8f9fa', padding: '12px 16px', fontSize: '14px', fontWeight: 600, color: '#333', display: 'flex', justifyContent: 'space-between' }}>
                <span>Session {sn}</span>
                <span style={{ fontSize: '11px', color: '#666', fontWeight: 400 }}>{groupedItems[sn].length} absences</span>
              </div>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '14px' }}>
                <thead>
                  <tr style={{ textAlign: 'left', background: 'white', borderBottom: '1px solid #eee' }}>
                    <th style={{ padding: '12px', width: '25%' }}>Student</th>
                    <th style={{ padding: '12px', width: '15%' }}>Status</th>
                    <th style={{ padding: '12px', width: '25%' }}>Marked At</th>
                    <th style={{ padding: '12px', width: '20%' }}>Follow-up</th>
                    <th style={{ padding: '12px', width: '15%' }}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {groupedItems[sn].map((item, idx) => (
                    <tr key={idx} style={{ borderBottom: '1px solid #eee' }}>
                      <td style={{ padding: '12px' }}>
                        <div style={{ fontWeight: 600 }}>{item.studentName}</div>
                        <div style={{ fontSize: '12px', color: '#666' }}>{item.studentPhone}</div>
                      </td>
                      <td style={{ padding: '12px' }}>
                        <span style={{
                          padding: '2px 6px',
                          borderRadius: '4px',
                          fontSize: '11px',
                          fontWeight: 600,
                          background: item.status === 'ABSENT' ? '#f8d7da' : '#fff3cd',
                          color: item.status === 'ABSENT' ? '#721c24' : '#856404',
                        }}>
                          {item.status}
                        </span>
                        {item.mentorNote && (
                          <div style={{ fontSize: '11px', color: '#888', fontStyle: 'italic', marginTop: '4px' }}>
                            "{item.mentorNote}"
                          </div>
                        )}
                      </td>
                      <td style={{ padding: '12px' }}>
                        <div style={{ fontSize: '12px', fontWeight: 500 }}>{item.sessionDate}</div>
                        <div style={{ fontSize: '11px', color: '#999' }}>{new Date(item.markedAt).toLocaleString()}</div>
                        <div style={{ fontSize: '11px', color: '#999' }}>By: {item.markedBy}</div>
                      </td>
                      <td style={{ padding: '12px' }}>
                        {item.followUp ? (
                          <div>
                            <span style={{
                              padding: '2px 6px',
                              borderRadius: '4px',
                              fontSize: '10px',
                              fontWeight: 600,
                              background: item.followUp.status === 'RESOLVED' ? '#d4edda' : '#e2e3e5',
                              color: item.followUp.status === 'RESOLVED' ? '#155724' : '#383d41',
                            }}>
                              {item.followUp.status}
                            </span>
                            {item.followUp.lastNote && (
                              <div style={{ fontSize: '11px', color: '#666', marginTop: '4px' }}>
                                {item.followUp.lastNote}
                              </div>
                            )}
                          </div>
                        ) : (
                          <span style={{ fontSize: '11px', color: '#999' }}>No follow-up yet</span>
                        )}
                      </td>
                      <td style={{ padding: '12px' }}>
                        <div style={{ display: 'flex', gap: '8px' }}>
                          <a
                            href={`https://wa.me/${item.studentPhone.replace(/\D/g, '')}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            title="Open WhatsApp"
                            style={{
                              padding: '4px',
                              borderRadius: '4px',
                              background: '#25D366',
                              color: 'white',
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              width: '24px',
                              height: '24px',
                              textDecoration: 'none'
                            }}
                          >
                            W
                          </a>
                          <button
                            onClick={() => setFollowUpModal({ open: true, item })}
                            title="Add Follow-up Note"
                            style={{
                              padding: '4px 8px',
                              borderRadius: '4px',
                              border: '1px solid #007bff',
                              background: '#fff',
                              color: '#007bff',
                              fontSize: '11px',
                              cursor: 'pointer'
                            }}
                          >
                            Note
                          </button>
                          {item.followUp && (
                            item.followUp.resolved ? (
                              <span
                                style={{
                                  padding: '4px 8px',
                                  borderRadius: '4px',
                                  background: '#d4edda',
                                  color: '#155724',
                                  fontSize: '11px',
                                  fontWeight: 600
                                }}
                              >
                                RESOLVED
                              </span>
                            ) : (
                              <button
                                onClick={() => handleMarkResolved(item.followUp.id)}
                                title="Mark Resolved"
                                style={{
                                  padding: '4px 8px',
                                  borderRadius: '4px',
                                  border: '1px solid #28a745',
                                  background: '#fff',
                                  color: '#28a745',
                                  fontSize: '11px',
                                  cursor: 'pointer'
                                }}
                              >
                                Resolve
                              </button>
                            )
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))
        )}
      </div>

      {followUpModal.open && (
        <div
          style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2000 }}
          onClick={() => setFollowUpModal({ open: false, item: null })}
        >
          <div
            style={{ background: 'white', padding: '24px', borderRadius: '12px', width: '400px', maxWidth: '90%' }}
            onClick={(e) => e.stopPropagation()}
          >
            <h3 style={{ marginBottom: '16px' }}>Add Follow-up Note</h3>
            <p style={{ fontSize: '14px', color: '#666', marginBottom: '16px' }}>
              Student: <strong>{followUpModal.item.studentName}</strong> (S{followUpModal.item.sessionNumber})
            </p>
            <div style={{ marginBottom: '16px' }}>
              <label style={{ display: 'block', fontSize: '12px', color: '#666', marginBottom: '4px' }}>Status</label>
              <select
                id="followup-status"
                defaultValue={followUpModal.item.followUp?.status || 'contacted'}
                style={{ width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid #ddd' }}
              >
                <option value="none">None</option>
                <option value="contacted">Contacted</option>
                <option value="replied">Replied</option>
                <option value="no_response">No Response</option>
              </select>
            </div>
            <div style={{ marginBottom: '16px' }}>
              <label style={{ display: 'block', fontSize: '12px', color: '#666', marginBottom: '4px' }}>Follow-up Note</label>
              <textarea
                id="followup-note"
                defaultValue={followUpModal.item.followUp?.lastNote || ''}
                placeholder="Enter follow-up details..."
                style={{ width: '100%', height: '100px', padding: '12px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px' }}
              />
            </div>
            <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
              <button
                onClick={() => setFollowUpModal({ open: false, item: null })}
                style={{ padding: '8px 16px', borderRadius: '6px', border: '1px solid #ddd', background: '#fff', cursor: 'pointer' }}
              >
                Cancel
              </button>
              <button
                onClick={async () => {
                  const note = (document.getElementById('followup-note') as HTMLTextAreaElement).value
                  const status = (document.getElementById('followup-status') as HTMLSelectElement).value
                  if (!note) return alert('Please enter a note')
                  try {
                    if (followUpModal.item.followUp) {
                      await api.updateFollowUp(followUpModal.item.followUp.id, {
                        status: status,
                        note: note,
                        resolved: false
                      })
                    } else {
                      await api.addFollowUp({
                        class_key: classKey,
                        lead_id: followUpModal.item.studentId,
                        session_number: followUpModal.item.sessionNumber,
                        note,
                        status: status
                      })
                    }
                    setFollowUpModal({ open: false, item: null })
                    loadFeed()
                  } catch (err) {
                    alert(err instanceof Error ? err.message : 'Failed to save note')
                  }
                }}
                style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: '#007bff', color: '#fff', cursor: 'pointer' }}
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function StudentSuccessStudentModal({
  student,
  onClose,
}: {
  student: StudentRow
  onClose: () => void
}) {
  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handleEsc)
    return () => document.removeEventListener('keydown', handleEsc)
  }, [onClose])

  return (
    <div
      role="dialog"
      aria-modal="true"
      onClick={(e) => e.target === e.currentTarget && onClose()}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(0,0,0,0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
      }}
    >
      <div
        style={{
          background: 'white',
          borderRadius: '12px',
          maxWidth: '480px',
          width: '90%',
          maxHeight: '85vh',
          overflow: 'auto',
          boxShadow: '0 8px 32px rgba(0,0,0,0.2)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={{ padding: '20px', borderBottom: '1px solid #eee', display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <div>
            <h2 style={{ fontSize: '20px', marginBottom: '4px' }}>{student.full_name}</h2>
            <p style={{ fontSize: '14px', color: '#666', marginBottom: '4px' }}>{student.phone}</p>
            <p style={{ fontSize: '12px', color: '#999', fontFamily: 'monospace' }}>ID: {student.lead_id}</p>
            <span
              style={{
                display: 'inline-block',
                marginTop: '8px',
                padding: '4px 10px',
                borderRadius: '6px',
                fontSize: '12px',
                fontWeight: 600,
                background: student.missed_count === 0 ? '#d4edda' : student.missed_count <= 2 ? '#fff3cd' : '#f8d7da',
                color: student.missed_count === 0 ? '#155724' : student.missed_count <= 2 ? '#856404' : '#721c24',
              }}
            >
              {student.missed_count} missed
            </span>
          </div>
          <button
            onClick={onClose}
            style={{
              background: 'none',
              border: 'none',
              fontSize: '24px',
              cursor: 'pointer',
              color: '#666',
              padding: '0 4px',
            }}
          >
            X
          </button>
        </div>
        <div style={{ padding: '20px' }}>
          <p style={{ color: '#666', fontSize: '14px' }}>Student details. Notes / Absence Feed / Follow-ups integration coming later.</p>
        </div>
      </div>
    </div>
  )
}

