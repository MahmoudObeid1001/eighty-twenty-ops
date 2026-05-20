import { useEffect, useMemo, useState, useRef } from 'react'
import { api, MentorHeadCalendarResponse } from '../api/client'

// ─── Constants ───────────────────────────────────────────────────────────────

const SLOT_A = { key: 'slotA' as const, label: 'Slot A', start: '19:30', end: '21:30', color: '#2b8a3e', bg: '#ebfbee', border: '#c3fae8' }
const SLOT_B = { key: 'slotB' as const, label: 'Slot B', start: '22:00', end: '00:00', color: '#4c6ef5', bg: '#edf2ff', border: '#d0ebff' }

const WEEK_DAYS = [
  { label: 'Sat', jsDay: 6 },
  { label: 'Sun', jsDay: 0 },
  { label: 'Mon', jsDay: 1 },
  { label: 'Tue', jsDay: 2 },
  { label: 'Wed', jsDay: 3 },
  { label: 'Thu', jsDay: 4 },
  { label: 'Fri', jsDay: 5 },
]

function currentMonth() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

function normalizeMonthString(str: string): string {
  const trimmed = str.trim()
  if (!trimmed) return currentMonth()
  const parts = trimmed.split('-')
  if (parts.length === 2) {
    const y = parts[0]
    let m = parts[1]
    if (m.length === 1) {
      m = '0' + m
    }
    return `${y}-${m}`
  }
  return str
}

function nextMonth(month: string) {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(Date.UTC(y, m, 1))
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`
}

function prevMonth(month: string) {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(Date.UTC(y, m - 1, 1))
  d.setUTCMonth(d.getUTCMonth() - 1)
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`
}

function getDaysInMonth(month: string): Date[] {
  const [y, m] = month.split('-').map(Number)
  const days: Date[] = []
  const d = new Date(Date.UTC(y, m - 1, 1))
  while (d.getUTCMonth() === m - 1) {
    days.push(new Date(d))
    d.setUTCDate(d.getUTCDate() + 1)
  }
  return days
}

function dateKey(d: Date) {
  return d.toISOString().slice(0, 10)
}

function colIndex(jsDay: number): number {
  return jsDay === 6 ? 0 : jsDay + 1
}

function isFriday(d: Date) {
  return d.getUTCDay() === 5
}

function matchSlot(startTime: string): 'slotA' | 'slotB' | null {
  const t = startTime.slice(0, 5)
  if (t === SLOT_A.start) return 'slotA'
  if (t === SLOT_B.start) return 'slotB'
  const h = parseInt(t.split(':')[0], 10)
  if (!isNaN(h)) return h < 21 ? 'slotA' : 'slotB'
  return null
}

interface MentorSlotInfo {
  mentorId: string
  name: string
  slotKey: 'slotA' | 'slotB'
  startTime: string
  endTime: string
  note?: string
}

export default function MentorHeadCalendarPage() {
  const [month, setMonth] = useState(currentMonth)
  const [data, setData] = useState<MentorHeadCalendarResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedSlot, setSelectedSlot] = useState<MentorSlotInfo | null>(null)
  const [selectedDaySlots, setSelectedDaySlots] = useState<{ dateStr: string; slots: MentorSlotInfo[] } | null>(null)

  const popoverRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handler(e: MouseEvent) {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        setSelectedSlot(null)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  useEffect(() => {
    void loadCalendar()
  }, [month])

  async function loadCalendar() {
    const norm = normalizeMonthString(month)
    if (norm !== month) {
      setMonth(norm)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const res = await api.getMentorHeadCalendar(month)
      setData(res)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load availability calendar')
    } finally {
      setLoading(false)
    }
  }

  const days = useMemo(() => getDaysInMonth(month), [month])
  const firstColIndex = useMemo(() => colIndex(days[0].getUTCDay()), [days])

  const calendarCells = useMemo(() => {
    const cells: Array<{ date: Date | null; key: string | null }> = []
    for (let i = 0; i < firstColIndex; i++) cells.push({ date: null, key: null })
    for (const d of days) cells.push({ date: d, key: dateKey(d) })
    while (cells.length % 7 !== 0) cells.push({ date: null, key: null })
    return cells
  }, [days, firstColIndex])

  const calendarRows = useMemo(() => {
    const rows: Array<Array<{ date: Date | null; key: string | null }>> = []
    for (let i = 0; i < calendarCells.length; i += 7) {
      rows.push(calendarCells.slice(i, i + 7))
    }
    return rows
  }, [calendarCells])

  // Group availability by date for easy rendering
  const availabilityMap = useMemo(() => {
    const map: Record<string, MentorSlotInfo[]> = {}
    if (!data?.mentors) return map

    for (const mentor of data.mentors) {
      for (const win of mentor.windows) {
        const k = win.available_date.slice(0, 10)
        const slotKey = matchSlot(win.start_time)
        if (!slotKey) continue

        if (!map[k]) map[k] = []
        map[k].push({
          mentorId: mentor.mentor_user_id,
          name: mentor.name,
          slotKey,
          startTime: win.start_time,
          endTime: win.end_time,
          note: win.note,
        })
      }
    }
    return map
  }, [data])

  // Stats
  const stats = useMemo(() => {
    if (!data?.mentors) return { totalMentors: 0, activeDays: 0, totalSlots: 0 }
    let totalSlots = 0
    const activeDatesSet = new Set<string>()
    const activeMentorsSet = new Set<string>()

    for (const mentor of data.mentors) {
      if (mentor.windows && mentor.windows.length > 0) {
        activeMentorsSet.add(mentor.mentor_user_id)
        totalSlots += mentor.windows.length
        for (const w of mentor.windows) {
          activeDatesSet.add(w.available_date.slice(0, 10))
        }
      }
    }
    return {
      totalMentors: activeMentorsSet.size,
      activeDays: activeDatesSet.size,
      totalSlots,
    }
  }, [data])

  return (
    <div style={{ padding: '24px', maxWidth: '1200px', margin: '0 auto', fontFamily: 'Inter, sans-serif' }}>
      <style>{`
        @keyframes spin {
          0% { transform: rotate(0deg); }
          100% { transform: rotate(360deg); }
        }
      `}</style>

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '16px', marginBottom: '24px' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: '28px', fontWeight: 800, color: '#1a1a2e', letterSpacing: '-0.02em' }}>Mentor Availability Calendar</h1>
          <p style={{ margin: '6px 0 0', color: '#666', fontSize: '14px' }}>
            Overview of all mentors' submitted availability slots for the selected month.
          </p>
        </div>

        {/* Controls */}
        <div style={{ display: 'flex', gap: '12px', alignItems: 'center' }}>
          <div style={{ display: 'flex', border: '1px solid #e2e8f0', borderRadius: '8px', overflow: 'hidden', background: '#fff' }}>
            <button
              onClick={() => setMonth(prevMonth(month))}
              style={{ border: 'none', background: 'none', padding: '8px 12px', cursor: 'pointer', borderRight: '1px solid #e2e8f0' }}
              title="Previous Month"
            >
              ◀
            </button>
            <input
              type="month"
              value={month}
              onChange={(e) => setMonth(e.target.value || currentMonth())}
              onBlur={(e) => setMonth(normalizeMonthString(e.target.value))}
              style={{ border: 'none', padding: '8px 12px', fontSize: '14px', fontWeight: 600, outline: 'none', color: '#1e293b' }}
            />
            <button
              onClick={() => setMonth(nextMonth(month))}
              style={{ border: 'none', background: 'none', padding: '8px 12px', cursor: 'pointer', borderLeft: '1px solid #e2e8f0' }}
              title="Next Month"
            >
              ▶
            </button>
          </div>
        </div>
      </div>

      {/* Stats Summary Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '16px', marginBottom: '24px' }}>
        <div style={statCard}>
          <div style={statLabel}>Available Mentors</div>
          <div style={statValue}>{stats.totalMentors} <span style={{ fontSize: '14px', color: '#64748b', fontWeight: 400 }}>active</span></div>
        </div>
        <div style={statCard}>
          <div style={statLabel}>Total Slots Offered</div>
          <div style={statValue}>{stats.totalSlots} <span style={{ fontSize: '14px', color: '#64748b', fontWeight: 400 }}>slots</span></div>
        </div>
        <div style={statCard}>
          <div style={statLabel}>Days Covered</div>
          <div style={statValue}>{stats.activeDays} <span style={{ fontSize: '14px', color: '#64748b', fontWeight: 400 }}>days</span></div>
        </div>
      </div>

      {/* Legend */}
      <div style={{ display: 'flex', gap: '20px', flexWrap: 'wrap', marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: '#f8fafc', border: '1px solid #f1f5f9', fontSize: '13px' }}>
        <span style={{ color: '#64748b', fontWeight: 600 }}>Legend:</span>
        <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span style={{ width: '12px', height: '12px', borderRadius: '3px', background: SLOT_A.color }} />
          <span style={{ color: '#334155' }}>Slot A ({SLOT_A.start} - {SLOT_A.end})</span>
        </span>
        <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span style={{ width: '12px', height: '12px', borderRadius: '3px', background: SLOT_B.color }} />
          <span style={{ color: '#334155' }}>Slot B ({SLOT_B.start} - {SLOT_B.end})</span>
        </span>
      </div>

      {error && <div style={errorBanner}>{error}</div>}

      {/* Calendar Grid */}
      {loading ? (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: '300px', gap: '12px' }}>
          <div className="spinner" style={spinnerStyle} />
          <span style={{ color: '#64748b', fontSize: '15px', fontWeight: 500 }}>Loading availability calendar…</span>
        </div>
      ) : (
        <div style={panelCard}>
          {/* Day Headers */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: '8px', marginBottom: '12px' }}>
            {WEEK_DAYS.map((wd) => (
              <div
                key={wd.label}
                style={{
                  textAlign: 'center',
                  fontWeight: 700,
                  fontSize: '13px',
                  letterSpacing: '0.05em',
                  textTransform: 'uppercase',
                  color: wd.jsDay === 5 ? '#cbd5e1' : '#475569',
                  padding: '8px 0',
                  borderBottom: '2px solid',
                  borderColor: wd.jsDay === 5 ? '#f1f5f9' : '#e2e8f0',
                }}
              >
                {wd.label}
              </div>
            ))}
          </div>

          {/* Calendar Rows */}
          {calendarRows.map((row, ri) => (
            <div key={ri} style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: '8px', marginBottom: '8px' }}>
              {row.map((cell, ci) => {
                if (!cell.date || !cell.key) {
                  return <div key={ci} style={{ minHeight: '130px', background: '#f8fafc', borderRadius: '8px', opacity: 0.4 }} />
                }
                const isFri = isFriday(cell.date)
                const dayNum = cell.date.getUTCDate()
                const slots = availabilityMap[cell.key] || []
                const visibleSlots = slots.slice(0, 3)
                const overflowCount = slots.length - 3

                return (
                  <div
                    key={cell.key}
                    style={{
                      minHeight: '130px',
                      borderRadius: '10px',
                      border: '1px solid',
                      borderColor: '#e2e8f0',
                      background: isFri ? '#f8fafc' : '#fff',
                      padding: '8px',
                      position: 'relative',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '6px',
                      boxShadow: '0 1px 2px 0 rgba(0, 0, 0, 0.05)',
                      overflow: 'hidden',
                    }}
                  >
                    {/* Day Number */}
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span style={{ fontSize: '13px', fontWeight: 700, color: isFri ? '#94a3b8' : '#1e293b' }}>{dayNum}</span>
                      {isFri && <span style={{ fontSize: '11px', color: '#94a3b8', fontWeight: 600 }}>OFF</span>}
                      {!isFri && slots.length > 0 && (
                        <button
                          onClick={() => setSelectedDaySlots({ dateStr: cell.key!, slots })}
                          style={{
                            border: 'none',
                            fontSize: '11px',
                            color: '#0b7285',
                            fontWeight: 700,
                            background: '#e0f2fe',
                            padding: '1px 5px',
                            borderRadius: '4px',
                            cursor: 'pointer',
                            display: 'inline-flex',
                            alignItems: 'center',
                            transition: 'background 0.1s ease, transform 0.1s ease',
                          }}
                          onMouseEnter={(e) => {
                            e.currentTarget.style.background = '#cffafe';
                            e.currentTarget.style.transform = 'scale(1.05)';
                          }}
                          onMouseLeave={(e) => {
                            e.currentTarget.style.background = '#e0f2fe';
                            e.currentTarget.style.transform = 'scale(1)';
                          }}
                          title="Click to view all available mentors"
                        >
                          {slots.length} av.
                        </button>
                      )}
                    </div>

                    {/* Mentor List inside the cell */}
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', flex: 1, paddingRight: '2px' }}>
                      {!isFri && visibleSlots.map((slot, idx) => {
                        const sConf = slot.slotKey === 'slotA' ? SLOT_A : SLOT_B
                        return (
                          <button
                            key={idx}
                            onClick={() => setSelectedSlot(slot)}
                            style={{
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'space-between',
                              width: '100%',
                              padding: '4px 6px',
                              borderRadius: '6px',
                              background: sConf.bg,
                              border: `1px solid ${sConf.border}`,
                              color: '#1e293b',
                              fontSize: '11px',
                              fontWeight: 600,
                              textAlign: 'left',
                              cursor: 'pointer',
                              transition: 'transform 0.1s ease',
                            }}
                            title={`Click to view hours and note`}
                            onMouseEnter={(e) => { e.currentTarget.style.transform = 'scale(1.02)' }}
                            onMouseLeave={(e) => { e.currentTarget.style.transform = 'scale(1)' }}
                          >
                            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginRight: '4px' }}>
                              {slot.name}
                            </span>
                            <span style={{
                              width: '8px',
                              height: '8px',
                              borderRadius: '50%',
                              background: sConf.color,
                              flexShrink: 0,
                            }} />
                          </button>
                        )
                      })}
                      {!isFri && overflowCount > 0 && (
                        <button
                          onClick={() => setSelectedDaySlots({ dateStr: cell.key!, slots })}
                          style={{
                            background: 'none',
                            border: 'none',
                            color: '#0b7285',
                            fontSize: '11px',
                            fontWeight: 700,
                            cursor: 'pointer',
                            padding: '2px 6px',
                            textAlign: 'left',
                            borderRadius: '4px',
                            transition: 'background 0.1s ease, color 0.1s ease',
                          }}
                          onMouseEnter={(e) => {
                            e.currentTarget.style.background = '#e0f2fe';
                            e.currentTarget.style.color = '#0c8599';
                          }}
                          onMouseLeave={(e) => {
                            e.currentTarget.style.background = 'none';
                            e.currentTarget.style.color = '#0b7285';
                          }}
                        >
                          + {overflowCount} more
                        </button>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          ))}
        </div>
      )}

      {/* Detail Dialog/Popover when a slot is clicked */}
      {selectedSlot && (
        <div style={overlayStyle} onClick={() => setSelectedSlot(null)}>
          <div ref={popoverRef} style={popoverStyle} onClick={(e) => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '12px' }}>
              <h3 style={{ margin: 0, fontSize: '16px', fontWeight: 700, color: '#1e293b' }}>Mentor Slot Details</h3>
              <button
                onClick={() => setSelectedSlot(null)}
                style={{ border: 'none', background: 'none', fontSize: '18px', cursor: 'pointer', color: '#94a3b8', padding: 0 }}
              >
                ×
              </button>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '13px' }}>
              <div>
                <span style={{ color: '#64748b', fontWeight: 500 }}>Mentor:</span>{' '}
                <strong style={{ color: '#0f172a' }}>{selectedSlot.name}</strong>
              </div>
              <div>
                <span style={{ color: '#64748b', fontWeight: 500 }}>Slot:</span>{' '}
                <span style={{
                  padding: '2px 6px',
                  borderRadius: '4px',
                  fontWeight: 700,
                  fontSize: '11px',
                  background: selectedSlot.slotKey === 'slotA' ? SLOT_A.bg : SLOT_B.bg,
                  color: selectedSlot.slotKey === 'slotA' ? SLOT_A.color : SLOT_B.color,
                }}>
                  {selectedSlot.slotKey === 'slotA' ? 'Slot A' : 'Slot B'} ({selectedSlot.startTime} - {selectedSlot.endTime})
                </span>
              </div>
              {selectedSlot.note && (
                <div style={{ marginTop: '4px', padding: '8px', background: '#f8fafc', borderRadius: '6px', border: '1px solid #e2e8f0' }}>
                  <div style={{ fontSize: '11px', color: '#64748b', fontWeight: 600, marginBottom: '2px' }}>Note:</div>
                  <div style={{ color: '#334155', fontStyle: 'italic' }}>{selectedSlot.note}</div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Group Detail Dialog/Popover when "X av." or "+ X more" is clicked */}
      {selectedDaySlots && (
        <div style={overlayStyle} onClick={() => setSelectedDaySlots(null)}>
          <div style={{ ...popoverStyle, width: '480px' }} onClick={(e) => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '16px' }}>
              <div>
                <h3 style={{ margin: 0, fontSize: '18px', fontWeight: 700, color: '#1e293b' }}>
                  Available Mentors
                </h3>
                <p style={{ margin: '4px 0 0', fontSize: '13px', color: '#64748b', fontWeight: 500 }}>
                  {(() => {
                    try {
                      return new Date(selectedDaySlots.dateStr).toLocaleDateString(undefined, {
                        weekday: 'long',
                        year: 'numeric',
                        month: 'long',
                        day: 'numeric',
                        timeZone: 'UTC',
                      })
                    } catch (e) {
                      return selectedDaySlots.dateStr
                    }
                  })()}
                </p>
              </div>
              <button
                onClick={() => setSelectedDaySlots(null)}
                style={{ border: 'none', background: 'none', fontSize: '24px', cursor: 'pointer', color: '#94a3b8', padding: 0 }}
              >
                ×
              </button>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '20px', maxHeight: '400px', overflowY: 'auto', paddingRight: '4px' }}>
              {/* Grouping by Slot */}
              {['slotA', 'slotB'].map((slotKey) => {
                const sConf = slotKey === 'slotA' ? SLOT_A : SLOT_B
                const slotSlots = selectedDaySlots.slots.filter(s => s.slotKey === slotKey)

                if (slotSlots.length === 0) return null

                return (
                  <div key={slotKey} style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', borderBottom: '1px solid #f1f5f9', paddingBottom: '6px' }}>
                      <span style={{
                        width: '10px',
                        height: '10px',
                        borderRadius: '50%',
                        background: sConf.color,
                      }} />
                      <span style={{ fontSize: '14px', fontWeight: 700, color: '#334155' }}>
                        {sConf.label} ({sConf.start} - {sConf.end})
                      </span>
                      <span style={{
                        fontSize: '11px',
                        color: sConf.color,
                        background: sConf.bg,
                        padding: '1px 6px',
                        borderRadius: '10px',
                        fontWeight: 700,
                      }}>
                        {slotSlots.length} available
                      </span>
                    </div>

                    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                      {slotSlots.map((slot, idx) => (
                        <div
                          key={idx}
                          style={{
                            padding: '10px 12px',
                            borderRadius: '8px',
                            background: '#f8fafc',
                            border: '1px solid #e2e8f0',
                            display: 'flex',
                            flexDirection: 'column',
                            gap: '4px',
                          }}
                        >
                          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                            <span style={{ fontSize: '13px', fontWeight: 700, color: '#0f172a' }}>
                              {slot.name}
                            </span>
                            <span style={{ fontSize: '11px', color: '#64748b', fontWeight: 500 }}>
                              {slot.startTime} - {slot.endTime}
                            </span>
                          </div>
                          {slot.note && (
                            <div style={{ fontSize: '12px', color: '#475569', fontStyle: 'italic', background: '#fff', padding: '6px 8px', borderRadius: '4px', border: '1px solid #f1f5f9', marginTop: '2px' }}>
                              {slot.note}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}

    </div>
  )
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const statCard: React.CSSProperties = {
  background: '#fff',
  border: '1px solid #e2e8f0',
  borderRadius: '12px',
  padding: '16px 20px',
  boxShadow: '0 1px 3px rgba(0,0,0,0.02)',
}

const statLabel: React.CSSProperties = {
  fontSize: '13px',
  fontWeight: 600,
  color: '#64748b',
  marginBottom: '4px',
}

const statValue: React.CSSProperties = {
  fontSize: '24px',
  fontWeight: 800,
  color: '#0f172a',
  display: 'flex',
  alignItems: 'baseline',
  gap: '6px',
}

const panelCard: React.CSSProperties = {
  background: '#fff',
  border: '1px solid #e2e8f0',
  borderRadius: '14px',
  padding: '24px',
  boxShadow: '0 4px 6px -1px rgba(0, 0, 0, 0.05), 0 2px 4px -1px rgba(0, 0, 0, 0.03)',
}

const errorBanner: React.CSSProperties = {
  padding: '12px 16px',
  borderRadius: '8px',
  background: '#fef2f2',
  color: '#991b1b',
  border: '1px solid #fca5a5',
  marginBottom: '16px',
  fontSize: '14px',
}

const overlayStyle: React.CSSProperties = {
  position: 'fixed',
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: 'rgba(15, 23, 42, 0.3)',
  backdropFilter: 'blur(4px)',
  zIndex: 1000,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
}

const popoverStyle: React.CSSProperties = {
  background: '#fff',
  border: '1px solid #e2e8f0',
  borderRadius: '12px',
  padding: '20px',
  boxShadow: '0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04)',
  width: '320px',
  maxWidth: '90%',
}

const spinnerStyle: React.CSSProperties = {
  width: '40px',
  height: '40px',
  border: '4px solid #f1f5f9',
  borderTop: '4px solid #0b7285',
  borderRadius: '50%',
  animation: 'spin 1s linear infinite',
}
