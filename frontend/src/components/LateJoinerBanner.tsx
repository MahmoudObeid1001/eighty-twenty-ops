import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'

interface LateJoinerNotification {
    id: string
    lead_id: string
    full_name: string
    class_key: string
    joined_at_session_number: number
    created_at: string
}

interface LateJoinerBannerProps {
    userRole?: string
}

export default function LateJoinerBanner({ userRole }: LateJoinerBannerProps) {
    const [notifications, setNotifications] = useState<LateJoinerNotification[]>([])
    const [dismissing, setDismissing] = useState<Set<string>>(new Set())

    useEffect(() => {
        loadNotifications()
    }, [])

    async function loadNotifications() {
        try {
            const data = await api.getLateJoinerNotifications()
            setNotifications(data || [])
        } catch (err) {
            console.error('Failed to load late joiner notifications:', err)
        }
    }

    async function acknowledge(notificationId: string) {
        setDismissing(prev => new Set(prev).add(notificationId))
        try {
            await api.acknowledgeLateJoinerNotification(notificationId)
            setNotifications(prev => prev.filter(n => n.id !== notificationId))
        } catch (err) {
            console.error('Failed to acknowledge notification:', err)
            setDismissing(prev => {
                const next = new Set(prev)
                next.delete(notificationId)
                return next
            })
        }
    }

    if (!notifications || notifications.length === 0) {
        return null
    }

    const getClassPath = (classKey: string) => {
        const encodedKey = encodeURIComponent(classKey)
        if (userRole === 'mentor') return `/mentor/class?class_key=${encodedKey}`
        if (userRole === 'mentor_head') return `/mentor-head/class?class_key=${encodedKey}`
        if (userRole === 'student_success') return `/student-success/class?class_key=${encodedKey}`
        return `/mentor/class?class_key=${encodedKey}` // fallback
    }

    return (
        <div style={{ marginBottom: '20px' }}>
            {notifications.map(notification => (
                <div
                    key={notification.id}
                    style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        backgroundColor: '#f7fbff',
                        borderLeft: '4px solid #007bff',
                        borderRight: '1px solid #d1e9ff',
                        borderTop: '1px solid #d1e9ff',
                        borderBottom: '1px solid #d1e9ff',
                        padding: '16px',
                        borderRadius: '6px',
                        marginBottom: '12px',
                        boxShadow: '0 2px 4px rgba(0,0,0,0.05)',
                        animation: 'slideIn 0.3s ease-out',
                    }}
                >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
                        <div
                            style={{
                                backgroundColor: '#007bff',
                                color: 'white',
                                width: '36px',
                                height: '36px',
                                borderRadius: '50%',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                fontSize: '18px',
                            }}
                        >
                            🔔
                        </div>
                        <div>
                            <strong style={{ display: 'block', fontSize: '16px', color: '#333' }}>
                                Late Joiner: {notification.full_name}
                            </strong>
                            <span style={{ color: '#666', fontSize: '14px' }}>
                                Added to class <strong style={{ color: '#444' }}>{notification.class_key}</strong> at Session{' '}
                                {notification.joined_at_session_number}
                            </span>
                        </div>
                    </div>
                    <div style={{ display: 'flex', gap: '12px', alignItems: 'center' }}>
                        <Link
                            to={getClassPath(notification.class_key)}
                            className="btn btn-sm btn-outline-primary"
                            style={{
                                padding: '6px 14px',
                                textDecoration: 'none',
                                display: 'inline-flex',
                                alignItems: 'center',
                                backgroundColor: 'transparent',
                                border: '1px solid #007bff',
                                color: '#007bff',
                                borderRadius: '4px',
                                fontSize: '14px',
                                fontWeight: 500
                            }}
                        >
                            Open Class
                        </Link>
                        <button
                            onClick={() => acknowledge(notification.id)}
                            disabled={dismissing.has(notification.id)}
                            className="btn btn-sm"
                            title="Dismiss"
                            aria-label="Dismiss notification"
                            style={{
                                backgroundColor: 'transparent',
                                color: '#666',
                                border: '1px solid #d0d7de',
                                width: '32px',
                                height: '32px',
                                borderRadius: '50%',
                                cursor: dismissing.has(notification.id) ? 'not-allowed' : 'pointer',
                                fontSize: '18px',
                                lineHeight: 1,
                                opacity: dismissing.has(notification.id) ? 0.6 : 1,
                            }}
                        >
                            {dismissing.has(notification.id) ? '…' : '×'}
                        </button>
                    </div>
                </div>
            ))}
            <style>{`
        @keyframes slideIn {
          from {
            transform: translateY(-10px);
            opacity: 0;
          }
          to {
            transform: translateY(0);
            opacity: 1;
          }
        }
      `}</style>
        </div>
    )
}
