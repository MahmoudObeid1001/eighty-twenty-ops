import { useEffect } from 'react'

type AttendanceStatus = 'on-time' | 'late' | 'absent' | 'unknown'

interface MentorClassEvaluation {
  classKey: string
  level: number
  days: string
  time: string
  classNumber: number
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

interface MentorActiveTotalReportProps {
  mentors: MentorEvaluationItem[]
  scopeLabel?: string
  filterSummary?: string
  onClose: () => void
}

function escapeHtml(value: string | number): string {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function computeCollectiveKPI(classItem: MentorClassEvaluation): number {
  return classItem.classCollectiveScore
}

function colorForPercent(percent: number): string {
  if (percent >= 80) return '#2f9e44'
  if (percent >= 60) return '#f59f00'
  return '#e03131'
}

export default function MentorActiveTotalReport({ mentors, scopeLabel = 'Active', filterSummary = '', onClose }: MentorActiveTotalReportProps) {
  const generatedAt = new Date().toLocaleDateString()
  const mentorsWithActiveClasses = mentors.filter((m) => m.classes.length > 0)
  const totalActiveClasses = mentorsWithActiveClasses.reduce((acc, mentor) => acc + mentor.classes.length, 0)
  const totalRecordedSessions = mentorsWithActiveClasses.reduce(
    (acc, mentor) => acc + mentor.classes.reduce((classAcc, cls) => classAcc + cls.manual.recordedSessionCount, 0),
    0,
  )

  useEffect(() => {
    window.scrollTo({ top: 0, left: 0 })
  }, [])

  function handlePrint() {
    const logoSrc = `${window.location.origin}/static/logo/eighty-twenty-logo.png`

    const mentorSections = mentorsWithActiveClasses.map((mentor) => {
      const classScores = mentor.classes.map(computeCollectiveKPI)
      const mentorCollective = classScores.length > 0
        ? Math.round(classScores.reduce((acc, score) => acc + score, 0) / classScores.length)
        : 0

      const classRows = mentor.classes.map((cls) => {
        const kpi = computeCollectiveKPI(cls)
        return `
          <tr>
            <td>${escapeHtml(cls.classKey)}</td>
            <td>L${cls.level} • ${escapeHtml(cls.days)} • ${escapeHtml(cls.time)} • #${cls.classNumber}</td>
            <td>${kpi}%</td>
            <td>${cls.manual.sessionQuality > 0 ? `${cls.manual.sessionQuality}/10` : '-'}</td>
            <td>${cls.manual.recordedSessionCount}/8</td>
            <td>${cls.manual.studentsFeedback}/10</td>
            <td>${cls.manual.trelloCompliancePercent}%</td>
            <td>${cls.automatic.whatsAppManagementPercent}%</td>
            <td>${cls.automatic.attendancePunctualityPercent}%</td>
          </tr>
        `
      }).join('')

      return `
        <section class="mentor-block">
          <div class="mentor-head">
            <div>
              <div class="mentor-name">${escapeHtml(mentor.name)}</div>
              <div class="mentor-email">${escapeHtml(mentor.email)}</div>
            </div>
            <div class="mentor-stats">
              <span class="badge">${mentor.classes.length} active ${mentor.classes.length === 1 ? 'class' : 'classes'}</span>
              <span class="badge kpi">Collective KPI ${mentorCollective}%</span>
            </div>
          </div>
          <table>
            <thead>
              <tr>
                <th>Class Key</th>
                <th>Class</th>
                <th>Collective KPI</th>
                <th>Quality Avg</th>
                <th>Recorded</th>
                <th>Feedback</th>
                <th>Trello</th>
                <th>WhatsApp</th>
                <th>Punctuality</th>
              </tr>
            </thead>
            <tbody>
              ${classRows}
            </tbody>
          </table>
        </section>
      `
    }).join('')

    const html = `<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Mentor Total ${escapeHtml(scopeLabel)} Report</title>
  <style>
    @page { size: A4; margin: 10mm; }
    html, body { margin: 0; padding: 0; font-family: Arial, sans-serif; color: #0f172a; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    .header { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #dbe3ef; padding-bottom: 8px; margin-bottom: 12px; }
    .logo { width: 70px; height: auto; }
    .overview { background: #eef6ff; border: 1px solid #bfdbfe; border-radius: 8px; padding: 10px; margin-bottom: 12px; }
    .mentor-block { border: 1px solid #dbe3ef; border-radius: 8px; padding: 10px; margin-bottom: 10px; page-break-inside: avoid; }
    .mentor-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; gap: 10px; }
    .mentor-name { font-size: 18px; font-weight: 700; }
    .mentor-email { color: #475569; font-size: 12px; margin-top: 2px; }
    .mentor-stats { display: flex; gap: 6px; align-items: center; }
    .badge { display: inline-block; border-radius: 999px; padding: 4px 8px; font-size: 11px; font-weight: 700; background: #d4edda; color: #155724; }
    .badge.kpi { background: #e7f5ff; color: #1864ab; }
    table { width: 100%; border-collapse: collapse; font-size: 11px; table-layout: fixed; }
    th, td { border: 1px solid #cbd5e1; padding: 5px; text-align: center; }
    th { background: #f8fafc; }
  </style>
</head>
<body>
  <div class="header">
    <img class="logo" src="${logoSrc}" alt="Eighty Twenty" />
    <div>
      <div><strong>Mentor Total ${escapeHtml(scopeLabel)} Report</strong></div>
      <div><strong>Date:</strong> ${escapeHtml(generatedAt)}</div>
    </div>
  </div>
  <div class="overview">
    <div><strong>Mentors with ${escapeHtml(scopeLabel.toLowerCase())} classes:</strong> ${mentorsWithActiveClasses.length}</div>
    <div><strong>Total ${escapeHtml(scopeLabel.toLowerCase())} classes:</strong> ${totalActiveClasses}</div>
    <div><strong>Total recorded session-quality entries:</strong> ${totalRecordedSessions}</div>
    ${filterSummary ? `<div><strong>Filters:</strong> ${escapeHtml(filterSummary)}</div>` : ''}
  </div>
  ${mentorSections}
</body>
</html>`

    const frame = document.createElement('iframe')
    frame.setAttribute('aria-hidden', 'true')
    frame.style.position = 'fixed'
    frame.style.right = '0'
    frame.style.bottom = '0'
    frame.style.width = '0'
    frame.style.height = '0'
    frame.style.border = '0'
    frame.style.opacity = '0'
    document.body.appendChild(frame)

    frame.onload = () => {
      const printWindow = frame.contentWindow
      if (!printWindow) {
        frame.remove()
        return
      }
      printWindow.focus()
      const cleanup = () => setTimeout(() => frame.remove(), 200)
      printWindow.onafterprint = cleanup
      setTimeout(() => {
        printWindow.print()
        cleanup()
      }, 350)
    }
    frame.srcdoc = html
  }

  return (
    <div className="mentor-total-report-root">
      <style>{`
        .mentor-total-report-root { background: #f3f5f7; min-height: 100vh; min-width: 0; width: 100%; padding: 16px; }
        .mentor-total-report-shell { max-width: 1080px; min-width: 0; margin: 0 auto; }
        .mentor-total-report-toolbar { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; margin-bottom: 12px; }
        .mentor-total-report-btn { min-height: 44px; border: 1px solid #cbd5e1; background: white; border-radius: 6px; padding: 8px 12px; cursor: pointer; font-weight: 600; }
        .mentor-total-report-page { min-width: 0; background: white; border: 1px solid #e2e8f0; border-radius: 8px; padding: 20px; }
        .mentor-total-report-header { display: flex; justify-content: space-between; align-items: center; gap: 20px; border-bottom: 1px solid #dbe3ef; padding-bottom: 10px; margin-bottom: 12px; }
        .mentor-total-report-header-copy { min-width: 0; overflow-wrap: anywhere; }
        .mentor-total-report-mentor { min-width: 0; border: 1px solid #dbe3ef; border-radius: 8px; padding: 10px; }
        .mentor-total-report-mentor-head { display: flex; justify-content: space-between; align-items: center; gap: 10px; margin-bottom: 8px; }
        .mentor-total-report-mentor-identity { min-width: 0; overflow-wrap: anywhere; }
        .mentor-total-report-badges { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 6px; align-items: center; }
        .mentor-total-report-table-wrap { max-width: 100%; overflow-x: auto; -webkit-overflow-scrolling: touch; }
        .mentor-total-report-class-list { display: none; }
        @media (max-width: 640px) {
          .mentor-total-report-root { min-height: 0; padding: 0; background: transparent; }
          .mentor-total-report-toolbar { display: grid; grid-template-columns: 1fr 1fr; }
          .mentor-total-report-btn { width: 100%; padding-inline: 10px; }
          .mentor-total-report-page { padding: 14px; }
          .mentor-total-report-header { align-items: flex-start; gap: 12px; }
          .mentor-total-report-header img { width: 54px !important; flex: 0 0 auto; }
          .mentor-total-report-header-copy { font-size: 14px; line-height: 1.5; }
          .mentor-total-report-mentor-head { align-items: flex-start; flex-direction: column; }
          .mentor-total-report-badges { justify-content: flex-start; }
          .mentor-total-report-table-wrap { display: none; }
          .mentor-total-report-class-list { display: grid; gap: 8px; }
          .mentor-total-report-class-card { padding: 10px; border: 1px solid #cbd5e1; border-radius: 8px; background: #f8fafc; }
          .mentor-total-report-class-title { margin-bottom: 8px; font-size: 13px; font-weight: 800; overflow-wrap: anywhere; }
          .mentor-total-report-class-metrics { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 7px 12px; }
          .mentor-total-report-class-metric { min-width: 0; }
          .mentor-total-report-class-label { display: block; color: #64748b; font-size: 11px; }
          .mentor-total-report-class-value { display: block; margin-top: 1px; color: #0f172a; font-size: 13px; font-weight: 700; overflow-wrap: anywhere; }
        }
        @media (max-width: 360px) {
          .mentor-total-report-toolbar { grid-template-columns: 1fr; }
          .mentor-total-report-header { flex-direction: column; }
          .mentor-total-report-class-metrics { grid-template-columns: 1fr; }
        }
      `}</style>

      <div className="mentor-total-report-shell">
        <div className="mentor-total-report-toolbar">
          <button className="mentor-total-report-btn" onClick={handlePrint}>Print / Download PDF</button>
          <button className="mentor-total-report-btn" onClick={onClose}>Close</button>
        </div>
        <div className="mentor-total-report-page">
          <div className="mentor-total-report-header">
            <img src="/static/logo/eighty-twenty-logo.png" alt="Eighty Twenty" style={{ width: '74px' }} />
            <div className="mentor-total-report-header-copy">
              <div><strong>Mentor Total {scopeLabel} Report</strong></div>
              <div><strong>Scope:</strong> {scopeLabel}</div>
              <div><strong>Date:</strong> {generatedAt}</div>
            </div>
          </div>

          <div style={{ background: '#eef6ff', border: '1px solid #bfdbfe', borderRadius: '8px', padding: '10px', marginBottom: '12px' }}>
            <div><strong>Mentors with {scopeLabel.toLowerCase()} classes:</strong> {mentorsWithActiveClasses.length}</div>
            <div><strong>Total {scopeLabel.toLowerCase()} classes:</strong> {totalActiveClasses}</div>
            <div><strong>Total recorded session-quality entries:</strong> {totalRecordedSessions}</div>
            {filterSummary && <div><strong>Filters:</strong> {filterSummary}</div>}
          </div>

          <div style={{ display: 'grid', gap: '10px' }}>
            {mentorsWithActiveClasses.map((mentor) => {
              const classScores = mentor.classes.map(computeCollectiveKPI)
              const mentorCollective = classScores.length > 0
                ? Math.round(classScores.reduce((acc, score) => acc + score, 0) / classScores.length)
                : 0
              return (
                <div className="mentor-total-report-mentor" key={mentor.id}>
                  <div className="mentor-total-report-mentor-head">
                    <div className="mentor-total-report-mentor-identity">
                      <div style={{ fontSize: '18px', fontWeight: 700 }}>{mentor.name}</div>
                      <div style={{ color: '#475569', fontSize: '12px' }}>{mentor.email}</div>
                    </div>
                    <div className="mentor-total-report-badges">
                      <span style={{ borderRadius: '999px', padding: '4px 8px', fontSize: '11px', fontWeight: 700, background: '#d4edda', color: '#155724' }}>
                        {mentor.classes.length} active {mentor.classes.length === 1 ? 'class' : 'classes'}
                      </span>
                      <span style={{ borderRadius: '999px', padding: '4px 8px', fontSize: '11px', fontWeight: 700, background: '#e7f5ff', color: '#1864ab' }}>
                        Collective KPI {mentorCollective}%
                      </span>
                    </div>
                  </div>
                  <div className="mentor-total-report-table-wrap">
                    <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '11px', tableLayout: 'fixed' }}>
                      <thead>
                        <tr>
                          <th style={{ border: '1px solid #cbd5e1', padding: '5px', background: '#f8fafc' }}>Class Key</th>
                          <th style={{ border: '1px solid #cbd5e1', padding: '5px', background: '#f8fafc' }}>Class</th>
                          <th style={{ border: '1px solid #cbd5e1', padding: '5px', background: '#f8fafc' }}>Collective KPI</th>
                          <th style={{ border: '1px solid #cbd5e1', padding: '5px', background: '#f8fafc' }}>Quality Avg</th>
                          <th style={{ border: '1px solid #cbd5e1', padding: '5px', background: '#f8fafc' }}>Recorded</th>
                          <th style={{ border: '1px solid #cbd5e1', padding: '5px', background: '#f8fafc' }}>Feedback</th>
                          <th style={{ border: '1px solid #cbd5e1', padding: '5px', background: '#f8fafc' }}>Trello</th>
                          <th style={{ border: '1px solid #cbd5e1', padding: '5px', background: '#f8fafc' }}>WhatsApp</th>
                          <th style={{ border: '1px solid #cbd5e1', padding: '5px', background: '#f8fafc' }}>Punctuality</th>
                        </tr>
                      </thead>
                      <tbody>
                        {mentor.classes.map((cls) => {
                          const kpi = computeCollectiveKPI(cls)
                          return (
                            <tr key={cls.classKey}>
                              <td style={{ border: '1px solid #cbd5e1', padding: '5px', textAlign: 'center' }}>{cls.classKey}</td>
                              <td style={{ border: '1px solid #cbd5e1', padding: '5px', textAlign: 'center' }}>L{cls.level} • {cls.days} • {cls.time} • #{cls.classNumber}</td>
                              <td style={{ border: '1px solid #cbd5e1', padding: '5px', textAlign: 'center', fontWeight: 700, color: colorForPercent(kpi) }}>{kpi}%</td>
                              <td style={{ border: '1px solid #cbd5e1', padding: '5px', textAlign: 'center' }}>{cls.manual.sessionQuality > 0 ? `${cls.manual.sessionQuality}/10` : '-'}</td>
                              <td style={{ border: '1px solid #cbd5e1', padding: '5px', textAlign: 'center' }}>{cls.manual.recordedSessionCount}/8</td>
                              <td style={{ border: '1px solid #cbd5e1', padding: '5px', textAlign: 'center' }}>{cls.manual.studentsFeedback}/10</td>
                              <td style={{ border: '1px solid #cbd5e1', padding: '5px', textAlign: 'center' }}>{cls.manual.trelloCompliancePercent}%</td>
                              <td style={{ border: '1px solid #cbd5e1', padding: '5px', textAlign: 'center' }}>{cls.automatic.whatsAppManagementPercent}%</td>
                              <td style={{ border: '1px solid #cbd5e1', padding: '5px', textAlign: 'center' }}>{cls.automatic.attendancePunctualityPercent}%</td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                  <div className="mentor-total-report-class-list">
                    {mentor.classes.map((cls) => {
                      const kpi = computeCollectiveKPI(cls)
                      const metrics = [
                        ['Collective KPI', `${kpi}%`],
                        ['Quality Avg', cls.manual.sessionQuality > 0 ? `${cls.manual.sessionQuality}/10` : '-'],
                        ['Recorded', `${cls.manual.recordedSessionCount}/8`],
                        ['Feedback', `${cls.manual.studentsFeedback}/10`],
                        ['Trello', `${cls.manual.trelloCompliancePercent}%`],
                        ['WhatsApp', `${cls.automatic.whatsAppManagementPercent}%`],
                        ['Punctuality', `${cls.automatic.attendancePunctualityPercent}%`],
                      ]
                      return (
                        <div className="mentor-total-report-class-card" key={`mobile-${cls.classKey}`}>
                          <div className="mentor-total-report-class-title">
                            {cls.classKey} · L{cls.level} · {cls.days} · {cls.time} · #{cls.classNumber}
                          </div>
                          <div className="mentor-total-report-class-metrics">
                            {metrics.map(([label, value]) => (
                              <div className="mentor-total-report-class-metric" key={label}>
                                <span className="mentor-total-report-class-label">{label}</span>
                                <span
                                  className="mentor-total-report-class-value"
                                  style={label === 'Collective KPI' ? { color: colorForPercent(kpi) } : undefined}
                                >
                                  {value}
                                </span>
                              </div>
                            ))}
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}
