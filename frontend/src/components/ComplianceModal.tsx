import { CSSProperties, useEffect, useMemo, useState } from 'react'
import { api, ComplianceClassSession } from '../api/client'

type EditableRow = {
  session_number: number
  class_session_id?: string
  status?: string
  scheduled_date?: string
  scheduled_time?: string
  reminder_1d: boolean
  reminder_1h: boolean
  reminder_tasks: boolean
  delay_minutes: number
  is_absent: boolean
}

interface ComplianceModalProps {
  open: boolean
  classKey: string
  onClose: () => void
}

export default function ComplianceModal({ open, classKey, onClose }: ComplianceModalProps) {
  const [loading, setLoading] = useState(false)
  const [savingSession, setSavingSession] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [rows, setRows] = useState<EditableRow[]>([])
  const classDays = extractClassDaysFromClassKey(classKey)

  useEffect(() => {
    if (!open || !classKey) return
    void loadData()
  }, [open, classKey])

  const hasSchedulableRows = useMemo(() => rows.some((r) => !!r.class_session_id), [rows])
  const savedChecksCount = useMemo(
    () =>
      rows.filter(
        (r) =>
          !!r.class_session_id &&
          (r.reminder_1d || r.reminder_1h || r.reminder_tasks || r.delay_minutes > 0 || r.is_absent),
      ).length,
    [rows],
  )

  async function loadData() {
    try {
      setLoading(true)
      setError(null)
      const res = await api.getComplianceByClass(classKey)
      setRows(
        (res.sessions || []).map(mapSessionToRow),
      )
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load compliance data')
    } finally {
      setLoading(false)
    }
  }

  function mapSessionToRow(s: ComplianceClassSession): EditableRow {
    return {
      session_number: s.session_number,
      class_session_id: s.class_session_id,
      status: s.status,
      scheduled_date: s.scheduled_date,
      scheduled_time: s.scheduled_time,
      reminder_1d: s.check?.reminder_1d || false,
      reminder_1h: s.check?.reminder_1h || false,
      reminder_tasks: s.check?.reminder_tasks || false,
      delay_minutes: s.check?.delay_minutes || 0,
      is_absent: s.check?.is_absent || false,
    }
  }

  function updateRow(sessionNumber: number, patch: Partial<EditableRow>) {
    setRows((prev) => prev.map((row) => (row.session_number === sessionNumber ? { ...row, ...patch } : row)))
  }

  async function saveRow(row: EditableRow) {
    if (!row.class_session_id) return
    try {
      setSavingSession(row.session_number)
      setError(null)
      await api.upsertComplianceCheck({
        class_session_id: row.class_session_id,
        reminder_1d: row.reminder_1d,
        reminder_1h: row.reminder_1h,
        reminder_tasks: row.reminder_tasks,
        delay_minutes: Math.max(0, row.delay_minutes || 0),
        is_absent: row.is_absent,
      })
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : `Failed to save Session ${row.session_number}`)
    } finally {
      setSavingSession(null)
    }
  }

  if (!open) return null

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(0,0,0,0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 5000,
        padding: '16px',
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: '#fff',
          borderRadius: '12px',
          width: '1100px',
          maxWidth: '100%',
          maxHeight: '90vh',
          overflow: 'auto',
          boxShadow: '0 12px 24px rgba(0,0,0,0.2)',
        }}
      >
        <div style={{ padding: '16px 20px', borderBottom: '1px solid #eee', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div>
            <h3 style={{ margin: 0, marginBottom: '4px' }}>Mentor Compliance</h3>
            <div style={{ color: '#666', fontSize: '13px', display: 'flex', alignItems: 'center', gap: '10px' }}>
              <span>Class: {classKey}</span>
              <span
                style={{
                  background: '#eef6ff',
                  color: '#0056b3',
                  border: '1px solid #cfe3ff',
                  borderRadius: '999px',
                  padding: '2px 10px',
                  fontSize: '12px',
                  fontWeight: 700,
                }}
              >
                Completeness: {savedChecksCount}/8
              </span>
            </div>
          </div>
          <button onClick={onClose} style={{ border: 'none', background: 'transparent', fontSize: '24px', cursor: 'pointer', color: '#666' }}>×</button>
        </div>

        {error && <div style={{ margin: '12px 20px', background: '#f8d7da', color: '#721c24', padding: '10px 12px', borderRadius: '8px' }}>{error}</div>}
        {!hasSchedulableRows && !loading && (
          <div style={{ margin: '12px 20px', background: '#fff3cd', color: '#856404', padding: '10px 12px', borderRadius: '8px' }}>
            No class sessions found yet. Start the round first to enable compliance checks.
          </div>
        )}

        <div style={{ padding: '12px 20px 20px 20px' }}>
          {loading ? (
            <div style={{ padding: '24px', textAlign: 'center' }}>Loading compliance grid...</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '13px' }}>
              <thead>
                <tr style={{ background: '#f8f9fa', textAlign: 'left' }}>
                  <th style={thStyle}>Session</th>
                  <th style={thStyle}>Schedule</th>
                  <th style={thStyle}>1 Day Reminder</th>
                  <th style={thStyle}>1 Hour Reminder</th>
                  <th style={thStyle}>Tasks Reminder</th>
                  <th style={thStyle}>Delay (min)</th>
                  <th style={thStyle}>Absent</th>
                  <th style={thStyle}>Action</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => {
                  const disabled = !row.class_session_id || savingSession === row.session_number
                  return (
                    <tr key={row.session_number}>
                      <td style={tdStyle}>S{row.session_number}</td>
                      <td style={tdStyle}>
                        {row.scheduled_date ? `${row.scheduled_date} (${getSessionSlotLabel(classDays, row.session_number)})` : '-'}{' '}
                        {row.scheduled_time ? `@ ${row.scheduled_time.slice(0, 5)}` : ''}
                        {row.status ? <div style={{ fontSize: '11px', color: '#777', textTransform: 'uppercase' }}>{row.status}</div> : null}
                      </td>
                      <td style={tdStyle}>
                        <input type="checkbox" checked={row.reminder_1d} disabled={!row.class_session_id} onChange={(e) => updateRow(row.session_number, { reminder_1d: e.target.checked })} />
                      </td>
                      <td style={tdStyle}>
                        <input type="checkbox" checked={row.reminder_1h} disabled={!row.class_session_id} onChange={(e) => updateRow(row.session_number, { reminder_1h: e.target.checked })} />
                      </td>
                      <td style={tdStyle}>
                        <input type="checkbox" checked={row.reminder_tasks} disabled={!row.class_session_id} onChange={(e) => updateRow(row.session_number, { reminder_tasks: e.target.checked })} />
                      </td>
                      <td style={tdStyle}>
                        <input
                          type="number"
                          min={0}
                          value={row.delay_minutes}
                          disabled={!row.class_session_id}
                          onChange={(e) => updateRow(row.session_number, { delay_minutes: Math.max(0, Number(e.target.value || 0)) })}
                          style={{ width: '84px', padding: '6px 8px', borderRadius: '6px', border: '1px solid #ccc' }}
                        />
                      </td>
                      <td style={tdStyle}>
                        <input type="checkbox" checked={row.is_absent} disabled={!row.class_session_id} onChange={(e) => updateRow(row.session_number, { is_absent: e.target.checked })} />
                      </td>
                      <td style={tdStyle}>
                        <button
                          disabled={disabled}
                          onClick={() => void saveRow(row)}
                          style={{
                            padding: '6px 12px',
                            borderRadius: '6px',
                            border: 'none',
                            background: disabled ? '#ccc' : '#007bff',
                            color: 'white',
                            cursor: disabled ? 'not-allowed' : 'pointer',
                            fontWeight: 600,
                          }}
                        >
                          {savingSession === row.session_number ? 'Saving...' : 'Save'}
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

const thStyle: CSSProperties = {
  borderBottom: '1px solid #dee2e6',
  padding: '10px',
  fontWeight: 700,
}

const tdStyle: CSSProperties = {
  borderBottom: '1px solid #f0f0f0',
  padding: '10px',
  verticalAlign: 'middle',
}

function extractClassDaysFromClassKey(classKey: string): string {
  const parts = String(classKey || '').split('|')
  if (parts.length >= 2) return parts[1]
  return ''
}

function getSessionSlotLabel(classDays: string, sessionNumber: number): string {
  const normalized = String(classDays || '').replace('-', '/')
  const parts = normalized
    .split('/')
    .map((p) => p.trim())
    .filter(Boolean)
  if (parts.length === 0) return ''
  if (parts.length === 1) return parts[0]
  return sessionNumber % 2 === 1 ? parts[0] : parts[1]
}
