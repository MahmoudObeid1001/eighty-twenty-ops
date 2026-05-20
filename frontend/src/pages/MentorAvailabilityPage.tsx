import { useEffect, useMemo, useState, useRef } from 'react'
import { api, MentorAvailabilityWindow } from '../api/client'

// ─── Constants ───────────────────────────────────────────────────────────────

const SLOT_A = { key: 'slotA' as const, label: 'Slot A', start: '19:30', end: '21:30' }
const SLOT_B = { key: 'slotB' as const, label: 'Slot B', start: '22:00', end: '00:00' }
const SLOTS = [SLOT_A, SLOT_B]

// Sat=0 Sun=1 Mon=2 Tue=3 Wed=4 Thu=5 Fri=6 (our column order)
const WEEK_DAYS = [
  { label: 'Sat', jsDay: 6 },
  { label: 'Sun', jsDay: 0 },
  { label: 'Mon', jsDay: 1 },
  { label: 'Tue', jsDay: 2 },
  { label: 'Wed', jsDay: 3 },
  { label: 'Thu', jsDay: 4 },
  { label: 'Fri', jsDay: 5 },
]
// Work days (no Friday) — used for pattern pairs defined inline below

// Fixed day pairs: Sat↔Tue, Sun↔Wed, Mon↔Thu  (JS weekday numbers)
const PAIR_MAP: Record<number, number> = {
  6: 2, 2: 6,  // Sat ↔ Tue
  0: 3, 3: 0,  // Sun ↔ Wed
  1: 4, 4: 1,  // Mon ↔ Thu
}

// ─── Types ───────────────────────────────────────────────────────────────────

interface DayState {
  slotA: boolean     // is slot on (source OR auto)
  slotB: boolean
  slotAauto: boolean // true = auto-filled from paired day; user cannot toggle it directly
  slotBauto: boolean
  note: string
}

type SlotKey = 'slotA' | 'slotB'
type SlotAutoKey = 'slotAauto' | 'slotBauto'
type GridState = Record<string, DayState>

// ─── Helpers ─────────────────────────────────────────────────────────────────

function autoKey(slot: SlotKey): SlotAutoKey {
  return (slot + 'auto') as SlotAutoKey
}

function emptyDay(): DayState {
  return { slotA: false, slotB: false, slotAauto: false, slotBauto: false, note: '' }
}

function currentMonth() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

function nextMonth(month: string) {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(Date.UTC(y, m, 1))
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`
}

function prevMonth(month: string) {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(y, m - 1, 1)
  d.setMonth(d.getMonth() - 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
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

// Map JS weekday (0=Sun…6=Sat) → column index in our Sat-first grid (0–6)
function colIndex(jsDay: number): number {
  return jsDay === 6 ? 0 : jsDay + 1
}

function isFriday(d: Date) {
  return d.getUTCDay() === 5
}

function matchSlot(startTime: string): SlotKey | null {
  const t = startTime.slice(0, 5)
  if (t === SLOT_A.start) return 'slotA'
  if (t === SLOT_B.start) return 'slotB'
  const h = parseInt(t.split(':')[0], 10)
  if (!isNaN(h)) return h < 21 ? 'slotA' : 'slotB'
  return null
}

/**
 * Given a date string (YYYY-MM-DD), return the date string for its paired day
 * in the same Sat-starting week. Returns null for Friday (no pair).
 */
function getPairedDate(dateStr: string): string | null {
  const d = new Date(dateStr + 'T00:00:00Z')
  const jsDay = d.getUTCDay()
  const pairedJsDay = PAIR_MAP[jsDay]
  if (pairedJsDay === undefined) return null // Friday has no pair

  // Find Saturday of this week (week start in our Sat-first layout)
  const weekPos = colIndex(jsDay)  // how many days after Saturday this day falls
  const sat = new Date(d)
  sat.setUTCDate(d.getUTCDate() - weekPos)

  // Offset to the paired day
  const pairedPos = colIndex(pairedJsDay)
  const paired = new Date(sat)
  paired.setUTCDate(sat.getUTCDate() + pairedPos)
  return dateKey(paired)
}

/**
 * Given a date string, returns the 8 dates forming 4 consecutive weeks of paired sessions.
 * Returns an array of 8 date strings.
 */
function get8Sessions(dateStr: string): string[] {
  const dates: string[] = []
  const d = new Date(dateStr + 'T00:00:00Z')
  const jsDay = d.getUTCDay()
  const pairedJsDay = PAIR_MAP[jsDay]
  if (pairedJsDay === undefined) return [dateStr]

  const weekPos = colIndex(jsDay)
  const pairPos = colIndex(pairedJsDay)

  const sat = new Date(d)
  sat.setUTCDate(d.getUTCDate() - weekPos)

  for (let week = 0; week < 4; week++) {
    const d1 = new Date(sat)
    d1.setUTCDate(sat.getUTCDate() + (week * 7) + weekPos)
    dates.push(dateKey(d1))

    const d2 = new Date(sat)
    d2.setUTCDate(sat.getUTCDate() + (week * 7) + pairPos)
    dates.push(dateKey(d2))
  }
  return dates
}

/**
 * Convert raw API windows into GridState.
 * Loaded data is all treated as non-auto (source).
 */
function windowsToGrid(windows: MentorAvailabilityWindow[]): GridState {
  const grid: GridState = {}
  for (const w of windows) {
    const k = w.available_date.slice(0, 10)
    if (!grid[k]) grid[k] = emptyDay()
    const slot = matchSlot(w.start_time)
    if (slot) grid[k][slot] = true
    if (w.note && !grid[k].note) grid[k].note = w.note
  }
  return grid
}

/**
 * Convert GridState back into API windows for saving.
 * Both source and auto cells become individual records.
 */
function gridToWindows(grid: GridState): MentorAvailabilityWindow[] {
  const out: MentorAvailabilityWindow[] = []
  for (const [date, state] of Object.entries(grid)) {
    for (const slot of SLOTS) {
      if (state[slot.key]) {
        out.push({
          available_date: date,
          start_time: slot.start,
          end_time: slot.end,
          note: state.note || '',
        })
      }
    }
  }
  return out
}

function isLocked(dateStr: string, lockedDates: string[]): boolean {
  const todayStr = new Date().toISOString().slice(0, 10)
  if (dateStr < todayStr) return true
  return lockedDates.includes(dateStr)
}

/**
 * After setting source slots on a grid, propagate auto-fills to paired dates.
 * Mutates the gridCopy in place.
 */
function propagatePairs(gridCopy: GridState, allDates: Set<string>, lockedDates: string[]) {
  for (const [dateStr, state] of Object.entries(gridCopy)) {
    const pairedStr = getPairedDate(dateStr)
    if (!pairedStr) continue
    // Paired must be in the month and not locked
    if (!allDates.has(pairedStr)) continue
    if (isLocked(pairedStr, lockedDates)) continue

    for (const slot of SLOTS) {
      const ak = autoKey(slot.key)
      // If this day has a source slot (not auto), and paired day does not already have a source slot
      if (state[slot.key] && !state[ak]) {
        const pairedState = gridCopy[pairedStr] || emptyDay()
        if (!pairedState[slot.key]) {
          // Paired is off — set it as auto
          gridCopy[pairedStr] = { ...pairedState, [slot.key]: true, [ak]: true }
        }
        // If paired already has a source slot (value=true, auto=false), leave it alone
      }
    }
  }
}

// ─── Component ───────────────────────────────────────────────────────────────

export default function MentorAvailabilityPage() {
  const [month, setMonth] = useState(currentMonth)
  const [grid, setGrid] = useState<GridState>({})
  const [lockedDates, setLockedDates] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)

  const [notePopover, setNotePopover] = useState<string | null>(null)
  const [noteInput, setNoteInput] = useState('')
  const popoverRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handler(e: MouseEvent) {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        setNotePopover(null)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  useEffect(() => {
    void loadAvailability()
  }, [month])

  const days = useMemo(() => getDaysInMonth(month), [month])
  const nextDays = useMemo(() => getDaysInMonth(nextMonth(month)), [month])
  
  const allDates = useMemo(() => {
    const set = new Set<string>()
    for (const d of days) set.add(dateKey(d))
    for (const d of nextDays) set.add(dateKey(d))
    return set
  }, [days, nextDays])

  async function loadAvailability() {
    setLoading(true)
    setError(null)
    setMessage(null)
    try {
      const nm = nextMonth(month)
      const [data1, data2] = await Promise.all([
        api.getMyAvailability(month),
        api.getMyAvailability(nm)
      ])
      const grid1 = windowsToGrid(data1.windows || [])
      const grid2 = windowsToGrid(data2.windows || [])
      setGrid({ ...grid1, ...grid2 })
      const l1 = data1.locked_dates || []
      const l2 = data2.locked_dates || []
      setLockedDates(Array.from(new Set([...l1, ...l2])))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load availability')
    } finally {
      setLoading(false)
    }
  }

  async function saveAvailability() {
    setSaving(true)
    setError(null)
    setMessage(null)

    const nm = nextMonth(month)
    const gridCurrent: GridState = {}
    const gridNext: GridState = {}

    for (const [k, v] of Object.entries(grid)) {
      if (isLocked(k, lockedDates)) continue
      if (k.startsWith(month)) gridCurrent[k] = v
      else if (k.startsWith(nm)) gridNext[k] = v
    }

    try {
      const [data1, data2] = await Promise.all([
        api.updateMyAvailability(month, gridToWindows(gridCurrent)),
        api.updateMyAvailability(nm, gridToWindows(gridNext))
      ])
      const grid1 = windowsToGrid(data1.windows || [])
      const grid2 = windowsToGrid(data2.windows || [])
      setGrid({ ...grid1, ...grid2 })
      const l1 = data1.locked_dates || []
      const l2 = data2.locked_dates || []
      setLockedDates(Array.from(new Set([...l1, ...l2])))
      setMessage('Availability saved successfully.')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save availability')
    } finally {
      setSaving(false)
    }
  }

  async function copyFromLastMonth() {
    const prev = prevMonth(month)
    try {
      const data = await api.getMyAvailability(prev)
      const prevGrid = windowsToGrid(data.windows || [])
      // Re-map by weekday into current month
      const newGrid: GridState = { ...grid }
      for (const day of days) {
        if (isFriday(day)) continue
        const k = dateKey(day)
        if (isLocked(k, lockedDates)) continue
        const jsDay = day.getUTCDay()
        const prevMatch = Object.entries(prevGrid).find(([prevDate]) => {
          const pd = new Date(prevDate + 'T00:00:00Z')
          return pd.getUTCDay() === jsDay
        })
        if (prevMatch) {
          // Copy as source (non-auto)
          newGrid[k] = {
            slotA: prevMatch[1].slotA,
            slotB: prevMatch[1].slotB,
            slotAauto: false,
            slotBauto: false,
            note: prevMatch[1].note,
          }
        }
      }
      // Re-propagate pairs for the newly copied data
      const copy = { ...newGrid }
      propagatePairs(copy, allDates, lockedDates)
      setGrid(copy)
      setMessage('Copied from last month. Review and save when ready.')
    } catch {
      setError("Could not load last month's availability.")
    }
  }

  /**
   * Toggle a slot on a day.
   * - Clicking any active slot (source or auto-paired) will untoggle the entire 8-session block.
   * - Toggling an inactive slot ON propagates to 8 consecutive sessions (4 weeks).
   */
  function toggleSlot(dateStr: string, slot: SlotKey) {
    if (isLocked(dateStr, lockedDates)) return

    setGrid((prev) => {
      const state = prev[dateStr] || emptyDay()
      const ak = autoKey(slot)

      const newValue = !state[slot]
      // Block toggling ON if this cell is currently auto-filled in the current UI session
      if (newValue && state[ak]) return prev

      const next: GridState = { ...prev }
      const sessions = get8Sessions(dateStr)

      for (const d of sessions) {
        if (!allDates.has(d)) continue
        if (isLocked(d, lockedDates)) continue

        const dState = next[d] || emptyDay()

        if (newValue) {
          // Turning ON
          const isClickedDay = (d === dateStr)
          if (isClickedDay) {
            next[d] = { ...dState, [slot]: true, [ak]: false }
          } else {
            // Turning on auto for the other sessions
            if (!dState[slot] || dState[ak]) {
              next[d] = { ...dState, [slot]: true, [ak]: true }
            }
          }
        } else {
          // Turning OFF: Turn off ALL 8 sessions in this cohort block
          next[d] = { ...dState, [slot]: false, [ak]: false }
        }
      }

      return next
    })
  }

  function clearAll() {
    const ok = window.confirm("Are you sure you want to clear all your availability slots for the selected months? (You will need to click 'Save Availability' at the bottom to save your changes to the database.)")
    if (!ok) return
    setGrid((prev) => {
      const next: GridState = { ...prev }
      for (const k of Object.keys(next)) {
        if (!isLocked(k, lockedDates)) {
          next[k] = { ...next[k], slotA: false, slotB: false, slotAauto: false, slotBauto: false }
        }
      }
      return next
    })
    setMessage('Availability slots cleared in the grid. Please click "Save Availability" at the bottom to save your changes!')
  }



  function openNote(dateStr: string) {
    setNotePopover(dateStr)
    setNoteInput(grid[dateStr]?.note || '')
  }

  function saveNote() {
    if (!notePopover) return
    setGrid((prev) => {
      const cur = prev[notePopover] || emptyDay()
      return { ...prev, [notePopover]: { ...cur, note: noteInput } }
    })
    setNotePopover(null)
  }

  // Calendar grid
  const firstColIndex = useMemo(() => colIndex(days[0].getUTCDay()), [days])
  const calendarCells = useMemo(() => {
    const cells: Array<{ date: Date | null; key: string | null }> = []
    for (let i = 0; i < firstColIndex; i++) cells.push({ date: null, key: null })
    for (const d of days) cells.push({ date: d, key: dateKey(d) })
    while (cells.length % 7 !== 0) cells.push({ date: null, key: null })
    return cells
  }, [days, firstColIndex])

  const calendarRows: Array<Array<{ date: Date | null; key: string | null }>> = []
  for (let i = 0; i < calendarCells.length; i += 7) {
    calendarRows.push(calendarCells.slice(i, i + 7))
  }

  // Slot counter


  const hasEditable = useMemo(
    () => days.some((d) => !isLocked(dateKey(d), lockedDates) && !isFriday(d)),
    [days, lockedDates]
  )

  return (
    <div style={{ padding: '24px', maxWidth: '1100px', margin: '0 auto' }}>
      {/* ── Header ── */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '16px', marginBottom: '20px' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: '28px', fontWeight: 700, color: '#1a1a2e' }}>Availability</h1>
          <p style={{ margin: '6px 0 0', color: '#666', fontSize: '14px' }}>
            Pick the slots you can teach. Days are paired: Sat↔Tue, Sun↔Wed, Mon↔Thu — selecting one auto-fills its partner.
          </p>
        </div>
        <div style={{ display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap' }}>
          <button type="button" onClick={clearAll} disabled={loading || saving} style={outlineBtn} title="Clear all unlocked slots">
            🗑️ Clear all
          </button>
          <button type="button" onClick={copyFromLastMonth} disabled={loading || saving} style={outlineBtn} title="Copy last month's availability as a starting point">
            ↩ Copy from last month
          </button>
          <label style={{ display: 'grid', gap: '4px', fontSize: '13px', fontWeight: 700 }}>
            Month
            <input
              type="month"
              value={month}
              min={currentMonth()}
              onChange={(e) => setMonth(e.target.value || currentMonth())}
              style={{ padding: '8px 10px', border: '1px solid #ced4da', borderRadius: '6px', fontSize: '14px' }}
            />
          </label>
        </div>
      </div>

      {/* ── Slot counter removed ── */}

      {/* ── Legend ── */}
      <div style={{ display: 'flex', gap: '16px', flexWrap: 'wrap', marginBottom: '16px', fontSize: '12px', alignItems: 'center' }}>
        <span style={{ color: '#555', fontWeight: 600 }}>Legend:</span>
        <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
          <span style={{ ...chipSample, background: '#1b6e3d', color: '#fff' }}>Slot A</span>
          <span style={{ color: '#555' }}>= you selected</span>
        </span>
        <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
          <span style={{ ...chipSample, background: '#2e7d32', color: '#fff', opacity: 0.75 }}>🔗 Slot A</span>
          <span style={{ color: '#555' }}>= auto-paired (clear source to remove)</span>
        </span>
        <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
          <span style={{ ...chipSample, background: '#e8f4f8', color: '#0b7285' }}>Slot A</span>
          <span style={{ color: '#555' }}>= available to select</span>
        </span>
      </div>

      {/* ── Messages ── */}
      {error && <div style={errorBanner}>{error}</div>}
      {message && <div style={successBanner}>{message}</div>}


      {/* ── Weekly Pattern Removed ── */}

      {/* ── Calendar Grid ── */}
      {loading ? (
        <div style={{ textAlign: 'center', padding: '40px', color: '#888' }}>Loading availability…</div>
      ) : (
        <div style={panelCard}>
          {/* Day headers */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: '6px', marginBottom: '8px' }}>
            {WEEK_DAYS.map((wd) => (
              <div
                key={wd.label}
                style={{
                  textAlign: 'center',
                  fontWeight: 700,
                  fontSize: '12px',
                  letterSpacing: '0.06em',
                  textTransform: 'uppercase',
                  color: wd.jsDay === 5 ? '#bbb' : '#555',
                  padding: '4px 0',
                  borderBottom: '2px solid',
                  borderColor: wd.jsDay === 5 ? '#eee' : '#e0f0f4',
                }}
              >
                {wd.label}
              </div>
            ))}
          </div>

          {/* Calendar rows */}
          {calendarRows.map((row, ri) => (
            <div key={ri} style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: '6px', marginBottom: '6px' }}>
              {row.map((cell, ci) => {
                if (!cell.date || !cell.key) {
                  return <div key={ci} style={{ minHeight: '90px' }} />
                }
                const isFri = isFriday(cell.date)
                const todayStr = new Date().toISOString().slice(0, 10)
                const isPast = cell.key < todayStr
                const isClassLocked = lockedDates.includes(cell.key)
                const locked = isPast || isClassLocked
                const dayNum = cell.date.getUTCDate()
                const state = grid[cell.key] || emptyDay()
                const hasNote = state.note.trim().length > 0
                const isNoteOpen = notePopover === cell.key

                return (
                  <div
                    key={cell.key}
                    style={{
                      minHeight: '90px',
                      borderRadius: '8px',
                      border: '1px solid',
                      borderColor: isFri ? '#eee' : locked ? '#e0e0e0' : '#dde8ec',
                      background: isFri ? '#f9f9f9' : locked ? '#fafafa' : '#fff',
                      padding: '6px',
                      position: 'relative',
                      opacity: isFri ? 0.6 : 1,
                    }}
                  >
                    {/* Day number + note icon */}
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '5px' }}>
                      <span style={{ fontSize: '12px', fontWeight: 700, color: locked ? '#aaa' : '#333' }}>{dayNum}</span>
                      <div style={{ display: 'flex', gap: '3px', alignItems: 'center' }}>
                        {!isFri && !locked && (
                          <button
                            type="button"
                            onClick={() => openNote(cell.key!)}
                            title={hasNote ? state.note : 'Add note'}
                            style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '12px', padding: '0 1px', lineHeight: 1, color: hasNote ? '#e67700' : '#ccc' }}
                          >
                            📝
                          </button>
                        )}
                        {isClassLocked && (
                          <span style={{ fontSize: '11px', cursor: 'help' }} title="You have a class session on this day">📌</span>
                        )}
                        {!isClassLocked && locked && (
                          <span style={{ fontSize: '11px' }} title="Past date">🔒</span>
                        )}
                      </div>
                    </div>

                    {/* Slot chips or Friday label */}
                    {isFri ? (
                      <div style={{ textAlign: 'center', color: '#bbb', fontSize: '11px', fontWeight: 700, marginTop: '10px' }}>Off</div>
                    ) : (
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                        {SLOTS.map((slot) => {
                          const isOn = state[slot.key]
                          const isAuto = state[autoKey(slot.key)]
                          const isSource = isOn && !isAuto
                          const canToggle = !locked

                          return (
                            <button
                              key={slot.key}
                              type="button"
                              disabled={locked}
                              onClick={() => canToggle ? toggleSlot(cell.key!, slot.key) : undefined}
                              title={isAuto ? 'Auto-paired — click to remove the entire cohort block' : undefined}
                              style={{
                                border: isAuto ? '1.5px dashed #2e7d32' : 'none',
                                borderRadius: '5px',
                                padding: '4px 5px',
                                fontSize: '11px',
                                fontWeight: 700,
                                cursor: canToggle ? 'pointer' : 'default',
                                background: isSource
                                  ? '#1b6e3d'
                                  : isAuto
                                  ? 'rgba(46,125,50,0.12)'
                                  : locked
                                  ? '#f0f0f0'
                                  : '#e8f4f8',
                                color: isSource
                                  ? '#fff'
                                  : isAuto
                                  ? '#2e7d32'
                                  : locked
                                  ? '#bbb'
                                  : '#0b7285',
                                transition: 'background 0.12s',
                                textAlign: 'left',
                                lineHeight: '1.3',
                                display: 'flex',
                                alignItems: 'center',
                                gap: '3px',
                              }}
                              onMouseEnter={(e) => {
                                if (canToggle && !isOn) (e.currentTarget.style.background = '#d0edf5')
                              }}
                              onMouseLeave={(e) => {
                                if (canToggle && !isOn) (e.currentTarget.style.background = '#e8f4f8')
                              }}
                            >
                              {isAuto && <span style={{ fontSize: '10px' }}>🔗</span>}
                              {slot.label}
                              <span style={{ fontSize: '9px', opacity: 0.75, marginLeft: '2px' }}>{slot.start}</span>
                            </button>
                          )
                        })}
                      </div>
                    )}

                    {/* Note popover */}
                    {isNoteOpen && (
                      <div
                        ref={popoverRef}
                        style={{
                          position: 'absolute', zIndex: 100, top: '100%', left: 0,
                          background: '#fff', border: '1px solid #ced4da', borderRadius: '8px',
                          boxShadow: '0 4px 16px rgba(0,0,0,0.13)', padding: '12px',
                          width: '220px', marginTop: '4px',
                        }}
                      >
                        <div style={{ fontWeight: 700, fontSize: '12px', color: '#555', marginBottom: '6px' }}>
                          Note for {cell.date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' })}
                        </div>
                        <textarea
                          autoFocus
                          value={noteInput}
                          onChange={(e) => setNoteInput(e.target.value)}
                          placeholder="Optional note…"
                          rows={3}
                          style={{ width: '100%', resize: 'vertical', border: '1px solid #ced4da', borderRadius: '5px', padding: '6px', fontSize: '13px', fontFamily: 'inherit', boxSizing: 'border-box' }}
                        />
                        <div style={{ display: 'flex', gap: '6px', marginTop: '8px' }}>
                          <button type="button" onClick={saveNote} style={{ ...primaryBtn, fontSize: '12px', padding: '5px 10px' }}>Save</button>
                          <button type="button" onClick={() => setNotePopover(null)} style={{ ...outlineBtn, fontSize: '12px', padding: '5px 10px' }}>Cancel</button>
                        </div>
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          ))}
        </div>
      )}

      {/* ── Save ── */}
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '16px' }}>
        <button
          type="button"
          onClick={saveAvailability}
          disabled={saving || loading || !hasEditable}
          style={{
            ...primaryBtn,
            opacity: saving || loading || !hasEditable ? 0.6 : 1,
            cursor: saving || loading || !hasEditable ? 'not-allowed' : 'pointer',
            padding: '10px 28px',
            fontSize: '15px',
          }}
        >
          {saving ? 'Saving…' : 'Save Availability'}
        </button>
      </div>
    </div>
  )
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const primaryBtn: React.CSSProperties = {
  border: 'none',
  borderRadius: '7px',
  background: '#0b7285',
  color: '#fff',
  padding: '9px 16px',
  cursor: 'pointer',
  fontWeight: 700,
  fontSize: '13px',
}

const outlineBtn: React.CSSProperties = {
  border: '1px solid #adb5bd',
  borderRadius: '7px',
  background: '#fff',
  color: '#333',
  padding: '9px 14px',
  cursor: 'pointer',
  fontWeight: 600,
  fontSize: '13px',
}

const panelCard: React.CSSProperties = {
  background: '#fff',
  border: '1px solid #e0eaef',
  borderRadius: '12px',
  padding: '20px',
  marginBottom: '20px',
  boxShadow: '0 1px 4px rgba(0,0,0,0.05)',
}

const chipSample: React.CSSProperties = {
  display: 'inline-block',
  borderRadius: '4px',
  padding: '2px 7px',
  fontSize: '11px',
  fontWeight: 700,
}



const errorBanner: React.CSSProperties = {
  padding: '12px 16px',
  borderRadius: '8px',
  background: '#f8d7da',
  color: '#721c24',
  marginBottom: '12px',
  fontSize: '14px',
}

const successBanner: React.CSSProperties = {
  padding: '12px 16px',
  borderRadius: '8px',
  background: '#d4edda',
  color: '#155724',
  marginBottom: '12px',
  fontSize: '14px',
}


