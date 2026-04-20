import { useEffect, useState } from 'react'
import { api, StudentSearchResult } from '../api/client'
import StudentProfileModal from './StudentProfileModal'

const tableHeaderStyle = {
    padding: '14px 16px',
    fontSize: '12px',
    textTransform: 'uppercase' as const,
    letterSpacing: '0.06em',
    color: '#64748b',
    background: '#f8fafc',
    borderBottom: '1px solid #e2e8f0',
    textAlign: 'left' as const,
}

const tableCellStyle = {
    padding: '16px',
    borderBottom: '1px solid #eef2f7',
    verticalAlign: 'middle' as const,
}

export default function StudentSearch() {
    const [query, setQuery] = useState('')
    const [results, setResults] = useState<StudentSearchResult[]>([])
    const [loading, setLoading] = useState(false)
    const [hasSearched, setHasSearched] = useState(false)
    const [selectedStudentId, setSelectedStudentId] = useState<string | null>(null)
    const normalizedQuery = query.trim()

    useEffect(() => {
        if (normalizedQuery.length < 2) {
            setResults([])
            setLoading(false)
            return
        }

        const timer = setTimeout(async () => {
            setLoading(true)
            setHasSearched(true)
            try {
                const data = await api.searchStudents(normalizedQuery)
                setResults(data || [])
            } catch (err) {
                console.error('Search failed:', err)
                setResults([])
            } finally {
                setLoading(false)
            }
        }, 250)

        return () => clearTimeout(timer)
    }, [normalizedQuery])

    return (
        <>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '18px' }}>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                    <div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', gap: '12px', alignItems: 'flex-start' }}>
                        <div>
                            <h2 style={{ fontSize: '24px', fontWeight: 800, letterSpacing: '-0.02em', color: '#111827', marginBottom: '6px' }}>
                                Find a student quickly
                            </h2>
                            <p style={{ fontSize: '15px', color: '#6b7280', lineHeight: 1.7, maxWidth: '760px' }}>
                                Start with at least 2 characters from the student name or enter the phone number. Results stay visible below so admin can scan and open the right profile without chasing a dropdown.
                            </p>
                        </div>
                        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
                            <InfoBadge label="Search by name" />
                            <InfoBadge label="Search by phone" />
                            <InfoBadge label="Open full profile" />
                        </div>
                    </div>

                    <div style={{ position: 'relative', marginTop: '4px' }}>
                        <input
                            type="text"
                            placeholder="Type student name or phone number"
                            value={query}
                            onChange={(e) => setQuery(e.target.value)}
                            style={{
                                width: '100%',
                                padding: '16px 48px 16px 16px',
                                border: '1px solid #cbd5e1',
                                borderRadius: '14px',
                                fontSize: '16px',
                                background: '#fff',
                                boxShadow: 'inset 0 1px 2px rgba(15, 23, 42, 0.04)',
                            }}
                        />
                        <div style={{ position: 'absolute', right: '16px', top: '50%', transform: 'translateY(-50%)', color: '#64748b', fontSize: '18px' }}>
                            {loading ? '...' : '🔎'}
                        </div>
                    </div>
                </div>

                <div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', gap: '12px', alignItems: 'center' }}>
                    <div style={{ fontSize: '14px', color: '#64748b' }}>
                        {normalizedQuery.length < 2 && 'Enter at least 2 characters to search.'}
                        {normalizedQuery.length >= 2 && loading && `Searching for "${normalizedQuery}"...`}
                        {normalizedQuery.length >= 2 && !loading && `${results.length} student${results.length === 1 ? '' : 's'} found`}
                    </div>
                    {normalizedQuery.length >= 2 && (
                        <button
                            type="button"
                            onClick={() => {
                                setQuery('')
                                setResults([])
                                setHasSearched(false)
                            }}
                            style={{
                                border: '1px solid #d1d5db',
                                background: '#fff',
                                color: '#374151',
                                borderRadius: '10px',
                                padding: '10px 14px',
                                fontSize: '14px',
                                fontWeight: 600,
                                cursor: 'pointer',
                            }}
                        >
                            Clear search
                        </button>
                    )}
                </div>

                <div style={{ border: '1px solid #e5e7eb', borderRadius: '16px', overflow: 'hidden', background: '#fff' }}>
                    {normalizedQuery.length < 2 && (
                        <EmptyState
                            title="Student results will appear here"
                            description="Use this page during admin calls or follow-up work. Once you type a name or phone number, the matching students will appear in a clean list below."
                        />
                    )}

                    {normalizedQuery.length >= 2 && !loading && results.length === 0 && hasSearched && (
                        <EmptyState
                            title="No matching students"
                            description="Try a wider name fragment, a different phone format, or check whether the student record exists under another spelling."
                        />
                    )}

                    {results.length > 0 && (
                        <>
                            <div className="table-container" style={{ marginBottom: 0 }}>
                                <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: '760px' }}>
                                    <thead>
                                        <tr>
                                            <th style={tableHeaderStyle}>Student</th>
                                            <th style={tableHeaderStyle}>Phone</th>
                                            <th style={tableHeaderStyle}>Assigned level</th>
                                            <th style={tableHeaderStyle}>Status</th>
                                            <th style={tableHeaderStyle}>Action</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {results.map((result) => (
                                            <tr key={result.lead_id} style={{ background: '#fff' }}>
                                                <td style={tableCellStyle}>
                                                    <div style={{ fontWeight: 700, color: '#111827', marginBottom: '4px' }}>{result.full_name}</div>
                                                    <div style={{ fontSize: '13px', color: '#6b7280' }}>Student profile and learning history</div>
                                                </td>
                                                <td style={tableCellStyle}>
                                                    <span style={{ fontWeight: 600, color: '#1f2937' }}>{result.phone || '-'}</span>
                                                </td>
                                                <td style={tableCellStyle}>
                                                    <span style={{ color: '#1f2937', fontWeight: 600 }}>
                                                        {result.current_level > 0 ? `Level ${result.current_level}` : 'Not assigned yet'}
                                                    </span>
                                                </td>
                                                <td style={tableCellStyle}>
                                                    <StatusBadge status={result.status} />
                                                </td>
                                                <td style={tableCellStyle}>
                                                    <button
                                                        type="button"
                                                        onClick={() => setSelectedStudentId(result.lead_id)}
                                                        style={{
                                                            border: 'none',
                                                            background: '#4ec6e0',
                                                            color: '#fff',
                                                            borderRadius: '10px',
                                                            padding: '10px 16px',
                                                            fontSize: '14px',
                                                            fontWeight: 700,
                                                            cursor: 'pointer',
                                                        }}
                                                    >
                                                        Open profile
                                                    </button>
                                                </td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>

                            <div className="ss-placement-cards" style={{ padding: '16px' }}>
                                {results.map((result) => (
                                    <div key={result.lead_id} className="ss-placement-card">
                                        <div className="ss-placement-card-head">
                                            <div>
                                                <div className="ss-placement-card-name">{result.full_name}</div>
                                                <div className="ss-placement-card-phone">{result.phone || '-'}</div>
                                            </div>
                                            <StatusBadge status={result.status} />
                                        </div>
                                        <div className="ss-placement-card-meta">
                                            <div className="ss-placement-card-field">
                                                <span className="ss-placement-card-label">Assigned level</span>
                                                <span className="ss-placement-card-value">
                                                    {result.current_level > 0 ? `Level ${result.current_level}` : 'Not assigned yet'}
                                                </span>
                                            </div>
                                        </div>
                                        <div className="ss-placement-card-actions">
                                            <button
                                                type="button"
                                                className="ss-placement-card-primary"
                                                onClick={() => setSelectedStudentId(result.lead_id)}
                                            >
                                                Open profile
                                            </button>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </>
                    )}
                </div>
            </div>

            {selectedStudentId && (
                <StudentProfileModal
                    studentId={selectedStudentId}
                    onStudentUpdated={(updatedStudent) => {
                        setResults((current) => current.map((item) => (
                            item.lead_id === updatedStudent.lead_id
                                ? { ...item, full_name: updatedStudent.full_name, phone: updatedStudent.phone }
                                : item
                        )))
                    }}
                    onClose={() => setSelectedStudentId(null)}
                />
            )}
        </>
    )
}

function InfoBadge({ label }: { label: string }) {
    return (
        <span
            style={{
                display: 'inline-flex',
                alignItems: 'center',
                padding: '8px 12px',
                borderRadius: '999px',
                background: '#f8fafc',
                border: '1px solid #e2e8f0',
                color: '#475569',
                fontSize: '13px',
                fontWeight: 700,
            }}
        >
            {label}
        </span>
    )
}

function EmptyState({ title, description }: { title: string; description: string }) {
    return (
        <div style={{ padding: '36px 24px', textAlign: 'center' }}>
            <div style={{ fontSize: '34px', marginBottom: '10px' }}>👩‍🎓</div>
            <h3 style={{ fontSize: '20px', fontWeight: 800, color: '#1f2937', marginBottom: '8px' }}>{title}</h3>
            <p style={{ maxWidth: '680px', margin: '0 auto', color: '#6b7280', lineHeight: 1.7 }}>{description}</p>
        </div>
    )
}

function StatusBadge({ status }: { status: string }) {
    const { label, background, color } = getStatusPresentation(status)

    return (
        <span
            style={{
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                padding: '8px 12px',
                borderRadius: '999px',
                background,
                color,
                fontSize: '12px',
                fontWeight: 800,
                textTransform: 'uppercase',
                letterSpacing: '0.04em',
                whiteSpace: 'nowrap',
            }}
        >
            {label}
        </span>
    )
}

function getStatusPresentation(status: string) {
    const normalized = (status || '').trim().toLowerCase()

    switch (normalized) {
        case 'in_classes':
            return { label: 'In Classes', background: '#dcfce7', color: '#166534' }
        case 'ready_to_start':
            return { label: 'Main Feed', background: '#dbeafe', color: '#1d4ed8' }
        case 'waiting_for_round':
            return { label: 'Waiting List', background: '#fef3c7', color: '#92400e' }
        case 'offer_sent':
            return { label: 'Offer Sent', background: '#ede9fe', color: '#6d28d9' }
        case 'paid_full':
            return { label: 'Paid Full', background: '#cffafe', color: '#155e75' }
        case 'deposit_paid':
            return { label: 'Deposit Paid', background: '#e0f2fe', color: '#075985' }
        case 'tested':
            return { label: 'Tested', background: '#fee2e2', color: '#b91c1c' }
        case 'sleeping':
            return { label: 'Sleeping', background: '#f3f4f6', color: '#4b5563' }
        default:
            return {
                label: status ? status.replace(/_/g, ' ') : 'Unknown',
                background: '#f3f4f6',
                color: '#374151',
            }
    }
}
