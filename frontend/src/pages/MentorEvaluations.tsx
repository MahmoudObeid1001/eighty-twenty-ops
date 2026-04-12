import { useEffect, useMemo, useState } from 'react'
import { api } from '../api/client'
import MentorRoundReport from '../components/MentorRoundReport'
import MentorActiveTotalReport from '../components/MentorActiveTotalReport'

type AttendanceStatus = 'on-time' | 'late' | 'absent' | 'unknown'

interface MentorClassEvaluation {
  classKey: string
  level: number
  days: string
  time: string
  classNumber: number
  roundStatus: 'active' | 'closed'
  classCollectiveScore: number
  manual: {
    sessionQuality: number
    sessionQualityBySession: number[]
    recordedSessionCount: number
    studentsFeedback: number
    trelloSessionChecks: boolean[]
    trelloCompliancePercent: number
  }
  automatic: {
    whatsAppManagementPercent: number
    attendancePunctualityPercent: number
    attendanceStatuses: AttendanceStatus[]
  }
}

interface MentorEvaluationItem {
  id: string
  email: string
  name: string
  activeClassCount: number
  classes: MentorClassEvaluation[]
}

interface EditTarget {
  mentorId: string
  mentorName: string
  classItem: MentorClassEvaluation
}

interface TestimonialTarget {
  mentorId: string
  mentorName: string
  classes: MentorClassEvaluation[]
}

interface ReportTarget {
  mentorName: string
  mentorEmail: string
  classItem: MentorClassEvaluation
}

const TOTAL_TAB_ID = '__total_active__'

function normalizeSessionQualityBySession(values: number[] | undefined): number[] {
  const normalized = new Array(8).fill(0)
  for (let i = 0; i < normalized.length && i < (values || []).length; i++) {
    const value = Number(values?.[i] || 0)
    if (!Number.isFinite(value)) continue
    normalized[i] = Math.max(0, Math.min(10, Math.round(value)))
  }
  return normalized
}

function averageRecordedSessionQuality(values: number[]): number {
  const recorded = values.filter((value) => value > 0)
  if (recorded.length === 0) return 0
  return Math.round(recorded.reduce((acc, value) => acc + value, 0) / recorded.length)
}

function computeCollectiveKPI(classItem: MentorClassEvaluation): number {
  return classItem.classCollectiveScore
}

function statusColor(status: AttendanceStatus): string {
  switch (status) {
    case 'on-time':
      return '#2f9e44'
    case 'late':
      return '#f59f00'
    case 'absent':
      return '#e03131'
    default:
      return '#6c757d'
  }
}

function average(values: number[]): number {
  if (values.length === 0) return 0
  return Math.round(values.reduce((acc, value) => acc + value, 0) / values.length)
}

function mentorSummary(mentor: MentorEvaluationItem) {
  const classScores = mentor.classes.map((cls) => computeCollectiveKPI(cls))
  const sessionQuality = mentor.classes.map((cls) => cls.manual.sessionQuality).filter((value) => value > 0)
  const feedback = mentor.classes.map((cls) => cls.manual.studentsFeedback).filter((value) => value > 0)
  const trello = mentor.classes.map((cls) => cls.manual.trelloCompliancePercent)
  const punctuality = mentor.classes.map((cls) => cls.automatic.attendancePunctualityPercent)
  const whatsapp = mentor.classes.map((cls) => cls.automatic.whatsAppManagementPercent)
  const recordedSessions = mentor.classes.reduce((acc, cls) => acc + cls.manual.recordedSessionCount, 0)

  return {
    collective: average(classScores),
    avgSessionQuality: average(sessionQuality),
    avgFeedback: average(feedback),
    avgTrello: average(trello),
    avgPunctuality: average(punctuality),
    avgWhatsapp: average(whatsapp),
    recordedSessions,
  }
}

export default function MentorEvaluations() {
  const [mentors, setMentors] = useState<MentorEvaluationItem[]>([])
  const [scope, setScope] = useState<'active' | 'closed'>('active')
  const [activeTab, setActiveTab] = useState<string>(TOTAL_TAB_ID)
  const [closedSearch, setClosedSearch] = useState('')
  const [closedFrom, setClosedFrom] = useState('')
  const [closedTo, setClosedTo] = useState('')
  const [closedAppliedFilters, setClosedAppliedFilters] = useState<{ q?: string; from?: string; to?: string } | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [editing, setEditing] = useState<EditTarget | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [testimonialTarget, setTestimonialTarget] = useState<TestimonialTarget | null>(null)
  const [savingTestimonial, setSavingTestimonial] = useState(false)
  const [testimonialError, setTestimonialError] = useState<string | null>(null)
  const [testimonialSuccess, setTestimonialSuccess] = useState<string | null>(null)
  const [reportTarget, setReportTarget] = useState<ReportTarget | null>(null)
  const [totalReportOpen, setTotalReportOpen] = useState(false)
  const [closedFilterError, setClosedFilterError] = useState<string | null>(null)

  useEffect(() => {
    void load()
  }, [scope, closedAppliedFilters])

  async function load() {
    try {
      if (scope === 'closed' && !closedAppliedFilters) {
        setMentors([])
        setError(null)
        setLoading(false)
        return
      }
      setLoading(true)
      setError(null)
      const data = await api.getMentorEvaluations(
        scope,
        scope === 'closed' ? closedAppliedFilters || undefined : undefined,
      )
      const nextMentors = data.mentors as MentorEvaluationItem[]
      setMentors(nextMentors)
      if (scope === 'active') {
        setActiveTab((current) => {
          if (current === TOTAL_TAB_ID) return current
          return nextMentors.some((mentor) => mentor.id === current) ? current : TOTAL_TAB_ID
        })
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load mentor evaluations')
    } finally {
      setLoading(false)
    }
  }

  function applyClosedFilters() {
    const q = closedSearch.trim()
    const from = closedFrom.trim()
    const to = closedTo.trim()
    if (!q && !from && !to) {
      setClosedFilterError('Enter at least one filter, then click Apply.')
      return
    }
    setClosedFilterError(null)
    setClosedAppliedFilters({
      q: q || undefined,
      from: from || undefined,
      to: to || undefined,
    })
  }

  async function saveClassManualEvaluation(payload: {
    mentorId: string
    classKey: string
    manual: { sessionQualityBySession: number[]; studentsFeedback: number; trelloSessionChecks: boolean[] }
  }) {
    try {
      setSaving(true)
      setSaveError(null)
      const res = await api.updateMentorEvaluation(payload.mentorId, {
        classKey: payload.classKey,
        manual: payload.manual,
      })

      setMentors((prev) =>
        prev.map((mentor) =>
          mentor.id !== payload.mentorId
            ? mentor
            : {
                ...mentor,
                classes: mentor.classes.map((cls) => {
                  if (cls.classKey !== payload.classKey) return cls
                  const updatedManual = {
                    ...cls.manual,
                    sessionQuality: res.manual.sessionQuality,
                    sessionQualityBySession: normalizeSessionQualityBySession(res.manual.sessionQualityBySession),
                    recordedSessionCount: res.manual.recordedSessionCount,
                    studentsFeedback: res.manual.studentsFeedback,
                    trelloSessionChecks: res.manual.trelloSessionChecks,
                    trelloCompliancePercent: res.manual.trelloCompliancePct,
                  }
                  const updatedClass: MentorClassEvaluation = {
                    ...cls,
                    manual: updatedManual,
                    classCollectiveScore: computePreviewCollective(cls, updatedManual),
                  }
                  return updatedClass
                }),
              },
        ),
      )
      setEditing(null)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save evaluation')
    } finally {
      setSaving(false)
    }
  }

  async function saveTestimonial(payload: { mentorId: string; classKey: string; testimonialText: string }) {
    try {
      setSavingTestimonial(true)
      setTestimonialError(null)
      setTestimonialSuccess(null)
      await api.createMentorTestimonial(payload.mentorId, {
        class_key: payload.classKey,
        testimonial_text: payload.testimonialText,
      })
      setTestimonialTarget(null)
      setTestimonialSuccess('Testimonial saved successfully.')
    } catch (err) {
      setTestimonialError(err instanceof Error ? err.message : 'Failed to save testimonial')
    } finally {
      setSavingTestimonial(false)
    }
  }

  const activeMentor = useMemo(
    () => mentors.find((mentor) => mentor.id === activeTab) || null,
    [mentors, activeTab],
  )

  if (loading) {
    return <div style={{ padding: '24px' }}>Loading mentor evaluations...</div>
  }

  if (error) {
    return (
      <div style={{ padding: '24px', color: '#721c24', background: '#f8d7da', borderRadius: '6px' }}>
        {error}
      </div>
    )
  }

  return (
    <div style={{ padding: '24px' }}>
      <h1 style={{ marginBottom: '16px', fontSize: '30px' }}>Mentor Evaluations</h1>
      <p style={{ marginTop: 0, marginBottom: '20px', color: '#555' }}>
        Active rounds are organized by mentor. Session Quality is recorded per session, while final round scores use only the sessions MH actually evaluated.
      </p>

      <div style={{ display: 'flex', gap: '8px', marginBottom: '16px' }}>
        <button
          onClick={() => {
            setScope('active')
            setClosedFilterError(null)
          }}
          style={{
            border: '1px solid #1c7ed6',
            borderRadius: '6px',
            background: scope === 'active' ? '#1c7ed6' : '#fff',
            color: scope === 'active' ? '#fff' : '#1c7ed6',
            padding: '6px 12px',
            cursor: 'pointer',
            fontWeight: 700,
          }}
        >
          Active Rounds
        </button>
        <button
          onClick={() => setScope('closed')}
          style={{
            border: '1px solid #495057',
            borderRadius: '6px',
            background: scope === 'closed' ? '#495057' : '#fff',
            color: scope === 'closed' ? '#fff' : '#495057',
            padding: '6px 12px',
            cursor: 'pointer',
            fontWeight: 700,
          }}
        >
          Closed Rounds
        </button>
      </div>

      {scope === 'closed' && (
        <ClosedRoundFilters
          closedSearch={closedSearch}
          closedFrom={closedFrom}
          closedTo={closedTo}
          closedFilterError={closedFilterError}
          closedAppliedFilters={closedAppliedFilters}
          onSearchChange={setClosedSearch}
          onFromChange={setClosedFrom}
          onToChange={setClosedTo}
          onApply={applyClosedFilters}
          onClear={() => {
            setClosedSearch('')
            setClosedFrom('')
            setClosedTo('')
            setClosedAppliedFilters(null)
            setClosedFilterError(null)
          }}
        />
      )}

      {testimonialSuccess && (
        <div style={{ marginBottom: '12px', padding: '10px 12px', borderRadius: '6px', background: '#d4edda', color: '#155724', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{testimonialSuccess}</span>
          <button onClick={() => setTestimonialSuccess(null)} style={{ background: 'none', border: 'none', fontSize: '18px', cursor: 'pointer', color: '#155724' }}>×</button>
        </div>
      )}

      {scope === 'active' ? (
        <>
          <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap', marginBottom: '16px', paddingBottom: '8px', borderBottom: '1px solid #e9ecef' }}>
            <TabChip
              active={activeTab === TOTAL_TAB_ID}
              label="Total Active Report"
              sub={`${mentors.length} mentors`}
              onClick={() => setActiveTab(TOTAL_TAB_ID)}
            />
            {mentors.map((mentor) => (
              <TabChip
                key={mentor.id}
                active={activeTab === mentor.id}
                label={mentor.name}
                sub={`${mentor.activeClassCount} class${mentor.activeClassCount === 1 ? '' : 'es'}`}
                onClick={() => setActiveTab(mentor.id)}
              />
            ))}
          </div>

          {mentors.length === 0 ? (
            <p style={{ color: '#666' }}>No mentors with active classes.</p>
          ) : activeTab === TOTAL_TAB_ID ? (
            <ActiveOverview
              mentors={mentors}
              onOpenTotalReport={() => setTotalReportOpen(true)}
              onOpenMentor={(mentorId) => setActiveTab(mentorId)}
            />
          ) : activeMentor ? (
            <MentorTabView
              mentor={activeMentor}
              onEditClass={(classItem) => setEditing({ mentorId: activeMentor.id, mentorName: activeMentor.name, classItem })}
              onOpenTestimonial={() => {
                setTestimonialError(null)
                setTestimonialTarget({ mentorId: activeMentor.id, mentorName: activeMentor.name, classes: activeMentor.classes })
              }}
              onViewReport={(classItem) => setReportTarget({ mentorName: activeMentor.name, mentorEmail: activeMentor.email, classItem })}
            />
          ) : (
            <p style={{ color: '#666' }}>Select a mentor tab.</p>
          )}
        </>
      ) : (
        <>
          {closedAppliedFilters && mentors.length > 0 && (
            <div style={{ marginBottom: '16px' }}>
              <button
                onClick={() => setTotalReportOpen(true)}
                style={{
                  border: '1px solid #0c8599',
                  borderRadius: '6px',
                  background: '#fff',
                  color: '#0c8599',
                  padding: '7px 12px',
                  cursor: 'pointer',
                  fontWeight: 700,
                }}
              >
                Total Closed Report
              </button>
            </div>
          )}

          {mentors.length === 0 ? (
            <p style={{ color: '#666' }}>No mentors found for the selected filters.</p>
          ) : (
            <div style={{ display: 'grid', gap: '14px' }}>
              {mentors.map((mentor) => (
                <MentorClosedCard
                  key={mentor.id}
                  mentor={mentor}
                  onOpenTestimonial={() => {
                    setTestimonialError(null)
                    setTestimonialTarget({ mentorId: mentor.id, mentorName: mentor.name, classes: mentor.classes })
                  }}
                  onViewReport={(classItem) => setReportTarget({ mentorName: mentor.name, mentorEmail: mentor.email, classItem })}
                />
              ))}
            </div>
          )}
        </>
      )}

      {editing && (
        <EditClassEvaluationModal
          mentorName={editing.mentorName}
          classItem={editing.classItem}
          saving={saving}
          error={saveError}
          onClose={() => {
            if (!saving) {
              setEditing(null)
              setSaveError(null)
            }
          }}
          onSave={(manual) =>
            saveClassManualEvaluation({
              mentorId: editing.mentorId,
              classKey: editing.classItem.classKey,
              manual,
            })
          }
        />
      )}

      {testimonialTarget && (
        <AddTestimonialModal
          mentorName={testimonialTarget.mentorName}
          classes={testimonialTarget.classes}
          saving={savingTestimonial}
          error={testimonialError}
          onClose={() => {
            if (!savingTestimonial) {
              setTestimonialTarget(null)
              setTestimonialError(null)
            }
          }}
          onSave={(payload) => saveTestimonial({ mentorId: testimonialTarget.mentorId, classKey: payload.classKey, testimonialText: payload.testimonialText })}
        />
      )}

      {reportTarget && (
        <div
          className="report-overlay"
          style={{
            position: 'fixed',
            inset: 0,
            zIndex: 5000,
            background: 'rgba(0,0,0,0.5)',
            overflow: 'auto',
          }}
          onClick={(e) => {
            if (e.target === e.currentTarget) {
              setReportTarget(null)
            }
          }}
        >
          <MentorRoundReport
            report={{
              mentorName: reportTarget.mentorName,
              mentorEmail: reportTarget.mentorEmail,
              classKey: reportTarget.classItem.classKey,
              level: reportTarget.classItem.level,
              days: reportTarget.classItem.days,
              time: reportTarget.classItem.time,
              classNumber: reportTarget.classItem.classNumber,
              roundStatus: reportTarget.classItem.roundStatus,
              generatedAt: new Date().toISOString(),
              collectiveKpi: computeCollectiveKPI(reportTarget.classItem),
              manual: reportTarget.classItem.manual,
              automatic: reportTarget.classItem.automatic,
            }}
            onClose={() => setReportTarget(null)}
          />
        </div>
      )}

      {totalReportOpen && (
        <div
          className="report-overlay"
          style={{
            position: 'fixed',
            inset: 0,
            zIndex: 5000,
            background: 'rgba(0,0,0,0.5)',
            overflow: 'auto',
          }}
          onClick={(e) => {
            if (e.target === e.currentTarget) {
              setTotalReportOpen(false)
            }
          }}
        >
          <MentorActiveTotalReport
            mentors={mentors}
            scopeLabel={scope === 'closed' ? 'Closed' : 'Active'}
            filterSummary={
              scope === 'closed'
                ? [
                    closedAppliedFilters?.q ? `Mentor=${closedAppliedFilters.q}` : '',
                    closedAppliedFilters?.from ? `From=${closedAppliedFilters.from}` : '',
                    closedAppliedFilters?.to ? `To=${closedAppliedFilters.to}` : '',
                  ].filter(Boolean).join(' | ')
                : ''
            }
            onClose={() => setTotalReportOpen(false)}
          />
        </div>
      )}
    </div>
  )
}

function ClosedRoundFilters({
  closedSearch,
  closedFrom,
  closedTo,
  closedFilterError,
  closedAppliedFilters,
  onSearchChange,
  onFromChange,
  onToChange,
  onApply,
  onClear,
}: {
  closedSearch: string
  closedFrom: string
  closedTo: string
  closedFilterError: string | null
  closedAppliedFilters: { q?: string; from?: string; to?: string } | null
  onSearchChange: (value: string) => void
  onFromChange: (value: string) => void
  onToChange: (value: string) => void
  onApply: () => void
  onClear: () => void
}) {
  return (
    <div style={{ marginBottom: '16px', maxWidth: '860px' }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', gap: '8px', alignItems: 'end' }}>
        <div>
          <label style={{ display: 'block', fontSize: '12px', color: '#555', marginBottom: '4px' }}>Mentor</label>
          <input
            type="text"
            placeholder="Search by mentor name/email/phone"
            value={closedSearch}
            onChange={(e) => onSearchChange(e.target.value)}
            style={{ width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid #ced4da' }}
          />
        </div>
        <div>
          <label style={{ display: 'block', fontSize: '12px', color: '#555', marginBottom: '4px' }}>From</label>
          <input
            type="date"
            value={closedFrom}
            onChange={(e) => onFromChange(e.target.value)}
            style={{ width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid #ced4da' }}
          />
        </div>
        <div>
          <label style={{ display: 'block', fontSize: '12px', color: '#555', marginBottom: '4px' }}>To</label>
          <input
            type="date"
            value={closedTo}
            onChange={(e) => onToChange(e.target.value)}
            style={{ width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid #ced4da' }}
          />
        </div>
        <button onClick={onApply} style={primaryButtonStyle}>Apply</button>
        <button onClick={onClear} style={secondaryButtonStyle}>Clear</button>
      </div>
      {closedFilterError && (
        <div style={{ marginTop: '6px', color: '#b02a37', fontSize: '12px' }}>{closedFilterError}</div>
      )}
      {!closedAppliedFilters && !closedFilterError && (
        <div style={{ marginTop: '6px', color: '#555', fontSize: '12px' }}>
          No data is shown until you apply at least one filter.
        </div>
      )}
    </div>
  )
}

function TabChip({ active, label, sub, onClick }: { active: boolean; label: string; sub: string; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      style={{
        border: active ? '1px solid #0d6efd' : '1px solid #d0d7de',
        background: active ? '#e7f1ff' : '#fff',
        color: '#0f172a',
        borderRadius: '999px',
        padding: '10px 14px',
        minWidth: '160px',
        textAlign: 'left',
        cursor: 'pointer',
      }}
    >
      <div style={{ fontWeight: 800, fontSize: '14px', color: active ? '#0d6efd' : '#111827' }}>{label}</div>
      <div style={{ marginTop: '2px', fontSize: '12px', color: '#6b7280' }}>{sub}</div>
    </button>
  )
}

function ActiveOverview({
  mentors,
  onOpenTotalReport,
  onOpenMentor,
}: {
  mentors: MentorEvaluationItem[]
  onOpenTotalReport: () => void
  onOpenMentor: (mentorId: string) => void
}) {
  const allClasses = mentors.flatMap((mentor) => mentor.classes)
  const overallCollective = average(allClasses.map((cls) => computeCollectiveKPI(cls)))
  const overallSessionQuality = average(allClasses.map((cls) => cls.manual.sessionQuality).filter((value) => value > 0))
  const overallFeedback = average(allClasses.map((cls) => cls.manual.studentsFeedback).filter((value) => value > 0))
  const totalRecordedSessions = allClasses.reduce((acc, cls) => acc + cls.manual.recordedSessionCount, 0)

  return (
    <div style={{ display: 'grid', gap: '14px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
        <div>
          <h2 style={{ margin: 0, fontSize: '24px' }}>Total Active Report</h2>
          <div style={{ marginTop: '4px', color: '#6b7280' }}>Overview across all active mentors and classes.</div>
        </div>
        <button onClick={onOpenTotalReport} style={secondaryButtonStyle}>Print / Export</button>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '12px' }}>
        <SummaryMetric title="Active Mentors" value={String(mentors.length)} />
        <SummaryMetric title="Active Classes" value={String(allClasses.length)} />
        <SummaryMetric title="Collective KPI" value={`${overallCollective}%`} />
        <SummaryMetric title="Avg Session Quality" value={overallSessionQuality > 0 ? `${overallSessionQuality}/10` : '-'} />
        <SummaryMetric title="Avg Students Feedback" value={overallFeedback > 0 ? `${overallFeedback}/10` : '-'} />
        <SummaryMetric title="Recorded Sessions" value={`${totalRecordedSessions}`} sub="MH session-quality entries" />
      </div>

      <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: '12px', overflow: 'hidden' }}>
        <div style={{ padding: '14px 16px', borderBottom: '1px solid #e5e7eb', fontWeight: 800 }}>Active Mentors Summary</div>
        <div style={{ display: 'grid', gap: '1px', background: '#e5e7eb' }}>
          {mentors.map((mentor) => {
            const summary = mentorSummary(mentor)
            return (
              <button
                key={mentor.id}
                onClick={() => onOpenMentor(mentor.id)}
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'minmax(0, 1.4fr) repeat(4, minmax(90px, 1fr))',
                  gap: '10px',
                  alignItems: 'center',
                  textAlign: 'left',
                  background: '#fff',
                  border: 'none',
                  padding: '14px 16px',
                  cursor: 'pointer',
                }}
              >
                <div>
                  <div style={{ fontWeight: 800, color: '#111827' }}>{mentor.name}</div>
                  <div style={{ marginTop: '2px', color: '#6b7280', fontSize: '13px' }}>{mentor.email}</div>
                </div>
                <MetricCell label="Classes" value={String(mentor.classes.length)} />
                <MetricCell label="KPI" value={`${summary.collective}%`} />
                <MetricCell label="Quality" value={summary.avgSessionQuality > 0 ? `${summary.avgSessionQuality}/10` : '-'} />
                <MetricCell label="Recorded" value={`${summary.recordedSessions}`} />
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}

function MentorTabView({
  mentor,
  onEditClass,
  onOpenTestimonial,
  onViewReport,
}: {
  mentor: MentorEvaluationItem
  onEditClass: (classItem: MentorClassEvaluation) => void
  onOpenTestimonial: () => void
  onViewReport: (classItem: MentorClassEvaluation) => void
}) {
  const summary = mentorSummary(mentor)

  return (
    <div style={{ display: 'grid', gap: '14px' }}>
      <div style={{ background: 'linear-gradient(135deg, #f8fbff 0%, #eef6ff 100%)', border: '1px solid #dbeafe', borderRadius: '14px', padding: '18px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'flex-start', flexWrap: 'wrap' }}>
          <div>
            <h2 style={{ margin: 0, fontSize: '26px', color: '#0f172a' }}>{mentor.name}</h2>
            <div style={{ marginTop: '4px', color: '#475569' }}>{mentor.email}</div>
          </div>
          <button onClick={onOpenTestimonial} style={secondaryButtonStyle}>Testimonials</button>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '12px', marginTop: '16px' }}>
          <SummaryMetric title="Active Classes" value={String(mentor.classes.length)} />
          <SummaryMetric title="Collective KPI" value={`${summary.collective}%`} />
          <SummaryMetric title="Avg Session Quality" value={summary.avgSessionQuality > 0 ? `${summary.avgSessionQuality}/10` : '-'} />
          <SummaryMetric title="Avg Students Feedback" value={summary.avgFeedback > 0 ? `${summary.avgFeedback}/10` : '-'} />
          <SummaryMetric title="Avg Trello" value={`${summary.avgTrello}%`} />
          <SummaryMetric title="Recorded Sessions" value={String(summary.recordedSessions)} sub="Only MH-entered quality sessions" />
        </div>
      </div>

      <div style={{ display: 'grid', gap: '12px' }}>
        {mentor.classes.map((cls) => (
          <ClassCard
            key={`${mentor.id}-${cls.classKey}`}
            classItem={cls}
            canEdit={cls.roundStatus === 'active'}
            onEdit={() => onEditClass(cls)}
            onViewReport={() => onViewReport(cls)}
          />
        ))}
      </div>
    </div>
  )
}

function MentorClosedCard({
  mentor,
  onOpenTestimonial,
  onViewReport,
}: {
  mentor: MentorEvaluationItem
  onOpenTestimonial: () => void
  onViewReport: (classItem: MentorClassEvaluation) => void
}) {
  const summary = mentorSummary(mentor)

  return (
    <div style={{ border: '1px solid #dee2e6', borderRadius: '12px', background: '#fff', padding: '16px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '10px', alignItems: 'flex-start', flexWrap: 'wrap', marginBottom: '12px' }}>
        <div>
          <div style={{ fontWeight: 800 }}>{mentor.name}</div>
          <div style={{ color: '#666', fontSize: '13px' }}>{mentor.email}</div>
        </div>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' }}>
          <span style={badgeInfoStyle}>Collective KPI {summary.collective}%</span>
          <span style={badgeSuccessStyle}>{mentor.classes.length} closed class{mentor.classes.length === 1 ? '' : 'es'}</span>
          <button onClick={onOpenTestimonial} style={secondaryButtonStyle}>Testimonials</button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: '12px' }}>
        {mentor.classes.map((cls) => (
          <ClassCard
            key={`${mentor.id}-${cls.classKey}`}
            classItem={cls}
            canEdit={false}
            onEdit={() => {}}
            onViewReport={() => onViewReport(cls)}
          />
        ))}
      </div>
    </div>
  )
}

function SummaryMetric({ title, value, sub }: { title: string; value: string; sub?: string }) {
  return (
    <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: '12px', padding: '14px' }}>
      <div style={{ fontSize: '13px', color: '#6b7280', fontWeight: 700 }}>{title}</div>
      <div style={{ marginTop: '6px', fontSize: '30px', lineHeight: 1, fontWeight: 900, color: '#111827' }}>{value}</div>
      {sub && <div style={{ marginTop: '6px', color: '#6b7280', fontSize: '12px' }}>{sub}</div>}
    </div>
  )
}

function MetricCell({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div style={{ fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.04em', color: '#6b7280', fontWeight: 700 }}>{label}</div>
      <div style={{ marginTop: '4px', fontSize: '18px', fontWeight: 800, color: '#111827' }}>{value}</div>
    </div>
  )
}

function ScoreBar({ label, score, max = 10 }: { label: string; score: number; max?: number }) {
  const percent = Math.max(0, Math.min(100, (score / max) * 100))
  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px' }}>
        <span style={{ fontSize: '14px' }}>{label}</span>
        <strong style={{ fontSize: '14px' }}>
          {score}/{max}
        </strong>
      </div>
      <div style={{ height: '8px', background: '#eceff1', borderRadius: '4px', overflow: 'hidden' }}>
        <div style={{ width: `${percent}%`, height: '100%', background: '#1c7ed6' }} />
      </div>
    </div>
  )
}

function PercentBar({ label, percent }: { label: string; percent: number }) {
  const clamped = Math.max(0, Math.min(100, percent))
  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px' }}>
        <span style={{ fontSize: '14px' }}>{label}</span>
        <strong style={{ fontSize: '14px' }}>{clamped}%</strong>
      </div>
      <div style={{ height: '8px', background: '#eceff1', borderRadius: '4px', overflow: 'hidden' }}>
        <div
          style={{
            width: `${clamped}%`,
            height: '100%',
            background: clamped >= 80 ? '#2f9e44' : clamped >= 60 ? '#f59f00' : '#e03131',
          }}
        />
      </div>
    </div>
  )
}

function SessionQualityStrip({ values }: { values: number[] }) {
  return (
    <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
      {normalizeSessionQualityBySession(values).map((value, index) => {
        const active = value > 0
        return (
          <div
            key={index}
            title={active ? `S${index + 1}: ${value}/10` : `S${index + 1}: not recorded`}
            style={{
              width: '38px',
              height: '38px',
              borderRadius: '10px',
              border: `1px solid ${active ? '#bfdbfe' : '#e5e7eb'}`,
              background: active ? '#eff6ff' : '#f8fafc',
              color: active ? '#1d4ed8' : '#94a3b8',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              fontWeight: 800,
            }}
          >
            <div style={{ fontSize: '10px' }}>S{index + 1}</div>
            <div style={{ fontSize: '12px' }}>{active ? value : '-'}</div>
          </div>
        )
      })}
    </div>
  )
}

function ClassCard({
  classItem,
  canEdit,
  onEdit,
  onViewReport,
}: {
  classItem: MentorClassEvaluation
  canEdit: boolean
  onEdit: () => void
  onViewReport: () => void
}) {
  const collective = computeCollectiveKPI(classItem)
  return (
    <div style={{ border: '1px solid #e5e7eb', borderRadius: '12px', padding: '14px', background: '#fff' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '10px', marginBottom: '12px' }}>
        <div style={{ flex: '1 1 240px' }}>
          <strong>
            Level {classItem.level} • {classItem.days} • {classItem.time} • Class #{classItem.classNumber}
          </strong>
          <div style={{ color: '#666', fontSize: '12px', marginTop: '4px' }}>{classItem.classKey}</div>
        </div>
        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          <button onClick={onViewReport} style={secondaryButtonStyle}>View Report</button>
          {canEdit && (
            <button onClick={onEdit} style={primaryButtonStyle}>Edit Class</button>
          )}
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '12px' }}>
        <PercentBar label="Collective KPI Ratio (Weighted)" percent={collective} />
        <ScoreBar label="Students Feedback (Manual)" score={classItem.manual.studentsFeedback} />
        <PercentBar label="Trello Compliance (Manual)" percent={classItem.manual.trelloCompliancePercent} />
        <PercentBar label="WhatsApp Groups Management (Auto)" percent={classItem.automatic.whatsAppManagementPercent} />
        <PercentBar label="Attendance Punctuality (Auto)" percent={classItem.automatic.attendancePunctualityPercent} />
      </div>

      <div style={{ marginTop: '14px', padding: '12px', borderRadius: '12px', background: '#f8fafc', border: '1px solid #e5e7eb' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: '8px', alignItems: 'center', flexWrap: 'wrap', marginBottom: '10px' }}>
          <div style={{ fontWeight: 800, color: '#111827' }}>Session Quality by Session</div>
          <div style={{ fontSize: '13px', color: '#475569' }}>
            Average {classItem.manual.sessionQuality > 0 ? `${classItem.manual.sessionQuality}/10` : '-'} · {classItem.manual.recordedSessionCount}/8 recorded
          </div>
        </div>
        <SessionQualityStrip values={classItem.manual.sessionQualityBySession} />
      </div>

      <div style={{ marginTop: '12px' }}>
        <div style={{ marginBottom: '6px', fontSize: '13px', color: '#555' }}>Attendance by Session (Auto)</div>
        <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
          {classItem.automatic.attendanceStatuses.map((status, idx) => (
            <div
              key={`${classItem.classKey}-${idx}`}
              title={`Session ${idx + 1}: ${status}`}
              style={{
                width: '30px',
                height: '30px',
                borderRadius: '50%',
                background: statusColor(status),
                color: '#fff',
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                fontSize: '11px',
                fontWeight: 700,
              }}
            >
              {idx + 1}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function computePreviewCollective(
  classItem: MentorClassEvaluation,
  manual: {
    sessionQuality: number
    studentsFeedback: number
    trelloCompliancePercent: number
  },
): number {
  const punctuality = classItem.automatic.attendancePunctualityPercent
  const sessionQuality = manual.sessionQuality * 10
  const feedback = manual.studentsFeedback * 10
  const whatsapp = classItem.automatic.whatsAppManagementPercent
  const trello = manual.trelloCompliancePercent
  return Math.round(
    punctuality * 0.25 +
      sessionQuality * 0.25 +
      feedback * 0.20 +
      whatsapp * 0.10 +
      trello * 0.20,
  )
}

function EditClassEvaluationModal({
  mentorName,
  classItem,
  saving,
  error,
  onClose,
  onSave,
}: {
  mentorName: string
  classItem: MentorClassEvaluation
  saving: boolean
  error: string | null
  onClose: () => void
  onSave: (manual: { sessionQualityBySession: number[]; studentsFeedback: number; trelloSessionChecks: boolean[] }) => void
}) {
  const [sessionQualityBySession, setSessionQualityBySession] = useState<number[]>(() =>
    normalizeSessionQualityBySession(classItem.manual.sessionQualityBySession),
  )
  const [studentsFeedback, setStudentsFeedback] = useState(Math.max(1, classItem.manual.studentsFeedback || 1))
  const [trelloChecks, setTrelloChecks] = useState<boolean[]>(() => {
    const fixed = new Array(8).fill(false) as boolean[]
    const src = classItem.manual.trelloSessionChecks || []
    for (let i = 0; i < Math.min(8, src.length); i++) fixed[i] = !!src[i]
    return fixed
  })

  const avgSessionQuality = useMemo(
    () => averageRecordedSessionQuality(sessionQualityBySession),
    [sessionQualityBySession],
  )
  const recordedSessionCount = useMemo(
    () => sessionQualityBySession.filter((value) => value > 0).length,
    [sessionQualityBySession],
  )
  const trelloPercent = useMemo(
    () => Math.round((trelloChecks.filter(Boolean).length / 8) * 100),
    [trelloChecks],
  )
  const collectivePreview = useMemo(
    () => computePreviewCollective(classItem, {
      sessionQuality: avgSessionQuality,
      studentsFeedback,
      trelloCompliancePercent: trelloPercent,
    }),
    [classItem, avgSessionQuality, studentsFeedback, trelloPercent],
  )

  function updateSessionQuality(index: number, value: number) {
    setSessionQualityBySession((prev) => prev.map((item, idx) => (idx === index ? value : item)))
  }

  function toggleSession(index: number) {
    setTrelloChecks((prev) => prev.map((v, i) => (i === index ? !v : v)))
  }

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(0,0,0,0.45)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
        padding: '16px',
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{ width: '96%', maxWidth: '920px', maxHeight: '92vh', overflow: 'auto', background: '#fff', borderRadius: '12px', padding: '20px' }}
      >
        <div style={{ marginBottom: '14px' }}>
          <h3 style={{ margin: 0 }}>Edit Class Evaluation</h3>
          <div style={{ color: '#666', fontSize: '13px', marginTop: '4px' }}>
            {mentorName} • Level {classItem.level} • {classItem.days} • {classItem.time} • Class #{classItem.classNumber}
          </div>
        </div>

        {error && <div style={{ color: '#721c24', background: '#f8d7da', padding: '8px', borderRadius: '4px' }}>{error}</div>}

        <div style={{ marginTop: '12px', display: 'grid', gap: '18px' }}>
          <PercentBar label="Collective KPI Ratio (Preview)" percent={collectivePreview} />

          <div style={{ padding: '14px', borderRadius: '12px', background: '#f8fafc', border: '1px solid #e5e7eb' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: '8px', alignItems: 'center', flexWrap: 'wrap', marginBottom: '12px' }}>
              <div>
                <div style={{ fontWeight: 800 }}>Session Quality by Session</div>
                <div style={{ fontSize: '13px', color: '#6b7280' }}>Leave any session at `0` if MH did not evaluate it yet.</div>
              </div>
              <div style={{ fontSize: '13px', color: '#475569' }}>
                Average {avgSessionQuality > 0 ? `${avgSessionQuality}/10` : '-'} · {recordedSessionCount}/8 recorded
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '12px' }}>
              {sessionQualityBySession.map((score, index) => (
                <div key={index} style={{ padding: '12px', borderRadius: '10px', background: '#fff', border: '1px solid #e5e7eb' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: '8px', marginBottom: '8px' }}>
                    <strong>S{index + 1}</strong>
                    <span style={{ color: score > 0 ? '#0d6efd' : '#6b7280', fontWeight: 700 }}>{score > 0 ? `${score}/10` : 'Not recorded'}</span>
                  </div>
                  <input
                    type="range"
                    min={0}
                    max={10}
                    value={score}
                    onChange={(e) => updateSessionQuality(index, parseInt(e.target.value, 10))}
                    style={{ width: '100%' }}
                  />
                </div>
              ))}
            </div>
          </div>

          <div>
            <label style={{ display: 'block', marginBottom: '6px' }}>Students Feedback (1-10): {studentsFeedback}</label>
            <input type="range" min={1} max={10} value={studentsFeedback} onChange={(e) => setStudentsFeedback(parseInt(e.target.value, 10))} style={{ width: '100%' }} />
          </div>

          <div>
            <div style={{ marginBottom: '8px', fontWeight: 600 }}>Trello Compliance by Session (Manual)</div>
            <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
              {trelloChecks.map((checked, idx) => (
                <label
                  key={idx}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                    background: '#f1f3f5',
                    borderRadius: '4px',
                    padding: '6px 8px',
                  }}
                >
                  <input type="checkbox" checked={checked} onChange={() => toggleSession(idx)} />
                  S{idx + 1}
                </label>
              ))}
            </div>
            <div style={{ marginTop: '6px', fontSize: '13px', color: '#555' }}>Total: {trelloPercent}%</div>
          </div>
        </div>

        <div style={{ marginTop: '16px', display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
          <button onClick={onClose} disabled={saving} style={secondaryButtonStyle}>
            Cancel
          </button>
          <button
            disabled={saving}
            onClick={() => onSave({ sessionQualityBySession, studentsFeedback, trelloSessionChecks: trelloChecks })}
            style={primaryButtonStyle}
          >
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  )
}

function AddTestimonialModal({
  mentorName,
  classes,
  saving,
  error,
  onClose,
  onSave,
}: {
  mentorName: string
  classes: MentorClassEvaluation[]
  saving: boolean
  error: string | null
  onClose: () => void
  onSave: (payload: { classKey: string; testimonialText: string }) => void
}) {
  const [classKey, setClassKey] = useState(classes[0]?.classKey || '')
  const [testimonialText, setTestimonialText] = useState('')

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        background: 'rgba(0,0,0,0.45)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{ width: '92%', maxWidth: '720px', background: '#fff', borderRadius: '8px', padding: '16px' }}
      >
        <h3 style={{ marginTop: 0, marginBottom: '6px' }}>Add Testimonial</h3>
        <div style={{ color: '#666', fontSize: '13px', marginBottom: '12px' }}>{mentorName}</div>

        {error && <div style={{ color: '#721c24', background: '#f8d7da', padding: '8px', borderRadius: '4px', marginBottom: '10px' }}>{error}</div>}

        <div style={{ marginBottom: '10px' }}>
          <label style={{ display: 'block', marginBottom: '6px' }}>Source Class</label>
          <select value={classKey} onChange={(e) => setClassKey(e.target.value)} style={{ width: '100%', padding: '8px', border: '1px solid #ced4da', borderRadius: '4px' }}>
            {classes.map((cls) => (
              <option key={cls.classKey} value={cls.classKey}>
                {`Level ${cls.level} ${cls.days} ${cls.time} (${cls.classKey})`}
              </option>
            ))}
          </select>
        </div>

        <div style={{ marginBottom: '10px' }}>
          <label style={{ display: 'block', marginBottom: '6px' }}>Testimonial Text</label>
          <textarea
            value={testimonialText}
            onChange={(e) => setTestimonialText(e.target.value)}
            rows={6}
            placeholder="Paste selected feedback from Feedback Collected..."
            style={{ width: '100%', padding: '10px', border: '1px solid #ced4da', borderRadius: '4px' }}
          />
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
          <button onClick={onClose} disabled={saving} style={secondaryButtonStyle}>
            Cancel
          </button>
          <button
            disabled={saving || !classKey || !testimonialText.trim()}
            onClick={() => onSave({ classKey, testimonialText })}
            style={primaryButtonStyle}
          >
            {saving ? 'Saving...' : 'Save Testimonial'}
          </button>
        </div>
      </div>
    </div>
  )
}

const primaryButtonStyle = {
  border: 'none',
  borderRadius: '6px',
  background: '#1c7ed6',
  color: '#fff',
  padding: '8px 12px',
  cursor: 'pointer',
  fontWeight: 700,
} as const

const secondaryButtonStyle = {
  border: '1px solid #cbd5e1',
  borderRadius: '6px',
  background: '#fff',
  color: '#0f172a',
  padding: '8px 12px',
  cursor: 'pointer',
  fontWeight: 700,
} as const

const badgeInfoStyle = {
  padding: '4px 8px',
  borderRadius: '999px',
  background: '#e7f5ff',
  color: '#1864ab',
  fontSize: '12px',
  fontWeight: 700,
} as const

const badgeSuccessStyle = {
  padding: '4px 8px',
  borderRadius: '999px',
  background: '#d4edda',
  color: '#155724',
  fontSize: '12px',
  fontWeight: 700,
} as const
