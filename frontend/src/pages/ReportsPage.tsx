import { CSSProperties, Fragment, ReactNode, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  api,
  BIReportPayload,
  DailyReportPayload,
  ManagerOpsPayload,
  MentorClassReportItem,
  MentorReportChecklistItem,
  MentorReportItem,
} from '../api/client'

type ReportsViewMode = 'bi' | 'mentor' | 'daily' | 'ops'
const CAIRO_TIME_ZONE = 'Africa/Cairo'

export default function ReportsPage() {
  const [searchParams] = useSearchParams()
  const requestedTab = searchParams.get('tab')
  const requestedDate = searchParams.get('date')
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

  const today = new Date()
  const defaultFrom = new Date(today)
  defaultFrom.setMonth(defaultFrom.getMonth() - 5)
  defaultFrom.setDate(1)

  const [biFrom, setBIFrom] = useState<string>(formatDateInput(defaultFrom))
  const [biTo, setBITo] = useState<string>(formatDateInput(today))
  const [biAppliedFrom, setBIAppliedFrom] = useState<string>(formatDateInput(defaultFrom))
  const [biAppliedTo, setBIAppliedTo] = useState<string>(formatDateInput(today))
  const [biLoading, setBILoading] = useState(false)
  const [biError, setBIError] = useState<string | null>(null)
  const [biData, setBIData] = useState<BIReportPayload | null>(null)
  const [dailyDate, setDailyDate] = useState<string>(requestedDate || '')
  const [dailyLoading, setDailyLoading] = useState(false)
  const [dailyError, setDailyError] = useState<string | null>(null)
  const [dailyData, setDailyData] = useState<DailyReportPayload | null>(null)
  const [opsDate, setOpsDate] = useState<string>(requestedDate || '')
  const [opsLoading, setOpsLoading] = useState(false)
  const [opsError, setOpsError] = useState<string | null>(null)
  const [opsData, setOpsData] = useState<ManagerOpsPayload | null>(null)

  const canViewBI = userRole === 'admin' || userRole === 'mentor_head' || userRole === 'manager'
  const canViewMentor = userRole === 'student_success' || userRole === 'mentor_head' || userRole === 'manager'
  const canViewDaily = userRole === 'mentor_head' || userRole === 'manager'
  const canViewManagerOps = userRole === 'manager'

  const [viewMode, setViewMode] = useState<ReportsViewMode>('bi')

  useEffect(() => {
    void loadMe()
  }, [])

  useEffect(() => {
    if (!userRole) return
    if (requestedTab === 'ops' && canViewManagerOps) {
      if (requestedDate) {
        setOpsDate(requestedDate)
      }
      setViewMode('ops')
      void loadManagerOps(requestedDate || undefined)
      return
    }
    if (requestedTab === 'daily' && canViewDaily) {
      if (requestedDate) {
        setDailyDate(requestedDate)
      }
      setViewMode('daily')
      void loadDailyReport(requestedDate || undefined)
      return
    }
    if (canViewManagerOps) {
      setViewMode('ops')
      return
    }
    if (canViewBI) {
      setViewMode('bi')
      return
    }
    if (canViewDaily) {
      setViewMode('daily')
      return
    }
    if (canViewMentor) {
      setViewMode('mentor')
    }
  }, [userRole, canViewBI, canViewDaily, canViewManagerOps, canViewMentor, requestedTab, requestedDate])

  useEffect(() => {
    if (!userRole || !canViewMentor || viewMode !== 'mentor') {
      return
    }
    void loadReports()
  }, [mentorFilter, userRole, canViewMentor, viewMode])

  useEffect(() => {
    if (!userRole || !canViewBI || viewMode !== 'bi') {
      return
    }
    void loadBIReports(biAppliedFrom, biAppliedTo)
  }, [userRole, canViewBI, viewMode, biAppliedFrom, biAppliedTo])

  useEffect(() => {
    if (!userRole || !canViewDaily || viewMode !== 'daily') {
      return
    }
    void loadDailyReport(dailyDate || undefined)
  }, [userRole, canViewDaily, viewMode])

  useEffect(() => {
    if (!userRole || !canViewManagerOps || viewMode !== 'ops') {
      return
    }
    void loadManagerOps(opsDate || undefined)
  }, [userRole, canViewManagerOps, viewMode])

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

  async function loadBIReports(from: string, to: string) {
    try {
      setBILoading(true)
      setBIError(null)
      const data = await api.getBIReports({ from, to })
      setBIData(data)
    } catch (err) {
      setBIError(err instanceof Error ? err.message : 'Failed to load BI reports')
    } finally {
      setBILoading(false)
    }
  }

  async function loadDailyReport(date?: string) {
    try {
      setDailyLoading(true)
      setDailyError(null)
      const data = await api.getDailyReport(date)
      setDailyData(data)
      setDailyDate(data.report_date)
    } catch (err) {
      setDailyError(err instanceof Error ? err.message : 'Failed to load daily report')
    } finally {
      setDailyLoading(false)
    }
  }

  async function loadManagerOps(date?: string) {
    try {
      setOpsLoading(true)
      setOpsError(null)
      const data = await api.getManagerOpsReport(date)
      setOpsData(data)
      setOpsDate(data.report_date)
    } catch (err) {
      setOpsError(err instanceof Error ? err.message : 'Failed to load manager ops report')
    } finally {
      setOpsLoading(false)
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
  const canRemove = userRole === 'mentor_head' || userRole === 'manager'
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
      const requestID = ++checklistRequestRef.current
      const activeRes = await api.getMentorReportChecklist({ mentor_id: row.mentor_id, round_status: 'active' })
      if (requestID === checklistRequestRef.current) {
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

  function applyDateFilter() {
    if (!biFrom || !biTo) {
      setBIError('Please select both From and To dates.')
      return
    }
    if (biTo < biFrom) {
      setBIError('To date must be on or after From date.')
      return
    }
    setBIAppliedFrom(biFrom)
    setBIAppliedTo(biTo)
  }

  function applyDailyDateFilter() {
    if (!dailyDate) {
      setDailyError('Please select a report date.')
      return
    }
    void loadDailyReport(dailyDate)
  }

  function applyOpsDateFilter() {
    if (!opsDate) {
      setOpsError('Please select an operations date.')
      return
    }
    void loadManagerOps(opsDate)
  }

  const reportTabs = [
    canViewManagerOps ? { key: 'ops' as const, label: 'Manager Ops' } : null,
    canViewBI ? { key: 'bi' as const, label: 'Business Intelligence' } : null,
    canViewDaily ? { key: 'daily' as const, label: 'Daily Reports' } : null,
    canViewMentor ? { key: 'mentor' as const, label: 'Mentor Compliance' } : null,
  ].filter(Boolean) as { key: ReportsViewMode; label: string }[]

  return (
    <>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <h1>Reports</h1>
      </div>

      {reportTabs.length > 1 && (
        <div style={{ display: 'flex', gap: '10px', marginBottom: '16px' }}>
          {reportTabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setViewMode(tab.key)}
              style={{
                ...tabButtonStyle,
                ...(viewMode === tab.key ? activeTabButtonStyle : null),
              }}
            >
              {tab.label}
            </button>
          ))}
        </div>
      )}

      {!canViewManagerOps && !canViewBI && !canViewMentor && !canViewDaily && (
        <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: '12px', padding: '24px' }}>
          <h3 style={{ marginTop: 0 }}>No Reports Access</h3>
          <p style={{ color: '#555', marginBottom: 0 }}>Your role does not have access to reports.</p>
        </div>
      )}

      {canViewManagerOps && viewMode === 'ops' && (
        <>
          <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: '12px', padding: '16px', marginBottom: '16px' }}>
            <div style={{ display: 'flex', gap: '10px', alignItems: 'end', flexWrap: 'wrap' }}>
              <div>
                <label style={filterLabelStyle}>Operations Date</label>
                <input type="date" value={opsDate} onChange={(e) => setOpsDate(e.target.value)} style={filterInputStyle} />
              </div>
              <button onClick={applyOpsDateFilter} style={actionBtnStyle}>Load Manager Ops</button>
              {opsData && (
                <span style={{ color: '#6b7280', fontSize: '13px', paddingBottom: '10px' }}>
                  Timezone {opsData.timezone} · Generated {formatDateTime(opsData.generated_at)}
                </span>
              )}
            </div>
          </div>

          {opsError && <div style={{ background: '#f8d7da', color: '#721c24', padding: '12px', borderRadius: '8px', marginBottom: '16px' }}>{opsError}</div>}

          {opsLoading && (
            <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: '12px', padding: '24px' }}>Loading manager ops...</div>
          )}

          {!opsLoading && opsData && <ManagerOpsView data={opsData} />}
        </>
      )}

      {canViewBI && viewMode === 'bi' && (
        <>
          <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: '12px', padding: '16px', marginBottom: '16px' }}>
            <div className="bi-filter-row" style={{ display: 'flex', gap: '10px', alignItems: 'end', flexWrap: 'wrap' }}>
              <div>
                <label style={filterLabelStyle}>From</label>
                <input type="date" value={biFrom} onChange={(e) => setBIFrom(e.target.value)} style={filterInputStyle} />
              </div>
              <div>
                <label style={filterLabelStyle}>To</label>
                <input type="date" value={biTo} onChange={(e) => setBITo(e.target.value)} style={filterInputStyle} />
              </div>
              <button onClick={applyDateFilter} style={actionBtnStyle}>Apply</button>
            </div>
          </div>

          {biError && <div style={{ background: '#f8d7da', color: '#721c24', padding: '12px', borderRadius: '8px', marginBottom: '16px' }}>{biError}</div>}

          {biLoading && (
            <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: '12px', padding: '24px' }}>Loading BI dashboard...</div>
          )}

          {!biLoading && biData && (
            <BIDashboard data={biData} />
          )}
        </>
      )}

      {canViewDaily && viewMode === 'daily' && (
        <>
          <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: '12px', padding: '16px', marginBottom: '16px' }}>
            <div style={{ display: 'flex', gap: '10px', alignItems: 'end', flexWrap: 'wrap' }}>
              <div>
                <label style={filterLabelStyle}>Report Date</label>
                <input type="date" value={dailyDate} onChange={(e) => setDailyDate(e.target.value)} style={filterInputStyle} />
              </div>
              <button onClick={applyDailyDateFilter} style={actionBtnStyle}>Load Daily Report</button>
              {dailyData && (
                <span style={{ color: '#6b7280', fontSize: '13px', paddingBottom: '10px' }}>
                  Ready at {formatDateTime(dailyData.ready_at)}
                </span>
              )}
            </div>
          </div>

          {dailyError && <div style={{ background: '#f8d7da', color: '#721c24', padding: '12px', borderRadius: '8px', marginBottom: '16px' }}>{dailyError}</div>}

          {dailyLoading && (
            <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: '12px', padding: '24px' }}>Loading daily report...</div>
          )}

          {!dailyLoading && dailyData && <DailyReportView data={dailyData} />}
        </>
      )}

      {canViewMentor && viewMode === 'mentor' && (
        <>
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
      )}
    </>
  )
}

function BIDashboard({ data }: { data: BIReportPayload }) {
  const conversion = data.report1.conversion
  const renewal = data.report1.renewal
  const liability = data.report2.refund_liability
  const revenuePulse = data.report2.revenue_pulse

  return (
    <div className="bi-dashboard" style={{ display: 'grid', gap: '14px' }}>
      <div className="bi-metrics-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '12px' }}>
        <MetricCard
          title="Test→Paid (30d)"
          value={`${conversion.conversion_rate.toFixed(1)}%`}
          sub={`${conversion.converted_count}/${conversion.test_booked_count} converted`}
        />
        <MetricCard
          title="Returning Renewal Rate"
          value={`${renewal.renewal_rate.toFixed(1)}%`}
          sub={`${renewal.renewed_count}/${renewal.returning_count} renewed`}
        />
        <MetricCard
          title="Refund Liability"
          value={`${liability.total_value.toLocaleString()} EGP`}
          sub={`${liability.students_count} students · bundle-weighted credit pricing`}
        />
        <MetricCard
          title="Revenue Pulse"
          value={`${revenuePulse.total_collected.toLocaleString()} EGP`}
          sub={revenuePulse.active_round_start ? `From ${String(revenuePulse.active_round_start).slice(0, 10)}` : 'No active round'}
        />
      </div>

      <ReportPanel title="Report 1 · Bottleneck Leads (offer_sent > 3 days)">
        <SimpleTable
          columns={['Name', 'Phone', 'Status', 'Days Stuck']}
          rows={data.report1.bottleneck.map((row) => [row.full_name, row.phone, row.status, String(row.days_in_status)])}
          emptyText="No bottlenecks found."
        />
      </ReportPanel>

      <ReportPanel title="Report 2 · Ghost Students (in_classes but underpaid)">
        <SimpleTable
          columns={['Name', 'Phone', 'Offer Price', 'Paid', 'Shortfall']}
          rows={data.report2.ghost_students.map((row) => [
            row.full_name,
            row.phone,
            `${row.offer_price} EGP`,
            `${row.total_paid} EGP`,
            `${row.shortfall} EGP`,
          ])}
          emptyText="No ghost students detected."
        />
      </ReportPanel>

      <div className="bi-dual-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: '12px' }}>
        <ReportPanel title="Report 3 · Lost Students (renewal_pending, 0 credits)">
          <SimpleTable
            columns={['Name', 'Phone', 'Last Level', 'Completed']}
            rows={data.report3.lost.map((row) => [row.full_name, row.phone, `L${row.last_level}`, row.last_completed_at])}
            emptyText="No lost students in the recent window."
          />
        </ReportPanel>

        <ReportPanel title="Report 3 · Stalled Students (waiting_for_round, credits > 0)">
          <SimpleTable
            columns={['Name', 'Phone', 'Credits', 'Last Level']}
            rows={data.report3.stalled.map((row) => [row.full_name, row.phone, String(row.remaining_credits), `L${row.last_level}`])}
            emptyText="No stalled students found."
          />
        </ReportPanel>
      </div>

      <ReportPanel title={`Report 4 · Active Classes Trend (${data.filters.from} → ${data.filters.to})`}>
        <ActiveClassesBarChart points={data.report4.active_classes_by_month} />
        <div style={{ height: '12px' }} />
        <SimpleTable
          columns={['Month', 'Active Classes Running']}
          rows={data.report4.active_classes_by_month.map((row) => [row.month, String(row.classes_count)])}
          emptyText="No class trend data for selected range."
        />
      </ReportPanel>

      <div className="bi-dual-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: '12px' }}>
        <ReportPanel title="Report 4 · Started Learning in Selected Range">
          <div style={{ fontSize: '24px', fontWeight: 800, color: '#1f2937' }}>{data.report4.started_learners} learners</div>
          <div style={{ color: '#6b7280', marginTop: '4px' }}>
            Distinct students who entered learning in the selected period (by enrollment start date, including late joiners).
          </div>
        </ReportPanel>

        <ReportPanel title="Report 4 · Finished Level in Selected Range">
          <div style={{ fontSize: '24px', fontWeight: 800, color: '#1f2937' }}>{data.report4.finished_learners} learners</div>
          <div style={{ color: '#6b7280', marginTop: '4px' }}>
            Distinct students with a completed class enrollment in the selected period.
          </div>
        </ReportPanel>
      </div>
    </div>
  )
}

function ManagerOpsView({ data }: { data: ManagerOpsPayload }) {
  const summary = data.summary
  const sessionsMissingCompletion = Math.max(summary.sessions_scheduled - summary.sessions_completed, 0)
  const attentionCards = [
    { label: 'Mentors Late', value: summary.late_mentor_sessions, tone: '#92400e', background: '#fef3c7' },
    { label: 'Mentors Absent', value: summary.absent_mentor_sessions, tone: '#991b1b', background: '#fee2e2' },
    { label: 'Mentor Checks Missing', value: summary.unchecked_mentor_sessions, tone: '#075985', background: '#e0f2fe' },
    { label: 'Attendance Pending', value: summary.sessions_attendance_pending, tone: '#7c2d12', background: '#ffedd5' },
    { label: 'Sessions Unfinished', value: sessionsMissingCompletion, tone: '#7f1d1d', background: '#fee2e2' },
    { label: 'Placement Tests Pending', value: summary.placement_tests_pending, tone: '#1d4ed8', background: '#dbeafe' },
  ]

  return (
    <div style={{ display: 'grid', gap: '14px' }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '12px' }}>
        <MetricCard
          title="Live Sessions"
          value={`${summary.sessions_live_now}/${summary.sessions_scheduled}`}
          sub={`${summary.sessions_completed} completed so far`}
        />
        <MetricCard
          title="Attendance Coverage"
          value={`${summary.sessions_attendance_done}/${summary.sessions_scheduled}`}
          sub={`${summary.sessions_attendance_pending} sessions still missing attendance`}
        />
        <MetricCard
          title="Students Attended"
          value={`${summary.attended_students}/${summary.expected_students}`}
          sub="Present or late out of students expected"
        />
        <MetricCard
          title="Cash In"
          value={`${summary.today_revenue.toLocaleString()} EGP`}
          sub={`${summary.paying_leads_count} paying lead${summary.paying_leads_count === 1 ? '' : 's'}`}
        />
        <MetricCard
          title="Placement Tests"
          value={`${summary.placement_tests_completed}/${summary.placement_tests_scheduled}`}
          sub={`${summary.placement_tests_pending} still waiting for results`}
        />
      </div>

      <ReportPanel title="Needs Attention">
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '10px' }}>
          {attentionCards.map((item) => (
            <div key={item.label} style={{ borderRadius: '12px', padding: '14px', background: item.background, color: item.tone, border: '1px solid rgba(0,0,0,0.06)' }}>
              <div style={{ fontSize: '13px', fontWeight: 800 }}>{item.label}</div>
              <div style={{ marginTop: '6px', fontSize: '28px', fontWeight: 900 }}>{item.value}</div>
            </div>
          ))}
        </div>
      </ReportPanel>

      <ReportPanel title={`Sessions for ${data.report_date}`}>
        {data.session_rows.length === 0 ? (
          <div style={{ color: '#6b7280' }}>No active sessions scheduled for this Cairo business day.</div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: '1240px' }}>
              <thead>
                <tr style={{ background: '#f8fafc', textAlign: 'left' }}>
                  <th style={thStyle}>Phase</th>
                  <th style={thStyle}>Class</th>
                  <th style={thStyle}>Mentor</th>
                  <th style={thStyle}>Session</th>
                  <th style={thStyle}>Schedule</th>
                  <th style={thStyle}>Mentor Status</th>
                  <th style={thStyle}>Attendance</th>
                  <th style={thStyle}>Completion</th>
                </tr>
              </thead>
              <tbody>
                {data.session_rows.map((row) => (
                  <tr key={row.session_id}>
                    <td style={tdStyle}>
                      <StatusPill status={row.session_phase} />
                    </td>
                    <td style={tdStyle}>
                      <div style={{ fontWeight: 700 }}>{row.class_label}</div>
                      <div style={{ color: '#6b7280', fontSize: '12px' }}>{row.class_key}</div>
                      <a href={`/app/mentor-head/class?class_key=${encodeURIComponent(row.class_key)}`} style={{ color: '#0d6efd', fontSize: '12px', fontWeight: 700, textDecoration: 'none' }}>
                        Open class
                      </a>
                    </td>
                    <td style={tdStyle}>
                      <div style={{ fontWeight: 700 }}>{row.mentor_name || row.mentor_email || 'Unassigned'}</div>
                      <div style={{ color: '#6b7280', fontSize: '12px' }}>{row.mentor_email || 'No mentor assigned'}</div>
                    </td>
                    <td style={tdStyle}>S{row.session_number}</td>
                    <td style={tdStyle}>
                      <div>{row.scheduled_date}</div>
                      <div style={{ color: '#6b7280', fontSize: '12px' }}>{formatBusinessTimeLabel(row.scheduled_time)}</div>
                    </td>
                    <td style={tdStyle}>
                      <div style={{ display: 'grid', gap: '4px' }}>
                        <StatusPill status={row.mentor_status} />
                        {row.compliance_checked && !row.mentor_absent && (
                          <span style={{ color: '#6b7280', fontSize: '12px' }}>
                            {row.delay_minutes} min delay
                          </span>
                        )}
                      </div>
                    </td>
                    <td style={tdStyle}>
                      <div style={{ display: 'grid', gap: '4px' }}>
                        <StatusPill status={row.attendance_status} />
                        <span style={{ color: '#1f2937', fontSize: '12px' }}>
                          {row.attended_students}/{row.expected_students} attended
                        </span>
                        <span style={{ color: '#6b7280', fontSize: '12px' }}>
                          {row.attendance_marked} marked · {row.absent_students} absent
                        </span>
                      </div>
                    </td>
                    <td style={tdStyle}>
                      <div style={{ display: 'grid', gap: '4px' }}>
                        <StatusPill status={row.session_status} />
                        {row.actual_time && (
                          <span style={{ color: '#6b7280', fontSize: '12px' }}>
                            Actual {formatBusinessTimeLabel(row.actual_time)}
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </ReportPanel>
    </div>
  )
}

function DailyReportView({ data }: { data: DailyReportPayload }) {
  return (
    <div style={{ display: 'grid', gap: '14px' }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '12px' }}>
        <MetricCard
          title="Classes Taught"
          value={`${data.classes_taught}/${data.classes_scheduled}`}
          sub={`${data.classes_missing_report} mentor report(s) missing`}
        />
        <MetricCard
          title="Student Absence"
          value={`${data.absent_students}/${data.expected_students}`}
          sub="Absent out of students expected today"
        />
        <MetricCard
          title="Report Date"
          value={data.report_date}
          sub={`Generated ${formatDateTime(data.generated_at)}`}
        />
      </div>

      <ReportPanel title="Daily Classes">
        {data.class_rows.length === 0 ? (
          <div style={{ color: '#6b7280' }}>No active sessions were scheduled for this date.</div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: '980px' }}>
              <thead>
                <tr style={{ background: '#f8fafc', textAlign: 'left' }}>
                  <th style={thStyle}>Class</th>
                  <th style={thStyle}>Mentor</th>
                  <th style={thStyle}>Session</th>
                  <th style={thStyle}>Scheduled</th>
                  <th style={thStyle}>Report</th>
                  <th style={thStyle}>Punctuality</th>
                  <th style={thStyle}>Absence</th>
                </tr>
              </thead>
              <tbody>
                {data.class_rows.map((row) => (
                  <tr key={row.session_id}>
                    <td style={tdStyle}>
                      <div style={{ fontWeight: 700 }}>{row.class_label}</div>
                      <div style={{ color: '#6b7280', fontSize: '12px' }}>{row.class_key}</div>
                    </td>
                    <td style={tdStyle}>{row.mentor_email || 'Unassigned'}</td>
                    <td style={tdStyle}>S{row.session_number}</td>
                    <td style={tdStyle}>
                      {row.scheduled_date} · {formatBusinessTimeLabel(row.scheduled_time)}
                    </td>
                    <td style={tdStyle}>
                      <StatusPill status={row.report_status} />
                    </td>
                    <td style={tdStyle}>
                      <div style={{ display: 'grid', gap: '4px' }}>
                        <StatusPill status={row.punctuality_status} />
                        {row.compliance_checked && !row.mentor_absent && (
                          <span style={{ color: '#6b7280', fontSize: '12px' }}>
                            {row.delay_minutes} min delay
                          </span>
                        )}
                        {!row.compliance_checked && row.report_status === 'filled' && (
                          <span style={{ color: '#6b7280', fontSize: '12px' }}>
                            Mentor actual {formatBusinessTimeLabel(row.actual_time) || '-'}
                          </span>
                        )}
                      </div>
                    </td>
                    <td style={tdStyle}>
                      {row.absent_students}/{row.expected_students}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </ReportPanel>
    </div>
  )
}

function StatusPill({ status }: { status: string }) {
  const normalized = String(status || '').toLowerCase()
  const palette =
    normalized === 'filled' || normalized === 'on_time' || normalized === 'done' || normalized === 'completed' || normalized === 'live_now'
      ? { background: '#dcfce7', color: '#166534', border: '#bbf7d0' }
      : normalized === 'late' || normalized === 'partial' || normalized === 'upcoming'
        ? { background: '#fef3c7', color: '#92400e', border: '#fde68a' }
        : normalized === 'ss_check_missing' || normalized === 'not_checked' || normalized === 'not_started' || normalized === 'scheduled'
          ? { background: '#e0f2fe', color: '#075985', border: '#bae6fd' }
          : normalized === 'none_expected'
            ? { background: '#f3f4f6', color: '#4b5563', border: '#d1d5db' }
            : { background: '#fee2e2', color: '#991b1b', border: '#fecaca' }
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        width: 'fit-content',
        padding: '4px 10px',
        borderRadius: '999px',
        border: `1px solid ${palette.border}`,
        background: palette.background,
        color: palette.color,
        fontSize: '12px',
        fontWeight: 800,
        textTransform: 'capitalize',
      }}
    >
      {normalized.replace(/_/g, ' ')}
    </span>
  )
}

function MetricCard({ title, value, sub }: { title: string; value: string; sub: string }) {
  return (
    <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: '12px', padding: '14px' }}>
      <div style={{ color: '#6b7280', fontSize: '13px', fontWeight: 700 }}>{title}</div>
      <div style={{ marginTop: '8px', fontSize: '24px', fontWeight: 800, color: '#111827' }}>{value}</div>
      <div style={{ marginTop: '4px', fontSize: '13px', color: '#6b7280' }}>{sub}</div>
    </div>
  )
}

function ActiveClassesBarChart({ points }: { points: { month: string; classes_count: number }[] }) {
  if (!points.length) {
    return <div style={{ color: '#6b7280' }}>No chart data in selected range.</div>
  }

  const maxValue = Math.max(...points.map((p) => p.classes_count), 1)

  const getBarColor = (index: number): string => {
    if (index === 0) return '#2563eb'
    const previous = points[index-1]?.classes_count ?? points[index].classes_count
    const current = points[index].classes_count
    if (current > previous) return '#16a34a'
    if (current < previous) return '#dc2626'
    return '#f59e0b'
  }

  return (
    <div style={{ display: 'grid', gap: '8px' }}>
      {points.map((point, index) => {
        const width = Math.max((point.classes_count / maxValue) * 100, point.classes_count > 0 ? 3 : 0)
        const barColor = getBarColor(index)
        return (
          <div key={point.month} style={{ display: 'grid', gridTemplateColumns: '80px 1fr 40px', alignItems: 'center', gap: '8px' }}>
            <div style={{ fontSize: '12px', color: '#6b7280', fontWeight: 700 }}>{point.month}</div>
            <div style={{ background: '#e5e7eb', borderRadius: '999px', height: '10px', overflow: 'hidden' }}>
              <div
                style={{
                  width: `${width}%`,
                  height: '100%',
                  background: barColor,
                }}
              />
            </div>
            <div style={{ fontSize: '12px', color: '#1f2937', fontWeight: 700, textAlign: 'right' }}>{point.classes_count}</div>
          </div>
        )
      })}
    </div>
  )
}

function ReportPanel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="report-panel" style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: '12px', padding: '14px' }}>
      <h3 style={{ margin: 0, marginBottom: '10px', color: '#1f2937' }}>{title}</h3>
      {children}
    </section>
  )
}

function SimpleTable({
  columns,
  rows,
  emptyText,
}: {
  columns: string[]
  rows: string[][]
  emptyText: string
}) {
  if (rows.length === 0) {
    return <div style={{ color: '#6b7280' }}>{emptyText}</div>
  }
  return (
    <div style={{ overflowX: 'auto' }}>
      <table className="simple-table" style={{ width: '100%', borderCollapse: 'collapse', minWidth: '520px' }}>
        <thead>
          <tr style={{ background: '#f8fafc' }}>
            {columns.map((column) => (
              <th key={column} style={{ ...thStyle, borderBottom: '1px solid #e5e7eb' }}>
                {column}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, idx) => (
            <tr key={`${idx}-${row.join('-')}`}>
              {row.map((cell, cellIdx) => (
                <td key={`${idx}-${cellIdx}`} style={tdStyle}>
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
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
              {row.class_days} @ {formatBusinessTimeLabel(row.class_time)}
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
                            {row.scheduled_date || '-'} ({getSessionSlotLabel(row.class_days, row.session_number)}) @ {formatBusinessTimeLabel(row.scheduled_time)}
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

function formatDateInput(value: Date): string {
  return value.toISOString().slice(0, 10)
}

function formatDateTime(value: string): string {
  if (!value) return '-'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleString(undefined, {
    timeZone: CAIRO_TIME_ZONE,
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

function formatBusinessTimeLabel(value: string): string {
  if (!value) return ''
  const raw = String(value).slice(0, 5)
  const [hourRaw, minuteRaw] = raw.split(':')
  const hour = Number(hourRaw)
  if (!Number.isFinite(hour)) return raw
  const displayHour = hour > 0 && hour < 12 ? hour + 12 : hour
  const date = new Date(2000, 0, 1, displayHour, Number(minuteRaw || 0), 0)
  return new Intl.DateTimeFormat('en-US', {
    hour: 'numeric',
    minute: '2-digit',
  }).format(date)
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

const tabButtonStyle: CSSProperties = {
  padding: '8px 14px',
  borderRadius: '8px',
  border: '1px solid #ced4da',
  background: '#fff',
  color: '#495057',
  cursor: 'pointer',
  fontWeight: 700,
}

const activeTabButtonStyle: CSSProperties = {
  background: '#0d6efd',
  color: '#fff',
  borderColor: '#0d6efd',
}

const filterLabelStyle: CSSProperties = {
  display: 'block',
  fontSize: '12px',
  color: '#6b7280',
  marginBottom: '6px',
  fontWeight: 700,
}

const filterInputStyle: CSSProperties = {
  padding: '10px 12px',
  borderRadius: '8px',
  border: '1px solid #ccc',
}

const actionBtnStyle: CSSProperties = {
  padding: '10px 14px',
  borderRadius: '8px',
  border: '1px solid #0d6efd',
  background: '#0d6efd',
  color: '#fff',
  cursor: 'pointer',
  fontWeight: 700,
}
