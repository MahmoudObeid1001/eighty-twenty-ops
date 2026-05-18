import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type OpsNotificationSummary } from '../api/client'

export default function OpsNotificationBanner({ userRole }: { userRole: string }) {
  const [summary, setSummary] = useState<OpsNotificationSummary | null>(null)
  const [loading, setLoading] = useState(false)
  const [dismissingClass, setDismissingClass] = useState(false)
  const [dismissingReschedule, setDismissingReschedule] = useState(false)
  const navigate = useNavigate()

  const enabled = userRole === 'mentor_head' || userRole === 'manager' || userRole === 'student_success'

  useEffect(() => {
    if (!enabled) {
      setSummary(null)
      return
    }
    let cancelled = false
    async function load(showLoading: boolean) {
      try {
        if (showLoading) {
          setLoading(true)
        }
        const data = await api.getOpsNotifications()
        if (!cancelled) {
          setSummary(data)
        }
      } catch (err) {
        console.warn('Failed to load ops notifications:', err)
      } finally {
        if (!cancelled && showLoading) {
          setLoading(false)
        }
      }
    }

    function refreshIfVisible() {
      if (document.visibilityState === 'visible') {
        void load(false)
      }
    }

    void load(true)
    const intervalId = window.setInterval(refreshIfVisible, 30000)
    window.addEventListener('focus', refreshIfVisible)
    document.addEventListener('visibilitychange', refreshIfVisible)

    return () => {
      cancelled = true
      window.clearInterval(intervalId)
      window.removeEventListener('focus', refreshIfVisible)
      document.removeEventListener('visibilitychange', refreshIfVisible)
    }
  }, [enabled])

  if (!enabled || loading || !summary || (!summary.daily_report && !summary.complaint && !summary.class_sent && !summary.session_reschedule)) {
    return null
  }

  async function openDailyReport() {
    const report = summary?.daily_report
    if (!report) return
    try {
      await api.markDailyReportRead(report.report_date)
    } catch (err) {
      console.warn('Failed to mark daily report read:', err)
    }
    setSummary((current) => (current ? { ...current, daily_report: undefined } : current))
    navigate(`/reports?tab=daily&date=${encodeURIComponent(report.report_date)}`)
  }

  async function openComplaint() {
    const complaint = summary?.complaint
    if (!complaint) return
    try {
      await api.markComplaintRead(complaint.id)
    } catch (err) {
      console.warn('Failed to mark complaint read:', err)
    }
    setSummary((current) => (current ? { ...current, complaint: undefined } : current))
    navigate(`/mentor-head?tab=complaints&complaint_id=${encodeURIComponent(complaint.id)}`)
  }

  async function dismissClassSent() {
    const item = summary?.class_sent
    if (!item || dismissingClass) return
    try {
      setDismissingClass(true)
      await api.dismissClassSentNotification(item.class_key)
      setSummary((current) => (current ? { ...current, class_sent: undefined } : current))
    } catch (err) {
      console.warn('Failed to dismiss class-sent notification:', err)
    } finally {
      setDismissingClass(false)
    }
  }

  function openClassSent() {
    const item = summary?.class_sent
    if (!item) return
    navigate('/mentor-head')
  }

  async function dismissSessionReschedule() {
    const item = summary?.session_reschedule
    if (!item || dismissingReschedule) return
    try {
      setDismissingReschedule(true)
      await api.dismissSessionRescheduleNotification(item.id)
      setSummary((current) => (current ? { ...current, session_reschedule: undefined } : current))
    } catch (err) {
      console.warn('Failed to dismiss session-reschedule notification:', err)
    } finally {
      setDismissingReschedule(false)
    }
  }

  function openSessionReschedule() {
    const item = summary?.session_reschedule
    if (!item) return
    navigate(`/student-success/class?class_key=${encodeURIComponent(item.class_key)}`)
  }

  return (
    <div style={{ display: 'grid', gap: '10px', marginBottom: '16px' }}>
      {summary.session_reschedule && (
        <div
          style={{
            ...bannerStyle,
            borderColor: '#f59e0b',
            background: '#fff7e6',
            color: '#8a4b00',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: '12px',
          }}
        >
          <button
            type="button"
            onClick={openSessionReschedule}
            style={{ ...bannerButtonStyle, color: '#8a4b00' }}
          >
            <strong>Session rescheduled by Mentor Head.</strong>{' '}
            {summary.session_reschedule.unread_count > 1 ? `${summary.session_reschedule.unread_count} unread changes · ` : ''}
            L{summary.session_reschedule.level} · Class {summary.session_reschedule.class_number} · S{summary.session_reschedule.session_number} ·{' '}
            {summary.session_reschedule.old_date} {summary.session_reschedule.old_time.slice(0, 5)} → {summary.session_reschedule.new_date} {summary.session_reschedule.new_time.slice(0, 5)}
          </button>
          <button
            type="button"
            onClick={() => void dismissSessionReschedule()}
            disabled={dismissingReschedule}
            aria-label="Dismiss reschedule notification"
            title="Dismiss"
            style={dismissButtonStyle}
          >
            {dismissingReschedule ? '…' : '×'}
          </button>
        </div>
      )}

      {summary.class_sent && (
        <div
          style={{
            ...bannerStyle,
            borderColor: '#198754',
            background: '#eaf8ef',
            color: '#0f5132',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: '12px',
          }}
        >
          <button
            type="button"
            onClick={openClassSent}
            style={{ ...bannerButtonStyle, color: '#0f5132' }}
          >
            <strong>New class sent to Mentor Head.</strong>{' '}
            {summary.class_sent.unread_count > 1 ? `${summary.class_sent.unread_count} unread classes · ` : ''}
            L{summary.class_sent.level} · Class {summary.class_sent.class_number} · {summary.class_sent.student_count} students
          </button>
          <button
            type="button"
            onClick={() => void dismissClassSent()}
            disabled={dismissingClass}
            aria-label="Dismiss class notification"
            title="Dismiss"
            style={dismissButtonStyle}
          >
            {dismissingClass ? '…' : '×'}
          </button>
        </div>
      )}

      {summary.daily_report && (
        <button
          type="button"
          onClick={() => void openDailyReport()}
          style={{
            ...bannerStyle,
            borderColor: '#0d6efd',
            background: '#e7f1ff',
            color: '#084298',
          }}
        >
          <strong>Daily report is ready.</strong>{' '}
          {summary.daily_report.classes_taught}/{summary.daily_report.classes_scheduled} classes taught ·{' '}
          {summary.daily_report.classes_missing_report} missing mentor report(s) ·{' '}
          {summary.daily_report.absent_students}/{summary.daily_report.expected_students} students absent
        </button>
      )}

      {summary.complaint && (
        <button
          type="button"
          onClick={() => void openComplaint()}
          style={{
            ...bannerStyle,
            borderColor: '#dc3545',
            background: '#f8d7da',
            color: '#842029',
          }}
        >
          <strong>New complaint from Student Success.</strong>{' '}
          {summary.complaint.unread_count > 1 ? `${summary.complaint.unread_count} unread complaints · ` : ''}
          {summary.complaint.student_name || summary.complaint.student_phone} · {summary.complaint.class_key}
        </button>
      )}
    </div>
  )
}

const bannerStyle = {
  width: '100%',
  border: '1px solid',
  borderRadius: '10px',
  padding: '14px 16px',
  textAlign: 'left' as const,
  cursor: 'pointer',
  fontSize: '15px',
  lineHeight: 1.45,
  boxShadow: '0 4px 14px rgba(0,0,0,0.06)',
}

const bannerButtonStyle = {
  appearance: 'none' as const,
  background: 'transparent',
  border: 0,
  padding: 0,
  margin: 0,
  textAlign: 'left' as const,
  cursor: 'pointer',
  font: 'inherit',
  flex: 1,
}

const dismissButtonStyle = {
  backgroundColor: 'transparent',
  color: '#466652',
  border: '1px solid #b7d6c0',
  width: '32px',
  height: '32px',
  borderRadius: '50%',
  cursor: 'pointer',
  fontSize: '18px',
  lineHeight: 1,
  flexShrink: 0,
}
