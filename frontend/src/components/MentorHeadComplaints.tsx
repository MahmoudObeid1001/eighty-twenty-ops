import { useEffect, useState } from 'react'
import { api, type ComplaintListItem } from '../api/client'

export default function MentorHeadComplaints() {
    const [complaints, setComplaints] = useState<ComplaintListItem[]>([])
    const [loading, setLoading] = useState(true)
    const [error, setError] = useState<string | null>(null)
    const [showResolved, setShowResolved] = useState(false)
    const [actionModal, setActionModal] = useState<{
        type: 'update' | 'resolve' | null
        complaint: ComplaintListItem | null
    }>({ type: null, complaint: null })
    const [viewComplaint, setViewComplaint] = useState<ComplaintListItem | null>(null)

    useEffect(() => {
        loadComplaints()
    }, [showResolved])

    async function loadComplaints() {
        try {
            setLoading(true)
            setError(null)
            const res = await api.getMentorHeadComplaints(showResolved)
            // Backend returns { complaints: [...] }, not just [...]
            if (res && Array.isArray(res.complaints)) {
                setComplaints(res.complaints)
            } else {
                // Safety guard: if response shape is unexpected, default to empty array
                console.warn('Unexpected API response shape:', res)
                setComplaints([])
            }
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to load complaints')
        } finally {
            setLoading(false)
        }
    }

    if (loading) {
        return (
            <div style={{ padding: '40px', textAlign: 'center', background: 'white', borderRadius: '8px' }}>
                <p>Loading complaints...</p>
            </div>
        )
    }

    if (error) {
        return (
            <div style={{ padding: '40px', background: 'white', borderRadius: '8px' }}>
                <div style={{ background: '#f8d7da', padding: '16px', borderRadius: '8px', color: '#721c24' }}>
                    <strong>Error:</strong> {error}
                </div>
            </div>
        )
    }

    return (
        <>
            <div style={{ background: 'white', borderRadius: '8px', border: '1px solid #dee2e6', overflow: 'hidden' }}>
                <div style={{ padding: '16px', borderBottom: '1px solid #eee', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <h2 style={{ fontSize: '20px', margin: 0, color: '#dc3545' }}>Complaints</h2>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '14px' }}>
                        <label>Show Resolved:</label>
                        <input
                            type="checkbox"
                            checked={showResolved}
                            onChange={(e) => setShowResolved(e.target.checked)}
                        />
                    </div>
                </div>

                <div style={{ overflowX: 'auto' }}>
                    {complaints.length === 0 ? (
                        <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>
                            {showResolved ? 'No resolved complaints found.' : 'No active complaints.'}
                        </div>
                    ) : (
                        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '14px' }}>
                            <thead>
                                <tr style={{ textAlign: 'left', background: '#f8f9fa', borderBottom: '1px solid #eee' }}>
                                    <th style={{ padding: '12px' }}>Student</th>
                                    <th style={{ padding: '12px' }}>Class</th>
                                    <th style={{ padding: '12px' }}>Category</th>
                                    <th style={{ padding: '12px' }}>Urgency</th>
                                    <th style={{ padding: '12px' }}>Complaint</th>
                                    <th style={{ padding: '12px' }}>Status</th>
                                    <th style={{ padding: '12px' }}>Created</th>
                                    <th style={{ padding: '12px' }}>Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                {complaints.sort((a, b) => {
                                    const urgencyOrder: Record<string, number> = { 'critical': 0, 'high': 1, 'medium': 2, 'low': 3 };
                                    const aOrder = urgencyOrder[a.urgency.toLowerCase()] ?? 99;
                                    const bOrder = urgencyOrder[b.urgency.toLowerCase()] ?? 99;
                                    return aOrder - bOrder;
                                }).map((complaint) => (
                                    <tr
                                        key={complaint.id}
                                        style={{
                                            borderBottom: '1px solid #eee',
                                            borderLeft: (complaint.urgency.toLowerCase() === 'high' || complaint.urgency.toLowerCase() === 'critical') ? '4px solid #dc3545' : 'none',
                                            backgroundColor: (complaint.urgency.toLowerCase() === 'high' || complaint.urgency.toLowerCase() === 'critical') ? '#fffafa' : 'transparent'
                                        }}
                                    >
                                        <td style={{ padding: '12px' }}>
                                            <div style={{ fontWeight: 600 }}>{complaint.student_phone}</div>
                                        </td>
                                        <td style={{ padding: '12px' }}>
                                            <div style={{ fontSize: '12px' }}>{complaint.class_key}</div>
                                        </td>
                                        <td style={{ padding: '12px' }}>
                                            <span style={{
                                                padding: '4px 8px',
                                                borderRadius: '4px',
                                                fontSize: '11px',
                                                fontWeight: 600,
                                                background: '#e7f3ff',
                                                color: '#004085',
                                            }}>
                                                {complaint.category.replace(/_/g, ' ').toUpperCase()}
                                            </span>
                                        </td>
                                        <td style={{ padding: '12px' }}>
                                            <span style={{
                                                padding: '4px 8px',
                                                borderRadius: '4px',
                                                fontSize: '11px',
                                                fontWeight: 600,
                                                background: complaint.urgency === 'high' ? '#f8d7da' : complaint.urgency === 'medium' ? '#fff3cd' : '#d4edda',
                                                color: complaint.urgency === 'high' ? '#721c24' : complaint.urgency === 'medium' ? '#856404' : '#155724',
                                            }}>
                                                {complaint.urgency.toUpperCase()}
                                            </span>
                                        </td>
                                        <td style={{ padding: '12px', maxWidth: '300px' }}>
                                            <div
                                                style={{
                                                    fontSize: '13px',
                                                    color: '#666',
                                                    overflow: 'hidden',
                                                    textOverflow: 'ellipsis',
                                                    whiteSpace: 'nowrap',
                                                    cursor: (complaint.complaint_text || complaint.last_note) ? 'pointer' : 'default'
                                                }}
                                                onClick={() => {
                                                    if (complaint.complaint_text || complaint.last_note) {
                                                        setViewComplaint(complaint)
                                                    }
                                                }}
                                                title={complaint.complaint_text || complaint.last_note || ''}
                                            >
                                                {complaint.complaint_text || complaint.last_note || '-'}
                                            </div>
                                        </td>
                                        <td style={{ padding: '12px' }}>
                                            <span style={{
                                                padding: '4px 8px',
                                                borderRadius: '4px',
                                                fontSize: '11px',
                                                fontWeight: 600,
                                                background: complaint.status === 'resolved' ? '#d4edda' : '#e2e3e5',
                                                color: complaint.status === 'resolved' ? '#155724' : '#383d41',
                                            }}>
                                                {complaint.status.toUpperCase()}
                                            </span>
                                        </td>
                                        <td style={{ padding: '12px', fontSize: '12px', color: '#666' }}>
                                            {new Date(complaint.created_at).toLocaleString()}
                                        </td>
                                        <td style={{ padding: '12px' }}>
                                            <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                                                {complaint.status !== 'resolved' && (
                                                    <>
                                                        <button
                                                            onClick={() => setActionModal({ type: 'update', complaint })}
                                                            style={{
                                                                padding: '4px 10px',
                                                                borderRadius: '4px',
                                                                border: '1px solid #007bff',
                                                                background: '#fff',
                                                                color: '#007bff',
                                                                fontSize: '11px',
                                                                cursor: 'pointer',
                                                                fontWeight: 600,
                                                            }}
                                                        >
                                                            Update
                                                        </button>
                                                        <button
                                                            onClick={() => setActionModal({ type: 'resolve', complaint })}
                                                            style={{
                                                                padding: '4px 10px',
                                                                borderRadius: '4px',
                                                                border: '1px solid #28a745',
                                                                background: '#fff',
                                                                color: '#28a745',
                                                                fontSize: '11px',
                                                                cursor: 'pointer',
                                                                fontWeight: 600,
                                                            }}
                                                        >
                                                            Resolve
                                                        </button>
                                                    </>
                                                )}
                                                <button
                                                    onClick={() => setViewComplaint(complaint)}
                                                    style={{
                                                        padding: '4px 10px',
                                                        borderRadius: '4px',
                                                        border: '1px solid #6c757d',
                                                        background: '#fff',
                                                        color: '#6c757d',
                                                        fontSize: '11px',
                                                        cursor: 'pointer',
                                                        fontWeight: 600,
                                                    }}
                                                >
                                                    View
                                                </button>
                                            </div>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    )}
                </div>
            </div>

            {/* Action Modal */}
            {actionModal.type === 'update' && actionModal.complaint && (
                <ComplaintActionModal
                    type="update"
                    complaint={actionModal.complaint}
                    onClose={() => setActionModal({ type: null, complaint: null })}
                    onSuccess={() => {
                        setActionModal({ type: null, complaint: null })
                        loadComplaints()
                    }}
                />
            )}
            {actionModal.type === 'resolve' && actionModal.complaint && (
                <ComplaintActionModal
                    type="resolve"
                    complaint={actionModal.complaint}
                    onClose={() => setActionModal({ type: null, complaint: null })}
                    onSuccess={() => {
                        setActionModal({ type: null, complaint: null })
                        loadComplaints()
                    }}
                />
            )}
            {viewComplaint && (
                <div
                    style={{
                        position: 'fixed',
                        inset: 0,
                        background: 'rgba(0,0,0,0.5)',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        zIndex: 3000
                    }}
                    onClick={() => setViewComplaint(null)}
                >
                    <div
                        style={{
                            background: 'white',
                            padding: '24px',
                            borderRadius: '12px',
                            width: '600px',
                            maxWidth: '90%',
                            maxHeight: '80vh',
                            overflowY: 'auto'
                        }}
                        onClick={(e) => e.stopPropagation()}
                    >
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
                            <h3 style={{ margin: 0 }}>Complaint Details</h3>
                            <button
                                onClick={() => setViewComplaint(null)}
                                style={{
                                    background: 'none',
                                    border: 'none',
                                    fontSize: '20px',
                                    cursor: 'pointer',
                                    color: '#666'
                                }}
                            >
                                ×
                            </button>
                        </div>
                        <div style={{ fontSize: '13px', color: '#666', marginBottom: '12px' }}>
                            <div><strong>Student:</strong> {viewComplaint.student_phone}</div>
                            <div><strong>Class:</strong> {viewComplaint.class_key}</div>
                            <div><strong>Category:</strong> {viewComplaint.category?.replace(/_/g, ' ')}</div>
                            <div><strong>Urgency:</strong> {viewComplaint.urgency}</div>
                            <div><strong>Created:</strong> {new Date(viewComplaint.created_at).toLocaleString()}</div>
                        </div>
                        <div style={{ background: '#f8f9fa', padding: '16px', borderRadius: '8px', marginBottom: '20px' }}>
                            <div style={{ fontWeight: 600, marginBottom: '8px', fontSize: '14px' }}>Initial Complaint:</div>
                            <div style={{ whiteSpace: 'pre-wrap', lineHeight: 1.6, color: '#333' }}>
                                {viewComplaint.complaint_text || '-'}
                            </div>
                        </div>

                        {viewComplaint.notes && viewComplaint.notes.length > 0 && (
                            <div style={{ marginTop: '24px' }}>
                                <h4 style={{ fontSize: '15px', fontWeight: 600, marginBottom: '16px', color: '#333', borderBottom: '1px solid #eee', paddingBottom: '8px' }}>
                                    Activity History
                                </h4>
                                <div style={{ display: 'flex', flexDirection: 'column', gap: '20px', position: 'relative', paddingLeft: '20px' }}>
                                    {/* Timeline line */}
                                    <div style={{ position: 'absolute', left: '7px', top: '5px', bottom: '5px', width: '2px', background: '#e9ecef' }} />

                                    {viewComplaint.notes.map((note: any, idx: number) => (
                                        <div key={note.id || idx} style={{ position: 'relative' }}>
                                            {/* dot */}
                                            <div style={{
                                                position: 'absolute',
                                                left: '-17px',
                                                top: '5px',
                                                width: '10px',
                                                height: '10px',
                                                borderRadius: '50%',
                                                background: note.note_type === 'status_change' ? '#007bff' : note.note_type === 'resolution' ? '#28a745' : note.note_type === 'system' ? '#6c757d' : '#adb5bd',
                                                border: '2px solid white',
                                                zIndex: 1
                                            }} />

                                            <div style={{ fontSize: '12px', color: '#666', marginBottom: '4px', display: 'flex', justifyContent: 'space-between' }}>
                                                <span>
                                                    <strong>{note.created_by_email || 'System'}</strong>
                                                    <span style={{ marginLeft: '8px', padding: '2px 6px', borderRadius: '4px', fontSize: '10px', background: '#e9ecef', color: '#666', fontWeight: 600 }}>
                                                        {note.note_type.replace('_', ' ').toUpperCase()}
                                                    </span>
                                                </span>
                                                <span>{new Date(note.created_at).toLocaleString()}</span>
                                            </div>
                                            <div style={{
                                                fontSize: '13px',
                                                color: '#333',
                                                lineHeight: 1.5,
                                                background: note.note_type === 'status_change' ? '#f0f7ff' : note.note_type === 'resolution' ? '#f0fff4' : '#fff',
                                                padding: '8px 12px',
                                                borderRadius: '6px',
                                                border: '1px solid #eee'
                                            }}>
                                                {note.note_text}
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            )}
        </>
    )
}

// Complaint Action Modal
function ComplaintActionModal({ type, complaint, onClose, onSuccess }: {
    type: 'update' | 'resolve'
    complaint: ComplaintListItem
    onClose: () => void
    onSuccess: () => void
}) {
    const [status, setStatus] = useState(complaint.status || 'contacted')
    const [note, setNote] = useState('')
    const [isSubmitting, setIsSubmitting] = useState(false)
    const [error, setError] = useState<string | null>(null)

    async function handleSubmit() {
        setError(null)
        if (!note.trim()) {
            setError('Please enter a note')
            return
        }

        setIsSubmitting(true)
        try {
            if (type === 'resolve') {
                await api.resolveComplaint(complaint.id, note)
            } else {
                await api.updateComplaintStatus(complaint.id, status, note)
            }
            onSuccess()
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to update complaint')
        } finally {
            setIsSubmitting(false)
        }
    }

    return (
        <div
            style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 3000 }}
            onClick={onClose}
        >
            <div
                style={{ background: 'white', padding: '24px', borderRadius: '12px', width: '500px', maxWidth: '90%' }}
                onClick={(e) => e.stopPropagation()}
            >
                <h3 style={{ marginBottom: '20px', color: type === 'resolve' ? '#28a745' : '#007bff' }}>
                    {type === 'resolve' ? 'Resolve Complaint' : 'Update Complaint Status'}
                </h3>

                <div style={{ marginBottom: '16px', padding: '12px', background: '#f8f9fa', borderRadius: '6px' }}>
                    <div style={{ fontSize: '12px', color: '#666', marginBottom: '4px' }}>Student: <strong>{complaint.student_phone}</strong></div>
                    <div style={{ fontSize: '12px', color: '#666', marginBottom: '4px' }}>Category: <strong>{complaint.category}</strong></div>
                    <div style={{ fontSize: '12px', color: '#666' }}>Complaint: {complaint.complaint_text || complaint.last_note || '-'}</div>
                </div>

                {error && (
                    <div style={{ color: '#721c24', background: '#f8d7da', padding: '8px 12px', borderRadius: '6px', marginBottom: '12px', fontSize: '13px' }}>
                        {error}
                    </div>
                )}

                {type === 'update' && (
                    <div style={{ marginBottom: '16px' }}>
                        <label style={{ display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '6px' }}>
                            Status
                        </label>
                        <select
                            value={status}
                            onChange={(e) => setStatus(e.target.value)}
                            style={{ width: '100%', padding: '10px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px' }}
                        >
                            <option value="contacted">Contacted</option>
                            <option value="investigating">Investigating</option>
                            <option value="escalated">Escalated</option>
                        </select>
                    </div>
                )}

                <div style={{ marginBottom: '20px' }}>
                    <label style={{ display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '6px' }}>
                        {type === 'resolve' ? 'Resolution Note' : 'Update Note'} <span style={{ color: '#dc3545' }}>*</span>
                    </label>
                    <textarea
                        value={note}
                        onChange={(e) => {
                            setNote(e.target.value)
                            if (error) setError(null)
                        }}
                        placeholder={type === 'resolve' ? 'Describe how the complaint was resolved...' : 'Add an update or note...'}
                        style={{ width: '100%', height: '120px', padding: '12px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px', resize: 'vertical' }}
                    />
                </div>

                <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
                    <button
                        onClick={onClose}
                        disabled={isSubmitting}
                        style={{ padding: '10px 20px', borderRadius: '6px', border: '1px solid #ddd', background: '#fff', cursor: 'pointer', fontSize: '14px' }}
                    >
                        Cancel
                    </button>
                    <button
                        onClick={handleSubmit}
                        disabled={isSubmitting || !note.trim()}
                        style={{
                            padding: '10px 20px',
                            borderRadius: '6px',
                            border: 'none',
                            background: type === 'resolve' ? '#28a745' : '#007bff',
                            color: '#fff',
                            cursor: 'pointer',
                            fontSize: '14px',
                            fontWeight: 600,
                            opacity: (isSubmitting || !note.trim()) ? 0.6 : 1,
                        }}
                    >
                        {isSubmitting ? (type === 'resolve' ? 'Resolving...' : 'Updating...') : (type === 'resolve' ? 'Resolve Complaint' : 'Update Status')}
                    </button>
                </div>
            </div>
        </div>
    )
}
