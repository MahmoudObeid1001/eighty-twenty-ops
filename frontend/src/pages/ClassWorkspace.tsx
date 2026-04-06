import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, ClassDetail, GradePreview, Student, StudentReportCardData } from '../api/client'
import StudentModal from '../components/StudentModal'
import FeedbackCollectedTab from '../components/FeedbackCollectedTab'
import ComplianceModal from '../components/ComplianceModal'
import StudentReportCard from '../components/StudentReportCard'

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
  const [shiftStartModalOpen, setShiftStartModalOpen] = useState(false)
  const [shiftStartDate, setShiftStartDate] = useState('')
  const [shiftStartSaving, setShiftStartSaving] = useState(false)

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

      <div className="workspace-tabs" style={{ display: 'flex', gap: '24px', borderBottom: '1px solid #dee2e6', marginBottom: '24px' }}>
        <button
          onClick={() => setActiveTab('sessions')}
          style={{
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
            <div className="session-chip-row" style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', background: '#f8f9fa', padding: '12px', borderRadius: '12px' }}>
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
              <div className="workspace-student-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: '16px' }}>
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
                            {student.joined_at_session_number && (
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

                    <input
                      className="workspace-grade-note-input"
                      type="text"
                      placeholder="Add final notes..."
                      value={currentGrade.notes}
                      disabled={!canInteractGradeFields}
                      onChange={(e) => {
                        if (!canEditGrades) return
                        setGradeDrafts((prev) => ({ ...prev, [student.lead_id]: { ...currentGrade, notes: e.target.value } }))
                      }}
                      style={{
                        padding: '8px 12px',
                        borderRadius: '6px',
                        border: '1px solid #ccc',
                        width: '240px',
                        fontSize: '14px',
                        background: canEditGrades ? 'white' : '#f5f5f5',
                      }}
                    />

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
              This shifts the full 8-session schedule for this class. It is only allowed before any session is completed.
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
      {canOpenCompliance && (
        <ComplianceModal open={complianceOpen} classKey={classKey} onClose={() => setComplianceOpen(false)} />
      )}
    </div>
  )
}
