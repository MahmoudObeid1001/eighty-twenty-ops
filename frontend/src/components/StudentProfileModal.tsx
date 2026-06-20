// StudentProfileModal component - Universal Student Profile
import { useState, useEffect } from 'react'
import { api, UniversalStudentProfile, AcademicHistoryItem, CurrentClassStatus, TimelineItem, StudentPaymentHistoryItem } from '../api/client'

interface StudentProfileModalProps {
    studentId: string
    onClose: () => void
    onStudentUpdated?: (student: UniversalStudentProfile) => void
}

export default function StudentProfileModal({ studentId, onClose, onStudentUpdated }: StudentProfileModalProps) {
    const [activeTab, setActiveTab] = useState<'history' | 'current' | 'notes' | 'payments'>('history')
    const [profile, setProfile] = useState<UniversalStudentProfile | null>(null)
    const [history, setHistory] = useState<AcademicHistoryItem[]>([])
    const [currentStatus, setCurrentStatus] = useState<CurrentClassStatus | null>(null)
    const [notes, setNotes] = useState<TimelineItem[]>([])
    const [paymentHistory, setPaymentHistory] = useState<StudentPaymentHistoryItem[]>([])
    const [userRole, setUserRole] = useState<string>('')
    const [isEditingBasicInfo, setIsEditingBasicInfo] = useState(false)
    const [editFullName, setEditFullName] = useState('')
    const [editGender, setEditGender] = useState('')
    const [editPhone, setEditPhone] = useState('')
    const [savingBasicInfo, setSavingBasicInfo] = useState(false)
    const [basicInfoError, setBasicInfoError] = useState<string | null>(null)
    const [loading, setLoading] = useState(true)
    const [error, setError] = useState<string | null>(null)

    useEffect(() => {
        loadData()
    }, [studentId])

    async function loadData() {
        setLoading(true)
        setError(null)
        try {
            const [profileData, historyData, statusData, notesData, me] = await Promise.all([
                api.getStudentProfile(studentId),
                api.getStudentHistory(studentId),
                api.getStudentCurrentStatus(studentId),
                api.getStudentNotes(studentId),
                api.getMe(),
            ])
            const role = me.role || ''
            const paymentsData = role === 'admin' || role === 'manager'
                ? await api.getStudentPaymentHistory(studentId)
                : []

            setProfile(profileData)
            setHistory(historyData || [])
            setCurrentStatus(statusData)
            setNotes(notesData || [])
            setPaymentHistory(paymentsData || [])
            setUserRole(role)
        } catch (err) {
            console.error('Failed to load student data:', err)
            setError(err instanceof Error ? err.message : 'Failed to load student data')
        } finally {
            setLoading(false)
        }
    }

    const canEditBasicInfo = userRole === 'admin' || userRole === 'mentor_head' || userRole === 'manager'
    const canViewPaymentHistory = userRole === 'admin' || userRole === 'manager'

    function startEditingBasicInfo() {
        if (!profile) return
        setEditFullName(profile.full_name)
        setEditGender(profile.gender || '')
        setEditPhone(profile.phone)
        setBasicInfoError(null)
        setIsEditingBasicInfo(true)
    }

    async function handleSaveBasicInfo() {
        if (!profile) return

        const fullName = editFullName.trim()
        const phone = editPhone.trim()

        if (!fullName || !phone) {
            setBasicInfoError('Full name and phone are required.')
            return
        }

        setSavingBasicInfo(true)
        setBasicInfoError(null)
        try {
            const updatedProfile = await api.updateStudentBasicInfo(studentId, {
                full_name: fullName,
                gender: editGender,
                phone,
            })
            setProfile(updatedProfile)
            setIsEditingBasicInfo(false)
            onStudentUpdated?.(updatedProfile)
        } catch (err) {
            setBasicInfoError(err instanceof Error ? err.message : 'Failed to update student info')
        } finally {
            setSavingBasicInfo(false)
        }
    }

    if (loading || !profile) {
        return (
            <div
                style={{
                    position: 'fixed',
                    top: 0,
                    left: 0,
                    right: 0,
                    bottom: 0,
                    background: 'rgba(0,0,0,0.5)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    zIndex: 9999,
                }}
                onClick={onClose}
            >
                <div
                    style={{
                        background: 'white',
                        borderRadius: '8px',
                        padding: '40px',
                        maxWidth: '900px',
                        width: '90%',
                    }}
                    onClick={(e) => e.stopPropagation()}
                >
                    <p style={{ textAlign: 'center', color: '#666' }}>
                        {error ? `Error: ${error}` : 'Loading...'}
                    </p>
                </div>
            </div>
        )
    }

    return (
        <div
            style={{
                position: 'fixed',
                top: 0,
                left: 0,
                right: 0,
                bottom: 0,
                background: 'rgba(0,0,0,0.5)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                zIndex: 9999,
                padding: '20px',
            }}
            onClick={onClose}
        >
            <div
                style={{
                    background: 'white',
                    borderRadius: '8px',
                    padding: '30px',
                    maxWidth: '900px',
                    width: '100%',
                    maxHeight: '90vh',
                    overflow: 'auto',
                }}
                onClick={(e) => e.stopPropagation()}
            >
                {/* Header */}
                <div style={{ borderBottom: '2px solid #e0e0e0', paddingBottom: '20px', marginBottom: '20px' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start' }}>
                        <div>
                            <h2 style={{ margin: 0, marginBottom: '8px', display: 'flex', alignItems: 'center', gap: '8px' }}>
                                {profile.full_name}
                                {profile.is_returning && (
                                    <span style={{ color: '#FFD700', fontSize: '20px' }} title="Promoted Student">⭐</span>
                                )}
                            </h2>
                            <div style={{ color: '#666', fontSize: '14px', marginBottom: '4px' }}>
                                {profile.phone} • Level {profile.current_level} • {profile.status}
                            </div>
                            <div style={{ marginTop: '8px', fontSize: '14px' }}>
                                <strong>Remaining Credits:</strong> {profile.remaining_credits}
                            </div>
                        </div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                            {canEditBasicInfo && !isEditingBasicInfo && (
                                <button
                                    type="button"
                                    onClick={startEditingBasicInfo}
                                    style={{
                                        border: '1px solid #cbd5e1',
                                        background: '#fff',
                                        color: '#0f172a',
                                        borderRadius: '10px',
                                        padding: '10px 14px',
                                        cursor: 'pointer',
                                        fontWeight: 700,
                                        fontSize: '14px',
                                    }}
                                >
                                    Edit name / phone
                                </button>
                            )}
                            <button
                                onClick={onClose}
                                style={{
                                    fontSize: '28px',
                                    border: 'none',
                                    background: 'none',
                                    cursor: 'pointer',
                                    color: '#666',
                                    padding: '0',
                                    lineHeight: '1',
                                }}
                            >
                                ×
                            </button>
                        </div>
                    </div>
                    {isEditingBasicInfo && (
                        <div
                            style={{
                                marginTop: '18px',
                                padding: '16px',
                                border: '1px solid #dbe4ea',
                                borderRadius: '12px',
                                background: '#f8fbfd',
                            }}
                        >
                            <div style={{ fontSize: '15px', fontWeight: 700, marginBottom: '12px', color: '#0f172a' }}>
                                Edit student basic info
                            </div>
                            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '12px' }}>
                                <div>
                                    <label style={{ display: 'block', fontSize: '13px', fontWeight: 600, color: '#475569', marginBottom: '6px' }}>
                                        Full name
                                    </label>
                                    <input
                                        type="text"
                                        value={editFullName}
                                        onChange={(e) => setEditFullName(e.target.value)}
                                        style={basicInfoInputStyle}
                                    />
                                </div>
                                <div>
                                    <label style={{ display: 'block', fontSize: '13px', fontWeight: 600, color: '#475569', marginBottom: '6px' }}>
                                        Gender
                                    </label>
                                    <select
                                        value={editGender}
                                        onChange={(e) => setEditGender(e.target.value)}
                                        style={basicInfoInputStyle}
                                    >
                                        <option value="">Unknown</option>
                                        <option value="male">Male</option>
                                        <option value="female">Female</option>
                                    </select>
                                </div>
                                <div>
                                    <label style={{ display: 'block', fontSize: '13px', fontWeight: 600, color: '#475569', marginBottom: '6px' }}>
                                        Phone
                                    </label>
                                    <input
                                        type="tel"
                                        value={editPhone}
                                        onChange={(e) => setEditPhone(e.target.value)}
                                        style={basicInfoInputStyle}
                                    />
                                </div>
                            </div>
                            {basicInfoError && (
                                <div style={{ marginTop: '12px', color: '#b91c1c', fontSize: '13px', fontWeight: 600 }}>
                                    {basicInfoError}
                                </div>
                            )}
                            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '16px' }}>
                                <button
                                    type="button"
                                    onClick={() => {
                                        setIsEditingBasicInfo(false)
                                        setBasicInfoError(null)
                                    }}
                                    style={secondaryButtonStyle}
                                >
                                    Cancel
                                </button>
                                <button
                                    type="button"
                                    onClick={handleSaveBasicInfo}
                                    disabled={savingBasicInfo}
                                    style={{
                                        ...primaryButtonStyle,
                                        opacity: savingBasicInfo ? 0.7 : 1,
                                        cursor: savingBasicInfo ? 'default' : 'pointer',
                                    }}
                                >
                                    {savingBasicInfo ? 'Saving...' : 'Save changes'}
                                </button>
                            </div>
                        </div>
                    )}
                </div>

                {/* Tabs */}
                <div style={{ display: 'flex', gap: '20px', borderBottom: '1px solid #e0e0e0', marginBottom: '20px' }}>
                    <button
                        onClick={() => setActiveTab('history')}
                        style={{
                            padding: '10px 20px',
                            border: 'none',
                            background: 'none',
                            borderBottom: activeTab === 'history' ? '3px solid #007bff' : 'none',
                            fontWeight: activeTab === 'history' ? 600 : 400,
                            cursor: 'pointer',
                            color: activeTab === 'history' ? '#007bff' : '#666',
                        }}
                    >
                        Academic History
                    </button>
                    <button
                        onClick={() => setActiveTab('current')}
                        style={{
                            padding: '10px 20px',
                            border: 'none',
                            background: 'none',
                            borderBottom: activeTab === 'current' ? '3px solid #007bff' : 'none',
                            fontWeight: activeTab === 'current' ? 600 : 400,
                            cursor: 'pointer',
                            color: activeTab === 'current' ? '#007bff' : '#666',
                        }}
                    >
                        Current Status
                    </button>
                    <button
                        onClick={() => setActiveTab('notes')}
                        style={{
                            padding: '10px 20px',
                            border: 'none',
                            background: 'none',
                            borderBottom: activeTab === 'notes' ? '3px solid #007bff' : 'none',
                            fontWeight: activeTab === 'notes' ? 600 : 400,
                            cursor: 'pointer',
                            color: activeTab === 'notes' ? '#007bff' : '#666',
                        }}
                    >
                        Notes Timeline
                    </button>
                    {canViewPaymentHistory && (
                        <button
                            onClick={() => setActiveTab('payments')}
                            style={{
                                padding: '10px 20px',
                                border: 'none',
                                background: 'none',
                                borderBottom: activeTab === 'payments' ? '3px solid #007bff' : 'none',
                                fontWeight: activeTab === 'payments' ? 600 : 400,
                                cursor: 'pointer',
                                color: activeTab === 'payments' ? '#007bff' : '#666',
                            }}
                        >
                            Payment History
                        </button>
                    )}
                </div>

                {/* Tab Content */}
                <div style={{ maxHeight: '500px', overflowY: 'auto' }}>
                    {activeTab === 'history' && <AcademicHistoryTab history={history} />}
                    {activeTab === 'current' && <CurrentStatusTab status={currentStatus} />}
                    {activeTab === 'notes' && <NotesTimelineTab notes={notes} />}
                    {activeTab === 'payments' && canViewPaymentHistory && <PaymentHistoryTab payments={paymentHistory} />}
                </div>
            </div>
        </div>
    )
}

// Academic History Tab
function AcademicHistoryTab({ history }: { history: AcademicHistoryItem[] }) {
    if (history.length === 0) {
        return (
            <div style={{ textAlign: 'center', padding: '40px', color: '#666' }}>
                No academic history found
            </div>
        )
    }

    return (
        <div style={{ overflowX: 'auto', width: '100%' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
                <tr style={{ background: '#f8f9fa', borderBottom: '2px solid #dee2e6' }}>
                    <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Level</th>
                    <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Schedule</th>
                    <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Mentor</th>
                    <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Grade</th>
                    <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Outcome</th>
                    <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Completed</th>
                </tr>
            </thead>
            <tbody>
                {history.map((item) => (
                    <tr key={item.id} style={{ borderBottom: '1px solid #dee2e6' }}>
                        <td style={{ padding: '12px' }}>Level {item.level}</td>
                        <td style={{ padding: '12px' }}>{item.class_days} {item.class_time}</td>
                        <td style={{ padding: '12px' }}>{item.mentor_name || '-'}</td>
                        <td style={{ padding: '12px' }}>
                            {item.final_grade ? (
                                <span style={{
                                    padding: '4px 8px',
                                    borderRadius: '4px',
                                    background: item.final_grade === 'A' ? '#d4edda' : item.final_grade === 'B' ? '#d1ecf1' : item.final_grade === 'C' ? '#fff3cd' : '#f8d7da',
                                    color: item.final_grade === 'A' ? '#155724' : item.final_grade === 'B' ? '#0c5460' : item.final_grade === 'C' ? '#856404' : '#721c24',
                                    fontWeight: 600,
                                }}>
                                    {item.final_grade}
                                </span>
                            ) : '-'}
                        </td>
                        <td style={{ padding: '12px' }}>
                            {item.outcome ? (
                                <span style={{
                                    padding: '4px 8px',
                                    borderRadius: '4px',
                                    background: item.outcome === 'promoted' ? '#d4edda' : '#fff3cd',
                                    color: item.outcome === 'promoted' ? '#155724' : '#856404',
                                }}>
                                    {item.outcome}
                                </span>
                            ) : '-'}
                        </td>
                        <td style={{ padding: '12px', fontSize: '13px', color: '#666' }}>
                            {item.completed_at ? new Date(item.completed_at).toLocaleDateString() : 'In Progress'}
                        </td>
                    </tr>
                ))}
            </tbody>
        </table>
        </div>
    )
}

// Current Status Tab
function CurrentStatusTab({ status }: { status: CurrentClassStatus | null }) {
    if (!status) {
        return (
            <div style={{ textAlign: 'center', padding: '40px', color: '#666' }}>
                Student is not currently enrolled in a class
            </div>
        )
    }

    return (
        <div>
            {/* Class Info */}
            <div style={{ background: '#f8f9fa', padding: '20px', borderRadius: '8px', marginBottom: '20px' }}>
                <h3 style={{ margin: '0 0 12px 0', fontSize: '18px' }}>Current Class</h3>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: '12px', fontSize: '14px' }}>
                    <div><strong>Level:</strong> {status.level}</div>
                    <div><strong>Schedule:</strong> {status.class_days} {status.class_time}</div>
                    <div><strong>Mentor:</strong> {status.mentor_name || 'Not assigned'}</div>
                    <div><strong>Current Session:</strong> {status.current_session} / 8</div>
                </div>
            </div>

            {/* Attendance Stats */}
            <div style={{ marginBottom: '20px' }}>
                <h3 style={{ margin: '0 0 12px 0', fontSize: '18px' }}>Attendance Summary</h3>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(70px, 1fr))', gap: '12px' }}>
                    <div style={{ background: '#d4edda', padding: '16px', borderRadius: '8px', textAlign: 'center' }}>
                        <div style={{ fontSize: '24px', fontWeight: 600, color: '#155724' }}>{status.attendance_stats.present}</div>
                        <div style={{ fontSize: '12px', color: '#155724' }}>Present</div>
                    </div>
                    <div style={{ background: '#f8d7da', padding: '16px', borderRadius: '8px', textAlign: 'center' }}>
                        <div style={{ fontSize: '24px', fontWeight: 600, color: '#721c24' }}>{status.attendance_stats.absent}</div>
                        <div style={{ fontSize: '12px', color: '#721c24' }}>Absent</div>
                    </div>
                    <div style={{ background: '#fff3cd', padding: '16px', borderRadius: '8px', textAlign: 'center' }}>
                        <div style={{ fontSize: '24px', fontWeight: 600, color: '#856404' }}>{status.attendance_stats.late}</div>
                        <div style={{ fontSize: '12px', color: '#856404' }}>Late</div>
                    </div>
                    <div style={{ background: '#d1ecf1', padding: '16px', borderRadius: '8px', textAlign: 'center' }}>
                        <div style={{ fontSize: '24px', fontWeight: 600, color: '#0c5460' }}>{status.attendance_stats.total}</div>
                        <div style={{ fontSize: '12px', color: '#0c5460' }}>Total</div>
                    </div>
                </div>
            </div>

            {/* Session Details */}
            <div>
                <h3 style={{ margin: '0 0 12px 0', fontSize: '18px' }}>Session Details</h3>
                <div style={{ overflowX: 'auto', width: '100%' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                    <thead>
                        <tr style={{ background: '#f8f9fa', borderBottom: '2px solid #dee2e6' }}>
                            <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Session</th>
                            <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Date</th>
                            <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Status</th>
                        </tr>
                    </thead>
                    <tbody>
                        {(status.session_details || []).map((session) => (
                            <tr key={session.session_number} style={{ borderBottom: '1px solid #dee2e6' }}>
                                <td style={{ padding: '12px' }}>Session {session.session_number}</td>
                                <td style={{ padding: '12px', fontSize: '13px', color: '#666' }}>{session.date}</td>
                                <td style={{ padding: '12px' }}>
                                    <span style={{
                                        padding: '4px 8px',
                                        borderRadius: '4px',
                                        background: session.status === 'PRESENT' ? '#d4edda' : session.status === 'ABSENT' ? '#f8d7da' : session.status === 'LATE' ? '#fff3cd' : '#e9ecef',
                                        color: session.status === 'PRESENT' ? '#155724' : session.status === 'ABSENT' ? '#721c24' : session.status === 'LATE' ? '#856404' : '#495057',
                                        fontSize: '12px',
                                        fontWeight: 600,
                                    }}>
                                        {session.status}
                                    </span>
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
                </div>
            </div>
        </div>
    )
}

// Payment History Tab
function PaymentHistoryTab({ payments }: { payments: StudentPaymentHistoryItem[] }) {
    if (payments.length === 0) {
        return (
            <div style={{ textAlign: 'center', padding: '40px', color: '#666' }}>
                No payment history found
            </div>
        )
    }

    const totalIn = payments
        .filter((payment) => payment.direction === 'in')
        .reduce((sum, payment) => sum + payment.amount, 0)
    const totalOut = payments
        .filter((payment) => payment.direction === 'out')
        .reduce((sum, payment) => sum + payment.amount, 0)
    const net = totalIn - totalOut

    return (
        <div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: '12px', marginBottom: '16px' }}>
                <div style={{ background: '#e8f5e9', padding: '14px', borderRadius: '8px' }}>
                    <div style={{ fontSize: '12px', color: '#2e7d32', fontWeight: 700 }}>Total Paid In</div>
                    <div style={{ fontSize: '20px', color: '#1b5e20', fontWeight: 800 }}>{totalIn} EGP</div>
                </div>
                <div style={{ background: '#ffebee', padding: '14px', borderRadius: '8px' }}>
                    <div style={{ fontSize: '12px', color: '#b71c1c', fontWeight: 700 }}>Refunds / Out</div>
                    <div style={{ fontSize: '20px', color: '#b71c1c', fontWeight: 800 }}>{totalOut} EGP</div>
                </div>
                <div style={{ background: '#eef6ff', padding: '14px', borderRadius: '8px' }}>
                    <div style={{ fontSize: '12px', color: '#1d4ed8', fontWeight: 700 }}>Net</div>
                    <div style={{ fontSize: '20px', color: '#1e3a8a', fontWeight: 800 }}>{net} EGP</div>
                </div>
            </div>
            <div style={{ overflowX: 'auto', width: '100%' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                    <thead>
                        <tr style={{ background: '#f8f9fa', borderBottom: '2px solid #dee2e6' }}>
                            <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Date</th>
                            <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Type</th>
                            <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Amount</th>
                            <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Method</th>
                            <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Source</th>
                            <th style={{ padding: '12px', textAlign: 'left', fontWeight: 600 }}>Notes</th>
                        </tr>
                    </thead>
                    <tbody>
                        {payments.map((payment) => (
                            <tr key={payment.id} style={{ borderBottom: '1px solid #dee2e6' }}>
                                <td style={{ padding: '12px', fontSize: '13px', color: '#666', whiteSpace: 'nowrap' }}>
                                    {payment.payment_date || new Date(payment.created_at).toLocaleDateString()}
                                </td>
                                <td style={{ padding: '12px' }}>{formatPaymentLabel(payment.type)}</td>
                                <td style={{ padding: '12px', whiteSpace: 'nowrap' }}>
                                    <span style={{
                                        padding: '4px 8px',
                                        borderRadius: '4px',
                                        background: payment.direction === 'out' ? '#f8d7da' : '#d4edda',
                                        color: payment.direction === 'out' ? '#721c24' : '#155724',
                                        fontWeight: 700,
                                    }}>
                                        {payment.direction === 'out' ? '-' : '+'}{payment.amount} EGP
                                    </span>
                                </td>
                                <td style={{ padding: '12px' }}>{formatPaymentLabel(payment.payment_method) || '-'}</td>
                                <td style={{ padding: '12px' }}>{formatPaymentLabel(payment.source)}</td>
                                <td style={{ padding: '12px', color: '#666' }}>{payment.notes || '-'}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        </div>
    )
}

// Notes Timeline Tab
function NotesTimelineTab({ notes }: { notes: TimelineItem[] }) {
    if (notes.length === 0) {
        return (
            <div style={{ textAlign: 'center', padding: '40px', color: '#666' }}>
                No notes or follow-ups found
            </div>
        )
    }

    return (
        <div>
            {notes.map((note) => (
                <div
                    key={note.id}
                    style={{
                        borderLeft: '3px solid ' + getTimelineColor(note.type),
                        padding: '16px',
                        marginBottom: '16px',
                        background: '#f8f9fa',
                        borderRadius: '4px',
                    }}
                >
                    <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '8px' }}>
                        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                            <span style={{
                                padding: '2px 8px',
                                borderRadius: '4px',
                                background: getTimelineColor(note.type),
                                color: 'white',
                                fontSize: '11px',
                                fontWeight: 600,
                                textTransform: 'uppercase',
                            }}>
                                {formatTimelineType(note.type)}
                            </span>
                            {note.is_private && (
                                <span style={{
                                    padding: '2px 8px',
                                    borderRadius: '4px',
                                    background: '#ffc107',
                                    color: '#000',
                                    fontSize: '11px',
                                    fontWeight: 600,
                                }}>
                                    PRIVATE
                                </span>
                            )}
                            {note.class_key && (
                                <span style={{ fontSize: '12px', color: '#666' }}>
                                    {note.class_key} {note.session > 0 && `• Session ${note.session}`}
                                </span>
                            )}
                        </div>
                        <div style={{ fontSize: '12px', color: '#666' }}>
                            {new Date(note.created_at).toLocaleString()}
                        </div>
                    </div>
                    {note.type === 'grade_note' && note.translated_text && note.translated_text !== note.text ? (
                        <>
                            <div
                                style={{
                                    marginBottom: '10px',
                                    padding: '10px 12px',
                                    background: '#eef6ff',
                                    borderRadius: '8px',
                                    color: '#0f172a',
                                }}
                            >
                                <div style={{ fontSize: '11px', fontWeight: 700, color: '#2563eb', marginBottom: '6px', textTransform: 'uppercase' }}>
                                    Arabic translation
                                </div>
                                <div style={{ fontSize: '14px', lineHeight: 1.6, direction: 'rtl', textAlign: 'right' }}>
                                    {note.translated_text}
                                </div>
                            </div>
                            <div style={{ fontSize: '13px', marginBottom: '8px', color: '#475569' }}>{note.text}</div>
                        </>
                    ) : (
                        <div
                            style={{ fontSize: '14px', marginBottom: '8px' }}
                        >
                            {note.text}
                        </div>
                    )}
                    <div style={{ fontSize: '12px', color: '#666' }}>
                        By: {note.created_by}
                    </div>
                </div>
            ))}
        </div>
    )
}

function getTimelineColor(type: TimelineItem['type']): string {
    if (type === 'note') return '#007bff'
    if (type === 'followup') return '#28a745'
    return '#6f42c1'
}

function formatTimelineType(type: TimelineItem['type']): string {
    if (type === 'grade_note') return 'final grade note'
    return type
}

function formatPaymentLabel(value: string): string {
    return value
        .split('_')
        .filter(Boolean)
        .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
        .join(' ')
}

const basicInfoInputStyle = {
    width: '100%',
    padding: '10px 12px',
    border: '1px solid #cbd5e1',
    borderRadius: '8px',
    fontSize: '14px',
    background: '#fff',
}

const primaryButtonStyle = {
    border: 'none',
    background: '#4ec6e0',
    color: '#fff',
    borderRadius: '10px',
    padding: '10px 16px',
    fontWeight: 700,
    fontSize: '14px',
}

const secondaryButtonStyle = {
    border: '1px solid #cbd5e1',
    background: '#fff',
    color: '#334155',
    borderRadius: '10px',
    padding: '10px 16px',
    fontWeight: 700,
    fontSize: '14px',
    cursor: 'pointer',
}
