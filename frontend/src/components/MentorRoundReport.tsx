interface MentorRoundReportProps {
  report: {
    mentorName: string
    mentorEmail: string
    classKey: string
    level: number
    days: string
    time: string
    classNumber: number
    roundStatus: 'active' | 'closed'
    generatedAt: string
    collectiveKpi: number
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
      attendanceStatuses: string[]
    }
  }
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

function percentColor(percent: number): string {
  if (percent >= 80) return '#2f9e44'
  if (percent >= 60) return '#f59f00'
  return '#e03131'
}

function normalizeSessionQualityBySession(values: number[] | undefined): number[] {
  const normalized = new Array(8).fill(0)
  for (let i = 0; i < normalized.length && i < (values || []).length; i++) {
    const value = Number(values?.[i] || 0)
    if (!Number.isFinite(value)) continue
    normalized[i] = Math.max(0, Math.min(10, Math.round(value)))
  }
  return normalized
}

export default function MentorRoundReport({ report, onClose }: MentorRoundReportProps) {
  const generatedDate = new Date(report.generatedAt).toLocaleDateString()
  const sessionQualityBySession = normalizeSessionQualityBySession(report.manual.sessionQualityBySession)

  function handlePrint() {
    const logoSrc = `${window.location.origin}/static/logo/eighty-twenty-logo.png`
    const attendanceDots = report.automatic.attendanceStatuses
      .map((status, index) => {
        const color = status === 'on-time' ? '#2f9e44' : status === 'late' ? '#f59f00' : status === 'absent' ? '#e03131' : '#6c757d'
        return `<span style="display:inline-flex;align-items:center;justify-content:center;width:26px;height:26px;border-radius:50%;background:${color};color:white;font-size:11px;font-weight:700;">${index + 1}</span>`
      })
      .join('')
    const trelloCells = new Array(8).fill(false).map((_, index) => {
      const checked = report.manual.trelloSessionChecks[index]
      return `<td>${checked ? 'OK' : '-'}</td>`
    }).join('')
    const sessionQualityCells = sessionQualityBySession
      .map((score) => `<td>${score > 0 ? `${score}/10` : '-'}</td>`)
      .join('')

    const html = `<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Mentor Round Report - ${escapeHtml(report.mentorName)}</title>
  <style>
    @page { size: A4; margin: 10mm; }
    html, body { margin: 0; padding: 0; font-family: Arial, sans-serif; color: #0f172a; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    .header { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #dbe3ef; padding-bottom: 8px; margin-bottom: 12px; }
    .logo { width: 70px; height: auto; }
    .score { text-align: center; background: #eef6ff; border: 1px solid #bfdbfe; border-radius: 8px; padding: 10px; margin: 10px 0 12px; }
    .score-value { font-size: 34px; font-weight: 800; }
    .subtle { color: #475569; font-size: 12px; margin-top: 4px; }
    .bar-wrap { margin: 10px 0; }
    .bar-head { display: flex; justify-content: space-between; font-size: 12px; margin-bottom: 4px; }
    .bar { height: 11px; background: #e2e8f0; border-radius: 999px; overflow: hidden; }
    .fill { height: 100%; }
    table { width: 100%; border-collapse: collapse; margin-top: 12px; font-size: 12px; }
    th, td { border: 1px solid #cbd5e1; padding: 6px; text-align: center; }
    th:first-child, td:first-child { text-align: left; font-weight: 700; background: #f8fafc; }
  </style>
</head>
<body>
  <section>
    <div class="header">
      <img class="logo" src="${logoSrc}" alt="Eighty Twenty" />
      <div>
        <div><strong>Mentor:</strong> ${escapeHtml(report.mentorName)} (${escapeHtml(report.mentorEmail)})</div>
        <div><strong>Class:</strong> Level ${report.level} | ${escapeHtml(report.days)} | ${escapeHtml(report.time)} | Class #${report.classNumber}</div>
        <div><strong>Class Key:</strong> ${escapeHtml(report.classKey)}</div>
        <div><strong>Round:</strong> ${escapeHtml(report.roundStatus.toUpperCase())}</div>
        <div><strong>Date:</strong> ${escapeHtml(generatedDate)}</div>
      </div>
    </div>
    <div class="score">
      <div class="score-value">${report.collectiveKpi}%</div>
      <div>Collective KPI Ratio (Weighted)</div>
      <div class="subtle">Session Quality uses only MH-recorded sessions: ${report.manual.recordedSessionCount}/8 recorded</div>
    </div>

    <div class="bar-wrap">
      <div class="bar-head"><span>Session Quality (Recorded Average)</span><span>${report.manual.sessionQuality > 0 ? `${report.manual.sessionQuality}/10` : '-'}</span></div>
      <div class="bar"><div class="fill" style="width:${Math.max(0, Math.min(100, report.manual.sessionQuality * 10))}%;background:#1c7ed6;"></div></div>
    </div>
    <div class="bar-wrap">
      <div class="bar-head"><span>Students Feedback (Overall Manual)</span><span>${report.manual.studentsFeedback}/10</span></div>
      <div class="bar"><div class="fill" style="width:${Math.max(0, Math.min(100, report.manual.studentsFeedback * 10))}%;background:#1c7ed6;"></div></div>
    </div>
    <div class="bar-wrap">
      <div class="bar-head"><span>Trello Compliance (Manual)</span><span>${report.manual.trelloCompliancePercent}%</span></div>
      <div class="bar"><div class="fill" style="width:${Math.max(0, Math.min(100, report.manual.trelloCompliancePercent))}%;background:${percentColor(report.manual.trelloCompliancePercent)};"></div></div>
    </div>
    <div class="bar-wrap">
      <div class="bar-head"><span>WhatsApp Management (Auto)</span><span>${report.automatic.whatsAppManagementPercent}%</span></div>
      <div class="bar"><div class="fill" style="width:${Math.max(0, Math.min(100, report.automatic.whatsAppManagementPercent))}%;background:${percentColor(report.automatic.whatsAppManagementPercent)};"></div></div>
    </div>
    <div class="bar-wrap">
      <div class="bar-head"><span>Attendance Punctuality (Auto)</span><span>${report.automatic.attendancePunctualityPercent}%</span></div>
      <div class="bar"><div class="fill" style="width:${Math.max(0, Math.min(100, report.automatic.attendancePunctualityPercent))}%;background:${percentColor(report.automatic.attendancePunctualityPercent)};"></div></div>
    </div>

    <table>
      <thead>
        <tr>
          <th>Evidence</th>
          <th>S1</th>
          <th>S2</th>
          <th>S3</th>
          <th>S4</th>
          <th>S5</th>
          <th>S6</th>
          <th>S7</th>
          <th>S8</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td>Session Quality (Manual)</td>
          ${sessionQualityCells}
        </tr>
        <tr>
          <td>Trello Session Checks</td>
          ${trelloCells}
        </tr>
      </tbody>
    </table>

    <div style="margin-top: 12px;">
      <div style="font-size: 13px; margin-bottom: 6px;"><strong>Attendance by Session (Auto)</strong></div>
      <div style="display:flex;gap:6px;flex-wrap:wrap;">${attendanceDots}</div>
    </div>
  </section>
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
    <div className="mentor-report-root">
      <style>{`
        .mentor-report-root { background: #f3f5f7; min-height: 100vh; padding: 16px; }
        .mentor-report-shell { max-width: 940px; margin: 0 auto; }
        .mentor-report-toolbar { display: flex; justify-content: flex-end; gap: 8px; margin-bottom: 12px; }
        .mentor-report-btn { border: 1px solid #cbd5e1; background: white; border-radius: 6px; padding: 8px 12px; cursor: pointer; font-weight: 600; }
        .mentor-report-page { background: white; border: 1px solid #e2e8f0; border-radius: 8px; padding: 20px; }
        .mentor-report-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; border-bottom: 1px solid #e2e8f0; padding-bottom: 10px; }
        .mentor-report-logo { width: 74px; height: auto; }
        .mentor-report-score { text-align: center; margin: 10px 0 14px; padding: 12px; border-radius: 10px; background: #eef6ff; border: 1px solid #bfdbfe; }
        .mentor-report-score-value { font-size: 40px; line-height: 1; font-weight: 800; color: #1e293b; }
        .mentor-report-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); gap: 10px; }
        .mentor-report-metric-head { display: flex; justify-content: space-between; font-size: 13px; margin-bottom: 4px; color: #334155; }
        .mentor-report-metric-bar { height: 12px; background: #e2e8f0; border-radius: 999px; overflow: hidden; }
        .mentor-report-evidence { width: 100%; border-collapse: collapse; margin-top: 12px; font-size: 12px; }
        .mentor-report-evidence th, .mentor-report-evidence td { border: 1px solid #cbd5e1; padding: 6px; text-align: center; }
        .mentor-report-evidence th:first-child, .mentor-report-evidence td:first-child { text-align: left; font-weight: 700; background: #f8fafc; }
      `}</style>

      <div className="mentor-report-shell">
        <div className="mentor-report-toolbar">
          <button className="mentor-report-btn" onClick={handlePrint}>Print / Download PDF</button>
          <button className="mentor-report-btn" onClick={onClose}>Close</button>
        </div>

        <div className="mentor-report-page">
          <div className="mentor-report-header">
            <img className="mentor-report-logo" src="/static/logo/eighty-twenty-logo.png" alt="Eighty Twenty" />
            <div>
              <div><strong>Mentor:</strong> {report.mentorName} ({report.mentorEmail})</div>
              <div><strong>Class:</strong> Level {report.level} | {report.days} | {report.time} | Class #{report.classNumber}</div>
              <div><strong>Round:</strong> {report.roundStatus.toUpperCase()}</div>
              <div><strong>Date:</strong> {generatedDate}</div>
            </div>
          </div>

          <div className="mentor-report-score">
            <div className="mentor-report-score-value">{report.collectiveKpi}%</div>
            <div>Collective KPI Ratio (Weighted)</div>
            <div style={{ marginTop: '6px', color: '#475569', fontSize: '13px' }}>
              Session Quality uses only MH-recorded sessions: {report.manual.recordedSessionCount}/8 recorded
            </div>
          </div>

          <div className="mentor-report-grid">
            {[
              {
                label: 'Session Quality (Recorded Average)',
                value: report.manual.sessionQuality * 10,
                raw: report.manual.sessionQuality > 0 ? `${report.manual.sessionQuality}/10` : '-',
              },
              { label: 'Recorded Sessions', value: (report.manual.recordedSessionCount / 8) * 100, raw: `${report.manual.recordedSessionCount}/8` },
              { label: 'Students Feedback (Overall Manual)', value: report.manual.studentsFeedback * 10, raw: `${report.manual.studentsFeedback}/10` },
              { label: 'Trello Compliance (Manual)', value: report.manual.trelloCompliancePercent, raw: `${report.manual.trelloCompliancePercent}%` },
              { label: 'WhatsApp Management (Auto)', value: report.automatic.whatsAppManagementPercent, raw: `${report.automatic.whatsAppManagementPercent}%` },
              { label: 'Attendance Punctuality (Auto)', value: report.automatic.attendancePunctualityPercent, raw: `${report.automatic.attendancePunctualityPercent}%` },
            ].map((metric) => (
              <div key={metric.label}>
                <div className="mentor-report-metric-head">
                  <span>{metric.label}</span>
                  <strong>{metric.raw}</strong>
                </div>
                <div className="mentor-report-metric-bar">
                  <div
                    style={{
                      width: `${Math.max(0, Math.min(100, metric.value))}%`,
                      height: '100%',
                      background: percentColor(metric.value),
                    }}
                  />
                </div>
              </div>
            ))}
          </div>

          <table className="mentor-report-evidence">
            <thead>
              <tr>
                <th>Evidence</th>
                <th>S1</th>
                <th>S2</th>
                <th>S3</th>
                <th>S4</th>
                <th>S5</th>
                <th>S6</th>
                <th>S7</th>
                <th>S8</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Session Quality (Manual)</td>
                {sessionQualityBySession.map((score, index) => (
                  <td key={index}>{score > 0 ? `${score}/10` : '-'}</td>
                ))}
              </tr>
              <tr>
                <td>Trello Session Checks</td>
                {new Array(8).fill(false).map((_, index) => (
                  <td key={index}>{report.manual.trelloSessionChecks[index] ? 'OK' : '-'}</td>
                ))}
              </tr>
            </tbody>
          </table>

          <div style={{ marginTop: '12px' }}>
            <div style={{ marginBottom: '6px', fontSize: '13px', color: '#555' }}>Attendance by Session (Auto)</div>
            <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
              {report.automatic.attendanceStatuses.map((status, idx) => {
                const color = status === 'on-time' ? '#2f9e44' : status === 'late' ? '#f59f00' : status === 'absent' ? '#e03131' : '#6c757d'
                return (
                  <div
                    key={`${report.classKey}-${idx}`}
                    title={`Session ${idx + 1}: ${status}`}
                    style={{
                      width: '30px',
                      height: '30px',
                      borderRadius: '50%',
                      background: color,
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
                )
              })}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
