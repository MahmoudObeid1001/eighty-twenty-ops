import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api, type StudentReportCardData, type StudentSuccessClassDetail } from '../api/client'
import FeedbackCollectedTab from '../components/FeedbackCollectedTab'
import StudentModal from '../components/StudentModal'
import ComplianceModal from '../components/ComplianceModal'
import StudentReportCard from '../components/StudentReportCard'
import GradeNotesModal from '../components/GradeNotesModal'
import { buildWhatsAppLink, openWhatsAppLink } from '../utils/whatsapp'

type Tab = 'students' | 'absence' | 'followups' | 'feedback' | 'feedback_collected' | 'final_grading'

type StudentRow = StudentSuccessClassDetail['students'][number]

type ConfirmDialogState = {
  open: boolean
  title: string
  body?: string
  confirmLabel?: string
  cancelLabel?: string
  tone?: 'primary' | 'success' | 'danger'
  onConfirm?: () => void | Promise<void>
}

function ConfirmDialog({ open, title, body, confirmLabel = 'Confirm', cancelLabel = 'Cancel', tone = 'primary', onConfirm, onCancel }: {
  open: boolean
  title: string
  body?: string
  confirmLabel?: string
  cancelLabel?: string
  tone?: 'primary' | 'success' | 'danger'
  onConfirm?: () => void | Promise<void>
  onCancel: () => void
}) {
  if (!open) return null

  const toneColor = tone === 'success' ? '#28a745' : tone === 'danger' ? '#dc3545' : '#007bff'

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 3000 }} onClick={onCancel}>
      <div style={{ background: 'white', padding: '20px', borderRadius: '10px', width: '420px', maxWidth: '90%' }} onClick={(e) => e.stopPropagation()}>
        <h3 style={{ margin: 0, marginBottom: '12px' }}>{title}</h3>
        {body && <p style={{ margin: 0, marginBottom: '16px', color: '#555', fontSize: '14px' }}>{body}</p>}
        <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
          <button onClick={onCancel} style={{ padding: '8px 16px', borderRadius: '6px', border: '1px solid #ddd', background: '#fff', cursor: 'pointer' }}>
            {cancelLabel}
          </button>
          <button onClick={onConfirm} style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: toneColor, color: '#fff', cursor: 'pointer', fontWeight: 600 }}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}

function FeedbackCheckpoint({ classKey, students, onUpdate, canEdit }: { classKey: string; students: any[]; onUpdate: () => void; canEdit: boolean }) {
  const [selected, setSelected] = useState<{ lead_id: string; full_name: string; session_number: number } | null>(null)
  const [feedbackText, setFeedbackText] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [modalError, setModalError] = useState<string | null>(null)
  const [optimisticStatus, setOptimisticStatus] = useState<Record<string, boolean>>({})

  useEffect(() => {
    setOptimisticStatus({})
  }, [students])

  async function handleSubmit() {
    if (!canEdit) return
    if (!selected || !feedbackText) return
    setIsSubmitting(true)
    setModalError(null)
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
      setModalError(err instanceof Error ? err.message : 'Failed to submit feedback')
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleStatusUpdate = async (leadId: string, session: number, status: 'received' | 'removed') => {
    if (!canEdit) return
    const key = `${leadId}-${session}`
    setOptimisticStatus((prev) => ({ ...prev, [key]: true }))
    setError(null)
    try {
      await api.updateFeedbackStatus(leadId, classKey, session, status)
      onUpdate()
    } catch (err) {
      setError(`Failed to update status: ${err instanceof Error ? err.message : 'Unknown error'}`)
      setOptimisticStatus((prev) => {
        const next = { ...prev }
        delete next[key]
        return next
      })
    }
  }

  const filteredStudents = students.filter((s) => {
    const s4Handled = (s.s4 && s.s4.status !== 'sent') || optimisticStatus[`${s.lead_id}-4`]
    const s8Handled = (s.s8 && s.s8.status !== 'sent') || optimisticStatus[`${s.lead_id}-8`]
    return !s4Handled || !s8Handled
  })

  return (
    <div style={{ background: 'white', borderRadius: '8px', border: '1px solid #dee2e6' }}>
      <div style={{ padding: '16px', borderBottom: '1px solid #eee' }}>
        <h2 style={{ fontSize: '18px', margin: 0 }}>Feedback Checkpoints (Session 4 & 8)</h2>
      </div>
      {error && (
        <div style={{ padding: '12px 16px', background: '#f8d7da', color: '#721c24', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{error}</span>
          <button onClick={() => setError(null)} style={{ background: 'none', border: 'none', fontSize: '18px', cursor: 'pointer', color: '#721c24' }}>
            ×
          </button>
        </div>
      )}
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
            {filteredStudents.map((s) => {
              const s4Handled = (s.s4 && s.s4.status !== 'sent') || optimisticStatus[`${s.lead_id}-4`]
              const s8Handled = (s.s8 && s.s8.status !== 'sent') || optimisticStatus[`${s.lead_id}-8`]

              return (
                <tr key={s.lead_id} style={{ borderBottom: '1px solid #eee' }}>
                  <td style={{ padding: '12px' }}>
                    <div style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '8px' }}>
                      {s.full_name}
                      {s.phone && (
                        <a
                          href={buildWhatsAppLink(s.phone)}
                          target="admin-whatsapp-chat"
                          onClick={(event) => {
                            event.preventDefault()
                            if (!openWhatsAppLink(buildWhatsAppLink(s.phone))) {
                              window.location.href = buildWhatsAppLink(s.phone)
                            }
                          }}
                          title="Open WhatsApp"
                          aria-label={`Open WhatsApp chat for ${s.full_name}`}
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            width: '28px',
                            height: '28px',
                            borderRadius: '999px',
                            background: '#25D366',
                            color: 'white',
                            textDecoration: 'none',
                            boxShadow: '0 2px 8px rgba(37, 211, 102, 0.28)',
                          }}
                        >
                          <WhatsAppIcon />
                        </a>
                      )}
                      {s.joined_at_session_number && s.joined_at_session_number > 1 && (
                        <span style={{ background: '#6c5ce7', color: 'white', padding: '2px 6px', borderRadius: '4px', fontSize: '10px' }}>
                          Late Join (S{s.joined_at_session_number})
                        </span>
                      )}
                    </div>
                  </td>
                  <td style={{ padding: '12px' }}>
                    {s4Handled ? (
                      <div style={{ color: '#28a745', fontWeight: 600, fontSize: '11px' }}>✓ COMPLETED</div>
                    ) : !canEdit ? (
                      <div style={{ color: '#666', fontWeight: 600, fontSize: '11px' }}>
                        {s.s4?.status === 'sent' ? 'SENT' : 'PENDING'}
                      </div>
                    ) : s.s4?.status === 'sent' ? (
                      <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                        <button
                          onClick={() => handleStatusUpdate(s.lead_id, 4, 'received')}
                          style={{ padding: '6px 12px', borderRadius: '4px', border: 'none', background: '#28a745', color: 'white', fontSize: '12px', cursor: 'pointer', fontWeight: 600 }}
                        >
                          Received
                        </button>
                        <button
                          onClick={() => handleStatusUpdate(s.lead_id, 4, 'removed')}
                          style={{ padding: '6px 12px', borderRadius: '4px', border: 'none', background: '#dc3545', color: 'white', fontSize: '12px', cursor: 'pointer', fontWeight: 600 }}
                        >
                          Remove
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={() => setSelected({ lead_id: s.lead_id, full_name: s.full_name, session_number: 4 })}
                        style={{ padding: '6px 12px', borderRadius: '4px', border: 'none', background: '#007bff', color: 'white', fontSize: '12px', cursor: 'pointer', fontWeight: 600 }}
                      >
                        Send
                      </button>
                    )}
                  </td>
                  <td style={{ padding: '12px' }}>
                    {s8Handled ? (
                      <div style={{ color: '#28a745', fontWeight: 600, fontSize: '11px' }}>✓ COMPLETED</div>
                    ) : !canEdit ? (
                      <div style={{ color: '#666', fontWeight: 600, fontSize: '11px' }}>
                        {s.s8?.status === 'sent' ? 'SENT' : 'PENDING'}
                      </div>
                    ) : s.s8?.status === 'sent' ? (
                      <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                        <button
                          onClick={() => handleStatusUpdate(s.lead_id, 8, 'received')}
                          style={{ padding: '6px 12px', borderRadius: '4px', border: 'none', background: '#28a745', color: 'white', fontSize: '12px', cursor: 'pointer', fontWeight: 600 }}
                        >
                          Received
                        </button>
                        <button
                          onClick={() => handleStatusUpdate(s.lead_id, 8, 'removed')}
                          style={{ padding: '6px 12px', borderRadius: '4px', border: 'none', background: '#dc3545', color: 'white', fontSize: '12px', cursor: 'pointer', fontWeight: 600 }}
                        >
                          Remove
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={() => setSelected({ lead_id: s.lead_id, full_name: s.full_name, session_number: 8 })}
                        style={{ padding: '6px 12px', borderRadius: '4px', border: 'none', background: '#007bff', color: 'white', fontSize: '12px', cursor: 'pointer', fontWeight: 600 }}
                      >
                        Send
                      </button>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      {selected && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 3000 }}>
          <div style={{ background: 'white', padding: '24px', borderRadius: '12px', width: '400px', maxWidth: '90%' }}>
            <h3 style={{ marginBottom: '16px' }}>Send Session {selected.session_number} Feedback</h3>
            <p style={{ fontSize: '14px', color: '#666', marginBottom: '16px' }}>
              Student: <strong>{selected.full_name}</strong>
            </p>
            {modalError && <div style={{ color: 'red', background: '#f8d7da', padding: '8px', borderRadius: '4px', marginBottom: '10px' }}>{modalError}</div>}
            <textarea
              value={feedbackText}
              onChange={(e) => setFeedbackText(e.target.value)}
              placeholder="Enter feedback details..."
              style={{ width: '100%', height: '120px', padding: '12px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px', marginBottom: '16px' }}
            />
            <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
              <button
                onClick={() => {
                  setSelected(null)
                  setFeedbackText('')
                  setModalError(null)
                }}
                style={{ padding: '8px 16px', borderRadius: '6px', border: '1px solid #ddd', background: '#fff', cursor: 'pointer' }}
              >
                Cancel
              </button>
              <button
                disabled={isSubmitting || !feedbackText}
                onClick={handleSubmit}
                style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: '#007bff', color: '#fff', cursor: 'pointer', opacity: isSubmitting || !feedbackText ? 0.6 : 1 }}
              >
                {isSubmitting ? 'Sending...' : 'Send Feedback'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default function StudentSuccessClass() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const classKey = searchParams.get('class_key') || localStorage.getItem('student_success_class_key') || ''
  const [data, setData] = useState<StudentSuccessClassDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [userRole, setUserRole] = useState<string>('')
  const [complianceOpen, setComplianceOpen] = useState(false)

  const [tab, setTabState] = useState<Tab>(() => {
    const tabParam = new URLSearchParams(window.location.search).get('tab') as Tab
    const validTabs: Tab[] = ['students', 'absence', 'followups', 'feedback', 'feedback_collected', 'final_grading']
    if (tabParam && validTabs.includes(tabParam)) {
      return tabParam
    }
    const savedTab = localStorage.getItem(`student_success_class_tab:${classKey}`) as Tab
    if (savedTab && validTabs.includes(savedTab)) {
      return savedTab
    }
    return 'students'
  })

  const setTab = (newTab: Tab) => {
    setTabState(newTab)
    if (classKey) {
      localStorage.setItem(`student_success_class_tab:${classKey}`, newTab)
      const nextParams = new URLSearchParams(window.location.search)
      nextParams.set('tab', newTab)
      setSearchParams(nextParams, { replace: true })
    }
  }

  const [selectedStudent, setSelectedStudent] = useState<StudentRow | null>(null)
  const [followUpModal, setFollowUpModal] = useState<{ open: boolean; item: any | null; error?: string }>({
    open: false,
    item: null,
  })
  const [confirmDialog, setConfirmDialog] = useState<ConfirmDialogState>({ open: false, title: '' })
  const [refreshNonce, setRefreshNonce] = useState(0)

  const [grades, setGrades] = useState<Record<string, { grade: string; notes: string }>>({})
  const [submittingGrade, setSubmittingGrade] = useState<string | null>(null)
  const [reportOpen, setReportOpen] = useState(false)
  const [reportLoading, setReportLoading] = useState(false)
  const [reportData, setReportData] = useState<StudentReportCardData | null>(null)
  const [gradeNotesModal, setGradeNotesModal] = useState<{ leadId: string; studentName: string; value: string } | null>(null)

  const triggerRefresh = () => setRefreshNonce((n) => n + 1)

  useEffect(() => {
    if (classKey) {
      const tabParam = searchParams.get('tab')
      if (tabParam && (tabParam === 'students' || tabParam === 'absence' || tabParam === 'followups' || tabParam === 'feedback' || tabParam === 'feedback_collected' || tabParam === 'final_grading')) {
        setTab(tabParam as Tab)
      }
      localStorage.setItem('student_success_class_key', classKey)
      loadClass()
    } else {
      setError('class_key is required')
      setLoading(false)
    }
  }, [classKey, refreshNonce])

  async function loadClass() {
    try {
      setLoading(true)
      setError(null)
      setActionError(null)
      const me = await api.getMe()
      if (me.role !== 'student_success' && me.role !== 'mentor_head' && me.role !== 'admin') {
        setError('No access. Student Success, Mentor Head, or Admin only.')
        setLoading(false)
        return
      }
      setUserRole(me.role)
      const res = await api.getStudentSuccessClass(classKey)
      setData(res)

      if (me.role === 'student_success' && searchParams.get('open_compliance') === '1') {
        setComplianceOpen(true)
        const nextParams = new URLSearchParams(window.location.search)
        nextParams.delete('open_compliance')
        setSearchParams(nextParams, { replace: true })
      }

      // Fetch existing grades
      const gradeRes = await api.getGrades(classKey)
      const gradeMap: Record<string, { grade: string; notes: string }> = {}
      gradeRes.grades?.forEach((g) => {
        gradeMap[g.lead_id] = { grade: g.grade, notes: g.notes }
      })
      setGrades(gradeMap)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load class')
    } finally {
      setLoading(false)
    }
  }

  async function handleUpdateGrade(leadId: string, grade: string, notes: string) {
    if (userRole !== 'mentor' && userRole !== 'mentor_head') return
    if (!grade) return
    try {
      setSubmittingGrade(leadId)
      setActionError(null)
      await api.createGrade({
        lead_id: leadId,
        class_key: classKey,
        grade,
        notes,
      })
      setGrades((prev) => ({ ...prev, [leadId]: { grade, notes } }))
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to save grade')
    } finally {
      setSubmittingGrade(null)
    }
  }

  async function handleClearGrade(leadId: string) {
    if (userRole !== 'mentor' && userRole !== 'mentor_head') return
    try {
      setSubmittingGrade(leadId)
      setActionError(null)
      await api.deleteGrade(leadId, classKey)
      setGrades((prev) => ({ ...prev, [leadId]: { grade: '', notes: '' } }))
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to clear grade')
    } finally {
      setSubmittingGrade(null)
    }
  }

  function getNotesButtonLabel(notes: string) {
    const trimmed = notes.trim()
    if (!trimmed) return 'Add Notes'
    if (trimmed.length <= 28) return trimmed
    return `${trimmed.slice(0, 28)}...`
  }

  function openGradeNotesModal(student: StudentRow) {
    setGradeNotesModal({
      leadId: student.lead_id,
      studentName: student.full_name,
      value: grades[student.lead_id]?.notes || '',
    })
  }

  async function saveGradeNotesModal() {
    if (!gradeNotesModal) return
    const grade = grades[gradeNotesModal.leadId]?.grade || ''
    if (!grade) return
    await handleUpdateGrade(gradeNotesModal.leadId, grade, gradeNotesModal.value)
    setGradeNotesModal(null)
  }

  async function handleOpenReport(leadId: string) {
    try {
      setReportLoading(true)
      setActionError(null)
      const res = await api.getStudentReportCard(leadId, classKey)
      if (!res.report_card) {
        throw new Error('Report card data is not available for this student yet.')
      }
      setReportData(res.report_card)
      setReportOpen(true)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to load student report')
    } finally {
      setReportLoading(false)
    }
  }

  if (loading) {
    return (
      <div style={{ padding: '40px', textAlign: 'center' }}>
        <p>Loading...</p>
      </div>
    )
  }

  const canEditGrades = userRole === 'mentor' || userRole === 'mentor_head'
  const canEditFeedback = userRole === 'student_success' || userRole === 'admin'
  const canOpenCompliance = userRole === 'student_success'

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
  const isClosed = c.round_status === 'closed'
  const tabs: { id: Tab; label: string }[] = [
    { id: 'students', label: 'Students' },
    { id: 'absence', label: 'Absence Feed' },
    { id: 'followups', label: 'Follow-ups' },
    { id: 'feedback', label: 'Feedback Checkpoints' },
    { id: 'feedback_collected', label: 'Feedback Collected' },
    { id: 'final_grading', label: 'Final Grading 🎓' },
  ]

  return (
    <>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%' }}>
          <h1>
            Level {c.level} · {c.days} · {c.time} · Class {c.class_number}
          </h1>
          {userRole === 'mentor_head' && (
            <button
              onClick={() => navigate('/mentor-head')}
              style={{
                padding: '8px 12px',
                borderRadius: '6px',
                border: '1px solid #007bff',
                background: '#fff',
                color: '#007bff',
                cursor: 'pointer',
                fontSize: '12px',
                fontWeight: 600,
              }}
            >
              Back to Mentor Head
            </button>
          )}
        </div>
      </div>

      <div style={{ display: 'flex', gap: '12px', alignItems: 'center', marginTop: '8px', marginBottom: '16px' }}>
        {isClosed ? (
          <span style={{ padding: '4px 12px', background: '#e9ecef', color: '#6c757d', borderRadius: '12px', fontSize: '12px', fontWeight: 600 }}>
            ARCHIVED · Completed Sessions: {data.completedSessionsCount}/{data.totalSessions}
          </span>
        ) : (
          <span style={{ padding: '4px 12px', background: '#d4edda', color: '#155724', borderRadius: '12px', fontSize: '12px', fontWeight: 600 }}>
            ACTIVE · Current Session: {Math.min(data.completedSessionsCount + 1, data.totalSessions)} · Total: {data.totalSessions}
          </span>
        )}
      </div>

      {actionError && (
        <div style={{ background: '#f8d7da', color: '#721c24', padding: '16px', borderRadius: '8px', marginBottom: '24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{actionError}</span>
          <button onClick={() => setActionError(null)} style={{ background: 'none', border: 'none', fontSize: '18px', cursor: 'pointer', color: '#721c24' }}>
            ×
          </button>
        </div>
      )}

      {/* Feedback notifications moved to dashboard */}

      <div style={{ display: 'flex', gap: '8px', marginBottom: '24px', flexWrap: 'wrap' }}>
        {tabs.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            style={{ padding: '8px 16px', border: `1px solid ${tab === t.id ? '#007bff' : '#dee2e6'}`, background: tab === t.id ? '#007bff' : '#fff', color: tab === t.id ? '#fff' : '#333', borderRadius: '6px', cursor: 'pointer', fontSize: '14px' }}
          >
            {t.label}
          </button>
        ))}
        {canOpenCompliance && (
          <button
            onClick={() => setComplianceOpen(true)}
            style={{ marginLeft: 'auto', padding: '8px 16px', border: '1px solid #198754', background: '#e9f7ef', color: '#198754', borderRadius: '6px', cursor: 'pointer', fontSize: '14px', fontWeight: 700 }}
          >
            Compliance
          </button>
        )}
      </div>

      {tab === 'students' && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: '16px' }}>
          {data.students.map((s) => (
            <div
              key={s.lead_id}
              onClick={() => setSelectedStudent(s)}
              style={{ background: 'white', padding: '20px', borderRadius: '8px', border: '2px solid #dee2e6', cursor: 'pointer', transition: 'all 0.2s', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}
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
              <h3 style={{ fontSize: '18px', marginBottom: '6px', color: '#333', fontWeight: 600, display: 'flex', alignItems: 'center', gap: '8px' }}>
                {s.full_name}
                {s.joined_at_session_number && s.joined_at_session_number > 1 && (
                  <span style={{ background: '#6c5ce7', color: 'white', padding: '2px 6px', borderRadius: '4px', fontSize: '10px' }}>
                    Late Join (S{s.joined_at_session_number})
                  </span>
                )}
              </h3>
              <p style={{ fontSize: '12px', color: '#999', marginBottom: '8px', fontFamily: 'monospace' }}>ID: {s.lead_id.substring(0, 8)}...</p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                <span style={{ display: 'inline-block', width: 'fit-content', padding: '4px 8px', background: s.missed_count === 0 ? '#d4edda' : s.missed_count <= 2 ? '#fff3cd' : '#f8d7da', color: s.missed_count === 0 ? '#155724' : s.missed_count <= 2 ? '#856404' : '#721c24', borderRadius: '4px', fontSize: '11px', fontWeight: 600 }}>
                  {s.missed_count} missed
                </span>
                {s.missed_sessions && s.missed_sessions.length > 0 && <div style={{ fontSize: '10px', color: '#666' }}>Missed Session {s.missed_sessions.join(', ')}</div>}
              </div>
            </div>
          ))}
        </div>
      )}

      {tab === 'absence' && <AbsenceFeed classKey={classKey} onOpenFollowUp={(item) => setFollowUpModal({ open: true, item })} refreshNonce={refreshNonce} triggerRefresh={triggerRefresh} setActionError={setActionError} />}

      {tab === 'followups' && <FollowUpsTab classKey={classKey} onOpenFollowUp={(item) => setFollowUpModal({ open: true, item })} refreshNonce={refreshNonce} />}

      {tab === 'feedback' && <FeedbackCheckpoint classKey={classKey} students={data.feedback} onUpdate={loadClass} canEdit={canEditFeedback} />}
      {tab === 'feedback_collected' && (
        <FeedbackCollectedTab classKey={classKey} students={data.students} canEdit={userRole === 'student_success'} />
      )}

      {tab === 'final_grading' && (
        <div style={{ background: 'white', borderRadius: '12px', border: '1px solid #dee2e6', padding: '24px' }}>
          <h2 style={{ fontSize: '20px', marginBottom: '8px' }}>Final Class Grading</h2>
          <p style={{ color: '#666', fontSize: '14px', marginBottom: '24px' }}>
            Assign final grades for all students. This is required before the round can be closed by the Mentor Head.
          </p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
            {data.students.map((student) => (
              <div key={student.lead_id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px', background: '#f8f9fa', borderRadius: '12px' }}>
                <div>
                  <div style={{ fontWeight: 600, fontSize: '16px' }}>{student.full_name}</div>
                  <div style={{ fontSize: '12px', color: '#999' }}>{student.phone}</div>
                </div>
                <div style={{ display: 'flex', gap: '12px', alignItems: 'center' }}>
                  {submittingGrade === student.lead_id && <span style={{ fontSize: '12px', color: '#007bff' }}>Saving...</span>}
                  {grades[student.lead_id]?.grade && submittingGrade !== student.lead_id && <span title="Saved" style={{ fontSize: '18px' }}>✅</span>}
                  <select
                    value={grades[student.lead_id]?.grade || ''}
                    disabled={!canEditGrades}
                    onChange={(e) => {
                      if (!canEditGrades) return
                      const nextGrade = e.target.value
                      setGrades((prev) => ({ ...prev, [student.lead_id]: { ...prev[student.lead_id], grade: nextGrade } }))
                      if (!nextGrade) {
                        if (grades[student.lead_id]?.grade) {
                          handleClearGrade(student.lead_id)
                        }
                        return
                      }
                      handleUpdateGrade(student.lead_id, nextGrade, grades[student.lead_id]?.notes || '')
                    }}
                    style={{ padding: '8px 12px', borderRadius: '8px', border: '1px solid #ddd', fontSize: '14px', fontWeight: 600, cursor: canEditGrades ? 'pointer' : 'not-allowed', background: canEditGrades ? '#fff' : '#f5f5f5' }}
                  >
                    <option value="">Select Grade</option>
                    <option value="A">Grade A</option>
                    <option value="B">Grade B</option>
                    <option value="C">Grade C</option>
                    <option value="F">Grade F</option>
                  </select>
                  <button
                    type="button"
                    disabled={(canEditGrades && !(grades[student.lead_id]?.grade)) || (!canEditGrades && !(grades[student.lead_id]?.notes || '').trim())}
                    onClick={() => openGradeNotesModal(student)}
                    style={{
                      padding: '8px 12px',
                      borderRadius: '8px',
                      border: '1px solid #ddd',
                      fontSize: '14px',
                      width: '240px',
                      background: canEditGrades ? '#fff' : '#f5f5f5',
                      textAlign: 'left',
                      color: (grades[student.lead_id]?.notes || '').trim() ? '#111827' : '#6b7280',
                      cursor: ((canEditGrades && !(grades[student.lead_id]?.grade)) || (!canEditGrades && !(grades[student.lead_id]?.notes || '').trim())) ? 'not-allowed' : 'pointer',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                    title={
                      grades[student.lead_id]?.grade
                        ? ((grades[student.lead_id]?.notes || '').trim() ? grades[student.lead_id]?.notes : 'Add final notes')
                        : 'Select a grade first'
                    }
                  >
                    {getNotesButtonLabel(grades[student.lead_id]?.notes || '')}
                  </button>
                  <button
                    onClick={() => handleOpenReport(student.lead_id)}
                    disabled={reportLoading}
                    style={{
                      padding: '8px 12px',
                      borderRadius: '8px',
                      border: '1px solid #cbd5e1',
                      background: '#fff',
                      cursor: 'pointer',
                      fontSize: '13px',
                      fontWeight: 600,
                    }}
                  >
                    View Report
                  </button>
                  {!canEditGrades && <span style={{ fontSize: '12px', color: '#999' }}>Read-only</span>}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {selectedStudent && (
        <StudentModal
          student={selectedStudent}
          classKey={classKey}
          sessionsCount={data.sessionsCount}
          totalSessions={data.totalSessions}
          attendedCount={(() => {
            const missed = selectedStudent.missed_sessions ? selectedStudent.missed_sessions.length : 0
            const attended = data.completedSessionsCount - missed
            return attended > 0 ? attended : 0
          })()}
          onClose={() => setSelectedStudent(null)}
        />
      )}

      {gradeNotesModal && (
        <GradeNotesModal
          open={true}
          studentName={gradeNotesModal.studentName}
          value={gradeNotesModal.value}
          onChange={(value) => setGradeNotesModal((prev) => (prev ? { ...prev, value } : prev))}
          onClose={() => setGradeNotesModal(null)}
          onSave={saveGradeNotesModal}
          saving={submittingGrade === gradeNotesModal.leadId}
          canEdit={canEditGrades}
        />
      )}

      {followUpModal.open && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2000 }} onClick={() => setFollowUpModal({ open: false, item: null })}>
          <div style={{ background: 'white', padding: '24px', borderRadius: '12px', width: '400px', maxWidth: '90%' }} onClick={(e) => e.stopPropagation()}>
            <h3 style={{ marginBottom: '16px' }}>Add Follow-up Note</h3>
            <p style={{ fontSize: '14px', color: '#666', marginBottom: '16px' }}>
              Student: <strong>{followUpModal.item.studentName || followUpModal.item.student_name}</strong> (S{followUpModal.item.sessionNumber || followUpModal.item.session_number})
            </p>
            {followUpModal.error && <div style={{ color: 'red', background: '#f8d7da', padding: '8px', borderRadius: '4px', marginBottom: '10px' }}>{followUpModal.error}</div>}
            <div style={{ marginBottom: '16px' }}>
              <label style={{ display: 'block', fontSize: '12px', color: '#666', marginBottom: '4px' }}>Status</label>
              <select id="followup-status" defaultValue={normalizeFollowUpStatusForSelect(followUpModal.item.followUp?.status || followUpModal.item.status || 'contacted')} style={{ width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid #ddd' }}>
                <option value="none">None</option>
                <option value="contacted">Contacted (same day)</option>
                <option value="not_replied">Not Replied (after 1 day)</option>
                <option value="no_response">No Response (after 4 days) → Escalates</option>
              </select>
            </div>
            <div style={{ marginBottom: '16px' }}>
              <label style={{ display: 'block', fontSize: '12px', color: '#666', marginBottom: '4px' }}>Follow-up Note</label>
              <textarea id="followup-note" defaultValue={followUpModal.item.followUp?.lastNote || followUpModal.item.note || ''} placeholder="Enter follow-up details..." style={{ width: '100%', height: '100px', padding: '12px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px' }} />
            </div>
            <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
              <button onClick={() => setFollowUpModal({ open: false, item: null })} style={{ padding: '8px 16px', borderRadius: '6px', border: '1px solid #ddd', background: '#fff', cursor: 'pointer' }}>
                Cancel
              </button>
              <button
                onClick={async () => {
                  const note = (document.getElementById('followup-note') as HTMLTextAreaElement).value
                  const status = (document.getElementById('followup-status') as HTMLSelectElement).value
                  if (!note) {
                    setFollowUpModal((prev) => ({ ...prev, error: 'Please enter a note' }))
                    return
                  }
                  try {
                    const followUpId = followUpModal.item.followUp?.id
                    if (followUpId) {
                      await api.updateFollowUp(followUpId, { status: status, note: note, resolved: false })
                    } else {
                      await api.addFollowUp({ class_key: classKey, lead_id: followUpModal.item.studentId || followUpModal.item.lead_id, session_number: followUpModal.item.sessionNumber || followUpModal.item.session_number, note, status: status })
                    }
                    setFollowUpModal({ open: false, item: null })
                    triggerRefresh()
                    if (status === 'no_response' && tab === 'absence') setTab('followups')
                  } catch (err) {
                    setFollowUpModal((prev) => ({ ...prev, error: err instanceof Error ? err.message : 'Failed to save note' }))
                  }
                }}
                style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: '#007bff', color: '#fff', cursor: 'pointer' }}
              >
                Save
              </button>
              {(followUpModal.item.followUp?.id || followUpModal.item.status) && (
                <button
                  onClick={async () => {
                    const note = (document.getElementById('followup-note') as HTMLTextAreaElement).value
                    const status = (document.getElementById('followup-status') as HTMLSelectElement).value
                    if (!note) {
                      setFollowUpModal((prev) => ({ ...prev, error: 'Please enter a final note' }))
                      return
                    }
                    setConfirmDialog({
                      open: true,
                      title: 'Mark as resolved?',
                      body: 'This will mark the follow-up as resolved and close the note.',
                      confirmLabel: 'Resolve & Close',
                      tone: 'success',
                      onConfirm: async () => {
                        setConfirmDialog({ open: false, title: '' })
                        try {
                          const followUpId = followUpModal.item.followUp?.id
                          if (followUpId) {
                            await api.updateFollowUp(followUpId, { status: status, note: note, resolved: true })
                          } else {
                            await api.resolveAbsence({
                              class_key: classKey,
                              lead_id: followUpModal.item.studentId || followUpModal.item.lead_id,
                              session_number: followUpModal.item.sessionNumber || followUpModal.item.session_number,
                              note,
                              status,
                            })
                          }
                          setFollowUpModal({ open: false, item: null })
                          triggerRefresh()
                        } catch (err) {
                          setFollowUpModal((prev) => ({ ...prev, error: err instanceof Error ? err.message : 'Failed to resolve' }))
                        }
                      },
                    })
                  }}
                  style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: '#28a745', color: '#fff', cursor: 'pointer' }}
                >
                  Resolve & Close
                </button>
              )}
            </div>
          </div>
        </div>
      )}
      <ConfirmDialog
        open={confirmDialog.open}
        title={confirmDialog.title}
        body={confirmDialog.body}
        confirmLabel={confirmDialog.confirmLabel}
        cancelLabel={confirmDialog.cancelLabel}
        tone={confirmDialog.tone}
        onConfirm={confirmDialog.onConfirm}
        onCancel={() => setConfirmDialog({ open: false, title: '' })}
      />
      {canOpenCompliance && (
        <ComplianceModal open={complianceOpen} classKey={classKey} onClose={() => setComplianceOpen(false)} />
      )}
      {reportOpen && reportData && (
        <div
          className="report-overlay"
          style={{
            position: 'fixed',
            inset: 0,
            zIndex: 5000,
            background: 'rgba(0,0,0,0.5)',
            overflow: 'auto',
          }}
          onClick={(e) => {
            if (e.target === e.currentTarget) {
              setReportOpen(false)
              setReportData(null)
            }
          }}
        >
          <StudentReportCard
            data={reportData}
            onClose={() => {
              setReportOpen(false)
              setReportData(null)
            }}
          />
        </div>
      )}
    </>
  )
}

function AbsenceFeed({ classKey, onOpenFollowUp, refreshNonce, triggerRefresh, setActionError }: { classKey: string; onOpenFollowUp: (item: any) => void; refreshNonce: number; triggerRefresh: () => void; setActionError: (error: string | null) => void }) {
  const [items, setItems] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<'all' | 'unresolved' | 'resolved' | 'absent' | 'late'>('all')
  const [search, setSearch] = useState('')
  const [confirmState, setConfirmState] = useState<{ open: boolean; item?: { followUpId?: string; studentId: string; sessionNum: number } }>({ open: false })

  useEffect(() => {
    loadFeed()
  }, [classKey, filter, search, refreshNonce])

  async function loadFeed() {
    try {
      setLoading(true)
      setActionError(null)
      const res = await api.getAbsenceFeed(classKey, filter, search)
      setItems(res || [])
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to load feed')
    } finally {
      setLoading(false)
    }
  }

  async function handleMarkResolved(followUpId: string | undefined, studentId: string, sessionNum: number) {
    setConfirmState({ open: true, item: { followUpId, studentId, sessionNum } })
  }

  async function confirmResolve() {
    if (!confirmState.item) return
    const { followUpId, studentId, sessionNum } = confirmState.item
    setConfirmState({ open: false })

    const originalItems = items
    setItems((prev) => prev.filter((item) => !(item.studentId === studentId && item.sessionNumber === sessionNum)))
    setActionError(null)

    try {
      if (followUpId) {
        await api.resolveFollowUp(followUpId)
      } else {
        await api.resolveAbsence({
          class_key: classKey,
          lead_id: studentId,
          session_number: sessionNum,
        })
      }
      triggerRefresh()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to resolve')
      setItems(originalItems)
    }
  }

  const groupedItems = items.reduce((acc: Record<number, any[]>, item) => {
    if (!acc[item.sessionNumber]) acc[item.sessionNumber] = []
    acc[item.sessionNumber].push(item)
    return acc
  }, {})

  const sessionNumbers = Object.keys(groupedItems)
    .map(Number)
    .sort((a, b) => b - a)

  if (loading) return <p style={{ padding: '20px' }}>Loading absence feed...</p>

  return (
    <>
      <div style={{ background: 'white', borderRadius: '8px', border: '1px solid #dee2e6', overflow: 'hidden' }}>
        <div style={{ padding: '16px', borderBottom: '1px solid #eee', display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '12px' }}>
          <div style={{ display: 'flex', gap: '8px' }}>
            {(['all', 'unresolved', 'resolved', 'absent', 'late'] as const).map((f) => (
              <button key={f} onClick={() => setFilter(f)} style={{ padding: '4px 12px', borderRadius: '4px', border: '1px solid #dee2e6', background: filter === f ? '#007bff' : '#fff', color: filter === f ? '#fff' : '#666', fontSize: '12px', cursor: 'pointer', textTransform: 'capitalize' }}>
                {f}
              </button>
            ))}
          </div>
          <input type="text" placeholder="Search name or phone..." value={search} onChange={(e) => setSearch(e.target.value)} style={{ padding: '6px 12px', borderRadius: '4px', border: '1px solid #dee2e6', fontSize: '14px', width: '200px' }} />
        </div>

        <div style={{ overflowX: 'auto' }}>
          {sessionNumbers.length === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>No absence cases found matching filters.</div>
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
                          <div style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '8px' }}>
                            {item.studentName}
                            {item.joinedAtSessionNumber && <span style={{ background: '#6c5ce7', color: 'white', padding: '2px 6px', borderRadius: '4px', fontSize: '10px' }}>Late Join (S{item.joinedAtSessionNumber})</span>}
                          </div>
                          <div style={{ fontSize: '12px', color: '#666' }}>{item.studentPhone}</div>
                        </td>
                        <td style={{ padding: '12px' }}>
                          <span style={{ padding: '2px 6px', borderRadius: '4px', fontSize: '11px', fontWeight: 600, background: item.status === 'ABSENT' ? '#f8d7da' : '#fff3cd', color: item.status === 'ABSENT' ? '#721c24' : '#856404' }}>{item.status}</span>
                          {item.mentorNote && <div style={{ fontSize: '11px', color: '#888', fontStyle: 'italic', marginTop: '4px' }}>"{item.mentorNote}"</div>}
                        </td>
                        <td style={{ padding: '12px' }}>
                          <div style={{ fontSize: '12px', fontWeight: 500 }}>{item.sessionDate}</div>
                          <div style={{ fontSize: '11px', color: '#999' }}>{new Date(item.markedAt).toLocaleString()}</div>
                          <div style={{ fontSize: '11px', color: '#999' }}>By: {item.markedBy}</div>
                        </td>
                        <td style={{ padding: '12px' }}>
                          {item.followUp ? (
                            <div>
                              <span style={{ padding: '2px 6px', borderRadius: '4px', fontSize: '10px', fontWeight: 600, background: item.followUp.status === 'RESOLVED' ? '#d4edda' : '#e2e3e5', color: item.followUp.status === 'RESOLVED' ? '#155724' : '#383d41' }}>{formatFollowUpStatus(item.followUp.status)}</span>
                              {item.followUp.lastNote && <div style={{ fontSize: '11px', color: '#666', marginTop: '4px' }}>{item.followUp.lastNote}</div>}
                              {item.followUp.notes?.length ? (
                                <div style={{ marginTop: '6px', display: 'grid', gap: '4px' }}>
                                  {item.followUp.notes.slice(0, 3).map((note: any) => (
                                    <div key={note.id} style={{ fontSize: '11px', color: '#555', background: '#f8f9fa', borderRadius: '4px', padding: '6px' }}>
                                      <div style={{ fontWeight: 600 }}>{formatFollowUpNoteType(note.note_text, note.note_type)}</div>
                                      <div style={{ color: '#777', marginTop: '2px' }}>
                                        {new Date(note.created_at).toLocaleString()} · {note.created_by_email || 'Unknown'}
                                      </div>
                                    </div>
                                  ))}
                                </div>
                              ) : null}
                            </div>
                          ) : (
                            <span style={{ fontSize: '11px', color: '#999' }}>No follow-up yet</span>
                          )}
                        </td>
                        <td style={{ padding: '12px' }}>
                          <div style={{ display: 'flex', gap: '8px' }}>
                            <a href={buildWhatsAppLink(item.studentPhone)} target="admin-whatsapp-chat" onClick={(event) => {
                              event.preventDefault()
                              if (!openWhatsAppLink(buildWhatsAppLink(item.studentPhone))) {
                                window.location.href = buildWhatsAppLink(item.studentPhone)
                              }
                            }} title="Open WhatsApp" aria-label={`Open WhatsApp chat for ${item.studentName}`} style={{ padding: '4px', borderRadius: '999px', background: '#25D366', color: 'white', display: 'flex', alignItems: 'center', justifyContent: 'center', width: '28px', height: '28px', textDecoration: 'none', boxShadow: '0 2px 8px rgba(37, 211, 102, 0.28)' }}>
                              <WhatsAppIcon />
                            </a>
                            <button onClick={() => onOpenFollowUp(item)} title="Add Follow-up Note" style={{ padding: '4px 8px', borderRadius: '4px', border: '1px solid #007bff', background: '#fff', color: '#007bff', fontSize: '11px', cursor: 'pointer' }}>
                              Follow up
                            </button>
                            {filter !== 'resolved' && (
                              <button onClick={() => handleMarkResolved(item.followUp?.id, item.studentId, item.sessionNumber)} title="Resolve" style={{ padding: '4px 8px', borderRadius: '4px', border: '1px solid #28a745', background: '#fff', color: '#28a745', fontSize: '11px', cursor: 'pointer' }}>
                                Resolve
                              </button>
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
      </div>
      <ConfirmDialog
        open={confirmState.open}
        title="Mark as resolved?"
        body="This will resolve the absence and remove it from the list."
        confirmLabel="Resolve"
        tone="success"
        onConfirm={confirmResolve}
        onCancel={() => setConfirmState({ open: false })}
      />
    </>
  )
}

function FollowUpsTab({ classKey, onOpenFollowUp, refreshNonce }: { classKey: string; onOpenFollowUp: (item: any) => void; refreshNonce: number }) {
  const [items, setItems] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showResolved, setShowResolved] = useState(false)
  const [showComplaintModal, setShowComplaintModal] = useState(false)

  useEffect(() => {
    loadFollowUps()
  }, [classKey, showResolved, refreshNonce])

  async function loadFollowUps() {
    try {
      setLoading(true)
      setError(null)
      const res = await api.getFollowUps(classKey, showResolved)
      setItems(res || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load follow-ups')
    } finally {
      setLoading(false)
    }
  }

  if (loading) return <p style={{ padding: '20px' }}>Loading follow-ups...</p>
  if (error) return <p style={{ color: 'red', padding: '20px' }}>{error}</p>

  return (
    <>
      <div style={{ background: 'white', borderRadius: '8px', border: '1px solid #dee2e6', overflow: 'hidden' }}>
        <div style={{ padding: '16px', borderBottom: '1px solid #eee', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ fontSize: '18px', margin: 0 }}>Follow-ups</h2>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <button onClick={() => setShowComplaintModal(true)} style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: '#dc3545', color: '#fff', cursor: 'pointer', fontSize: '14px', fontWeight: 600 }}>
              + New Complaint
            </button>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '14px' }}>
              <label>Show Resolved:</label>
              <input type="checkbox" checked={showResolved} onChange={(e) => setShowResolved(e.target.checked)} />
            </div>
          </div>
        </div>
        <div style={{ overflowX: 'auto' }}>
          {items.length === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>No active follow-ups for this class.</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '14px' }}>
              <thead>
                <tr style={{ textAlign: 'left', background: '#f8f9fa', borderBottom: '1px solid #eee' }}>
                  <th style={{ padding: '12px' }}>Type</th>
                  <th style={{ padding: '12px' }}>Student</th>
                  <th style={{ padding: '12px' }}>Session</th>
                  <th style={{ padding: '12px' }}>Details</th>
                  <th style={{ padding: '12px' }}>Status</th>
                  <th style={{ padding: '12px' }}>Created At</th>
                  <th style={{ padding: '12px' }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => {
                  const isComplaint = item.note && item.note.startsWith('[COMPLAINT')
                  let category = ''
                  let urgency = ''
                  let complaintText = item.note

                  if (isComplaint) {
                    const match = item.note.match(/\[COMPLAINT - ([^/]+)\/([^\]]+)\] (.+)/)
                    if (match) {
                      ;[category, urgency, complaintText] = match.slice(1)
                    }
                  }

                  return (
                    <tr key={item.id} style={{ borderBottom: '1px solid #eee' }}>
                      <td style={{ padding: '12px' }}>
                        <span style={{ padding: '4px 8px', borderRadius: '4px', fontSize: '11px', fontWeight: 600, background: isComplaint ? '#dc3545' : '#6c757d', color: 'white' }}>{isComplaint ? 'COMPLAINT' : 'ABSENCE'}</span>
                      </td>
                      <td style={{ padding: '12px' }}>
                        <div style={{ fontWeight: 600 }}>{item.student_name}</div>
                        <div style={{ fontSize: '12px', color: '#666' }}>{item.student_phone}</div>
                      </td>
                      <td style={{ padding: '12px' }}>{item.session_number ? `S${item.session_number}` : '-'}</td>
                      <td style={{ padding: '12px' }}>
                        {isComplaint ? (
                          <div>
                            <div style={{ fontSize: '11px', marginBottom: '4px' }}>
                              <span style={{ padding: '2px 6px', borderRadius: '3px', background: '#e7f3ff', color: '#004085', marginRight: '4px' }}>{category}</span>
                              <span style={{ padding: '2px 6px', borderRadius: '3px', background: urgency === 'high' ? '#f8d7da' : urgency === 'medium' ? '#fff3cd' : '#d4edda', color: urgency === 'high' ? '#721c24' : urgency === 'medium' ? '#856404' : '#155724' }}>{urgency.toUpperCase()}</span>
                            </div>
                            <div style={{ fontSize: '12px', color: '#666' }}>{complaintText}</div>
                          </div>
                        ) : (
                          <div>
                            <div style={{ fontSize: '12px', color: '#666' }}>{item.attendance_status || item.note}</div>
                            {item.notes?.length ? (
                              <div style={{ marginTop: '6px', display: 'grid', gap: '4px' }}>
                                {item.notes.slice(0, 3).map((note: any) => (
                                  <div key={note.id} style={{ fontSize: '11px', color: '#555', background: '#f8f9fa', borderRadius: '4px', padding: '6px' }}>
                                    <div style={{ fontWeight: 600 }}>{formatFollowUpNoteType(note.note_text, note.note_type)}</div>
                                    <div style={{ color: '#777', marginTop: '2px' }}>
                                      {new Date(note.created_at).toLocaleString()} · {note.created_by_email || 'Unknown'}
                                    </div>
                                  </div>
                                ))}
                              </div>
                            ) : null}
                          </div>
                        )}
                      </td>
                      <td style={{ padding: '12px' }}>
                        <span style={{ padding: '2px 6px', borderRadius: '4px', fontSize: '11px', fontWeight: 600, background: item.resolved ? '#d4edda' : '#e2e3e5', color: item.resolved ? '#155724' : '#383d41' }}>{item.resolved ? 'RESOLVED' : formatFollowUpStatus(item.status)}</span>
                      </td>
                      <td style={{ padding: '12px' }}>{new Date(item.created_at).toLocaleString()}</td>
                      <td style={{ padding: '12px' }}>{!item.resolved && <button onClick={() => onOpenFollowUp(item)} style={{ padding: '4px 8px', borderRadius: '4px', border: '1px solid #28a745', background: '#fff', color: '#28a745', fontSize: '11px', cursor: 'pointer' }}>Resolve</button>}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
        </div>
      </div>
      {showComplaintModal && <ComplaintModal classKey={classKey} onClose={() => setShowComplaintModal(false)} onSuccess={() => { setShowComplaintModal(false); loadFollowUps(); }} />}
    </>
  )
}

function ComplaintModal({ classKey, onClose, onSuccess }: { classKey: string; onClose: () => void; onSuccess: () => void }) {
  const [studentPhone, setStudentPhone] = useState('')
  const [category, setCategory] = useState('mentor_behavior')
  const [urgency, setUrgency] = useState('medium')
  const [complaintText, setComplaintText] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const categories = [{ value: 'mentor_behavior', label: 'Mentor Behavior' }, { value: 'session_quality', label: 'Session Quality' }, { value: 'technical', label: 'Technical Issues' }, { value: 'scheduling', label: 'Scheduling' }, { value: 'other', label: 'Other' }]
  const urgencies = [{ value: 'low', label: 'Low' }, { value: 'medium', label: 'Medium' }, { value: 'high', label: 'High' }]

  async function handleSubmit() {
    if (!studentPhone || !complaintText) {
      setError('Please fill in all required fields')
      return
    }
    setIsSubmitting(true)
    setError(null)
    try {
      await api.createComplaint({ class_key: classKey, student_phone: studentPhone, category, complaint_text: complaintText, urgency })
      onSuccess()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to file complaint')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 3000 }} onClick={onClose}>
      <div style={{ background: 'white', padding: '24px', borderRadius: '12px', width: '500px', maxWidth: '90%' }} onClick={(e) => e.stopPropagation()}>
        <h3 style={{ marginBottom: '20px', color: '#dc3545' }}>File New Complaint</h3>
        {error && <div style={{ color: 'red', background: '#f8d7da', padding: '8px', borderRadius: '4px', marginBottom: '10px' }}>{error}</div>}
        <div style={{ marginBottom: '16px' }}>
          <label style={{ display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '6px' }}>
            Student Phone <span style={{ color: '#dc3545' }}>*</span>
          </label>
          <input type="text" value={studentPhone} onChange={(e) => setStudentPhone(e.target.value)} placeholder="e.g., 01234567890" style={{ width: '100%', padding: '10px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px' }} />
        </div>
        <div style={{ marginBottom: '16px' }}>
          <label style={{ display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '6px' }}>
            Category <span style={{ color: '#dc3545' }}>*</span>
          </label>
          <select value={category} onChange={(e) => setCategory(e.target.value)} style={{ width: '100%', padding: '10px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px' }}>
            {categories.map((cat) => (<option key={cat.value} value={cat.value}> {cat.label} </option>))}
          </select>
        </div>
        <div style={{ marginBottom: '16px' }}>
          <label style={{ display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '6px' }}>
            Urgency <span style={{ color: '#dc3545' }}>*</span>
          </label>
          <select value={urgency} onChange={(e) => setUrgency(e.target.value)} style={{ width: '100%', padding: '10px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px' }}>
            {urgencies.map((urg) => (<option key={urg.value} value={urg.value}> {urg.label} </option>))}
          </select>
        </div>
        <div style={{ marginBottom: '20px' }}>
          <label style={{ display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '6px' }}>
            Complaint Details <span style={{ color: '#dc3545' }}>*</span>
          </label>
          <textarea value={complaintText} onChange={(e) => setComplaintText(e.target.value)} placeholder="Describe the complaint in detail..." style={{ width: '100%', height: '120px', padding: '12px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px', resize: 'vertical' }} />
        </div>
        <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
          <button onClick={onClose} disabled={isSubmitting} style={{ padding: '10px 20px', borderRadius: '6px', border: '1px solid #ddd', background: '#fff', cursor: 'pointer', fontSize: '14px' }}>
            Cancel
          </button>
          <button onClick={handleSubmit} disabled={isSubmitting || !studentPhone || !complaintText} style={{ padding: '10px 20px', borderRadius: '6px', border: 'none', background: '#dc3545', color: '#fff', cursor: 'pointer', fontSize: '14px', fontWeight: 600, opacity: isSubmitting || !studentPhone || !complaintText ? 0.6 : 1 }}>
            {isSubmitting ? 'Filing...' : 'File Complaint'}
          </button>
        </div>
      </div>
    </div>
  )
}

function normalizeFollowUpStatusForSelect(status?: string) {
  const normalized = String(status || '').trim().toLowerCase()
  if (!normalized) return 'none'
  if (normalized === 'not_contacted') return 'none'
  if (normalized === 'contacted') return 'contacted'
  if (normalized === 'not_replied') return 'not_replied'
  if (normalized === 'no_response') return 'no_response'
  if (normalized === 'resolved') return 'contacted'
  return normalized
}

function formatFollowUpStatus(status?: string) {
  const normalized = String(status || '').trim().toUpperCase()
  if (!normalized) return 'None'
  return normalized.replace(/_/g, ' ')
}

function formatFollowUpNoteType(text?: string, noteType?: string) {
  const normalizedType = String(noteType || '').trim().toLowerCase()
  const safeText = String(text || '').trim()
  if (normalizedType === 'status_change') return safeText || 'Status changed'
  if (normalizedType === 'resolution') return `Resolved: ${safeText || 'Case resolved'}`
  if (normalizedType === 'system') return safeText || 'System update'
  return safeText || 'Follow-up note'
}

function WhatsAppIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M20.52 3.48A11.86 11.86 0 0 0 12.06 0C5.5 0 .16 5.34.16 11.9c0 2.1.55 4.15 1.58 5.95L0 24l6.33-1.66a11.83 11.83 0 0 0 5.72 1.46h.01c6.56 0 11.9-5.34 11.9-11.9 0-3.18-1.24-6.17-3.44-8.42Zm-8.46 18.3h-.01a9.9 9.9 0 0 1-5.05-1.39l-.36-.21-3.76.99 1-3.66-.24-.38a9.88 9.88 0 0 1-1.52-5.23c0-5.46 4.45-9.9 9.92-9.9 2.65 0 5.13 1.03 7 2.9a9.83 9.83 0 0 1 2.9 7c0 5.47-4.45 9.9-9.88 9.9Zm5.43-7.42c-.3-.15-1.78-.88-2.06-.98-.28-.1-.48-.15-.68.15-.2.3-.78.98-.95 1.18-.17.2-.35.22-.64.08-.3-.15-1.24-.46-2.36-1.47-.88-.78-1.47-1.75-1.64-2.05-.17-.3-.02-.46.13-.61.13-.13.3-.35.45-.52.15-.18.2-.3.3-.5.1-.2.05-.38-.02-.53-.08-.15-.68-1.63-.93-2.23-.24-.58-.5-.5-.68-.5h-.58c-.2 0-.53.08-.8.38-.28.3-1.05 1.03-1.05 2.5s1.08 2.9 1.23 3.1c.15.2 2.11 3.23 5.12 4.52.72.31 1.28.5 1.72.64.72.23 1.37.2 1.88.12.57-.08 1.78-.73 2.03-1.43.25-.7.25-1.3.18-1.42-.08-.12-.28-.2-.58-.35Z" />
    </svg>
  )
}
