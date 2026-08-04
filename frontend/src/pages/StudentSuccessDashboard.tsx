import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type StudentSuccessClass, type PlacementTestQueueItem, type StudentSuccessAvailabilityWindow } from '../api/client'
import { buildWhatsAppLink, openWhatsAppLink } from '../utils/whatsapp'

interface Group {
  mentor_id?: string
  mentor_email?: string
  mentor_name?: string
  classes: StudentSuccessClass[]
}

interface SSAvailabilityWindowDraft {
  local_id: string
  start_time: string
  end_time: string
  note: string
}

function dateKey(date: Date) {
  return date.toISOString().slice(0, 10)
}

function monthKey(dateStr: string) {
  return dateStr.slice(0, 7)
}

function todayKey() {
  return dateKey(new Date())
}

function addDays(dateStr: string, days: number) {
  const date = new Date(`${dateStr}T00:00:00Z`)
  date.setUTCDate(date.getUTCDate() + days)
  return dateKey(date)
}

function weekDates(startDate: string) {
  return Array.from({ length: 10 }, (_, index) => addDays(startDate, index))
}

function defaultAvailabilityWindow(): SSAvailabilityWindowDraft {
  return {
    local_id: Math.random().toString(36).slice(2),
    start_time: '14:00',
    end_time: '14:30',
    note: '',
  }
}

function minutesFromClock(value: string) {
  const [hours, minutes] = value.split(':').map(Number)
  return hours * 60 + minutes
}

function clockFromMinutes(totalMinutes: number) {
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`
}

function nextAvailabilityWindow(existing: SSAvailabilityWindowDraft[]) {
  if (existing.length === 0) {
    return defaultAvailabilityWindow()
  }

  const sorted = [...existing].sort((a, b) => minutesFromClock(a.start_time) - minutesFromClock(b.start_time))
  const last = sorted[sorted.length - 1]
  const lastEnd = minutesFromClock(last.end_time)
  const nextStart = Math.max(14 * 60, lastEnd)
  const nextEnd = nextStart + 30

  if (nextEnd <= 23 * 60) {
    return {
      local_id: Math.random().toString(36).slice(2),
      start_time: clockFromMinutes(nextStart),
      end_time: clockFromMinutes(nextEnd),
      note: '',
    }
  }

  return {
    local_id: Math.random().toString(36).slice(2),
    start_time: '22:30',
    end_time: '23:00',
    note: '',
  }
}

function validateAvailabilityDrafts(days: Record<string, SSAvailabilityWindowDraft[]>) {
  for (const [date, windows] of Object.entries(days)) {
    const sorted = [...windows].sort((a, b) => minutesFromClock(a.start_time) - minutesFromClock(b.start_time))
    for (let i = 0; i < sorted.length; i++) {
      const current = sorted[i]
      if (!current.start_time || !current.end_time) {
        return `Every slot on ${date} needs both start and end time.`
      }
      if (minutesFromClock(current.start_time) >= minutesFromClock(current.end_time)) {
        return `On ${date}, each slot must end after it starts.`
      }
      if (i > 0) {
        const previous = sorted[i - 1]
        if (minutesFromClock(current.start_time) < minutesFromClock(previous.end_time)) {
          return `On ${date}, slot windows cannot overlap or repeat.`
        }
      }
    }
  }
  return null
}

export default function StudentSuccessDashboard() {
  const [classes, setClasses] = useState<StudentSuccessClass[]>([])
  const [placementTests, setPlacementTests] = useState<PlacementTestQueueItem[]>([])
  const [loading, setLoading] = useState(true)
  const [placementLoading, setPlacementLoading] = useState(false)
  const [availabilityLoading, setAvailabilityLoading] = useState(false)
  const [availabilitySaving, setAvailabilitySaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [placementError, setPlacementError] = useState<string | null>(null)
  const [availabilityError, setAvailabilityError] = useState<string | null>(null)
  const [availabilityMessage, setAvailabilityMessage] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'classes' | 'placement_tests' | 'availability'>('classes')
  const [showCompletedTests, setShowCompletedTests] = useState(false)
  const [pendingPlacementCount, setPendingPlacementCount] = useState(0)
  const [availabilityWeekStart, setAvailabilityWeekStart] = useState(todayKey())
  const [availabilityWindows, setAvailabilityWindows] = useState<StudentSuccessAvailabilityWindow[]>([])
  const [availabilityDays, setAvailabilityDays] = useState<Record<string, SSAvailabilityWindowDraft[]>>({})
  const [resultModal, setResultModal] = useState<{
    open: boolean
    item: PlacementTestQueueItem | null
    error?: string
  }>({ open: false, item: null })
  const [noShowModal, setNoShowModal] = useState<{
    open: boolean
    item: PlacementTestQueueItem | null
    submitting?: boolean
    error?: string
  }>({ open: false, item: null })
  const [assignedLevel, setAssignedLevel] = useState<number | ''>('')
  const [testNotes, setTestNotes] = useState('')
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

  async function loadPlacementTestsCount() {
    try {
      const data = await api.getStudentSuccessPlacementTests(false)
      setPendingPlacementCount(data.placement_tests?.length || 0)
    } catch {
      // Silent failure: don't block dashboard
      setPendingPlacementCount(0)
    }
  }

  async function loadPlacementTests(showLoading = true) {
    try {
      if (showLoading) {
        setPlacementLoading(true)
      }
      setPlacementError(null)
      const data = await api.getStudentSuccessPlacementTests(showCompletedTests)
      setPlacementTests(data.placement_tests || [])
      if (!showCompletedTests) {
        setPendingPlacementCount(data.placement_tests?.length || 0)
      }
    } catch (err) {
      setPlacementError(err instanceof Error ? err.message : 'Failed to load placement tests')
    } finally {
      if (showLoading) {
        setPlacementLoading(false)
      }
    }
  }

  async function markPlacementNoShow(item: PlacementTestQueueItem) {
    setPlacementError(null)
    await api.markPlacementTestNoShow({
      lead_id: item.lead_id,
      note: 'Placement test no-show. Admin should contact the lead and reschedule.',
    })
    await loadPlacementTests()
    await loadPlacementTestsCount()
  }

  async function loadAvailability() {
    try {
      setAvailabilityLoading(true)
      setAvailabilityError(null)
      setAvailabilityMessage(null)
      const dates = weekDates(availabilityWeekStart)
      const months = Array.from(new Set(dates.map(monthKey)))
      const responses = await Promise.all(months.map((month) => api.getStudentSuccessAvailability(month)))
      const windows = responses.flatMap((response) => response.windows || [])
      setAvailabilityWindows(windows)
      const nextDays: Record<string, SSAvailabilityWindowDraft[]> = {}
      for (const date of dates) {
        const matching = windows
          .filter((window) => window.available_date.slice(0, 10) === date)
          .map((window) => ({
            local_id: window.id || Math.random().toString(36).slice(2),
            start_time: window.start_time.slice(0, 5),
            end_time: window.end_time.slice(0, 5),
            note: window.note || '',
          }))
        nextDays[date] = matching
      }
      setAvailabilityDays(nextDays)
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load availability'
      setAvailabilityError(message === 'Not Found' ? 'Availability endpoint was not found. Restart the backend after deploying this change.' : message)
    } finally {
      setAvailabilityLoading(false)
    }
  }

  async function saveAvailability() {
    try {
      setAvailabilitySaving(true)
      setAvailabilityError(null)
      setAvailabilityMessage(null)
      const validationError = validateAvailabilityDrafts(availabilityDays)
      if (validationError) {
        setAvailabilityError(validationError)
        setAvailabilitySaving(false)
        return
      }
      const dates = weekDates(availabilityWeekStart)
      const dateSet = new Set(dates)
      const months = Array.from(new Set(dates.map(monthKey)))
      await Promise.all(months.map((month) => {
        const preserved = availabilityWindows.filter((window) => {
          const date = window.available_date.slice(0, 10)
          return date.startsWith(month) && !dateSet.has(date) && date >= todayKey()
        })
        const edited = dates
          .filter((date) => date.startsWith(month) && date >= todayKey())
          .flatMap((date) => (availabilityDays[date] || []).map((window) => ({
            available_date: date,
            start_time: window.start_time,
            end_time: window.end_time,
            note: window.note,
          })))
        return api.updateStudentSuccessAvailability(month, [...preserved, ...edited])
      }))
      setAvailabilityMessage('Availability saved.')
      await loadAvailability()
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save availability'
      setAvailabilityError(message === 'Not Found' ? 'Availability endpoint was not found. Restart the backend after deploying this change.' : message)
    } finally {
      setAvailabilitySaving(false)
    }
  }

  function addAvailabilityWindow(date: string) {
    setAvailabilityDays((prev) => ({
      ...prev,
      [date]: [...(prev[date] || []), nextAvailabilityWindow(prev[date] || [])],
    }))
  }

  function updateAvailabilityWindow(date: string, localID: string, patch: Partial<SSAvailabilityWindowDraft>) {
    setAvailabilityDays((prev) => ({
      ...prev,
      [date]: (prev[date] || []).map((window) => (
        window.local_id === localID ? { ...window, ...patch } : window
      )),
    }))
  }

  function removeAvailabilityWindow(date: string, localID: string) {
    setAvailabilityDays((prev) => ({
      ...prev,
      [date]: (prev[date] || []).filter((window) => window.local_id !== localID),
    }))
  }

  useEffect(() => {
    loadPlacementTestsCount()
  }, [])

  useEffect(() => {
    if (activeTab !== 'placement_tests') {
      return
    }

    function refreshIfVisible() {
      if (document.visibilityState === 'visible') {
        void loadPlacementTests(false)
        void loadPlacementTestsCount()
      }
    }

    void loadPlacementTests()
    const intervalID = window.setInterval(refreshIfVisible, 30000)
    window.addEventListener('focus', refreshIfVisible)
    document.addEventListener('visibilitychange', refreshIfVisible)

    return () => {
      window.clearInterval(intervalID)
      window.removeEventListener('focus', refreshIfVisible)
      document.removeEventListener('visibilitychange', refreshIfVisible)
    }
  }, [activeTab, showCompletedTests])

  useEffect(() => {
    if (activeTab === 'availability') {
      loadAvailability()
    }
  }, [activeTab, availabilityWeekStart])

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
  const midRoundClasses = classes.filter((c) => c.mid_round_required)
  const endRoundClasses = classes.filter((c) => c.end_round_required)
  const complianceDueClasses = classes.filter((c) => c.compliance_required)
  const currentWeekDates = weekDates(availabilityWeekStart)

  return (
    <div>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <h1>Student Success Dashboard</h1>
      </div>

      {midRoundClasses.length > 0 && (
        <div style={{ background: '#fff3cd', border: '1px solid #ffeeba', color: '#856404', padding: '16px', borderRadius: '8px', marginBottom: '16px' }}>
          <strong style={{ display: 'block', fontSize: '16px' }}>Mid-Round Feedback Required!</strong>
          <div style={{ fontSize: '14px', marginTop: '4px' }}>
            {midRoundClasses.length} class{midRoundClasses.length !== 1 ? 'es' : ''} reached Session 4 and still need mid-round feedback.
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginTop: '12px' }}>
            {midRoundClasses.map((cls) => (
              <button
                key={cls.class_key}
                onClick={() => navigate(`/student-success/class?class_key=${encodeURIComponent(cls.class_key)}&tab=feedback`)}
                style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #856404', background: '#fff', color: '#856404', cursor: 'pointer', fontSize: '12px', fontWeight: 600 }}
              >
                Level {cls.level} · Class {cls.class_number} · Go to Feedbacks
              </button>
            ))}
          </div>
        </div>
      )}

      {endRoundClasses.length > 0 && (
        <div style={{ background: '#fff3cd', border: '1px solid #ffeeba', color: '#856404', padding: '16px', borderRadius: '8px', marginBottom: '16px' }}>
          <strong style={{ display: 'block', fontSize: '16px' }}>End-of-Round Feedback Required!</strong>
          <div style={{ fontSize: '14px', marginTop: '4px' }}>
            {endRoundClasses.length} class{endRoundClasses.length !== 1 ? 'es' : ''} reached Session 8 and still need final feedback.
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginTop: '12px' }}>
            {endRoundClasses.map((cls) => (
              <button
                key={cls.class_key}
                onClick={() => navigate(`/student-success/class?class_key=${encodeURIComponent(cls.class_key)}&tab=feedback`)}
                style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #856404', background: '#fff', color: '#856404', cursor: 'pointer', fontSize: '12px', fontWeight: 600 }}
              >
                Level {cls.level} · Class {cls.class_number} · Go to Feedbacks
              </button>
            ))}
          </div>
        </div>
      )}

      {complianceDueClasses.length > 0 && (
        <div style={{ background: '#fde2e2', border: '1px solid #f5b5b5', color: '#8a1f1f', padding: '16px', borderRadius: '8px', marginBottom: '16px' }}>
          <strong style={{ display: 'block', fontSize: '16px' }}>Compliance Checklist Required!</strong>
          <div style={{ fontSize: '14px', marginTop: '4px' }}>
            {complianceDueClasses.length} class{complianceDueClasses.length !== 1 ? 'es' : ''} finished Session 8 and still have incomplete compliance checks.
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginTop: '12px' }}>
            {complianceDueClasses.map((cls) => (
              <button
                key={cls.class_key}
                onClick={() => navigate(`/student-success/class?class_key=${encodeURIComponent(cls.class_key)}&open_compliance=1`)}
                style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #8a1f1f', background: '#fff', color: '#8a1f1f', cursor: 'pointer', fontSize: '12px', fontWeight: 600 }}
              >
                Level {cls.level} · Class {cls.class_number} · Complete Checklist ({cls.compliance_done ?? 0}/{cls.compliance_total ?? 8})
              </button>
            ))}
          </div>
        </div>
      )}

      {pendingPlacementCount > 0 && (
        <div className="ss-dashboard-banner" style={{ background: '#E6F7FF', border: '2px solid #4EC6E0', color: '#0052A3', padding: '12px 16px', borderRadius: '8px', marginBottom: '16px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '12px' }}>
          <div>
            <strong>{pendingPlacementCount}</strong> new placement test{pendingPlacementCount !== 1 ? 's' : ''} need results.
          </div>
          <button
            onClick={() => setActiveTab('placement_tests')}
            className="ss-dashboard-banner-button"
            style={{ padding: '6px 12px', borderRadius: '6px', border: '1px solid #0052A3', background: '#fff', color: '#0052A3', cursor: 'pointer', fontWeight: 600 }}
          >
            View Placement Tests
          </button>
        </div>
      )}

      <div className="ss-dashboard-toolbar" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px', gap: '16px', flexWrap: 'wrap' }}>
        <p className="ss-dashboard-description" style={{ margin: 0, color: '#666' }}>
          {activeTab === 'classes'
            ? 'Active classes only (round started). Grouped by mentor.'
            : activeTab === 'placement_tests'
            ? 'Your assigned placement tests. Record level and notes after the test.'
            : 'Set the weekly time windows when admins can assign you placement tests.'}
        </p>
        <div className="ss-dashboard-tabs" style={{ display: 'flex', gap: '8px' }}>
          <button
            onClick={() => setActiveTab('classes')}
            className="ss-dashboard-tab-button"
            style={{
              padding: '8px 14px',
              borderRadius: '6px',
              border: activeTab === 'classes' ? '1px solid #007bff' : '1px solid #ddd',
              background: activeTab === 'classes' ? '#e7f1ff' : '#fff',
              color: activeTab === 'classes' ? '#007bff' : '#333',
              cursor: 'pointer',
              fontWeight: 600,
            }}
          >
            Classes
          </button>
          <button
            onClick={() => setActiveTab('placement_tests')}
            className="ss-dashboard-tab-button"
            style={{
              padding: '8px 14px',
              borderRadius: '6px',
              border: activeTab === 'placement_tests' ? '1px solid #007bff' : '1px solid #ddd',
              background: activeTab === 'placement_tests' ? '#e7f1ff' : '#fff',
              color: activeTab === 'placement_tests' ? '#007bff' : '#333',
              cursor: 'pointer',
              fontWeight: 600,
            }}
          >
            Placement Tests
          </button>
          <button
            onClick={() => setActiveTab('availability')}
            className="ss-dashboard-tab-button"
            style={{
              padding: '8px 14px',
              borderRadius: '6px',
              border: activeTab === 'availability' ? '1px solid #007bff' : '1px solid #ddd',
              background: activeTab === 'availability' ? '#e7f1ff' : '#fff',
              color: activeTab === 'availability' ? '#007bff' : '#333',
              cursor: 'pointer',
              fontWeight: 600,
            }}
          >
            Availability
          </button>
        </div>
      </div>

      <div>
        {activeTab === 'classes' && (
          <>
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
                            border: cls.has_high_priority ? '1px solid #dc3545' : '1px solid #eee',
                            background: cls.has_high_priority ? '#fffafa' : '#fff',
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
                                marginRight: '8px',
                              }}
                            >
                              ACTIVE
                            </span>
                            {cls.mid_round_required && (
                              <span
                                style={{
                                  display: 'inline-block',
                                  padding: '4px 10px',
                                  borderRadius: '12px',
                                  fontSize: '12px',
                                  fontWeight: 600,
                                  background: '#fff3cd',
                                  color: '#856404',
                                  marginRight: '8px',
                                }}
                              >
                                MID FEEDBACK DUE
                              </span>
                            )}
                            {cls.end_round_required && (
                              <span
                                style={{
                                  display: 'inline-block',
                                  padding: '4px 10px',
                                  borderRadius: '12px',
                                  fontSize: '12px',
                                  fontWeight: 600,
                                  background: '#ffe8a1',
                                  color: '#856404',
                                  marginRight: '8px',
                                }}
                              >
                                FINAL FEEDBACK DUE
                              </span>
                            )}
                            {cls.compliance_required && (
                              <span
                                style={{
                                  display: 'inline-block',
                                  padding: '4px 10px',
                                  borderRadius: '12px',
                                  fontSize: '12px',
                                  fontWeight: 600,
                                  background: '#fde2e2',
                                  color: '#8a1f1f',
                                  marginRight: '8px',
                                }}
                              >
                                COMPLIANCE DUE ({cls.compliance_done ?? 0}/{cls.compliance_total ?? 8})
                              </span>
                            )}
                            {cls.has_high_priority && (
                              <span
                                style={{
                                  display: 'inline-block',
                                  padding: '4px 10px',
                                  borderRadius: '12px',
                                  fontSize: '12px',
                                  fontWeight: 600,
                                  background: '#f8d7da',
                                  color: '#721c24',
                                }}
                                title={cls.high_priority_reason}
                              >
                                🚩 AT RISK
                              </span>
                            )}
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
          </>
        )}

        {activeTab === 'placement_tests' && (
          <>
            <div className="ss-placement-controls" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
              <div />
              <label style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '14px', color: '#333' }}>
                <input
                  type="checkbox"
                  checked={showCompletedTests}
                  onChange={(e) => setShowCompletedTests(e.target.checked)}
                />
                Show completed tests
              </label>
            </div>
            {placementError && (
              <div style={{ padding: '12px 16px', background: '#f8d7da', color: '#721c24', borderRadius: '8px', marginBottom: '16px' }}>
                <strong>Error:</strong> {placementError}
              </div>
            )}

            {placementLoading ? (
              <div style={{ padding: '24px', textAlign: 'center' }}>Loading placement tests...</div>
            ) : placementTests.length === 0 ? (
              <div style={{ padding: '24px', background: '#f9f9f9', borderRadius: '8px', textAlign: 'center' }}>
                <p>{showCompletedTests ? 'No completed placement tests.' : 'No placement tests waiting.'}</p>
              </div>
            ) : (
              <>
              <div className="ss-placement-table-wrap" style={{ background: '#fff', border: '1px solid #eee', borderRadius: '8px', overflow: 'hidden' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '14px' }}>
                  <thead>
                    <tr style={{ textAlign: 'left', background: '#f8f9fa' }}>
                      <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Lead</th>
                      <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Test Date</th>
                      <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Test Time</th>
                      <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Type</th>
                      <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {placementTests.map((item) => (
                      <tr key={item.lead_id} style={{ borderBottom: '1px solid #eee' }}>
                        <td style={{ padding: '12px' }}>
                          <div style={{ fontWeight: 600 }}>{item.full_name}</div>
                          <div style={{ fontSize: '12px', color: '#666' }}>{item.phone}</div>
                        </td>
                        <td style={{ padding: '12px' }}>{item.test_date || '-'}</td>
                        <td style={{ padding: '12px' }}>{item.test_time || '-'}</td>
                        <td style={{ padding: '12px' }}>{item.test_type || '-'}</td>
                        <td style={{ padding: '12px' }}>
                          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                            {item.phone && (
                              <a
                                href={buildWhatsAppLink(item.phone)}
                                target="admin-whatsapp-chat"
                                onClick={(event) => {
                                  event.preventDefault()
                                  if (!openWhatsAppLink(buildWhatsAppLink(item.phone))) {
                                    window.location.href = buildWhatsAppLink(item.phone)
                                  }
                                }}
                                title="Open WhatsApp"
                                aria-label={`Open WhatsApp chat for ${item.full_name}`}
                                style={{
                                  padding: '6px',
                                  borderRadius: '6px',
                                  background: '#25D366',
                                  color: '#fff',
                                  display: 'inline-flex',
                                  alignItems: 'center',
                                  justifyContent: 'center',
                                  width: '32px',
                                  height: '32px',
                                  textDecoration: 'none',
                                  border: '1px solid #25D366',
                                }}
                              >
                                <WhatsAppIcon />
                              </a>
                            )}
                            <button
                              onClick={() => {
                                setResultModal({ open: true, item, error: undefined })
                                setAssignedLevel(item.assigned_level ?? '')
                                setTestNotes(item.test_notes ?? '')
                              }}
                              style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #007bff', background: '#fff', color: '#007bff', cursor: 'pointer', fontSize: '12px' }}
                            >
                              {item.assigned_level ? 'Update Result' : 'Record Result'}
                            </button>
                            {!item.assigned_level && item.appointment_status !== 'completed' && (
                              <button
                                onClick={() => setNoShowModal({ open: true, item, error: undefined, submitting: false })}
                                style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #dc3545', background: '#fff', color: '#dc3545', cursor: 'pointer', fontSize: '12px' }}
                              >
                                No-show
                              </button>
                            )}
                            <a
                              href={`/pre-enrolment/${item.lead_id}`}
                              target="_blank"
                              rel="noopener noreferrer"
                              style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #6c757d', background: '#fff', color: '#6c757d', cursor: 'pointer', fontSize: '12px', textDecoration: 'none', display: 'inline-flex', alignItems: 'center' }}
                            >
                              Open Lead
                            </a>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="ss-placement-cards">
                {placementTests.map((item) => (
                  <div key={item.lead_id} className="ss-placement-card">
                    <div className="ss-placement-card-head">
                      <div>
                        <div className="ss-placement-card-name">{item.full_name}</div>
                        <div className="ss-placement-card-phone">{item.phone}</div>
                      </div>
                      <div className="ss-placement-card-type">{item.test_type || '-'}</div>
                    </div>
                    <div className="ss-placement-card-meta">
                      <div className="ss-placement-card-field">
                        <span className="ss-placement-card-label">Test Date</span>
                        <span className="ss-placement-card-value">{item.test_date || '-'}</span>
                      </div>
                      <div className="ss-placement-card-field">
                        <span className="ss-placement-card-label">Test Time</span>
                        <span className="ss-placement-card-value">{item.test_time || '-'}</span>
                      </div>
                    </div>
                    <div className="ss-placement-card-actions">
                      <div style={{ display: 'grid', gridTemplateColumns: '1fr auto', gap: '10px' }}>
                        <button
                          onClick={() => {
                            setResultModal({ open: true, item, error: undefined })
                            setAssignedLevel(item.assigned_level ?? '')
                            setTestNotes(item.test_notes ?? '')
                          }}
                          className="ss-placement-card-primary"
                        >
                          {item.assigned_level ? 'Update Result' : 'Record Result'}
                        </button>
                        {item.phone && (
                          <a
                            href={buildWhatsAppLink(item.phone)}
                            target="admin-whatsapp-chat"
                            onClick={(event) => {
                              event.preventDefault()
                              if (!openWhatsAppLink(buildWhatsAppLink(item.phone))) {
                                window.location.href = buildWhatsAppLink(item.phone)
                              }
                            }}
                            title="Open WhatsApp"
                            aria-label={`Open WhatsApp chat for ${item.full_name}`}
                            style={{
                              borderRadius: '8px',
                              background: '#25D366',
                              color: '#fff',
                              display: 'inline-flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              width: '44px',
                              minHeight: '42px',
                              textDecoration: 'none',
                              border: '1px solid #25D366',
                            }}
                          >
                            <WhatsAppIcon />
                          </a>
                        )}
                      </div>
                      <a
                        href={`/pre-enrolment/${item.lead_id}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="ss-placement-card-secondary"
                      >
                        Open Lead
                      </a>
                      {!item.assigned_level && item.appointment_status !== 'completed' && (
                        <button
                          onClick={() => setNoShowModal({ open: true, item, error: undefined, submitting: false })}
                          className="ss-placement-card-secondary"
                          style={{ borderColor: '#dc3545', color: '#dc3545' }}
                        >
                          No-show
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              </>
            )}
          </>
        )}

        {activeTab === 'availability' && (
          <div style={{ background: '#fff', border: '1px solid #eee', borderRadius: '10px', padding: '18px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', flexWrap: 'wrap', alignItems: 'center', marginBottom: '16px' }}>
              <div>
                <h2 style={{ margin: 0, fontSize: '18px' }}>Placement Test Availability</h2>
                <p style={{ margin: '6px 0 0', color: '#666', fontSize: '13px' }}>
                  Placement tests are 30 minutes. Set availability for the next 10 days between 14:00 and 23:00.
                </p>
              </div>
              <label style={{ display: 'grid', gap: '4px', fontSize: '13px', fontWeight: 700 }}>
                Window starts
                <input
                  type="date"
                  value={availabilityWeekStart}
                  min={todayKey()}
                  onChange={(event) => setAvailabilityWeekStart(event.target.value || todayKey())}
                  style={{ padding: '8px 10px', border: '1px solid #ddd', borderRadius: '6px' }}
                />
              </label>
            </div>

            {availabilityError && (
              <div style={{ padding: '12px 16px', background: '#f8d7da', color: '#721c24', borderRadius: '8px', marginBottom: '12px' }}>
                {availabilityError}
              </div>
            )}
            {availabilityMessage && (
              <div style={{ padding: '12px 16px', background: '#d4edda', color: '#155724', borderRadius: '8px', marginBottom: '12px' }}>
                {availabilityMessage}
              </div>
            )}

            {availabilityLoading ? (
              <div style={{ padding: '24px', textAlign: 'center' }}>Loading availability...</div>
            ) : (
              <div style={{ display: 'grid', gap: '10px' }}>
                {currentWeekDates.map((date) => {
                  const windows = availabilityDays[date] || []
                  const isPastDate = date < todayKey()
                  const label = new Date(`${date}T00:00:00Z`).toLocaleDateString('en-US', {
                    weekday: 'short',
                    month: 'short',
                    day: 'numeric',
                    timeZone: 'UTC',
                  })
                  return (
                    <div
                      key={date}
                      style={{
                        display: 'grid',
                        gap: '12px',
                        padding: '12px',
                        border: '1px solid #edf0f2',
                        borderRadius: '8px',
                        background: windows.length > 0 ? '#f7fcff' : '#fafafa',
                      }}
                    >
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
                        <div>
                          <div style={{ fontWeight: 700 }}>{label}</div>
                          <div style={{ color: '#666', fontSize: '13px', marginTop: '4px' }}>
                            {windows.length > 0 ? `${windows.length} slot window${windows.length === 1 ? '' : 's'} assigned.` : 'No assigned slots for this day.'}
                          </div>
                        </div>
                        <button
                          type="button"
                          onClick={() => addAvailabilityWindow(date)}
                          disabled={isPastDate}
                          style={{
                            padding: '8px 12px',
                            borderRadius: '6px',
                            border: `1px solid ${isPastDate ? '#c9d2d8' : '#0b7285'}`,
                            background: '#fff',
                            color: isPastDate ? '#8a98a5' : '#0b7285',
                            cursor: isPastDate ? 'not-allowed' : 'pointer',
                            fontWeight: 700,
                            opacity: isPastDate ? 0.7 : 1,
                          }}
                        >
                          Add Slot
                        </button>
                      </div>

                      {windows.length === 0 ? (
                        <div style={{ padding: '10px 12px', borderRadius: '6px', background: '#fff', color: '#666', fontSize: '13px', border: '1px dashed #d7dee3' }}>
                          {isPastDate
                            ? 'Past day. Availability can no longer be edited here.'
                            : 'Admins will see this day as unavailable until you add at least one slot window.'}
                        </div>
                      ) : (
                        <div style={{ display: 'grid', gap: '10px' }}>
                          {windows.map((window, index) => (
                            <div
                              key={window.local_id}
                              style={{
                                display: 'grid',
                                gridTemplateColumns: 'minmax(70px, auto) 120px 120px minmax(180px, 1fr) auto',
                                gap: '10px',
                                alignItems: 'center',
                              }}
                            >
                              <div style={{ fontSize: '13px', fontWeight: 700, color: '#345' }}>
                                Slot {index + 1}
                              </div>
                              <input
                                type="time"
                                min="14:00"
                                max="22:30"
                                step="1800"
                                value={window.start_time}
                                disabled={isPastDate}
                                onChange={(event) => updateAvailabilityWindow(date, window.local_id, { start_time: event.target.value })}
                                style={{ padding: '8px', border: '1px solid #ddd', borderRadius: '6px' }}
                              />
                              <input
                                type="time"
                                min="14:30"
                                max="23:00"
                                step="1800"
                                value={window.end_time}
                                disabled={isPastDate}
                                onChange={(event) => updateAvailabilityWindow(date, window.local_id, { end_time: event.target.value })}
                                style={{ padding: '8px', border: '1px solid #ddd', borderRadius: '6px' }}
                              />
                              <input
                                type="text"
                                value={window.note}
                                disabled={isPastDate}
                                onChange={(event) => updateAvailabilityWindow(date, window.local_id, { note: event.target.value })}
                                placeholder="Optional note"
                                style={{ padding: '8px', border: '1px solid #ddd', borderRadius: '6px' }}
                              />
                              <button
                                type="button"
                                onClick={() => removeAvailabilityWindow(date, window.local_id)}
                                disabled={isPastDate}
                                style={{
                                  padding: '8px 10px',
                                  borderRadius: '6px',
                                  border: `1px solid ${isPastDate ? '#c9d2d8' : '#dc3545'}`,
                                  background: '#fff',
                                  color: isPastDate ? '#8a98a5' : '#dc3545',
                                  cursor: isPastDate ? 'not-allowed' : 'pointer',
                                  fontWeight: 700,
                                  opacity: isPastDate ? 0.7 : 1,
                                }}
                              >
                                Remove
                              </button>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            )}

            <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '16px' }}>
              <button
                type="button"
                disabled={availabilityLoading || availabilitySaving}
                onClick={saveAvailability}
                style={{
                  padding: '10px 18px',
                  borderRadius: '7px',
                  border: 'none',
                  background: '#0b7285',
                  color: '#fff',
                  fontWeight: 700,
                  cursor: availabilityLoading || availabilitySaving ? 'not-allowed' : 'pointer',
                  opacity: availabilityLoading || availabilitySaving ? 0.65 : 1,
                }}
              >
                {availabilitySaving ? 'Saving...' : 'Save Availability'}
              </button>
            </div>
          </div>
        )}
      </div>

      {resultModal.open && resultModal.item && (
        <div
          style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 3000 }}
          onClick={() => setResultModal({ open: false, item: null })}
        >
          <div
            style={{ background: 'white', padding: '24px', borderRadius: '12px', width: '520px', maxWidth: '90%' }}
            onClick={(e) => e.stopPropagation()}
          >
            <h3 style={{ marginBottom: '16px' }}>Record Placement Test Result</h3>
            <p style={{ fontSize: '14px', color: '#666', marginBottom: '16px' }}>
              Lead: <strong>{resultModal.item.full_name}</strong> ({resultModal.item.phone})
            </p>

            {resultModal.error && (
              <div style={{ color: '#721c24', background: '#f8d7da', padding: '8px 12px', borderRadius: '6px', marginBottom: '12px', fontSize: '13px' }}>
                {resultModal.error}
              </div>
            )}

            <div style={{ marginBottom: '16px' }}>
              <label style={{ display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '6px' }}>Assigned Level *</label>
              <select
                value={assignedLevel}
                onChange={(e) => setAssignedLevel(e.target.value ? Number(e.target.value) : '')}
                style={{ width: '100%', padding: '10px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px' }}
              >
                <option value="">Select level</option>
                {[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((lvl) => (
                  <option key={lvl} value={lvl}>Level {lvl}</option>
                ))}
              </select>
            </div>

            <div style={{ marginBottom: '20px' }}>
              <label style={{ display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '6px' }}>Test Notes</label>
              <textarea
                value={testNotes}
                onChange={(e) => setTestNotes(e.target.value)}
                placeholder="Add observations and results..."
                style={{ width: '100%', height: '120px', padding: '12px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px', resize: 'vertical' }}
              />
            </div>

            <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
              <button
                onClick={() => setResultModal({ open: false, item: null })}
                style={{ padding: '8px 16px', borderRadius: '6px', border: '1px solid #ddd', background: '#fff', cursor: 'pointer' }}
              >
                Cancel
              </button>
              <button
                onClick={async () => {
                  if (!resultModal.item) return
                  if (!assignedLevel) {
                    setResultModal((prev) => ({ ...prev, error: 'Please select an assigned level.' }))
                    return
                  }
                  try {
                    await api.completePlacementTest({
                      lead_id: resultModal.item.lead_id,
                      assigned_level: assignedLevel,
                      test_notes: testNotes,
                    })
                    setResultModal({ open: false, item: null })
                    setAssignedLevel('')
                    setTestNotes('')
                    await loadPlacementTests()
                    await loadPlacementTestsCount()
                  } catch (err) {
                    setResultModal((prev) => ({ ...prev, error: err instanceof Error ? err.message : 'Failed to save result' }))
                  }
                }}
                style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: '#28a745', color: '#fff', cursor: 'pointer', fontWeight: 600 }}
              >
                Save Result
              </button>
            </div>
          </div>
        </div>
      )}

      {noShowModal.open && noShowModal.item && (
        <div
          style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 3000 }}
          onClick={() => !noShowModal.submitting && setNoShowModal({ open: false, item: null })}
        >
          <div
            style={{ background: 'white', padding: '24px', borderRadius: '12px', width: '460px', maxWidth: '90%' }}
            onClick={(e) => e.stopPropagation()}
          >
            <h3 style={{ marginBottom: '16px' }}>Confirm No-show</h3>
            <p style={{ fontSize: '14px', color: '#444', marginBottom: '16px', lineHeight: 1.5 }}>
              Confirm <strong>{noShowModal.item.full_name}</strong> did not attend the placement test.
            </p>

            {noShowModal.error && (
              <div style={{ color: '#721c24', background: '#f8d7da', padding: '8px 12px', borderRadius: '6px', marginBottom: '12px', fontSize: '13px' }}>
                {noShowModal.error}
              </div>
            )}

            <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
              <button
                onClick={() => setNoShowModal({ open: false, item: null })}
                disabled={noShowModal.submitting}
                style={{ padding: '8px 16px', borderRadius: '6px', border: '1px solid #ddd', background: '#fff', cursor: noShowModal.submitting ? 'not-allowed' : 'pointer', opacity: noShowModal.submitting ? 0.65 : 1 }}
              >
                Cancel
              </button>
              <button
                onClick={async () => {
                  if (!noShowModal.item) return
                  setNoShowModal((prev) => ({ ...prev, submitting: true, error: undefined }))
                  try {
                    await markPlacementNoShow(noShowModal.item)
                    setNoShowModal({ open: false, item: null })
                  } catch (err) {
                    setNoShowModal((prev) => ({
                      ...prev,
                      submitting: false,
                      error: err instanceof Error ? err.message : 'Failed to mark no-show',
                    }))
                  }
                }}
                disabled={noShowModal.submitting}
                style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: '#dc3545', color: '#fff', cursor: noShowModal.submitting ? 'not-allowed' : 'pointer', fontWeight: 600, opacity: noShowModal.submitting ? 0.65 : 1 }}
              >
                {noShowModal.submitting ? 'Saving...' : 'Confirm No-show'}
              </button>
            </div>
          </div>
        </div>
      )}

    </div>
  )
}

function WhatsAppIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M20.52 3.48A11.86 11.86 0 0 0 12.06 0C5.5 0 .16 5.34.16 11.9c0 2.1.55 4.15 1.58 5.95L0 24l6.33-1.66a11.83 11.83 0 0 0 5.72 1.46h.01c6.56 0 11.9-5.34 11.9-11.9 0-3.18-1.24-6.17-3.44-8.42Zm-8.46 18.3h-.01a9.9 9.9 0 0 1-5.05-1.39l-.36-.21-3.76.99 1-3.66-.24-.38a9.88 9.88 0 0 1-1.52-5.23c0-5.46 4.45-9.9 9.92-9.9 2.65 0 5.13 1.03 7 2.9a9.83 9.83 0 0 1 2.9 7c0 5.47-4.45 9.9-9.88 9.9Zm5.43-7.42c-.3-.15-1.78-.88-2.06-.98-.28-.1-.48-.15-.68.15-.2.3-.78.98-.95 1.18-.17.2-.35.22-.64.08-.3-.15-1.24-.46-2.36-1.47-.88-.78-1.47-1.75-1.64-2.05-.17-.3-.02-.46.13-.61.13-.13.3-.35.45-.52.15-.18.2-.3.3-.5.1-.2.05-.38-.02-.53-.08-.15-.68-1.63-.93-2.23-.24-.58-.5-.5-.68-.5h-.58c-.2 0-.53.08-.8.38-.28.3-1.05 1.03-1.05 2.5s1.08 2.9 1.23 3.1c.15.2 2.11 3.23 5.12 4.52.72.31 1.28.5 1.72.64.72.23 1.37.2 1.88.12.57-.08 1.78-.73 2.03-1.43.25-.7.25-1.3.18-1.42-.08-.12-.28-.2-.58-.35Z" />
    </svg>
  )
}
