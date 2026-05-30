import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api, type AbsencePromotionOverrideItem, type MentorAvailabilityWarning, type MentorHeadDashboard as MentorHeadDashboardData, MentorHeadClass } from '../api/client'
import MentorHeadComplaints from '../components/MentorHeadComplaints'

function availabilityWarningSummary(warnings?: MentorAvailabilityWarning[]) {
  if (!warnings || warnings.length === 0) return ''
  const sessions = warnings.map((w) => `S${w.session_number}`).join(', ')
  return `Availability warning for ${sessions}.`
}

export default function MentorHeadDashboard() {
  const [dashboard, setDashboard] = useState<MentorHeadDashboardData | null>(null)
  const [archive, setArchive] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<'active' | 'archive' | 'complaints'>('active')
  const [archiveSort, setArchiveSort] = useState<'oldest' | 'newest'>('oldest')
  const [archiveFrom, setArchiveFrom] = useState('')
  const [archiveTo, setArchiveTo] = useState('')
  const [collapsedMentors, setCollapsedMentors] = useState<Set<string>>(new Set())
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [assigning, setAssigning] = useState<string | null>(null)
  const [actioning, setActioning] = useState<string | null>(null)
  const [cardError, setCardError] = useState<Record<string, string>>({}) // per-class_key error (e.g. 409)
  const [selectedMentorIds, setSelectedMentorIds] = useState<Record<string, string>>({})
  const [shiftMentorIds, setShiftMentorIds] = useState<Record<string, string>>({})
  const [shiftReasons, setShiftReasons] = useState<Record<string, string>>({})
  const [checkingAvailability, setCheckingAvailability] = useState<string | null>(null)
  const [availabilityWarnings, setAvailabilityWarnings] = useState<Record<string, MentorAvailabilityWarning[]>>({})
  const [closeConfirm, setCloseConfirm] = useState<{ open: boolean; classKey: string | null }>({
    open: false,
    classKey: null,
  })
  const [closeOverrideItems, setCloseOverrideItems] = useState<AbsencePromotionOverrideItem[]>([])
  const [closeOverridesLoading, setCloseOverridesLoading] = useState(false)
  const [overrideActioning, setOverrideActioning] = useState<string | null>(null)
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const requestedTab = searchParams.get('tab')
  const openComplaintId = searchParams.get('complaint_id') || undefined

  useEffect(() => {
    if (requestedTab === 'complaints') {
      setActiveTab('complaints')
    }
  }, [requestedTab])

  useEffect(() => {
    loadData()
  }, [activeTab, archiveSort, archiveFrom, archiveTo])

  useEffect(() => {
    if (!message) {
      return
    }
    const timeoutId = window.setTimeout(() => {
      setMessage(null)
    }, 5000)
    return () => window.clearTimeout(timeoutId)
  }, [message])

  async function loadData() {
    try {
      setLoading(true)
      setError(null)
      if (activeTab === 'active') {
        const data = await api.getMentorHeadDashboard()
        setDashboard(data)
      } else if (activeTab === 'archive') {
        const archivedData = await api.getMentorHeadArchive(
          archiveSort,
          archiveFrom || undefined,
          archiveTo || undefined,
        )
        setArchive(archivedData)
      }
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

  async function handleMentorSelection(classKey: string, mentorUserId: string) {
    setSelectedMentorIds((prev) => ({ ...prev, [classKey]: mentorUserId }))
    setAvailabilityWarnings((prev) => ({ ...prev, [classKey]: [] }))
    clearCardError(classKey)
    if (!mentorUserId) return
    try {
      setCheckingAvailability(classKey)
      const res = await api.checkMentorAvailability(classKey, mentorUserId)
      setAvailabilityWarnings((prev) => ({ ...prev, [classKey]: res.availability_warnings || [] }))
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to check mentor availability'
      setCardError((prev) => ({ ...prev, [classKey]: msg }))
    } finally {
      setCheckingAvailability(null)
    }
  }

  async function handleAssignMentor(classKey: string, mentorUserId: string) {
    try {
      setAssigning(classKey)
      setMessage(null)
      clearCardError(classKey)
      const res = await api.assignMentor(classKey, mentorUserId)
      const warningSummary = availabilityWarningSummary(res.availability_warnings)
      setAvailabilityWarnings((prev) => ({ ...prev, [classKey]: res.availability_warnings || [] }))
      setMessage({ type: 'success', text: warningSummary ? `Mentor assigned. ${warningSummary} Assignment is still allowed.` : 'Mentor assigned successfully' })
      await loadData()
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to assign mentor'
      setCardError((prev) => ({ ...prev, [classKey]: msg }))
    } finally {
      setAssigning(null)
    }
  }

  async function handleShiftMentor(cls: MentorHeadClass) {
    const classKey = cls.class_key
    const mentorUserId = shiftMentorIds[classKey]
    const reason = (shiftReasons[classKey] || '').trim()
    if (!mentorUserId) {
      setCardError((prev) => ({ ...prev, [classKey]: 'Select the new mentor.' }))
      return
    }
    if (!reason) {
      setCardError((prev) => ({ ...prev, [classKey]: 'Enter the reason for shifting this class.' }))
      return
    }
    try {
      setActioning(`${classKey}:shift`)
      setMessage(null)
      clearCardError(classKey)
      const res = await api.shiftMentor(classKey, mentorUserId, reason, cls.next_session_number)
      const warningSummary = availabilityWarningSummary(res.availability_warnings)
      setAvailabilityWarnings((prev) => ({ ...prev, [classKey]: res.availability_warnings || [] }))
      setMessage({
        type: 'success',
        text: warningSummary
          ? `Mentor shifted from session ${res.effective_session_number}. ${warningSummary} Shift is still allowed.`
          : `Mentor shifted from session ${res.effective_session_number}.`,
      })
      setShiftMentorIds((prev) => ({ ...prev, [classKey]: '' }))
      setShiftReasons((prev) => ({ ...prev, [classKey]: '' }))
      await loadData()
    } catch (err) {
      setCardError((prev) => ({ ...prev, [classKey]: err instanceof Error ? err.message : 'Failed to shift mentor' }))
    } finally {
      setActioning(null)
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
      const res = await api.startRound(classKey)
      const warningSummary = availabilityWarningSummary(res.availability_warnings)
      setMessage({ type: 'success', text: warningSummary ? `Round started. ${warningSummary}` : 'Round started successfully' })
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

  async function openCloseConfirm(classKey: string) {
    setCloseConfirm({ open: true, classKey })
    setCloseOverrideItems([])
    setCloseOverridesLoading(true)
    try {
      const res = await api.getAbsencePromotionOverrides(classKey)
      setCloseOverrideItems(res.items || [])
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to load absence override requests' })
    } finally {
      setCloseOverridesLoading(false)
    }
  }

  function closeCloseConfirm() {
    setCloseConfirm({ open: false, classKey: null })
    setCloseOverrideItems([])
  }

  async function handleReviewOverride(item: AbsencePromotionOverrideItem, status: 'approved' | 'rejected') {
    if (!closeConfirm.classKey) return
    const note = window.prompt(status === 'approved' ? 'Optional approval note:' : 'Optional rejection note:') || ''
    try {
      setOverrideActioning(`${item.lead_id}:${status}`)
      await api.reviewAbsencePromotionOverride({
        lead_id: item.lead_id,
        class_key: closeConfirm.classKey,
        status,
        review_note: note.trim(),
      })
      const res = await api.getAbsencePromotionOverrides(closeConfirm.classKey)
      setCloseOverrideItems(res.items || [])
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to review absence override' })
    } finally {
      setOverrideActioning(null)
    }
  }

  async function confirmCloseRound() {
    if (!closeConfirm.classKey) {
      closeCloseConfirm()
      return
    }
    await handleCloseRound(closeConfirm.classKey)
    closeCloseConfirm()
  }

  async function handleReopenRound(classKey: string) {
    try {
      setActioning(`${classKey}:reopen`)
      setMessage(null)
      await api.reopenRound(classKey)
      setMessage({ type: 'success', text: 'Class reopened successfully' })
      await loadData()
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to reopen class' })
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

  if (!dashboard && activeTab === 'active') {
    return (
      <div style={{ padding: '40px', textAlign: 'center' }}>
        <p>No data available.</p>
      </div>
    )
  }

  const toggleMentorCollapse = (mentorEmail: string) => {
    setCollapsedMentors(prev => {
      const next = new Set(prev)
      if (next.has(mentorEmail)) next.delete(mentorEmail)
      else next.add(mentorEmail)
      return next
    })
  }

  const groups = dashboard ? groupClassesByMentor(dashboard.classes) : []
  const pendingCloseOverrides = closeOverrideItems.filter((item) => item.status === 'pending')

  return (
    <div>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <h1>Mentor Head Dashboard</h1>
      </div>

      <div style={{ display: 'flex', gap: '20px', marginBottom: '24px', borderBottom: '1px solid #ddd' }}>
        <button
          onClick={() => setActiveTab('active')}
          style={{
            padding: '10px 20px',
            background: 'none',
            border: 'none',
            borderBottom: activeTab === 'active' ? '3px solid #007bff' : '3px solid transparent',
            color: activeTab === 'active' ? '#007bff' : '#666',
            fontWeight: activeTab === 'active' ? 600 : 400,
            cursor: 'pointer',
            fontSize: '16px'
          }}
        >
          Active Classes
        </button>
        <button
          onClick={() => setActiveTab('archive')}
          style={{
            padding: '10px 20px',
            background: 'none',
            border: 'none',
            borderBottom: activeTab === 'archive' ? '3px solid #007bff' : '3px solid transparent',
            color: activeTab === 'archive' ? '#007bff' : '#666',
            fontWeight: activeTab === 'archive' ? 600 : 400,
            cursor: 'pointer',
            fontSize: '16px'
          }}
        >
          Closed Classes (Archive)
        </button>
        <button
          onClick={() => setActiveTab('complaints')}
          style={{
            padding: '10px 20px',
            background: 'none',
            border: 'none',
            borderBottom: activeTab === 'complaints' ? '3px solid #dc3545' : '3px solid transparent',
            color: activeTab === 'complaints' ? '#dc3545' : '#666',
            fontWeight: activeTab === 'complaints' ? 600 : 400,
            cursor: 'pointer',
            fontSize: '16px'
          }}
        >
          Complaints
        </button>
      </div>

      {activeTab === 'archive' && (
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '16px',
            gap: '12px',
            flexWrap: 'wrap'
          }}
        >
          <div style={{ display: 'flex', gap: '12px', alignItems: 'center' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <label style={{ fontSize: '13px', color: '#666' }}>From</label>
              <input
                type="date"
                value={archiveFrom}
                onChange={(e) => setArchiveFrom(e.target.value)}
                style={{
                  padding: '6px 8px',
                  borderRadius: '6px',
                  border: '1px solid #ddd',
                  fontSize: '13px'
                }}
              />
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <label style={{ fontSize: '13px', color: '#666' }}>To</label>
              <input
                type="date"
                value={archiveTo}
                onChange={(e) => setArchiveTo(e.target.value)}
                style={{
                  padding: '6px 8px',
                  borderRadius: '6px',
                  border: '1px solid #ddd',
                  fontSize: '13px'
                }}
              />
            </div>
            {(archiveFrom || archiveTo) && (
              <button
                onClick={() => {
                  setArchiveFrom('')
                  setArchiveTo('')
                }}
                style={{
                  padding: '6px 10px',
                  borderRadius: '6px',
                  border: '1px solid #ddd',
                  background: '#fff',
                  cursor: 'pointer',
                  fontSize: '13px'
                }}
              >
                Clear
              </button>
            )}
          </div>
          <div style={{ background: '#f8f9fa', padding: '4px', borderRadius: '8px', border: '1px solid #ddd' }}>
            <button
              onClick={() => setArchiveSort('oldest')}
              style={{
                padding: '4px 12px',
                background: archiveSort === 'oldest' ? 'white' : 'transparent',
                border: 'none',
                borderRadius: '6px',
                boxShadow: archiveSort === 'oldest' ? '0 2px 4px rgba(0,0,0,0.1)' : 'none',
                cursor: 'pointer',
                fontSize: '13px',
                fontWeight: archiveSort === 'oldest' ? 600 : 400
              }}
            >
              Oldest
            </button>
            <button
              onClick={() => setArchiveSort('newest')}
              style={{
                padding: '4px 12px',
                background: archiveSort === 'newest' ? 'white' : 'transparent',
                border: 'none',
                borderRadius: '6px',
                boxShadow: archiveSort === 'newest' ? '0 2px 4px rgba(0,0,0,0.1)' : 'none',
                cursor: 'pointer',
                fontSize: '13px',
                fontWeight: archiveSort === 'newest' ? 600 : 400
              }}
            >
              Newest
            </button>
          </div>
        </div>
      )}

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

      {activeTab === 'active' ? (
        groups.length === 0 ? (
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
                          {cls.suggested_start_date && ` · Start date: ${cls.suggested_start_date}`}
                        </p>
                        <p style={{ color: '#666', fontSize: '13px', marginBottom: '4px' }}>
                          {cls.student_count} student{cls.student_count !== 1 ? 's' : ''}
                          {!(cls.sent_to_mentor && cls.readiness === 'NOT READY') && ` · ${cls.readiness}`}
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
                            value={selectedMentorIds[cls.class_key] || ''}
                            onChange={(e) => void handleMentorSelection(cls.class_key, e.target.value)}
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
                            {dashboard?.mentors.map((m) => (
                              <option key={m.id} value={m.id}>
                                {m.email}
                              </option>
                            ))}
                          </select>
                          {checkingAvailability === cls.class_key && (
                            <div style={{ marginBottom: '6px', color: '#666', fontSize: '12px' }}>Checking availability...</div>
                          )}
                          {availabilityWarnings[cls.class_key]?.length > 0 && (
                            <div
                              style={{
                                marginBottom: '8px',
                                padding: '8px',
                                background: '#fff3bf',
                                color: '#5f3dc4',
                                borderRadius: '4px',
                                fontSize: '12px',
                              }}
                            >
                              <div style={{ fontWeight: 700, marginBottom: '4px' }}>Availability warning</div>
                              {availabilityWarnings[cls.class_key].slice(0, 3).map((warning) => (
                                <div key={`${warning.session_number}:${warning.scheduled_date}:${warning.code}`}>
                                  S{warning.session_number} · {warning.scheduled_date} · {warning.start_time}-{warning.end_time}
                                </div>
                              ))}
                              {availabilityWarnings[cls.class_key].length > 3 && (
                                <div>+{availabilityWarnings[cls.class_key].length - 3} more sessions</div>
                              )}
                            </div>
                          )}
                          <button
                            onClick={() => {
                              const mentorUserId = selectedMentorIds[cls.class_key]
                              if (mentorUserId) {
                                handleAssignMentor(cls.class_key, mentorUserId)
                              }
                            }}
                            disabled={assigning === cls.class_key || !selectedMentorIds[cls.class_key]}
                            style={{
                              width: '100%',
                              padding: '6px',
                              background: assigning === cls.class_key || !selectedMentorIds[cls.class_key] ? '#ccc' : '#007bff',
                              color: 'white',
                              border: 'none',
                              borderRadius: '4px',
                              cursor: assigning === cls.class_key || !selectedMentorIds[cls.class_key] ? 'not-allowed' : 'pointer',
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
                          {cls.round_status === 'active' && (
                            <div style={{ marginBottom: '8px', padding: '8px', background: '#f8f9fa', borderRadius: '4px' }}>
                              <div style={{ fontSize: '12px', fontWeight: 700, marginBottom: '6px' }}>
                                Shift from session {cls.next_session_number || '-'}
                              </div>
                              <select
                                value={shiftMentorIds[cls.class_key] || ''}
                                onChange={(e) => {
                                  setShiftMentorIds((prev) => ({ ...prev, [cls.class_key]: e.target.value }))
                                  clearCardError(cls.class_key)
                                }}
                                style={{
                                  width: '100%',
                                  padding: '6px',
                                  border: '1px solid #ddd',
                                  borderRadius: '4px',
                                  fontSize: '13px',
                                  marginBottom: '6px',
                                }}
                              >
                                <option value="">New mentor...</option>
                                {dashboard?.mentors
                                  .filter((m) => m.id !== cls.mentor_user_id)
                                  .map((m) => (
                                    <option key={m.id} value={m.id}>
                                      {m.email}
                                    </option>
                                  ))}
                              </select>
                              <textarea
                                value={shiftReasons[cls.class_key] || ''}
                                onChange={(e) => {
                                  setShiftReasons((prev) => ({ ...prev, [cls.class_key]: e.target.value }))
                                  clearCardError(cls.class_key)
                                }}
                                placeholder="Reason for shift"
                                rows={2}
                                style={{
                                  width: '100%',
                                  padding: '6px',
                                  border: '1px solid #ddd',
                                  borderRadius: '4px',
                                  fontSize: '13px',
                                  marginBottom: '6px',
                                  resize: 'vertical',
                                }}
                              />
                              <button
                                onClick={() => void handleShiftMentor(cls)}
                                disabled={
                                  actioning === `${cls.class_key}:shift` ||
                                  !shiftMentorIds[cls.class_key] ||
                                  !(shiftReasons[cls.class_key] || '').trim() ||
                                  !cls.next_session_number
                                }
                                style={{
                                  width: '100%',
                                  padding: '6px',
                                  background:
                                    actioning === `${cls.class_key}:shift` ||
                                    !shiftMentorIds[cls.class_key] ||
                                    !(shiftReasons[cls.class_key] || '').trim() ||
                                    !cls.next_session_number
                                      ? '#ccc'
                                      : '#0f766e',
                                  color: 'white',
                                  border: 'none',
                                  borderRadius: '4px',
                                  cursor:
                                    actioning === `${cls.class_key}:shift` ||
                                    !shiftMentorIds[cls.class_key] ||
                                    !(shiftReasons[cls.class_key] || '').trim() ||
                                    !cls.next_session_number
                                      ? 'not-allowed'
                                      : 'pointer',
                                  fontSize: '13px',
                                }}
                              >
                                {actioning === `${cls.class_key}:shift` ? 'Shifting...' : 'Shift Mentor'}
                              </button>
                            </div>
                          )}
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
                          onClick={() => openCloseConfirm(cls.class_key)}
                          disabled={actioning === `${cls.class_key}:close` || cls.all_graded === false}
                          style={{
                            width: '100%',
                            padding: '6px',
                            background: actioning === `${cls.class_key}:close` || cls.all_graded === false ? '#ccc' : '#ffc107',
                            color: '#333',
                            border: 'none',
                            borderRadius: '4px',
                            cursor: actioning === `${cls.class_key}:close` || cls.all_graded === false ? 'not-allowed' : 'pointer',
                            fontSize: '12px',
                          }}
                          title={cls.all_graded === false ? 'Complete final grading before closing the round' : ''}
                        >
                          {actioning === `${cls.class_key}:close` ? 'Closing...' : cls.all_graded === false ? 'Close Round (Grades Required)' : 'Close Round'}
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
        )
      ) : activeTab === 'archive' ? (
        /* Archive Tab Content */
        archive.length === 0 ? (
          <div style={{ padding: '40px', textAlign: 'center', background: 'white', borderRadius: '8px' }}>
            <p style={{ color: '#666' }}>No archived classes available.</p>
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
            {archive.map((group, idx) => (
              <div key={idx} style={{ background: 'white', borderRadius: '8px', border: '1px solid #ddd', overflow: 'hidden' }}>
                <div
                  onClick={() => toggleMentorCollapse(group.mentor.email)}
                  style={{
                    padding: '16px 24px',
                    background: '#f8f9fa',
                    borderBottom: collapsedMentors.has(group.mentor.email) ? 'none' : '1px solid #ddd',
                    cursor: 'pointer',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center'
                  }}
                >
                  <h2 style={{ fontSize: '18px', margin: 0, color: '#333' }}>
                    {group.mentor.email} <span style={{ fontSize: '14px', color: '#666', fontWeight: 400 }}>({group.classes.length} classes)</span>
                  </h2>
                  <span style={{ fontSize: '20px' }}>{collapsedMentors.has(group.mentor.email) ? '+' : '-'}</span>
                </div>

                {!collapsedMentors.has(group.mentor.email) && (
                  <div style={{ padding: '20px' }}>
                    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: '16px' }}>
                      {group.classes.map((cls: any) => (
                        <div
                          key={cls.class_key}
                          style={{
                            padding: '16px',
                            borderRadius: '6px',
                            border: '1px solid #eee',
                            background: 'white',
                            boxShadow: '0 1px 3px rgba(0,0,0,0.05)'
                          }}
                        >
                          <div style={{ marginBottom: '12px' }}>
                            <h3 style={{ fontSize: '15px', marginBottom: '6px' }}>
                              Level {cls.level} · Class {cls.class_number}
                            </h3>
                            <p style={{ color: '#666', fontSize: '13px', marginBottom: '4px' }}>
                              {cls.days} · {cls.time}
                            </p>
                            <p style={{ color: '#666', fontSize: '13px', marginBottom: '4px' }}>
                              {cls.student_count} student{cls.student_count !== 1 ? 's' : ''}
                            </p>
                            <p style={{ color: '#999', fontSize: '11px', fontStyle: 'italic' }}>
                              Closed: {new Date(cls.closed_at).toLocaleDateString()}
                            </p>
                            <p style={{ color: '#666', fontSize: '12px', marginTop: '6px' }}>
                              Completed sessions: {cls.completed_sessions_count ?? 0}/8
                            </p>
                          </div>
                          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                            <button
                              onClick={() => navigate(`/mentor-head/class?class_key=${encodeURIComponent(cls.class_key)}`)}
                              style={{
                                width: '100%',
                                padding: '8px',
                                background: '#f8f9fa',
                                color: '#007bff',
                                border: '1px solid #007bff',
                                borderRadius: '4px',
                                cursor: 'pointer',
                                fontSize: '13px',
                                fontWeight: 500
                              }}
                            >
                              Open Class
                            </button>
                            {cls.completed_sessions_count < 8 && (
                              <button
                                onClick={() => handleReopenRound(cls.class_key)}
                                disabled={actioning === `${cls.class_key}:reopen`}
                                style={{
                                  width: '100%',
                                  padding: '8px',
                                  background: actioning === `${cls.class_key}:reopen` ? '#ccc' : '#28a745',
                                  color: 'white',
                                  border: 'none',
                                  borderRadius: '4px',
                                  cursor: actioning === `${cls.class_key}:reopen` ? 'not-allowed' : 'pointer',
                                  fontSize: '13px',
                                  fontWeight: 600
                                }}
                              >
                                {actioning === `${cls.class_key}:reopen` ? 'Reopening...' : 'Reopen Class'}
                              </button>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )
      ) : activeTab === 'complaints' ? (
        <MentorHeadComplaints openComplaintId={openComplaintId} />
      ) : null}

      {closeConfirm.open && (
        <div
          onClick={closeCloseConfirm}
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(0,0,0,0.5)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 2000,
          }}
        >
          <div
            onClick={(e) => e.stopPropagation()}
            style={{
              background: 'white',
              padding: '24px',
              borderRadius: '10px',
              width: '420px',
              maxWidth: '90%',
              boxShadow: '0 4px 12px rgba(0,0,0,0.2)',
            }}
          >
            <h3 style={{ marginTop: 0, marginBottom: '12px', color: '#333' }}>Confirm Close</h3>
            <p style={{ marginTop: 0, marginBottom: '16px', color: '#666', fontSize: '14px' }}>
              Do you really want to close this class? This will move it to Closed Classes.
            </p>
            {closeOverridesLoading ? (
              <div style={{ marginBottom: '16px', padding: '12px', border: '1px solid #eee', borderRadius: '6px', color: '#666' }}>
                Loading absence override requests...
              </div>
            ) : closeOverrideItems.length > 0 ? (
              <div style={{ marginBottom: '16px', display: 'grid', gap: '10px' }}>
                {closeOverrideItems.map((item) => (
                  <div
                    key={item.lead_id}
                    style={{
                      border: '1px solid #f0d98c',
                      borderRadius: '6px',
                      padding: '10px',
                      background: '#fffdf2',
                      fontSize: '13px',
                    }}
                  >
                    <div style={{ fontWeight: 700, color: '#333' }}>
                      {item.full_name} · {item.absences} missed · Grade {item.final_grade || '-'}
                    </div>
                    {item.reason && (
                      <div style={{ marginTop: '6px', color: '#555' }}>
                        Justification: {item.reason}
                      </div>
                    )}
                    <div style={{ marginTop: '6px', color: item.status === 'approved' ? '#155724' : item.status === 'rejected' ? '#721c24' : '#856404', fontWeight: 700 }}>
                      Status: {item.status || 'not requested'}
                    </div>
                    {item.status === 'pending' && (
                      <div style={{ marginTop: '10px', display: 'flex', gap: '8px' }}>
                        <button
                          type="button"
                          onClick={() => handleReviewOverride(item, 'approved')}
                          disabled={!!overrideActioning}
                          style={{
                            padding: '6px 10px',
                            border: 'none',
                            borderRadius: '4px',
                            background: '#28a745',
                            color: 'white',
                            fontWeight: 700,
                            cursor: overrideActioning ? 'not-allowed' : 'pointer',
                          }}
                        >
                          Approve
                        </button>
                        <button
                          type="button"
                          onClick={() => handleReviewOverride(item, 'rejected')}
                          disabled={!!overrideActioning}
                          style={{
                            padding: '6px 10px',
                            border: '1px solid #dc3545',
                            borderRadius: '4px',
                            background: 'white',
                            color: '#dc3545',
                            fontWeight: 700,
                            cursor: overrideActioning ? 'not-allowed' : 'pointer',
                          }}
                        >
                          Reject
                        </button>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            ) : null}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
              <button
                onClick={closeCloseConfirm}
                style={{
                  padding: '8px 14px',
                  borderRadius: '6px',
                  border: '1px solid #ddd',
                  background: '#fff',
                  cursor: 'pointer',
                }}
              >
                Cancel
              </button>
              <button
                onClick={confirmCloseRound}
                disabled={actioning === `${closeConfirm.classKey}:close` || pendingCloseOverrides.length > 0}
                style={{
                  padding: '8px 14px',
                  borderRadius: '6px',
                  border: 'none',
                  background: actioning === `${closeConfirm.classKey}:close` || pendingCloseOverrides.length > 0 ? '#ccc' : '#dc3545',
                  color: 'white',
                  cursor: actioning === `${closeConfirm.classKey}:close` || pendingCloseOverrides.length > 0 ? 'not-allowed' : 'pointer',
                  fontWeight: 600,
                }}
              >
                {actioning === `${closeConfirm.classKey}:close` ? 'Closing...' : pendingCloseOverrides.length > 0 ? 'Review Required' : 'Yes, Close'}
              </button>
            </div>
          </div>
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
