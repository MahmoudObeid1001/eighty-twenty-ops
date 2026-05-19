import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, AvailabilityReminderNotification } from '../api/client'

export default function AvailabilityReminderBanner({ userRole }: { userRole: string }) {
  const [reminder, setReminder] = useState<AvailabilityReminderNotification | null>(null)
  const [dismissing, setDismissing] = useState(false)

  const enabled = userRole === 'mentor' || userRole === 'mentor_head'

  useEffect(() => {
    if (!enabled) {
      setReminder(null)
      return
    }
    let cancelled = false
    async function load() {
      try {
        const data = await api.getAvailabilityReminder()
        if (!cancelled) {
          setReminder(data.reminder || null)
        }
      } catch (err) {
        console.warn('Failed to load availability reminder:', err)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [enabled])

  async function dismiss() {
    if (!reminder || dismissing) return
    try {
      setDismissing(true)
      await api.dismissAvailabilityReminder(reminder.month)
      setReminder(null)
    } catch (err) {
      console.warn('Failed to dismiss availability reminder:', err)
    } finally {
      setDismissing(false)
    }
  }

  if (!enabled || !reminder) {
    return null
  }

  return (
    <div
      style={{
        marginBottom: '16px',
        border: '1px solid #f1c27d',
        background: 'linear-gradient(135deg, #fff8e7 0%, #fff3d1 100%)',
        color: '#7a4b11',
        borderRadius: '10px',
        padding: '14px 16px',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        gap: '12px',
        boxShadow: '0 4px 14px rgba(122, 75, 17, 0.08)',
      }}
    >
      <div style={{ flex: 1 }}>
        <div style={{ fontWeight: 700, marginBottom: '4px' }}>{reminder.title}</div>
        <div style={{ fontSize: '14px', lineHeight: 1.45 }}>{reminder.message}</div>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
        <Link
          to={reminder.action_path}
          style={{
            textDecoration: 'none',
            color: '#7a4b11',
            border: '1px solid #d9a95f',
            borderRadius: '6px',
            padding: '8px 12px',
            fontWeight: 700,
            background: '#fff',
            whiteSpace: 'nowrap',
          }}
        >
          {reminder.action_label}
        </Link>
        <button
          type="button"
          onClick={() => void dismiss()}
          disabled={dismissing}
          aria-label="Dismiss availability reminder"
          style={{
            background: 'transparent',
            border: 'none',
            color: '#7a4b11',
            fontSize: '20px',
            cursor: dismissing ? 'not-allowed' : 'pointer',
            padding: '0 4px',
            opacity: dismissing ? 0.6 : 1,
          }}
        >
          {dismissing ? '…' : '×'}
        </button>
      </div>
    </div>
  )
}
