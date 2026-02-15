import { StudentReportCardData } from '../api/client'

interface StudentReportCardProps {
  data: StudentReportCardData
  onClose: () => void
}

function scoreBarWidth(value: number, max: number): string {
  if (max <= 0) return '0%'
  const pct = Math.max(0, Math.min(100, (value / max) * 100))
  return `${pct}%`
}

function escapeHtml(value: string | number): string {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

export default function StudentReportCard({ data, onClose }: StudentReportCardProps) {
  const finalScore = Math.round(data.calculation.total_score)
  const finalGrade = (data.final_grade || data.calculation.calculated_grade || '').toUpperCase()
  const showCertificate = finalGrade !== 'F'
  const printDate = new Date(data.generated_at).toLocaleDateString()

  function handlePrint() {
    const logoSrc = `${window.location.origin}/static/logo/eighty-twenty-logo.png`
    const sessionHeaders = data.session_evidence.map((s) => `<th>S${s.session_number}</th>`).join('')
    const attendanceCells = data.session_evidence.map((s) => `<td>${escapeHtml(s.attendance_display || '—')}</td>`).join('')
    const taskCells = data.session_evidence.map((s) => `<td>${escapeHtml(s.task_display || '—')}</td>`).join('')
    const starCells = data.session_evidence.map((s) => `<td>${escapeHtml(s.participation_symbol || '—')}</td>`).join('')
    const mentorComment = escapeHtml(data.mentor_comment?.trim() || 'No comment provided.')

    const html = `<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Performance Report - ${escapeHtml(data.student_name)}</title>
  <style>
    @page { size: A4; margin: 10mm; }
    html, body { margin: 0; padding: 0; font-family: Arial, sans-serif; color: #0f172a; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    .page { box-sizing: border-box; width: 100%; }
    .report-page { page-break-after: always; }
    .report-page:last-child { page-break-after: auto; }
    .header { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #dbe3ef; padding-bottom: 8px; margin-bottom: 8px; }
    .logo { width: 62px; height: auto; }
    .score { text-align: center; background: #eef6ff; border: 1px solid #bfdbfe; border-radius: 8px; padding: 8px; margin: 8px 0; }
    .score-value { font-size: 26px; font-weight: 800; }
    .metric { margin: 6px 0; }
    .metric-head { display: flex; justify-content: space-between; font-size: 11px; margin-bottom: 3px; }
    .bar { height: 10px; background: #e2e8f0; border-radius: 999px; overflow: hidden; }
    .fill { height: 100%; background: #0ea5e9; }
    .fill.tasks { background: #22c55e; }
    .fill.part { background: #f59e0b; }
    table { width: 100%; border-collapse: collapse; font-size: 10px; margin-top: 8px; table-layout: fixed; }
    th, td { border: 1px solid #cbd5e1; padding: 4px; text-align: center; }
    th:first-child, td:first-child { text-align: left; font-weight: 700; background: #f8fafc; width: 130px; }
    .comment { margin-top: 8px; padding: 8px; border: 1px solid #e2e8f0; border-radius: 8px; background: #f8fafc; font-size: 12px; }
    .certificate { page-break-before: always; border: 8px solid #d4af37; height: 270mm; box-sizing: border-box; display: flex; align-items: center; justify-content: center; text-align: center; padding: 12mm; }
    .cert-logo { width: 96px; margin-bottom: 12px; }
    .cert-title { font-size: 40px; font-weight: 800; margin: 10px 0 14px; color: #1e293b; }
    .cert-text { font-size: 20px; color: #334155; line-height: 1.5; }
    .cert-name { font-size: 42px; font-weight: 800; margin: 12px 0; }
  </style>
</head>
<body>
  <section class="page report-page">
    <div class="header">
      <img class="logo" src="${logoSrc}" alt="Eighty Twenty" />
      <div>
        <div><strong>Student:</strong> ${escapeHtml(data.student_name)}</div>
        <div><strong>Level:</strong> ${escapeHtml(data.class_level)}</div>
        <div><strong>Date:</strong> ${escapeHtml(printDate)}</div>
      </div>
    </div>
    <div class="score">
      <div class="score-value">${finalScore} - ${escapeHtml(finalGrade)}</div>
      <div>Final Grade</div>
    </div>
    <div class="metric">
      <div class="metric-head"><span>Attendance</span><span>${data.calculation.attendance_score.toFixed(2)}/60 (${data.calculation.absences} Absences)</span></div>
      <div class="bar"><div class="fill" style="width:${scoreBarWidth(data.calculation.attendance_score, 60)}"></div></div>
    </div>
    <div class="metric">
      <div class="metric-head"><span>Tasks</span><span>${data.calculation.task_score.toFixed(2)}/30 (Missed ${data.calculation.missed_tasks} Tasks)</span></div>
      <div class="bar"><div class="fill tasks" style="width:${scoreBarWidth(data.calculation.task_score, 30)}"></div></div>
    </div>
    <div class="metric">
      <div class="metric-head"><span>Participation</span><span>${data.calculation.participation_score.toFixed(2)}/10 (${data.calculation.average_stars.toFixed(2)} Star Avg)</span></div>
      <div class="bar"><div class="fill part" style="width:${scoreBarWidth(data.calculation.participation_score, 10)}"></div></div>
    </div>
    <table>
      <thead><tr><th>Evidence</th>${sessionHeaders}</tr></thead>
      <tbody>
        <tr><td>Attendance</td>${attendanceCells}</tr>
        <tr><td>Tasks</td>${taskCells}</tr>
        <tr><td>Stars</td>${starCells}</tr>
      </tbody>
    </table>
    <div class="comment"><strong>Mentor Comment:</strong><div style="margin-top: 6px">${mentorComment}</div></div>
  </section>
  ${showCertificate ? `<section class="certificate">
    <div>
      <img class="cert-logo" src="${logoSrc}" alt="Eighty Twenty" />
      <div class="cert-title">Certificate of Completion</div>
      <div class="cert-text">Presented to</div>
      <div class="cert-name">${escapeHtml(data.student_name)}</div>
      <div class="cert-text">for successfully completing Level ${escapeHtml(data.class_level)} at Eighty Twenty.</div>
      <div style="margin-top: 20px; font-weight: 700;">Final Grade: ${finalScore} - ${escapeHtml(finalGrade)}</div>
    </div>
  </section>` : ''}
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
      const cleanup = () => {
        setTimeout(() => frame.remove(), 200)
      }
      printWindow.onafterprint = cleanup
      setTimeout(() => {
        printWindow.print()
        cleanup()
      }, 350)
    }

    frame.srcdoc = html
  }

  return (
    <div className="report-root">
      <style>{`
        .report-root { background: #f3f5f7; min-height: 100vh; padding: 16px; }
        .report-shell { max-width: 920px; margin: 0 auto; }
        .report-toolbar { display: flex; justify-content: flex-end; gap: 8px; margin-bottom: 12px; }
        .report-btn { border: 1px solid #cbd5e1; background: white; border-radius: 6px; padding: 8px 12px; cursor: pointer; font-weight: 600; }
        .report-page { background: white; border: 1px solid #e2e8f0; border-radius: 8px; padding: 20px; margin-bottom: 16px; }
        .report-main {}
        .report-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; border-bottom: 1px solid #e2e8f0; padding-bottom: 12px; }
        .report-logo { width: 84px; height: auto; }
        .score-box { text-align: center; margin: 14px 0 18px; padding: 14px; border-radius: 10px; background: #eef6ff; border: 1px solid #bfdbfe; }
        .score-value { font-size: 44px; line-height: 1; font-weight: 800; color: #1e293b; }
        .score-label { margin-top: 8px; color: #334155; font-weight: 600; }
        .metric { margin-bottom: 12px; }
        .metric-head { display: flex; justify-content: space-between; font-size: 13px; margin-bottom: 4px; color: #334155; }
        .metric-bar { height: 14px; background: #e2e8f0; border-radius: 999px; overflow: hidden; }
        .metric-fill { height: 100%; background: #0ea5e9; }
        .metric-fill.tasks { background: #22c55e; }
        .metric-fill.part { background: #f59e0b; }
        .evidence-table { width: 100%; border-collapse: collapse; margin-top: 12px; font-size: 13px; }
        .evidence-table th, .evidence-table td { border: 1px solid #cbd5e1; padding: 8px; text-align: center; }
        .evidence-table th:first-child, .evidence-table td:first-child { text-align: left; font-weight: 700; background: #f8fafc; }
        .mentor-comment { margin-top: 14px; padding: 12px; border: 1px solid #e2e8f0; border-radius: 8px; background: #f8fafc; }
        .certificate-page { display: flex; align-items: center; justify-content: center; min-height: 1000px; text-align: center; border: 8px solid #d4af37; }
        .certificate-logo { width: 110px; height: auto; margin-bottom: 16px; }
        .certificate-title { font-size: 42px; font-weight: 800; letter-spacing: 1px; margin-bottom: 18px; color: #1e293b; }
        .certificate-name { font-size: 36px; font-weight: 700; margin: 16px 0; color: #0f172a; }
        .certificate-text { font-size: 18px; color: #334155; max-width: 650px; margin: 0 auto; line-height: 1.7; }
        @media print {
          @page { size: A4 portrait; margin: 10mm; }
          html, body {
            margin: 0;
            padding: 0;
            background: white !important;
            -webkit-print-color-adjust: exact;
            print-color-adjust: exact;
          }
          body * { visibility: hidden !important; }
          .report-root, .report-root * { visibility: visible !important; }
          .report-overlay {
            position: static !important;
            inset: auto !important;
            overflow: visible !important;
            background: white !important;
          }
          .no-print { display: none !important; }
          .report-root {
            position: static !important;
            width: auto !important;
            padding: 0 !important;
            margin: 0 !important;
            background: white !important;
            min-height: 0 !important;
          }
          .report-shell { width: auto !important; margin: 0 !important; padding: 0 !important; max-width: none !important; }
          .report-page {
            border: none !important;
            margin: 0 !important;
            border-radius: 0 !important;
            box-shadow: none !important;
            width: auto !important;
            min-height: 0 !important;
            height: auto !important;
            padding: 6mm !important;
            box-sizing: border-box !important;
            overflow: visible !important;
            break-inside: avoid-page;
            page-break-inside: avoid;
          }
          .report-main {
            height: calc(297mm - 20mm) !important;
            max-height: calc(297mm - 20mm) !important;
            overflow: hidden !important;
          }
          .report-header { margin-bottom: 10px !important; padding-bottom: 8px !important; }
          .report-logo { width: 64px !important; }
          .score-box { margin: 8px 0 10px !important; padding: 8px !important; }
          .score-value { font-size: 30px !important; }
          .score-label { margin-top: 4px !important; }
          .metric { margin-bottom: 8px !important; }
          .metric-head { font-size: 12px !important; margin-bottom: 3px !important; }
          .metric-bar { height: 10px !important; }
          .evidence-table { margin-top: 8px !important; font-size: 11px !important; }
          .evidence-table th, .evidence-table td { padding: 5px !important; }
          .mentor-comment { margin-top: 8px !important; padding: 8px !important; }
          .certificate-page {
            page-break-before: always !important;
            break-before: page !important;
            border: 8px solid #d4af37 !important;
            min-height: calc(297mm - 16mm) !important;
            height: auto !important;
            padding: 12mm !important;
            display: flex;
            align-items: center;
            justify-content: center;
            text-align: center;
          }
          .certificate-logo { width: 90px !important; margin-bottom: 10px !important; }
          .certificate-title { font-size: 36px !important; margin-bottom: 12px !important; }
          .certificate-name { font-size: 34px !important; margin: 10px 0 !important; }
          .certificate-text { font-size: 18px !important; line-height: 1.5 !important; max-width: 640px !important; }
          .certificate-page > div > div[style] {
            margin-top: 16px !important;
          }
          .metric-fill, .metric-bar, .score-box, .certificate-page {
            -webkit-print-color-adjust: exact;
            print-color-adjust: exact;
          }
        }
      `}</style>

      <div className="report-shell">
        <div className="report-toolbar no-print">
          <button className="report-btn" onClick={handlePrint}>
            Print / Download PDF
          </button>
          <button className="report-btn" onClick={onClose}>
            Close
          </button>
        </div>

        <div className="report-page report-main">
          <div className="report-header">
            <img className="report-logo" src="/static/logo/eighty-twenty-logo.png" alt="Eighty Twenty" />
            <div>
              <div><strong>Student:</strong> {data.student_name}</div>
              <div><strong>Level:</strong> {data.class_level}</div>
              <div><strong>Date:</strong> {new Date(data.generated_at).toLocaleDateString()}</div>
            </div>
          </div>

          <div className="score-box">
            <div className="score-value">{finalScore} - {finalGrade}</div>
            <div className="score-label">Final Grade</div>
          </div>

          <div className="metric">
            <div className="metric-head">
              <span>Attendance</span>
              <span>{data.calculation.attendance_score.toFixed(2)}/60 ({data.calculation.absences} Absences)</span>
            </div>
            <div className="metric-bar"><div className="metric-fill" style={{ width: scoreBarWidth(data.calculation.attendance_score, 60) }} /></div>
          </div>

          <div className="metric">
            <div className="metric-head">
              <span>Tasks</span>
              <span>{data.calculation.task_score.toFixed(2)}/30 (Missed {data.calculation.missed_tasks} Tasks)</span>
            </div>
            <div className="metric-bar"><div className="metric-fill tasks" style={{ width: scoreBarWidth(data.calculation.task_score, 30) }} /></div>
          </div>

          <div className="metric">
            <div className="metric-head">
              <span>Participation</span>
              <span>{data.calculation.participation_score.toFixed(2)}/10 ({data.calculation.average_stars.toFixed(2)} Star Avg)</span>
            </div>
            <div className="metric-bar"><div className="metric-fill part" style={{ width: scoreBarWidth(data.calculation.participation_score, 10) }} /></div>
          </div>

          <table className="evidence-table">
            <thead>
              <tr>
                <th>Evidence</th>
                {data.session_evidence.map((s) => <th key={`h-${s.session_number}`}>S{s.session_number}</th>)}
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Attendance</td>
                {data.session_evidence.map((s) => <td key={`a-${s.session_number}`}>{s.attendance_display || '—'}</td>)}
              </tr>
              <tr>
                <td>Tasks</td>
                {data.session_evidence.map((s) => <td key={`t-${s.session_number}`}>{s.task_display || '—'}</td>)}
              </tr>
              <tr>
                <td>Stars</td>
                {data.session_evidence.map((s) => <td key={`p-${s.session_number}`}>{s.participation_symbol || '—'}</td>)}
              </tr>
            </tbody>
          </table>

          <div className="mentor-comment">
            <strong>Mentor Comment:</strong>
            <div style={{ marginTop: '6px' }}>{data.mentor_comment?.trim() || 'No comment provided.'}</div>
          </div>
        </div>

        {showCertificate && (
          <div className="report-page certificate-page">
            <div>
              <img className="certificate-logo" src="/static/logo/eighty-twenty-logo.png" alt="Eighty Twenty" />
              <div className="certificate-title">Certificate of Completion</div>
              <div className="certificate-text">
                Presented to
              </div>
              <div className="certificate-name">{data.student_name}</div>
              <div className="certificate-text">
                for successfully completing Level {data.class_level} at Eighty Twenty.
              </div>
              <div style={{ marginTop: '28px', fontWeight: 700, color: '#1e293b' }}>
                Final Grade: {finalScore} - {finalGrade}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
