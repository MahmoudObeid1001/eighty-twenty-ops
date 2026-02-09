import { CSSProperties, Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { api, MentorClassReportItem, MentorReportChecklistItem, MentorReportItem } from '../api/client'

export default function ReportsPage() {
  const [items, setItems] = useState<MentorReportItem[]>([])
  const [mentorFilter, setMentorFilter] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [classesLoading, setClassesLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [userRole, setUserRole] = useState('')
  const [removingMentorID, setRemovingMentorID] = useState<string | null>(null)
  const [confirmRemove, setConfirmRemove] = useState<MentorReportItem | null>(null)

  const [selectedMentor, setSelectedMentor] = useState<MentorReportItem | null>(null)
  const [selectedClassKey, setSelectedClassKey] = useState<string | null>(null)
  const [checklistRows, setChecklistRows] = useState<MentorReportChecklistItem[]>([])
  const [checklistLoading, setChecklistLoading] = useState(false)
  const [classRows, setClassRows] = useState<MentorClassReportItem[]>([])
  const checklistRequestRef = useRef(0)

  useEffect(() => {
    void loadMe()
    void loadReports()
  }, [])

  useEffect(() => {
    void loadReports()
  }, [mentorFilter])

  async function loadMe() {
    try {
      const me = await api.getMe()
      setUserRole(me.role || '')
    } catch {
      setUserRole('')
    }
  }

  async function loadReports() {
    try {
      setLoading(true)
      setClassesLoading(true)
      setError(null)
      const [res, classesRes] = await Promise.all([
        api.getMentorReports({
          round_status: 'active',
          mentor_id: mentorFilter || undefined,
        }),
        api.getMentorClassReports({
          round_status: 'active',
          mentor_id: mentorFilter || undefined,
        }),
      ])
      setItems(res.items || [])
      setClassRows(classesRes.items || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load mentor reports')
    } finally {
      setLoading(false)
      setClassesLoading(false)
    }
  }

  const mentorOptions = useMemo(
    () =>
      [...new Map(items.map((i) => [i.mentor_id, i])).values()].map((i) => ({
        value: i.mentor_id,
        label: i.mentor_email,
      })),
    [items],
  )
  const canRemove = userRole === 'admin' || userRole === 'mentor_head'
  const classRowsByMentor = useMemo(() => {
    const map = new Map<string, MentorClassReportItem[]>()
    for (const row of classRows) {
      const list = map.get(row.mentor_id) || []
      list.push(row)
      map.set(row.mentor_id, list)
    }
    return map
  }, [classRows])

  async function handleRemove(row: MentorReportItem) {
    setConfirmRemove(row)
  }

  async function confirmRemoveMentor() {
    if (!confirmRemove) return
    try {
      setRemovingMentorID(confirmRemove.mentor_id)
      setError(null)
      await api.excludeMentorReportRow({
        mentor_id: confirmRemove.mentor_id,
        round_status: 'all',
        reason: 'Paid and completed',
      })
      await loadReports()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove mentor row')
    } finally {
      setRemovingMentorID(null)
      setConfirmRemove(null)
    }
  }

  async function openChecklist(row: MentorReportItem) {
    try {
      setSelectedMentor(row)
      setSelectedClassKey(null)
      setChecklistRows([])
      setChecklistLoading(true)
      const requestId = ++checklistRequestRef.current
      const activeRes = await api.getMentorReportChecklist({ mentor_id: row.mentor_id, round_status: 'active' })
      if (requestId === checklistRequestRef.current) {
        setChecklistRows(activeRes.items || [])
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load mentor checklist')
    } finally {
      if (checklistRequestRef.current > 0) {
        setChecklistLoading(false)
      }
    }
  }

  async function openClassChecklist(mentor: MentorReportItem, classKey: string) {
    await openChecklist(mentor)
    setSelectedClassKey(classKey)
  }

  return (
    <>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <h1>Mentor Reports</h1>
      </div>

      <div style={{ display: 'flex', gap: '12px', alignItems: 'center', marginBottom: '20px', flexWrap: 'wrap' }}>
        <span style={{ fontWeight: 700, color: '#333' }}>Unified Report</span>
        <select
          value={mentorFilter}
          onChange={(e) => setMentorFilter(e.target.value)}
          style={{ padding: '10px 12px', borderRadius: '8px', border: '1px solid #ccc', minWidth: '220px' }}
        >
          <option value="">All Mentors</option>
          {mentorOptions.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>

      {error && <div style={{ background: '#f8d7da', color: '#721c24', padding: '12px', borderRadius: '8px', marginBottom: '16px' }}>{error}</div>}

      <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: '12px', overflow: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: '900px' }}>
          <thead>
            <tr style={{ background: '#f8f9fa', textAlign: 'left' }}>
              <th style={thStyle}>Mentor Name</th>
              <th style={thStyle}>Active Classes</th>
              <th style={thStyle}>Compliance %</th>
              <th style={thStyle}>Avg Delay (min)</th>
              <th style={thStyle}>Absences</th>
              <th style={thStyle}>Complaints</th>
              {canRemove && <th style={thStyle}>Action</th>}
            </tr>
          </thead>
          <tbody>
            {!loading && items.length === 0 ? (
              <tr>
                <td style={emptyStyle} colSpan={canRemove ? 7 : 6}>
                  No report rows found.
                </td>
              </tr>
            ) : (
              items.map((row) => (
                <Fragment key={row.mentor_id}>
                  <tr>
                    <td style={tdStyle}>
                      <button
                        onClick={() => void openChecklist(row)}
                        style={{ background: 'none', border: 'none', color: '#0d6efd', cursor: 'pointer', padding: 0, fontSize: 'inherit' }}
                        title="Open mentor compliance details"
                      >
                        {row.mentor_email}
                      </button>
                    </td>
                    <td style={tdStyle}>{row.classes_count}</td>
                    <td style={tdStyle}>
                      <ComplianceBar score={row.compliance_score} />
                    </td>
                    <td style={tdStyle}>{Number(row.avg_delay_minutes || 0).toFixed(1)}</td>
                    <td style={tdStyle}>{row.absence_count}</td>
                    <td style={tdStyle}>{row.complaints_count}</td>
                    {canRemove && (
                      <td style={tdStyle}>
                        <button
                          onClick={() => void handleRemove(row)}
                          disabled={removingMentorID === row.mentor_id}
                          style={{
                            padding: '6px 10px',
                            borderRadius: '6px',
                            border: '1px solid #dc3545',
                            background: '#fff',
                            color: '#dc3545',
                            cursor: removingMentorID === row.mentor_id ? 'not-allowed' : 'pointer',
                            fontWeight: 600,
                          }}
                        >
                          {removingMentorID === row.mentor_id ? 'Removing...' : 'Remove'}
                        </button>
                      </td>
                    )}
                  </tr>
                  <tr>
                    <td style={{ ...tdStyle, background: '#fafbfd' }} colSpan={canRemove ? 7 : 6}>
                      <MentorClassBreakdownTable
                        rows={classRowsByMentor.get(row.mentor_id) || []}
                        loading={classesLoading}
                        onOpenClass={(classRow) => void openClassChecklist(row, classRow.class_key)}
                      />
                    </td>
                  </tr>
                </Fragment>
              ))
            )}
          </tbody>
        </table>
      </div>

      {selectedMentor && (
        <MentorChecklistModal
          mentor={selectedMentor}
          rows={checklistRows}
          loading={checklistLoading}
          classKeyFilter={selectedClassKey}
          onClose={() => {
            setSelectedMentor(null)
            setSelectedClassKey(null)
          }}
        />
      )}
      {confirmRemove && (
        <div onClick={() => setConfirmRemove(null)} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.45)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 5000, padding: '16px' }}>
          <div onClick={(e) => e.stopPropagation()} style={{ background: '#fff', borderRadius: '12px', width: '520px', maxWidth: '100%', boxShadow: '0 12px 30px rgba(0,0,0,0.2)', padding: '20px' }}>
            <h3 style={{ margin: 0, marginBottom: '8px' }}>Remove Mentor From Report?</h3>
            <p style={{ margin: 0, color: '#555', lineHeight: 1.5 }}>
              This will hide <strong>{confirmRemove.mentor_email}</strong> from the report. Data will not be deleted.
            </p>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '20px' }}>
              <button onClick={() => setConfirmRemove(null)} style={{ padding: '8px 14px', borderRadius: '8px', border: '1px solid #ccc', background: '#fff', cursor: 'pointer' }}>
                Cancel
              </button>
              <button onClick={() => void confirmRemoveMentor()} style={{ padding: '8px 14px', borderRadius: '8px', border: 'none', background: '#dc3545', color: 'white', fontWeight: 700, cursor: 'pointer' }}>
                Yes, Remove
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}

function MentorClassBreakdownTable({
  rows,
  loading,
  onOpenClass,
}: {
  rows: MentorClassReportItem[]
  loading: boolean
  onOpenClass: (row: MentorClassReportItem) => void
}) {
  if (loading) return <div style={{ color: '#666' }}>Loading classes...</div>
  if (rows.length === 0) return <div style={{ color: '#666' }}>No active classes for this mentor.</div>

  return (
    <table style={{ width: '100%', borderCollapse: 'collapse' }}>
      <thead>
        <tr style={{ background: '#f1f4f8', textAlign: 'left' }}>
          <th style={subThStyle}>Class</th>
          <th style={subThStyle}>Schedule</th>
          <th style={subThStyle}>Compliance %</th>
          <th style={subThStyle}>Avg Delay</th>
          <th style={subThStyle}>Absences</th>
          <th style={subThStyle}>Complaints</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={`${row.mentor_id}-${row.class_key}`}>
            <td style={subTdStyle}>
              <button
                onClick={() => onOpenClass(row)}
                style={{ background: 'none', border: 'none', color: '#0d6efd', cursor: 'pointer', padding: 0, fontSize: 'inherit' }}
                title="Open this class compliance details"
              >
                Level {row.level} · Class {row.class_number}
              </button>
            </td>
            <td style={subTdStyle}>
              {row.class_days} @ {String(row.class_time || '').slice(0, 5)}
            </td>
            <td style={subTdStyle}>
              <ComplianceBar score={row.compliance_score} />
            </td>
            <td style={subTdStyle}>{Number(row.avg_delay_minutes || 0).toFixed(1)}</td>
            <td style={subTdStyle}>{row.absence_count}</td>
            <td style={subTdStyle}>{row.complaints_count}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function MentorChecklistModal({
  mentor,
  rows,
  loading,
  classKeyFilter,
  onClose,
}: {
  mentor: MentorReportItem
  rows: MentorReportChecklistItem[]
  loading: boolean
  classKeyFilter?: string | null
  onClose: () => void
}) {
  const visibleRows = useMemo(
    () => (classKeyFilter ? rows.filter((r) => r.class_key === classKeyFilter) : rows),
    [rows, classKeyFilter],
  )
  const grouped = useMemo(() => {
    const m = new Map<string, MentorReportChecklistItem[]>()
    for (const r of visibleRows) {
      const list = m.get(r.class_key) || []
      list.push(r)
      m.set(r.class_key, list)
    }
    return Array.from(m.entries()).map(([classKey, classRows]) => ({ classKey, classRows }))
  }, [visibleRows])

  return (
    <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.45)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 5000, padding: '16px' }}>
      <div onClick={(e) => e.stopPropagation()} style={{ background: '#fff', borderRadius: '12px', width: '1240px', maxWidth: '100%', maxHeight: '90vh', overflow: 'auto', boxShadow: '0 12px 30px rgba(0,0,0,0.2)', padding: '20px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
          <h3 style={{ margin: 0 }}>Compliance Checklist Details</h3>
          <button onClick={onClose} style={{ border: 'none', background: 'transparent', fontSize: '24px', cursor: 'pointer', color: '#666' }}>×</button>
        </div>
        <div style={{ marginBottom: '12px', color: '#555' }}>
          <strong>Mentor:</strong> {mentor.mentor_email}
        </div>
        {classKeyFilter && (
          <div style={{ marginBottom: '12px', color: '#555' }}>
            <strong>Class:</strong> {classKeyFilter}
          </div>
        )}
        <div style={{ marginBottom: '14px', display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
          <span style={pillStyle}>{grouped.length} classes</span>
          <span style={pillStyle}>{visibleRows.length} session checks</span>
        </div>
        {loading ? (
          <div style={{ padding: '20px', textAlign: 'center' }}>Loading checklist...</div>
        ) : (
          <>
            {grouped.length === 0 ? (
              <div style={emptyStyle}>No checklist records found for this mentor.</div>
            ) : (
              grouped.map(({ classKey, classRows }) => (
                <div key={classKey} style={{ border: '1px solid #e9ecef', borderRadius: '10px', marginBottom: '14px', overflow: 'hidden' }}>
                  <div style={{ background: '#f8f9fa', padding: '10px 12px', borderBottom: '1px solid #e9ecef', fontWeight: 700 }}>
                    {classKey}
                  </div>
                  <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: '980px' }}>
                    <thead>
                      <tr style={{ background: '#fff', textAlign: 'left' }}>
                        <th style={thStyle}>Session</th>
                        <th style={thStyle}>Schedule</th>
                        <th style={thStyle}>1D</th>
                        <th style={thStyle}>1H</th>
                        <th style={thStyle}>Tasks</th>
                        <th style={thStyle}>Delay</th>
                        <th style={thStyle}>Absent</th>
                        <th style={thStyle}>Checked By</th>
                      </tr>
                    </thead>
                    <tbody>
                      {classRows.map((row, idx) => (
                        <tr key={`${row.class_key}-${row.session_number}-${idx}`}>
                          <td style={tdStyle}>S{row.session_number}</td>
                          <td style={tdStyle}>
                            {row.scheduled_date || '-'} ({getSessionSlotLabel(row.class_days, row.session_number)}) @ {String(row.scheduled_time || '').slice(0, 5)}
                          </td>
                          <td style={tdStyle}>{row.reminder_1d ? '✓' : '—'}</td>
                          <td style={tdStyle}>{row.reminder_1h ? '✓' : '—'}</td>
                          <td style={tdStyle}>{row.reminder_tasks ? '✓' : '—'}</td>
                          <td style={tdStyle}>{row.delay_minutes}</td>
                          <td style={tdStyle}>{row.is_absent ? 'Yes' : 'No'}</td>
                          <td style={tdStyle}>{row.checked_by || '-'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ))
            )}
          </>
        )}
      </div>
    </div>
  )
}

function ComplianceBar({ score }: { score: number }) {
  const safe = Math.max(0, Math.min(100, Number(score || 0)))
  const color = safe > 90 ? '#28a745' : safe < 70 ? '#dc3545' : '#f0ad4e'
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
      <div style={{ width: '150px', height: '10px', background: '#ececec', borderRadius: '999px', overflow: 'hidden' }}>
        <div style={{ width: `${safe}%`, height: '100%', background: color, transition: 'width 0.25s ease' }} />
      </div>
      <span style={{ fontWeight: 700, color }}>{safe.toFixed(1)}%</span>
    </div>
  )
}

function getSessionSlotLabel(classDays: string, sessionNumber: number): string {
  const raw = String(classDays || '')
  const normalized = raw.replace('-', '/')
  const parts = normalized
    .split('/')
    .map((p) => p.trim())
    .filter(Boolean)
  if (parts.length === 0) return ''
  if (parts.length === 1) return parts[0]
  return sessionNumber%2 === 1 ? parts[0] : parts[1]
}

const thStyle: CSSProperties = {
  padding: '12px',
  borderBottom: '1px solid #ddd',
}

const tdStyle: CSSProperties = {
  padding: '12px',
  borderBottom: '1px solid #f1f1f1',
}

const emptyStyle: CSSProperties = {
  textAlign: 'center',
  color: '#666',
  padding: '24px',
}

const subThStyle: CSSProperties = {
  padding: '8px 10px',
  borderBottom: '1px solid #e5e9ef',
  fontSize: '12px',
  textTransform: 'uppercase',
  letterSpacing: '0.4px',
}

const subTdStyle: CSSProperties = {
  padding: '8px 10px',
  borderBottom: '1px solid #edf1f5',
}

const pillStyle: CSSProperties = {
  background: '#eef6ff',
  color: '#0b5ed7',
  border: '1px solid #cfe2ff',
  borderRadius: '999px',
  fontSize: '12px',
  fontWeight: 700,
  padding: '4px 10px',
}
