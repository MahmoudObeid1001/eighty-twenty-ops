import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, MentorReminder } from '../api/client'

export default function MentorReminderBanner() {
    const [reminders, setReminders] = useState<MentorReminder[]>([])
    const [dismissed, setDismissed] = useState<Set<string>>(new Set())
    const navigate = useNavigate()

    useEffect(() => {
        loadReminders()
    }, [])

    async function loadReminders() {
        try {
            const data = await api.getMentorReminders()
            setReminders(data.reminders || [])
        } catch (err) {
            console.error('Failed to load reminders:', err)
        }
    }

    function handleDismiss(reminder: MentorReminder) {
        const key = `${reminder.class_key}-${reminder.type}`
        setDismissed(new Set(dismissed).add(key))

        // Auto-dismiss after 10 seconds
        setTimeout(() => {
            setDismissed((prev) => {
                const next = new Set(prev)
                next.delete(key)
                return next
            })
        }, 10000)
    }

    function handleClick(reminder: MentorReminder) {
        navigate(`/mentor/class?class_key=${encodeURIComponent(reminder.class_key)}`)
    }

    const visibleReminders = reminders.filter((r) => {
        const key = `${r.class_key}-${r.type}`
        return !dismissed.has(key)
    })

    if (visibleReminders.length === 0) {
        return null
    }

    return (
        <div style={{ marginBottom: '20px' }}>
            {visibleReminders.map((reminder) => {
                const key = `${reminder.class_key}-${reminder.type}`
                const isAttendance = reminder.type === 'attendance'

                return (
                    <div
                        key={key}
                        style={{
                            background: isAttendance ? '#FFF3CD' : '#D1ECF1',
                            border: `1px solid ${isAttendance ? '#FFE69C' : '#BEE5EB'}`,
                            borderRadius: '8px',
                            padding: '16px',
                            marginBottom: '12px',
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'center',
                            cursor: 'pointer',
                        }}
                        onClick={() => handleClick(reminder)}
                    >
                        <div>
                            <div style={{ fontWeight: 'bold', marginBottom: '4px', color: '#333' }}>
                                {isAttendance ? '⚠️ Attendance Required' : '📝 Grading Required'}
                            </div>
                            <div style={{ fontSize: '14px', color: '#666', marginBottom: '4px' }}>
                                Level {reminder.level} · {reminder.class_days} · {reminder.class_time} · Class {reminder.class_number}
                            </div>
                            <div style={{ fontSize: '14px', color: '#555' }}>
                                {reminder.message}
                            </div>
                        </div>
                        <button
                            onClick={(e) => {
                                e.stopPropagation()
                                handleDismiss(reminder)
                            }}
                            style={{
                                background: 'transparent',
                                border: 'none',
                                fontSize: '20px',
                                cursor: 'pointer',
                                padding: '4px 8px',
                                color: '#666',
                            }}
                        >
                            ×
                        </button>
                    </div>
                )
            })}
        </div>
    )
}
