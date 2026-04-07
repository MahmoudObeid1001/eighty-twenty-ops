import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type OpsNotificationSummary } from '../api/client'

export default function OpsNotificationBanner({ userRole }: { userRole: string }) {
  const [summary, setSummary] = useState<OpsNotificationSummary | null>(null)
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  const enabled = userRole === 'mentor_head' || userRole === 'manager'

  useEffect(() => {
    if (!enabled) {
      setSummary(null)
      return
    }
    let cancelled = false
    async function load() {
      try {
        setLoading(true)
        const data = await api.getOpsNotifications()
        if (!cancelled) {
          setSummary(data)
        }
      } catch (err) {
        console.warn('Failed to load ops notifications:', err)
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [enabled])

  if (!enabled || loading || !summary || (!summary.daily_report && !summary.complaint)) {
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

  return (
    <div style={{ display: 'grid', gap: '10px', marginBottom: '16px' }}>
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
