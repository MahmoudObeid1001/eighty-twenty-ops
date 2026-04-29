import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, ClassDetail, ClassTransferOption, GradePreview, Student, StudentReportCardData } from '../api/client'
import StudentModal from '../components/StudentModal'
import FeedbackCollectedTab from '../components/FeedbackCollectedTab'
import ComplianceModal from '../components/ComplianceModal'
import StudentReportCard from '../components/StudentReportCard'
import GradeNotesModal from '../components/GradeNotesModal'

function formatSessionDateLabel(value: string) {
  const date = new Date(`${value}T00:00:00`)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    weekday: 'short',
  }).format(date)
}

function formatSessionTimeLabel(value: string) {
  if (!value) return ''
  const normalized = value.length === 5 ? `${value}:00` : value
  const date = new Date(`2000-01-01T${normalized}`)
  if (Number.isNaN(date.getTime())) return value.slice(0, 5)
  if (date.getHours() > 0 && date.getHours() < 12) {
    date.setHours(date.getHours() + 12)
  }
  return new Intl.DateTimeFormat('en-US', {
    hour: 'numeric',
    minute: '2-digit',
  }).format(date)
}

function formatSourceExitLabel(sessionNumber: number) {
  if (sessionNumber <= 0) return 'before session 1'
  return `after session ${sessionNumber}`
}

export default function ClassWorkspace() {
  const [searchParams, setSearchParams] = useSearchParams()
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

  const [activeTab, setActiveTabState] = useState<'sessions' | 'grades' | 'feedback_collected'>(() => {
    const tabParam = new URLSearchParams(window.location.search).get('tab')
    if (tabParam === 'grades' || tabParam === 'feedback_collected' || tabParam === 'sessions') {
      return tabParam as any
    }
    const savedTab = localStorage.getItem(`class_workspace_tab:${classKey}`)
    if (savedTab === 'grades' || savedTab === 'feedback_collected' || savedTab === 'sessions') {
      return savedTab as any
    }
    return 'sessions'
  })

  const setActiveTab = (tab: 'sessions' | 'grades' | 'feedback_collected') => {
    setActiveTabState(tab)
    if (classKey) {
      localStorage.setItem(`class_workspace_tab:${classKey}`, tab)
      const nextParams = new URLSearchParams(window.location.search)
      nextParams.set('tab', tab)
      setSearchParams(nextParams, { replace: true })
    }
  }

  const [grades, setGrades] = useState<Record<string, { grade: string; notes: string }>>({})
  const [gradeDrafts, setGradeDrafts] = useState<Record<string, { grade: string; notes: string }>>({})
  const [gradePreviews, setGradePreviews] = useState<Record<string, GradePreview>>({})
  const [savingAllGrades, setSavingAllGrades] = useState(false)
  const [userRole, setUserRole] = useState<string>('')
  const [complianceOpen, setComplianceOpen] = useState(false)
  const [actionSuccess, setActionSuccess] = useState<string | null>(null)
  const [reportOpen, setReportOpen] = useState(false)
  const [reportLoading, setReportLoading] = useState(false)
  const [reportData, setReportData] = useState<StudentReportCardData | null>(null)
  const [gradeNotesStudent, setGradeNotesStudent] = useState<Student | null>(null)
  const [shiftStartModalOpen, setShiftStartModalOpen] = useState(false)
  const [shiftStartDate, setShiftStartDate] = useState('')
  const [shiftStartSaving, setShiftStartSaving] = useState(false)
  const [rescheduleModalOpen, setRescheduleModalOpen] = useState(false)
  const [rescheduleDate, setRescheduleDate] = useState('')
  const [rescheduleTime, setRescheduleTime] = useState('')
  const [rescheduleSaving, setRescheduleSaving] = useState(false)
  const [rosterModalMode, setRosterModalMode] = useState<'transfer' | 'return' | 'early_repeat' | null>(null)
  const [rosterStudent, setRosterStudent] = useState<Student | null>(null)
  const [transferOptions, setTransferOptions] = useState<ClassTransferOption[]>([])
  const [transferOptionsLoading, setTransferOptionsLoading] = useState(false)
  const [rosterTargetClassKey, setRosterTargetClassKey] = useState('')
  const [rosterReason, setRosterReason] = useState('schedule_change')
  const [rosterNotes, setRosterNotes] = useState('')
  const [rosterSaving, setRosterSaving] = useState(false)

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
      const me = await api.getMe()
      setUserRole(me.role || '')
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

      // Fetch existing grades
      const gradeRes = await api.getGrades(classKey)
      const gradeMap: Record<string, { grade: string; notes: string }> = {}
      gradeRes.grades?.forEach((g) => {
        gradeMap[g.lead_id] = { grade: g.grade, notes: g.notes }
      })
      setGrades(gradeMap)
      setGradeDrafts(gradeMap)

      const previewRes = await api.getGradePreview(classKey)
      const previewMap: Record<string, GradePreview> = {}
      previewRes.previews?.forEach((p) => {
        previewMap[p.lead_id] = p
      })
      setGradePreviews(previewMap)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load class')
    } finally {
      if (!silent) setLoading(false)
    }
  }

  async function handleMarkAttendance(
    sessionId: string,
    leadId: string,
    status: string,
    taskCompleted?: boolean,
    participationScore?: number
  ) {
    try {
      setUpdating(`${leadId}-${sessionId}`)
      setActionError(null)
      await api.markAttendance(sessionId, leadId, status, classKey, '', taskCompleted, participationScore)
      await loadClass(true)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to mark attendance')
    } finally {
      setUpdating(null)
    }
  }

  async function handleSessionPerformanceChange(
    sessionId: string,
    leadId: string,
    nextTaskCompleted?: boolean,
    nextParticipationScore?: number
  ) {
    const student = classData?.students.find((s) => s.lead_id === leadId)
    if (!student) return
    const currentStatus = student.attendance?.[sessionId]
    if (!currentStatus || currentStatus === 'N/A') return
    const currentPerf = student.session_performance?.[sessionId]
    const taskCompleted = nextTaskCompleted ?? currentPerf?.task_completed ?? false
    const participationScore = nextParticipationScore ?? currentPerf?.participation_score ?? 3
    await handleMarkAttendance(sessionId, leadId, currentStatus, taskCompleted, participationScore)
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

  function handleOpenShiftStartModal() {
    if (!classData) return
    const firstSession = classData.sessions.find((s) => s.session_number === 1) || classData.sessions[0]
    setShiftStartDate(firstSession?.scheduled_date || '')
    setShiftStartModalOpen(true)
  }

  async function handleShiftRoundStartDate() {
    if (!shiftStartDate) {
      setActionError('Select a new start date first.')
      return
    }
    try {
      setShiftStartSaving(true)
      setActionError(null)
      setActionSuccess(null)
      await api.shiftRoundStartDate(classKey, shiftStartDate)
      await loadClass(true)
      setShiftStartModalOpen(false)
      setActionSuccess(`Class start date moved to ${shiftStartDate}. All scheduled sessions were shifted.`)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to change class start date')
    } finally {
      setShiftStartSaving(false)
    }
  }

  function handleOpenRescheduleModal() {
    if (!selectedSession) return
    setRescheduleDate(selectedSession.scheduled_date)
    setRescheduleTime(String(selectedSession.scheduled_time || '').slice(0, 5))
    setRescheduleModalOpen(true)
  }

  async function handleRescheduleSession() {
    if (!selectedSession || !rescheduleDate || !rescheduleTime) {
      setActionError('Choose both a new date and time.')
      return
    }

    try {
      setRescheduleSaving(true)
      setActionError(null)
      setActionSuccess(null)
      await api.rescheduleSession(classKey, selectedSession.id, rescheduleDate, rescheduleTime)
      await loadClass(true)
      setRescheduleModalOpen(false)
      setActionSuccess(`Session ${selectedSession.session_number} moved to ${rescheduleDate} at ${formatSessionTimeLabel(rescheduleTime)}.`)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to reschedule session')
    } finally {
      setRescheduleSaving(false)
    }
  }

  function closeRosterModal() {
    setRosterModalMode(null)
    setRosterStudent(null)
    setTransferOptions([])
    setTransferOptionsLoading(false)
    setRosterTargetClassKey('')
    setRosterReason('schedule_change')
    setRosterNotes('')
    setRosterSaving(false)
  }

  async function openTransferModal(student: Student) {
    try {
      setActionError(null)
      setActionSuccess(null)
      setRosterStudent(student)
      setRosterModalMode('transfer')
      setRosterReason('schedule_change')
      setRosterNotes('')
      setRosterTargetClassKey('')
      setTransferOptions([])
      setTransferOptionsLoading(true)
      const res = await api.getClassTransferOptions(student.lead_id, classKey)
      setTransferOptions(res.options || [])
      if (res.options && res.options.length > 0) {
        setRosterTargetClassKey(res.options[0].class_key)
      }
    } catch (err) {
      closeRosterModal()
      setActionError(err instanceof Error ? err.message : 'Failed to load target classes')
    } finally {
      setTransferOptionsLoading(false)
    }
  }

  function openReturnModal(student: Student) {
    setActionError(null)
    setActionSuccess(null)
    setRosterStudent(student)
    setRosterModalMode('return')
    setRosterReason('refund_to_admin')
    setRosterNotes('')
    setRosterTargetClassKey('')
    setTransferOptions([])
  }

  function openEarlyRepeatModal(student: Student) {
    setActionError(null)
    setActionSuccess(null)
    setRosterStudent(student)
    setRosterModalMode('early_repeat')
    setRosterReason('early_repeat_absence')
    setRosterNotes('')
    setRosterTargetClassKey('')
    setTransferOptions([])
  }

  async function handleSaveRosterChange() {
    if (!rosterStudent || !rosterModalMode) return

    try {
      setRosterSaving(true)
      setActionError(null)
      setActionSuccess(null)

      if (rosterModalMode === 'transfer') {
        if (!rosterTargetClassKey) {
          setActionError('Choose a target class first.')
          return
        }
        const res = await api.transferClassStudent({
          lead_id: rosterStudent.lead_id,
          source_class_key: classKey,
          target_class_key: rosterTargetClassKey,
          reason: rosterReason,
          notes: rosterNotes,
        })
        await loadClass(true)
        closeRosterModal()
        setActionSuccess(
          `${rosterStudent.full_name} was removed from the source class ${formatSourceExitLabel(res.source_exit_after_session_number)} and joins the target class at session ${res.target_joined_at_session_number}.`
        )
        return
      }

      if (rosterModalMode === 'return' && rosterReason === 'other_to_admin' && !rosterNotes.trim()) {
        setActionError('Notes are required when the admin return reason is Other.')
        return
      }

      if (rosterModalMode === 'early_repeat') {
        const res = await api.returnClassStudentAsEarlyRepeat({
          lead_id: rosterStudent.lead_id,
          source_class_key: classKey,
          notes: rosterNotes,
        })
        await loadClass(true)
        closeRosterModal()
        setActionSuccess(
          `${rosterStudent.full_name} was marked as repeated ${formatSourceExitLabel(res.source_exit_after_session_number)} and returned to the admin feed.`
        )
        return
      }

      const res = await api.returnClassStudentToAdmin({
        lead_id: rosterStudent.lead_id,
        source_class_key: classKey,
        reason: rosterReason,
        notes: rosterNotes,
      })
      await loadClass(true)
      closeRosterModal()
      const queueLabel =
        res.reason === 'other_to_admin'
          ? 'other'
          : res.ops_queue_reason === 'private_track'
            ? 'private track'
            : 'refund review'
      setActionSuccess(
        `${rosterStudent.full_name} was removed from the class ${formatSourceExitLabel(res.source_exit_after_session_number)} and sent back to Admin for ${queueLabel}.`
      )
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to update class roster')
    } finally {
      setRosterSaving(false)
    }
  }

  function normalizeGrade(v?: { grade: string; notes: string }) {
    return {
      grade: v?.grade || '',
      notes: v?.notes || '',
    }
  }

  function getTargetGrade(leadId: string): string {
    const calculatedGrade = gradePreviews[leadId]?.calculated_grade || ''
    if (userRole === 'mentor_head') {
      return gradeDrafts[leadId]?.grade || calculatedGrade
    }
    return calculatedGrade
  }

  function isGradeChanged(leadId: string): boolean {
    const saved = normalizeGrade(grades[leadId])
    const draft = normalizeGrade(gradeDrafts[leadId])
    const targetGrade = getTargetGrade(leadId)
    return saved.grade !== targetGrade || saved.notes !== draft.notes
  }

  async function handleSaveAllGrades() {
    if (userRole !== 'mentor' && userRole !== 'mentor_head') return
    if (!allSessionsCompleted) {
      setActionError("Final Grading is locked until all sessions are completed (e.g. Session 8 finished).")
      return
    }
    if (classData?.class.round_status === 'closed') {
      setActionError('This class is archived. Grades are read-only.')
      return
    }

    try {
      setSavingAllGrades(true)
      setActionError(null)
      setActionSuccess(null)
      for (const student of classData?.students || []) {
        if (!gradePreviews[student.lead_id]) {
          throw new Error(`Missing calculated breakdown for ${student.full_name}`)
        }
        if (!isGradeChanged(student.lead_id)) continue
        const draft = normalizeGrade(gradeDrafts[student.lead_id])

        const targetGrade = getTargetGrade(student.lead_id)
        if (!targetGrade) continue
        if (countGradeNoteWords(draft.notes) < 10) {
          throw new Error(`Final grading note for ${student.full_name} must be at least 10 words.`)
        }

        await api.createGrade({
          lead_id: student.lead_id,
          class_key: classKey,
          grade: targetGrade,
          notes: draft.notes,
        })
      }

      await loadClass(true)
      setActionSuccess('Final grading saved successfully.')
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to save grades')
    } finally {
      setSavingAllGrades(false)
    }
  }

  function getNotesButtonLabel(notes: string) {
    const trimmed = notes.trim()
    if (!trimmed) return 'Add Notes'
    if (trimmed.length <= 28) return trimmed
    return `${trimmed.slice(0, 28)}...`
  }

  function countGradeNoteWords(notes: string) {
    const trimmed = notes.trim()
    return trimmed ? trimmed.split(/\s+/).length : 0
  }

  const overdueSessions = useMemo(() => {
    if (!classData) return []
    const now = new Date()
    return classData.sessions.filter((s) => {
      if (s.status === 'completed') return false
      if (!s.scheduled_time) return false

      const sessionDateTime = new Date(`${s.scheduled_date}T${s.scheduled_time}`)
      if (sessionDateTime.getHours() > 0 && sessionDateTime.getHours() < 12) {
        sessionDateTime.setHours(sessionDateTime.getHours() + 12)
      }
      sessionDateTime.setHours(sessionDateTime.getHours() + 2)
      const diffHours = (now.getTime() - sessionDateTime.getTime()) / (1000 * 60 * 60)

      if (diffHours > 24) {
        return classData.students.some((student) => !student.attendance?.[s.id])
      }
      return false
    })
  }, [classData])

  const allSessionsCompleted = useMemo(() => {
    if (!classData) return false
    // A class is ready for grading if it has sessions and they are all completed, 
    // or if it specifically reached at least 8 completed sessions.
    const completedCount = classData.sessions.filter(s => s.status === 'completed').length
    return completedCount >= 8 || (classData.sessions.length > 0 && classData.sessions.every(s => s.status === 'completed'))
  }, [classData])

  useEffect(() => {
    // Keep UI consistent with lock rule: grading tab is inaccessible
    // until all sessions are completed.
    if (!allSessionsCompleted && activeTab === 'grades') {
      setActiveTab('sessions')
    }
  }, [allSessionsCompleted, activeTab])

  const canEditGrades = userRole === 'mentor' || userRole === 'mentor_head'
  const canViewFeedbackCollected = userRole === 'mentor_head' || userRole === 'admin'
  const canOpenCompliance = userRole === 'student_success'

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
  const mentorPreStartLocked = userRole === 'mentor' && classData.class.round_status !== 'active'
  const firstSession = classData.sessions.find((s) => s.session_number === 1) || classData.sessions[0] || null
  const hasCompletedSessions = classData.sessions.some((s) => s.status === 'completed')
  const canShiftRoundStartDate = (userRole === 'mentor_head' || userRole === 'manager') && classData.class.round_status !== 'closed' && classData.sessions.length > 0
  const canRescheduleSelectedSession = (userRole === 'mentor_head' || userRole === 'manager') && !!selectedSession && selectedSession.status !== 'completed' && classData.class.round_status !== 'closed'
  const canManageRoster = (userRole === 'mentor_head' || userRole === 'admin' || userRole === 'manager') && classData.class.round_status === 'active'
  const allowedStartDays = classData.class.days === 'Sat/Tues'
    ? 'Saturday or Tuesday'
    : classData.class.days === 'Sun/Wed'
      ? 'Sunday or Wednesday'
      : classData.class.days === 'Mon/Thu'
        ? 'Monday or Thursday'
        : 'one of the class days'

  return (
    <div className="class-workspace">
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
          className="workspace-toast workspace-toast-error"
          style={{
            position: 'fixed',
            bottom: '24px',
            right: '24px',
            background: '#f8d7da',
            color: '#721c24',
            padding: '12px 20px',
            borderRadius: '8px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: '12px',
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
            zIndex: 9999,
          }}
        >
          <span>{actionError}</span>
          <button onClick={() => setActionError(null)} style={{ background: 'none', border: 'none', fontSize: '20px', cursor: 'pointer', color: '#721c24' }}>
            ×
          </button>
        </div>
      )}

      {actionSuccess && (
        <div
          className="workspace-toast workspace-toast-success"
          style={{
            position: 'fixed',
            bottom: '24px',
            right: '24px',
            background: '#d4edda',
            color: '#155724',
            padding: '12px 20px',
            borderRadius: '8px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: '12px',
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
            zIndex: 9999,
          }}
        >
          <span>{actionSuccess}</span>
          <button onClick={() => setActionSuccess(null)} style={{ background: 'none', border: 'none', fontSize: '20px', cursor: 'pointer', color: '#155724' }}>
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

      {mentorPreStartLocked && (
        <div
          style={{
            background: '#fff3cd',
            color: '#856404',
            padding: '12px 16px',
            borderRadius: '8px',
            marginBottom: '16px',
            border: '1px solid #ffeeba',
            fontWeight: 600,
          }}
        >
          Class is visible before round start. Attendance and session completion will unlock once Mentor Head starts the round.
        </div>
      )}

      <div className="workspace-tabs" style={{ display: 'flex', flexWrap: 'nowrap', overflowX: 'auto', overflowY: 'hidden', gap: '16px', borderBottom: '1px solid #dee2e6', marginBottom: '24px', width: '100%', WebkitOverflowScrolling: 'touch' }}>
        <button
          onClick={() => setActiveTab('sessions')}
          style={{
            flexShrink: 0,
            whiteSpace: 'nowrap',
            padding: '12px 0',
            background: 'none',
            border: 'none',
            borderBottom: activeTab === 'sessions' ? '3px solid #007bff' : '3px solid transparent',
            color: activeTab === 'sessions' ? '#007bff' : '#666',
            fontWeight: 600,
            cursor: 'pointer',
            fontSize: '16px',
            marginBottom: '-1px',
          }}
        >
          Sessions & Attendance
        </button>
        <button
          onClick={() => {
            if (allSessionsCompleted) {
              setActiveTab('grades')
            } else {
              setActionError("Final Grading is locked until all sessions are completed (e.g. Session 8 finished).")
            }
          }}
          style={{
            flexShrink: 0,
            whiteSpace: 'nowrap',
            padding: '12px 0',
            background: 'none',
            border: 'none',
            borderBottom: activeTab === 'grades' ? '3px solid #007bff' : '3px solid transparent',
            color: activeTab === 'grades' ? '#007bff' : '#666',
            fontWeight: 600,
            cursor: allSessionsCompleted ? 'pointer' : 'not-allowed',
            fontSize: '16px',
            marginBottom: '-1px',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            opacity: allSessionsCompleted ? 1 : 0.6,
          }}
          title={!allSessionsCompleted ? "Grading unlocks after all sessions are completed" : ""}
        >
          Final Grading
          {!allSessionsCompleted && <span style={{ fontSize: '12px' }}>🔒</span>}
        </button>
        {canViewFeedbackCollected && (
          <button
            onClick={() => setActiveTab('feedback_collected')}
          style={{
            flexShrink: 0,
            whiteSpace: 'nowrap',
            padding: '12px 0',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'feedback_collected' ? '3px solid #007bff' : '3px solid transparent',
              color: activeTab === 'feedback_collected' ? '#007bff' : '#666',
              fontWeight: 600,
              cursor: 'pointer',
              fontSize: '16px',
              marginBottom: '-1px',
            }}
          >
            Feedback Collected
          </button>
        )}
        {canOpenCompliance && (
          <button
            onClick={() => setComplianceOpen(true)}
            style={{
              flexShrink: 0,
              whiteSpace: 'nowrap',
              marginLeft: 'auto',
              padding: '8px 12px',
              borderRadius: '8px',
              border: '1px solid #198754',
              background: '#e9f7ef',
              color: '#198754',
              fontWeight: 700,
              cursor: 'pointer',
            }}
            title="Open mentor compliance checklist"
          >
            Compliance
          </button>
        )}
        {canShiftRoundStartDate && (
          <button
            onClick={handleOpenShiftStartModal}
            disabled={hasCompletedSessions || shiftStartSaving}
            style={{
              flexShrink: 0,
              whiteSpace: 'nowrap',
              marginLeft: 'auto',
              padding: '8px 12px',
              borderRadius: '8px',
              border: '1px solid #fd7e14',
              background: hasCompletedSessions ? '#f8f9fa' : '#fff4e5',
              color: hasCompletedSessions ? '#6c757d' : '#c65d00',
              fontWeight: 700,
              cursor: hasCompletedSessions ? 'not-allowed' : 'pointer',
              opacity: hasCompletedSessions ? 0.7 : 1,
            }}
            title={hasCompletedSessions ? 'Start date can only be changed before any session is completed.' : 'Move the full 8-session schedule by changing session 1 date.'}
          >
            Change Start Date
          </button>
        )}
      </div>

      {activeTab === 'sessions' && (
        <>
          <div style={{ marginBottom: '24px' }}>
            <div className="session-chip-row" style={{ display: 'flex', gap: '8px', flexWrap: 'nowrap', overflowX: 'auto', WebkitOverflowScrolling: 'touch', width: '100%', background: '#f8f9fa', padding: '12px', borderRadius: '12px' }}>
              {classData.sessions.map((s) => {
                const isSelected = s.session_number === selectedSessionNumber
                const statusColor = s.status === 'completed' ? '#28a745' : s.status === 'scheduled' ? '#007bff' : '#6c757d'
                return (
                  <button
                    key={s.id}
                    onClick={() => setSelectedSessionNumber(s.session_number)}
                    style={{
                      flexShrink: 0,
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'flex-start',
                      gap: '4px',
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
                    <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <span>S{s.session_number}</span>
                      <span style={{ fontSize: '10px', textTransform: 'uppercase', opacity: 0.8 }}>{s.status}</span>
                    </span>
                    <span style={{ fontSize: '12px', opacity: 0.92 }}>
                      {formatSessionDateLabel(s.scheduled_date)}
                    </span>
                    <span style={{ fontSize: '11px', opacity: 0.85 }}>
                      {formatSessionTimeLabel(s.scheduled_time)}
                    </span>
                  </button>
                )
              })}
            </div>
          </div>

          {selectedSession && (
            <div
              style={{
                marginBottom: '20px',
                padding: '14px 16px',
                borderRadius: '12px',
                border: '1px solid #e2e8f0',
                background: '#f8fafc',
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                gap: '16px',
                flexWrap: 'wrap',
              }}
            >
              <div>
                <div style={{ fontSize: '15px', fontWeight: 700, color: '#1f2937', marginBottom: '4px' }}>
                  Session {selectedSession.session_number} schedule
                </div>
                <div style={{ fontSize: '14px', color: '#475569' }}>
                  {formatSessionDateLabel(selectedSession.scheduled_date)} at {formatSessionTimeLabel(selectedSession.scheduled_time)}
                </div>
              </div>
              {canRescheduleSelectedSession && (
                <button
                  onClick={handleOpenRescheduleModal}
                  disabled={rescheduleSaving}
                  style={{
                    padding: '8px 12px',
                    borderRadius: '8px',
                    border: '1px solid #0d6efd',
                    background: 'white',
                    color: '#0d6efd',
                    fontWeight: 700,
                    cursor: 'pointer',
                  }}
                >
                  Reschedule Session
                </button>
              )}
            </div>
          )}

          {selectedSession && selectedSession.status === 'scheduled' && classData.class.round_status !== 'closed' && (
            <div style={{ marginBottom: '24px' }}>
              <button
                onClick={() => handleCompleteSession(selectedSession.id, selectedSession.session_number)}
                disabled={mentorPreStartLocked}
                style={{
                  padding: '10px 20px',
                  background: '#28a745',
                  color: 'white',
                  border: 'none',
                  borderRadius: '8px',
                  fontWeight: 600,
                  cursor: mentorPreStartLocked ? 'not-allowed' : 'pointer',
                  opacity: mentorPreStartLocked ? 0.7 : 1,
                }}
              >
                ✓ Complete Session {selectedSession.session_number}
              </button>
            </div>
          )}

          <div style={{ display: 'flex', gap: '20px', position: 'relative' }}>
            <div style={{ flex: 1 }}>
              <h2 style={{ fontSize: '18px', marginBottom: '16px' }}>Students</h2>
              <div className="workspace-student-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(250px, 1fr))', gap: '16px' }}>
                {classData.students.map((student) => {
                  const status = selectedSession ? student.attendance?.[selectedSession.id] : undefined
                  const perf = selectedSession ? student.session_performance?.[selectedSession.id] : undefined
                  const isNA = status === 'N/A'
                  const taskCompleted = perf?.task_completed ?? false
                  const participationScore = perf?.participation_score ?? 3
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
                            {student.joined_at_session_number && student.joined_at_session_number > 1 && (
                              <span style={{ background: '#6c5ce7', color: 'white', padding: '2px 6px', borderRadius: '4px', fontSize: '10px' }}>
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

                      {canManageRoster && (
                        <div style={{ display: 'flex', gap: '8px', marginBottom: '12px', flexWrap: 'wrap' }}>
                          <button
                            onClick={() => openTransferModal(student)}
                            style={{
                              padding: '6px 10px',
                              borderRadius: '8px',
                              border: '1px solid #0d6efd',
                              background: 'white',
                              color: '#0d6efd',
                              fontWeight: 700,
                              cursor: 'pointer',
                              fontSize: '12px',
                            }}
                          >
                            Transfer
                          </button>
                          <button
                            onClick={() => openReturnModal(student)}
                            style={{
                              padding: '6px 10px',
                              borderRadius: '8px',
                              border: '1px solid #dc3545',
                              background: 'white',
                              color: '#dc3545',
                              fontWeight: 700,
                              cursor: 'pointer',
                              fontSize: '12px',
                            }}
                          >
                            Return to Admin
                          </button>
                          {student.missed_count !== undefined && student.missed_count > 2 && (
                            <button
                              onClick={() => openEarlyRepeatModal(student)}
                              style={{
                                padding: '6px 10px',
                                borderRadius: '8px',
                                border: '1px solid #9b2c2c',
                                background: 'white',
                                color: '#9b2c2c',
                                fontWeight: 700,
                                cursor: 'pointer',
                                fontSize: '12px',
                              }}
                            >
                              Early Repeat
                            </button>
                          )}
                        </div>
                      )}

                      {selectedSession ? (
                        <div style={{ background: '#f8f9fa', padding: '12px', borderRadius: '8px', opacity: isUpdating ? 0.6 : 1 }}>
                          <div style={{ fontSize: '12px', color: '#666', marginBottom: '8px' }}>Session {selectedSession.session_number} Attendance</div>
                          <div style={{ display: 'flex', gap: '8px' }}>
                            <button
                              disabled={isUpdating || classData.class.round_status === 'closed' || mentorPreStartLocked}
                              onClick={() => handleMarkAttendance(selectedSession.id, student.lead_id, 'PRESENT', taskCompleted, participationScore)}
                              style={{
                                flex: 1,
                                padding: '8px',
                                borderRadius: '6px',
                                border: 'none',
                                background: status === 'PRESENT' ? '#28a745' : '#e9ecef',
                                color: status === 'PRESENT' ? 'white' : '#666',
                                fontWeight: 600,
                                cursor: classData.class.round_status === 'closed' || mentorPreStartLocked ? 'not-allowed' : 'pointer',
                                fontSize: '13px',
                                opacity: classData.class.round_status === 'closed' || mentorPreStartLocked ? 0.7 : 1,
                              }}
                            >
                              Present
                            </button>
                            <button
                              disabled={isUpdating || classData.class.round_status === 'closed' || mentorPreStartLocked}
                              onClick={() => handleMarkAttendance(selectedSession.id, student.lead_id, 'ABSENT', false, 3)}
                              style={{
                                flex: 1,
                                padding: '8px',
                                borderRadius: '6px',
                                border: 'none',
                                background: status === 'ABSENT' ? '#dc3545' : '#e9ecef',
                                color: status === 'ABSENT' ? 'white' : '#666',
                                fontWeight: 600,
                                cursor: classData.class.round_status === 'closed' || mentorPreStartLocked ? 'not-allowed' : 'pointer',
                                fontSize: '13px',
                                opacity: classData.class.round_status === 'closed' || mentorPreStartLocked ? 0.7 : 1,
                              }}
                            >
                              Absent
                            </button>
                          </div>

                          {selectedSession.session_number > 1 && (
                            <label style={{ display: 'flex', alignItems: 'center', gap: '8px', marginTop: '10px', fontSize: '13px', color: '#444' }}>
                              <input
                                type="checkbox"
                                checked={taskCompleted}
                                disabled={isUpdating || classData.class.round_status === 'closed' || mentorPreStartLocked || isNA}
                                onChange={(e) => handleSessionPerformanceChange(selectedSession.id, student.lead_id, e.target.checked, participationScore)}
                              />
                              Task Completed
                            </label>
                          )}

                          <div style={{ marginTop: '10px', display: 'flex', alignItems: 'center', gap: '6px' }}>
                            {[1, 2, 3, 4, 5].map((star) => {
                              const active = star <= participationScore
                              return (
                                <button
                                  key={star}
                                  disabled={isUpdating || classData.class.round_status === 'closed' || mentorPreStartLocked || isNA}
                                  onClick={() => handleSessionPerformanceChange(selectedSession.id, student.lead_id, taskCompleted, star)}
                                  style={{
                                    border: 'none',
                                    background: 'transparent',
                                    cursor: isNA || mentorPreStartLocked ? 'not-allowed' : 'pointer',
                                    color: active ? '#f59f00' : '#cbd5e0',
                                    fontSize: '18px',
                                    lineHeight: 1,
                                    padding: 0,
                                  }}
                                  title={`Participation: ${star} star${star > 1 ? 's' : ''}`}
                                >
                                  ★
                                </button>
                              )
                            })}
                            <span style={{ fontSize: '12px', color: '#666' }}>Participation</span>
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
        </>
      )}

      {activeTab === 'feedback_collected' && (
        <div>
          <div style={{ marginBottom: '12px', color: '#666', fontSize: '13px' }}>Feedback Collected is read-only for Mentor Head.</div>
          <FeedbackCollectedTab classKey={classKey} students={classData.students} canEdit={false} />
        </div>
      )}

      {activeTab === 'grades' && (
        <div className="workspace-grading" style={{ background: 'white', padding: '24px', borderRadius: '12px', border: '1px solid #dee2e6' }}>
          <h2 style={{ fontSize: '18px', marginBottom: '8px' }}>Final Class Grading</h2>
          <p style={{ color: '#666', marginBottom: '24px', fontSize: '14px' }}>
            Grades are calculated from attendance, tasks, and participation. Save commits the calculated values.
          </p>

          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '14px' }}>
            <div style={{ fontSize: '13px', color: '#666' }}>
              {classData.students.filter((s) => isGradeChanged(s.lead_id)).length} unsaved change(s)
            </div>
            {canEditGrades && (
              <button
                onClick={handleSaveAllGrades}
                disabled={savingAllGrades || classData.class.round_status === 'closed' || !allSessionsCompleted}
                style={{
                  padding: '8px 14px',
                  borderRadius: '8px',
                  border: 'none',
                  background: '#007bff',
                  color: 'white',
                  fontWeight: 700,
                  cursor: savingAllGrades ? 'not-allowed' : 'pointer',
                  opacity: savingAllGrades || classData.class.round_status === 'closed' || !allSessionsCompleted ? 0.7 : 1,
                }}
              >
                {savingAllGrades ? 'Saving...' : 'Save All Grades'}
              </button>
            )}
          </div>

          <div style={{ display: 'grid', gap: '16px' }}>
            {classData.students.map((student) => {
              const currentGrade = gradeDrafts[student.lead_id] || { grade: '', notes: '' }
              const preview = gradePreviews[student.lead_id]
              const calculatedGrade = preview?.calculated_grade || ''
              const targetGrade = getTargetGrade(student.lead_id)
              const isDirty = isGradeChanged(student.lead_id)

              return (
                <div
                  className="workspace-grade-row"
                  key={student.lead_id}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '20px',
                    padding: '16px',
                    borderRadius: '8px',
                    border: '1px solid #eee',
                    background: '#fafafa',
                  }}
                >
                  <div style={{ flex: 1 }}>
                    <div style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: '8px' }}>
                      {student.full_name}
                      {targetGrade && <span style={{ color: '#28a745', fontSize: '14px' }}>✅</span>}
                      {isDirty && <span style={{ color: '#f59f00', fontSize: '12px', fontWeight: 700 }}>Unsaved</span>}
                      {student.missed_count !== undefined && student.missed_count > 2 && (
                        <span
                          style={{
                            padding: '4px 8px',
                            background: '#f8d7da',
                            color: '#721c24',
                            borderRadius: '12px',
                            fontSize: '11px',
                            fontWeight: 600,
                          }}
                        >
                          {student.missed_count} missed
                        </span>
                      )}
                    </div>
                    <div style={{ fontSize: '12px', color: '#666' }}>{student.phone}</div>
                    {preview ? (
                      <div style={{ marginTop: '6px', fontSize: '12px', color: '#444' }}>
                        Attendance: {preview.attendance_score.toFixed(2)}/50 | Tasks: {preview.task_score.toFixed(2)}/40 | Part: {preview.participation_score.toFixed(2)}/10 = <strong>{calculatedGrade}</strong>
                      </div>
                    ) : (
                      <div style={{ marginTop: '6px', fontSize: '12px', color: '#b02a37' }}>
                        Missing calculation data for this student.
                      </div>
                    )}
                  </div>

                  <div className="workspace-grade-controls" style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                    {/*
                      Interaction gate:
                      - UI remains interactive for mentors/MH unless class is archived/closed.
                      - "all sessions completed" is enforced on save attempts, not via dead-looking disabled inputs.
                    */}
                    {(() => {
                      const canInteractGradeFields = !savingAllGrades && classData.class.round_status !== 'closed' && canEditGrades
                      return (
                        <>
                    {userRole === 'mentor_head' && (
                      <select
                        className="workspace-grade-select"
                        value={currentGrade.grade || calculatedGrade}
                        disabled={!canInteractGradeFields}
                        onChange={(e) => {
                          if (!canEditGrades) return
                          const nextGrade = e.target.value
                          if (!allSessionsCompleted) {
                            setActionError("Final Grading is locked until all sessions are completed (e.g. Session 8 finished).")
                            return
                          }
                          setGradeDrafts((prev) => ({ ...prev, [student.lead_id]: { ...currentGrade, grade: nextGrade } }))
                        }}
                        style={{
                          padding: '8px 12px',
                          borderRadius: '6px',
                          border: '1px solid #ccc',
                          background: canEditGrades ? 'white' : '#f5f5f5',
                          fontWeight: 600,
                          cursor: canEditGrades ? 'pointer' : 'not-allowed',
                        }}
                        title="Mentor Head can override calculated grade"
                      >
                        <option value="A">Grade A</option>
                        <option value="B">Grade B</option>
                        <option value="C">Grade C</option>
                        <option value="F">Grade F</option>
                      </select>
                    )}

                    <button
                      type="button"
                      className="workspace-grade-note-input"
                      disabled={!canInteractGradeFields}
                      onClick={() => {
                        if (!canEditGrades) return
                        setGradeNotesStudent(student)
                      }}
                      style={{
                        padding: '8px 12px',
                        borderRadius: '6px',
                        border: '1px solid #ccc',
                        width: '240px',
                        fontSize: '14px',
                        background: canEditGrades ? 'white' : '#f5f5f5',
                        textAlign: 'left',
                        color: currentGrade.notes.trim() ? '#111827' : '#6b7280',
                        cursor: canInteractGradeFields ? 'pointer' : 'not-allowed',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                      title={currentGrade.notes.trim() ? currentGrade.notes : 'Add final notes'}
                    >
                      {getNotesButtonLabel(currentGrade.notes)}
                    </button>

                    <button
                      onClick={() => handleOpenReport(student.lead_id)}
                      disabled={reportLoading}
                      style={{
                        padding: '8px 12px',
                        borderRadius: '6px',
                        border: '1px solid #cbd5e1',
                        background: 'white',
                        cursor: 'pointer',
                        fontWeight: 600,
                      }}
                    >
                      View Report
                    </button>

                    {savingAllGrades && <span style={{ fontSize: '12px', color: '#666' }}>Saving...</span>}
                    {!canEditGrades && <span style={{ fontSize: '12px', color: '#999' }}>Read-only</span>}
                        </>
                      )
                    })()}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}

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

      {shiftStartModalOpen && (
        <div
          onClick={() => {
            if (shiftStartSaving) return
            setShiftStartModalOpen(false)
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
              width: '480px',
              maxWidth: '100%',
              boxShadow: '0 12px 30px rgba(0,0,0,0.2)',
            }}
          >
            <h3 style={{ marginTop: 0, marginBottom: '8px' }}>Change class start date</h3>
            <p style={{ marginTop: 0, marginBottom: '16px', color: '#555' }}>
              This shifts the full 8-session schedule for this class. It is allowed even if session 1 date already passed, as long as no session has been completed yet.
            </p>
            {firstSession && (
              <div style={{ background: '#f8f9fa', borderRadius: '8px', padding: '12px', marginBottom: '16px', fontSize: '14px', color: '#444' }}>
                <div><strong>Current session 1 date:</strong> {firstSession.scheduled_date}</div>
                <div><strong>Allowed weekdays:</strong> {allowedStartDays} for {classData.class.days} classes</div>
              </div>
            )}
            <label style={{ display: 'block', fontWeight: 600, marginBottom: '8px' }}>New first-session date</label>
            <input
              type="date"
              value={shiftStartDate}
              onChange={(e) => setShiftStartDate(e.target.value)}
              disabled={shiftStartSaving}
              style={{
                width: '100%',
                padding: '10px 12px',
                borderRadius: '8px',
                border: '1px solid #ced4da',
                fontSize: '14px',
                marginBottom: '16px',
              }}
            />
            <div style={{ fontSize: '13px', color: '#666', marginBottom: '20px' }}>
              The backend will reject dates that do not land on one of the class days.
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px' }}>
              <button
                onClick={() => setShiftStartModalOpen(false)}
                disabled={shiftStartSaving}
                style={{
                  padding: '10px 16px',
                  borderRadius: '8px',
                  border: '1px solid #ced4da',
                  background: 'white',
                  cursor: shiftStartSaving ? 'not-allowed' : 'pointer',
                }}
              >
                Cancel
              </button>
              <button
                onClick={handleShiftRoundStartDate}
                disabled={shiftStartSaving || !shiftStartDate}
                style={{
                  padding: '10px 16px',
                  borderRadius: '8px',
                  border: 'none',
                  background: '#fd7e14',
                  color: 'white',
                  fontWeight: 700,
                  cursor: shiftStartSaving || !shiftStartDate ? 'not-allowed' : 'pointer',
                  opacity: shiftStartSaving || !shiftStartDate ? 0.7 : 1,
                }}
              >
                {shiftStartSaving ? 'Changing...' : 'Change Start Date'}
              </button>
            </div>
          </div>
        </div>
      )}

      {rescheduleModalOpen && selectedSession && (
        <div
          onClick={() => {
            if (rescheduleSaving) return
            setRescheduleModalOpen(false)
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
              width: '480px',
              maxWidth: '100%',
              boxShadow: '0 12px 30px rgba(0,0,0,0.2)',
            }}
          >
            <h3 style={{ marginTop: 0, marginBottom: '8px' }}>Reschedule session {selectedSession.session_number}</h3>
            <p style={{ marginTop: 0, marginBottom: '16px', color: '#555' }}>
              Change only this session&apos;s date and time. Completed sessions stay locked.
            </p>
            <div style={{ background: '#f8f9fa', borderRadius: '8px', padding: '12px', marginBottom: '16px', fontSize: '14px', color: '#444' }}>
              <div><strong>Current date:</strong> {selectedSession.scheduled_date}</div>
              <div><strong>Current time:</strong> {formatSessionTimeLabel(selectedSession.scheduled_time)}</div>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px', marginBottom: '20px' }}>
              <div>
                <label style={{ display: 'block', fontWeight: 600, marginBottom: '8px' }}>New date</label>
                <input
                  type="date"
                  value={rescheduleDate}
                  onChange={(e) => setRescheduleDate(e.target.value)}
                  disabled={rescheduleSaving}
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: '8px',
                    border: '1px solid #ced4da',
                    fontSize: '14px',
                  }}
                />
              </div>
              <div>
                <label style={{ display: 'block', fontWeight: 600, marginBottom: '8px' }}>New time</label>
                <input
                  type="time"
                  value={rescheduleTime}
                  onChange={(e) => setRescheduleTime(e.target.value)}
                  disabled={rescheduleSaving}
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: '8px',
                    border: '1px solid #ced4da',
                    fontSize: '14px',
                  }}
                />
              </div>
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px' }}>
              <button
                onClick={() => setRescheduleModalOpen(false)}
                disabled={rescheduleSaving}
                style={{
                  padding: '10px 16px',
                  borderRadius: '8px',
                  border: '1px solid #ced4da',
                  background: 'white',
                  cursor: rescheduleSaving ? 'not-allowed' : 'pointer',
                }}
              >
                Cancel
              </button>
              <button
                onClick={handleRescheduleSession}
                disabled={rescheduleSaving || !rescheduleDate || !rescheduleTime}
                style={{
                  padding: '10px 16px',
                  borderRadius: '8px',
                  border: 'none',
                  background: '#0d6efd',
                  color: 'white',
                  fontWeight: 700,
                  cursor: rescheduleSaving || !rescheduleDate || !rescheduleTime ? 'not-allowed' : 'pointer',
                  opacity: rescheduleSaving || !rescheduleDate || !rescheduleTime ? 0.7 : 1,
                }}
              >
                {rescheduleSaving ? 'Saving...' : 'Save Session Date'}
              </button>
            </div>
          </div>
        </div>
      )}

      {rosterModalMode && rosterStudent && (
        <div
          onClick={closeRosterModal}
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(0,0,0,0.45)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 4000,
            padding: '16px',
          }}
        >
          <div
            onClick={(e) => e.stopPropagation()}
            style={{
              background: 'white',
              borderRadius: '12px',
              padding: '24px',
              width: '560px',
              maxWidth: '100%',
              boxShadow: '0 12px 30px rgba(0,0,0,0.2)',
            }}
          >
            <h3 style={{ marginTop: 0, marginBottom: '8px' }}>
              {rosterModalMode === 'transfer'
                ? 'Transfer Student'
                : rosterModalMode === 'early_repeat'
                  ? 'Return Student as Early Repeat'
                  : 'Return Student to Admin'}
            </h3>
            <p style={{ marginTop: 0, marginBottom: '16px', color: '#555' }}>
              <strong>{rosterStudent.full_name}</strong> will leave this class starting with its next uncompleted session.
              {rosterModalMode === 'transfer'
                ? ' The target class will receive the student on its own current session number.'
                : rosterModalMode === 'early_repeat'
                  ? ' They will be marked as repeated, treated as a returning student, and sent back to the admin feed.'
                  : ' Past attendance stays on this class.'}
            </p>

            {rosterModalMode === 'transfer' ? (
              <>
                <div style={{ marginBottom: '16px' }}>
                  <label style={{ display: 'block', fontWeight: 600, marginBottom: '8px' }}>Target class</label>
                  {transferOptionsLoading ? (
                    <div style={{ padding: '12px', borderRadius: '8px', background: '#f8f9fa', color: '#555' }}>Loading available classes...</div>
                  ) : transferOptions.length > 0 ? (
                    <select
                      value={rosterTargetClassKey}
                      onChange={(e) => setRosterTargetClassKey(e.target.value)}
                      disabled={rosterSaving}
                      style={{
                        width: '100%',
                        padding: '10px 12px',
                        borderRadius: '8px',
                        border: '1px solid #ced4da',
                        fontSize: '14px',
                      }}
                    >
                      {transferOptions.map((option) => (
                        <option key={option.class_key} value={option.class_key}>
                          {`L${option.level} · ${option.class_days} · ${option.class_time} · Class ${option.class_number} · S${option.current_session} · ${option.current_enrollment}/6`}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <div style={{ padding: '12px', borderRadius: '8px', background: '#fff3cd', color: '#856404' }}>
                      No eligible target classes are available right now.
                    </div>
                  )}
                </div>
                <div style={{ marginBottom: '16px' }}>
                  <label style={{ display: 'block', fontWeight: 600, marginBottom: '8px' }}>Reason</label>
                  <select
                    value={rosterReason}
                    onChange={(e) => setRosterReason(e.target.value)}
                    disabled={rosterSaving}
                    style={{
                      width: '100%',
                      padding: '10px 12px',
                      borderRadius: '8px',
                      border: '1px solid #ced4da',
                      fontSize: '14px',
                    }}
                  >
                    <option value="schedule_change">Schedule change</option>
                    <option value="promotion">Promotion</option>
                    <option value="demotion">Demotion</option>
                    <option value="other">Other</option>
                  </select>
                </div>
              </>
            ) : rosterModalMode === 'return' ? (
              <div style={{ marginBottom: '16px' }}>
                <label style={{ display: 'block', fontWeight: 600, marginBottom: '8px' }}>Admin queue</label>
                <select
                  value={rosterReason}
                  onChange={(e) => setRosterReason(e.target.value)}
                  disabled={rosterSaving}
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: '8px',
                    border: '1px solid #ced4da',
                    fontSize: '14px',
                  }}
                >
                  <option value="refund_to_admin">Refund review</option>
                  <option value="private_track_to_admin">Private track</option>
                  <option value="other_to_admin">Other</option>
                </select>
              </div>
            ) : (
              <div style={{ marginBottom: '16px', padding: '12px 14px', borderRadius: '10px', background: '#fff4e6', color: '#8c4a00', border: '1px solid #ffd8a8' }}>
                This action is only for students with more than 2 missed sessions. It records the student as repeated and returns them to Admin for the same level.
              </div>
            )}

            <div style={{ marginBottom: '20px' }}>
              <label style={{ display: 'block', fontWeight: 600, marginBottom: '8px' }}>Notes</label>
              <textarea
                value={rosterNotes}
                onChange={(e) => setRosterNotes(e.target.value)}
                disabled={rosterSaving}
                rows={3}
                style={{
                  width: '100%',
                  padding: '10px 12px',
                  borderRadius: '8px',
                  border: '1px solid #ced4da',
                  fontSize: '14px',
                  resize: 'vertical',
                }}
                placeholder={
                  rosterModalMode === 'return' && rosterReason === 'other_to_admin'
                    ? 'Required note for the admin return'
                    : rosterModalMode === 'early_repeat'
                      ? 'Optional note for the early repeat return'
                      : 'Optional note for the roster change'
                }
              />
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px' }}>
              <button
                onClick={closeRosterModal}
                disabled={rosterSaving}
                style={{
                  padding: '10px 16px',
                  borderRadius: '8px',
                  border: '1px solid #ced4da',
                  background: 'white',
                  cursor: rosterSaving ? 'not-allowed' : 'pointer',
                }}
              >
                Cancel
              </button>
              <button
                onClick={handleSaveRosterChange}
                disabled={
                  rosterSaving ||
                  (rosterModalMode === 'transfer' && (!rosterTargetClassKey || transferOptionsLoading)) ||
                  (rosterModalMode === 'return' && rosterReason === 'other_to_admin' && !rosterNotes.trim())
                }
                style={{
                  padding: '10px 16px',
                  borderRadius: '8px',
                  border: 'none',
                  background: rosterModalMode === 'transfer' ? '#0d6efd' : '#dc3545',
                  color: 'white',
                  fontWeight: 700,
                  cursor:
                    rosterSaving ||
                    (rosterModalMode === 'transfer' && (!rosterTargetClassKey || transferOptionsLoading)) ||
                    (rosterModalMode === 'return' && rosterReason === 'other_to_admin' && !rosterNotes.trim())
                      ? 'not-allowed'
                      : 'pointer',
                  opacity:
                    rosterSaving ||
                    (rosterModalMode === 'transfer' && (!rosterTargetClassKey || transferOptionsLoading)) ||
                    (rosterModalMode === 'return' && rosterReason === 'other_to_admin' && !rosterNotes.trim())
                      ? 0.7
                      : 1,
                }}
              >
                {rosterSaving
                  ? 'Saving...'
                  : rosterModalMode === 'transfer'
                    ? 'Transfer Student'
                    : rosterModalMode === 'early_repeat'
                      ? 'Return as Early Repeat'
                      : 'Send to Admin'}
              </button>
            </div>
          </div>
        </div>
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
      {gradeNotesStudent && (
        <GradeNotesModal
          open={true}
          studentName={gradeNotesStudent.full_name}
          value={gradeDrafts[gradeNotesStudent.lead_id]?.notes || ''}
          onChange={(value) => {
            setGradeDrafts((prev) => {
              const currentGrade = prev[gradeNotesStudent.lead_id] || grades[gradeNotesStudent.lead_id] || { grade: '', notes: '' }
              return { ...prev, [gradeNotesStudent.lead_id]: { ...currentGrade, notes: value } }
            })
          }}
          onClose={() => setGradeNotesStudent(null)}
          canEdit={!savingAllGrades && classData?.class.round_status !== 'closed' && canEditGrades}
        />
      )}
      {canOpenCompliance && (
        <ComplianceModal open={complianceOpen} classKey={classKey} onClose={() => setComplianceOpen(false)} />
      )}
    </div>
  )
}
