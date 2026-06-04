import { StudentReportCardData } from '../api/client'

interface StudentReportCardProps {
  data: StudentReportCardData
  onClose: () => void
}

interface CertificateJourneyContent {
  vocabularyCount: number
  achievedItems: Array<{ en: string; ar?: string }>
  nextLevel: number
  nextItems: Array<{ en: string; ar?: string }>
}

const JOURNEY_HEADING_AR = 'رحلتك في تعلم الإنجليزية — انظر كم قطعت من الطريق'
const JOURNEY_HEADING_EN = "Your English Journey — Look how far you've come"
const ACHIEVED_HEADING_AR = 'أصبحت الآن قادراً على…'
const ACHIEVED_HEADING_EN = 'You can now…'
const NEXT_HEADING_AR = 'ستكتشف في المستوى القادم…'
const NEXT_HEADING_EN = "Next, you'll unlock…"

const CERTIFICATE_JOURNEY_BY_LEVEL: Record<number, CertificateJourneyContent> = {
  1: {
    vocabularyCount: 123,
    achievedItems: [
      { en: 'Introduce yourself and other people', ar: 'تعريف نفسك والآخرين' },
      { en: 'Talk about your family', ar: 'التحدث عن عائلتك' },
      { en: 'Talk about your daily routine', ar: 'وصف روتينك اليومي' },
      { en: 'Talk about your favorite activities', ar: 'التحدث عن أنشطتك المفضلة' },
      { en: 'Hold a small conversation', ar: 'إجراء محادثة بسيطة' },
      { en: "Say why you're learning English", ar: 'التعبير عن سبب تعلّمك للإنجليزية' },
    ],
    nextLevel: 2,
    nextItems: [
      { en: 'Talking about your hobbies', ar: 'التحدث عن هواياتك' },
      { en: 'Talking about your free time', ar: 'التحدث عن وقت فراغك' },
      { en: 'Describing people', ar: 'وصف الأشخاص' },
      { en: 'Describing common pain & symptoms', ar: 'وصف الألم والأعراض الشائعة' },
      { en: 'Talking about your friendships', ar: 'التحدث عن علاقاتك وصداقاتك' },
      { en: 'Inviting someone to your home', ar: 'دعوة شخص ما إلى منزلك' },
      { en: 'Describing the weather', ar: 'وصف حالة الطقس' },
    ],
  },
  2: {
    vocabularyCount: 127,
    achievedItems: [
      { en: 'Talk about your hobbies.' },
      { en: 'Talk about what you do in your free time.' },
      { en: 'Talk about your future plans.' },
      { en: 'Describe people.' },
      { en: 'Describe pain when you feel it.' },
      { en: 'Show care to someone who is sick.' },
      { en: 'Talk about your best friends and how you got to know them.' },
      { en: 'Talk about old friends.' },
      { en: 'Describe the weather.' },
    ],
    nextLevel: 3,
    nextItems: [
      { en: 'Describe your neighborhood.' },
      { en: "Describe your home or someone's apartment." },
      { en: 'Describe your relationship with money.' },
      { en: 'Ask for the price of something.' },
      { en: 'Describe what you eat for breakfast, lunch, and dinner.' },
      { en: 'Describe how you go to work, college, and school.' },
      { en: 'Tell the difference between ways of transportation.' },
      { en: "Describe someone's clothes and your own clothes." },
      { en: 'Ask for and give directions to different places.' },
      { en: 'Make your own cleaning routine.' },
    ],
  },
  3: {
    vocabularyCount: 151,
    achievedItems: [
      { en: 'Describe your neighborhood.' },
      { en: "Describe your home or someone's apartment." },
      { en: 'Describe your relationship with money.' },
      { en: 'Ask for the price of something.' },
      { en: 'Talk about prices in English.' },
      { en: 'Describe food in general.' },
      { en: 'Describe how you go to work, college, and school.' },
      { en: 'Tell the difference between ways of transportation.' },
      { en: 'Talk about what you like to wear at different times of the day.' },
      { en: 'Give your opinion about winter and summer clothing.' },
      { en: 'Ask for and give directions to different places.' },
      { en: 'Name different places.' },
      { en: 'Make your own cleaning routine.' },
    ],
    nextLevel: 4,
    nextItems: [
      { en: 'Describe what you did yesterday.' },
      { en: 'Describe your personality.' },
      { en: 'Talk about cellphones.' },
      { en: 'Talk about what kind of movies you like to watch.' },
      { en: 'Describe your childhood.' },
      { en: "Describe what you feel when you're in pain." },
      { en: "Talk about countries' problems." },
    ],
  },
  4: {
    vocabularyCount: 131,
    achievedItems: [
      { en: 'Talk about what you did yesterday.' },
      { en: 'Describe your personality.' },
      { en: 'Talk about cellphones.' },
      { en: 'Talk about what kind of movies you like to watch.' },
      { en: 'Describe your favorite movie.' },
      { en: 'Comment on a bad movie.' },
      { en: 'Describe your childhood.' },
      { en: 'Talk about your relationship with your siblings and parents when you were a child.' },
      { en: "Describe what you feel when you're in pain." },
      { en: 'Describe other pains.' },
      { en: 'Tell someone you feel sorry for them.' },
      { en: "Talk about your country's problems." },
    ],
    nextLevel: 5,
    nextItems: [
      { en: 'Talk about yourself in detail in the form of a story.' },
      { en: 'Learn the most common interview questions and answers.' },
      { en: 'Describe your job.' },
      { en: 'Describe your friendships with people.' },
      { en: 'Learn general differences between men and women.' },
      { en: 'Learn some law and legal terms.' },
      { en: 'Learn the common kinds of crimes.' },
      { en: 'Talk about your college life.' },
    ],
  },
  5: {
    vocabularyCount: 151,
    achievedItems: [
      { en: 'Talk about yourself in detail in the form of a story.' },
      { en: 'Know what to do before, during, and after an interview.' },
      { en: 'Talk about the most common interview questions and their answers.' },
      { en: 'Talk about your job.' },
      { en: 'Talk about your dream job.' },
      { en: 'Describe your job.' },
      { en: 'Talk about why people leave their jobs.' },
      { en: 'Talk about why people love their jobs.' },
      { en: 'Describe your friendships with people.' },
      { en: 'Talk about the signs of a good friend.' },
      { en: 'Talk about the signs of a toxic friend.' },
      { en: 'Talk about why friends are so important.' },
      { en: 'Talk about the differences between men and women, both physically and psychologically.' },
      { en: 'Talk about whether women should work or not.' },
      { en: 'Talk about some law and legal terms.' },
      { en: 'Tell the differences between felony, misdemeanor, and infraction.' },
      { en: 'Talk about the common kinds of crimes.' },
      { en: 'Talk about your college life.' },
    ],
    nextLevel: 6,
    nextItems: [
      { en: 'Tell a story in detail the right way.' },
      { en: 'Talk about social media platforms.' },
      { en: 'Talk about a healthy lifestyle.' },
      { en: 'Talk about parenthood.' },
      { en: 'Discuss the difference between living in a first-world and a third-world country.' },
      { en: "Describe how you feel when you're annoyed." },
      { en: "Deal with someone you don't like." },
    ],
  },
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

function formatCompletionDate(value: string): string {
  return new Date(value).toLocaleDateString('en-GB', {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  })
}

function renderJourneyListHtml(items: Array<{ en: string; ar?: string }>, icon: string): string {
  return items
    .map(
      (item) =>
        `<li><span class="l1-ic">${escapeHtml(icon)}</span><span class="l1-li-text"><span class="l1-li-en">${escapeHtml(item.en)}</span>${item.ar ? `<span class="l1-li-ar">${escapeHtml(item.ar)}</span>` : ''}</span></li>`,
    )
    .join('')
}

export default function StudentReportCard({ data, onClose }: StudentReportCardProps) {
  const finalScore = Math.round(data.calculation.total_score)
  const finalGrade = (data.final_grade || data.calculation.calculated_grade || '').toUpperCase()
  const showCertificate = finalGrade !== 'F'
  const journeyContent = showCertificate ? CERTIFICATE_JOURNEY_BY_LEVEL[data.class_level] : undefined
  const showJourneyCertificate = !!journeyContent
  const completionDate = formatCompletionDate(data.completion_at || data.generated_at)
  const mentorSignatureName = (data.mentor_name || '').trim() || 'Class Mentor'
  const mentorHeadSignatureName = 'Mohamed Abdel Gawad'

  function handlePrint() {
    const logoSrc = `${window.location.origin}/static/logo/eighty-twenty-logo.png`

    if (!showCertificate) {
      return
    }

    const html = `<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Official Certificate - ${escapeHtml(data.student_name)}</title>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
  <link href="https://fonts.googleapis.com/css2?family=Birthstone+Bounce:wght@400;500&family=Fraunces:opsz,wght@9..144,400;9..144,600;9..144,900&family=Great+Vibes&family=Outfit:wght@300;400;500;600;700&family=Tajawal:wght@400;500;700;800&display=swap" rel="stylesheet" />
  <style>
    @page { size: A4; margin: 6mm; }
    html, body { margin: 0; padding: 0; font-family: Arial, sans-serif; color: #0f172a; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    .certificate { border: 8px solid #d4af37; height: 270mm; box-sizing: border-box; display: flex; align-items: center; justify-content: center; text-align: center; padding: 12mm; }
    .cert-logo { width: 96px; margin-bottom: 12px; }
    .cert-title { font-size: 40px; font-weight: 800; margin: 10px 0 14px; color: #1e293b; }
    .cert-text { font-size: 20px; color: #334155; line-height: 1.5; }
    .cert-name { font-size: 42px; font-weight: 800; margin: 12px 0; }
    .level-one-certificate { height: calc(297mm - 12mm); min-height: calc(297mm - 12mm); box-sizing: border-box; font-family: Outfit, Arial, sans-serif; color: #2b3947; background: #fff; position: relative; overflow: hidden; padding: 12px; }
    .l1-frame { position: absolute; inset: 12px; border: 3px solid #c9a227; border-radius: 4px; pointer-events: none; box-shadow: inset 0 0 0 1px rgba(201,162,39,.25); }
    .l1-frame::after { content: ""; position: absolute; inset: 6px; border: 1px solid rgba(201,162,39,.35); border-radius: 3px; }
    .l1-inner { position: relative; padding: 18px 22px 14px; }
    .l1-brand { text-align: center; margin-bottom: 6px; }
    .l1-logo { font-family: Fraunces, Georgia, serif; font-weight: 900; font-size: 30px; line-height: .82; letter-spacing: -1px; color: #3bb4e5; display: inline-block; }
    .l1-logo span { display: block; }
    .l1-logo span:last-child { color: #1a8fc4; }
    .l1-logo-word { font-family: Fraunces, Georgia, serif; font-weight: 600; font-size: 13px; letter-spacing: 3px; color: #1a8fc4; margin-top: 4px; }
    .l1-logo-tag { font-size: 7.5px; letter-spacing: 3px; color: #6b7886; text-transform: uppercase; margin-top: 2px; }
    .l1-cert-head { text-align: center; margin-top: 10px; }
    .l1-cert-title { font-family: Fraunces, Georgia, serif; font-weight: 900; font-size: 38px; letter-spacing: -1.3px; color: #1f2d3d; line-height: 1; }
    .l1-presented { font-size: 12px; letter-spacing: 2px; text-transform: uppercase; color: #6b7886; margin-top: 10px; }
    .l1-name { font-family: Fraunces, Georgia, serif; font-weight: 600; font-size: 32px; color: #1f2d3d; margin-top: 6px; letter-spacing: -.5px; }
    .l1-name-rule { width: 180px; height: 2px; margin: 10px auto 0; background: linear-gradient(90deg, transparent, #c9a227, transparent); }
    .l1-reason { font-size: 14px; color: #3c4a5b; margin-top: 12px; }
    .l1-reason strong { color: #1f2d3d; font-weight: 600; }
    .l1-grade-chip { display: inline-flex; align-items: center; gap: 8px; margin-top: 12px; padding: 8px 18px; border-radius: 999px; background: linear-gradient(180deg,#fbf8ec,#f3ecd2); border: 1px solid #e3c766; font-weight: 700; color: #1f2d3d; font-size: 14px; letter-spacing: .5px; }
    .l1-grade-chip .star { color: #c9a227; }
    .l1-journey { margin-top: 16px; padding-top: 14px; border-top: 1px dashed #e7edf2; }
    .l1-bi { display: flex; flex-direction: column; align-items: center; gap: 2px; text-align: center; }
    .l1-ar { font-family: Tajawal, Tahoma, Arial, sans-serif; direction: rtl; unicode-bidi: embed; }
    .l1-bi .l1-ar { font-size: 22px; font-weight: 800; color: #1f2d3d; line-height: 1.2; }
    .l1-en-sub { font-size: 10px; text-transform: uppercase; letter-spacing: 2.5px; color: #6b7886; }
    .l1-milestone { display: flex; align-items: center; justify-content: center; gap: 18px; margin: 12px auto 14px; padding: 10px 18px; max-width: 520px; background: linear-gradient(120deg,#0f2231,#1f3a4f); border-radius: 14px; color: #fff; box-shadow: 0 16px 30px -16px rgba(15,34,49,.7); }
    .l1-ms-block { text-align: center; }
    .l1-ms-num { font-family: Fraunces, Georgia, serif; font-weight: 900; font-size: 30px; line-height: 1; color: #e3c766; }
    .l1-ms-label-ar { font-family: Tajawal, Tahoma, Arial, sans-serif; direction: rtl; font-size: 12px; font-weight: 700; color: #fff; margin-top: 4px; }
    .l1-ms-label-en { font-size: 9px; letter-spacing: 1px; color: #7fa8c0; margin-top: 2px; text-transform: uppercase; }
    .l1-ms-divider { width: 1px; height: 42px; background: rgba(255,255,255,.18); }
    .l1-cols { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
    .l1-card { border-radius: 16px; padding: 12px 14px 14px; border: 1px solid #e7edf2; }
    .l1-card.now { background: linear-gradient(180deg,#f1faf5,#ffffff); border-color: #cdeada; }
    .l1-card.next { background: linear-gradient(180deg,#eef7fc,#ffffff); border-color: #cfe9f6; }
    .l1-card-tag { display: inline-flex; align-items: center; gap: 8px; font-size: 11px; letter-spacing: 2px; text-transform: uppercase; font-weight: 600; padding: 6px 12px; border-radius: 999px; margin-bottom: 8px; }
    .l1-card.now .l1-card-tag { background: #d8f1e3; color: #157a4f; }
    .l1-card.next .l1-card-tag { background: #d6edf9; color: #1a8fc4; }
    .l1-card-head { margin-bottom: 10px; }
    .l1-card-head .l1-ar { font-size: 17px; font-weight: 800; color: #1f2d3d; line-height: 1.15; display: block; }
    .l1-card-head .l1-small { font-size: 10px; color: #6b7886; letter-spacing: 1px; display: block; margin-top: 2px; }
    .l1-list { list-style: none; display: flex; flex-direction: column; gap: 5px; margin: 0; padding: 0; }
    .l1-list li { display: flex; gap: 8px; align-items: flex-start; text-align: left; }
    .l1-ic { flex: 0 0 18px; width: 18px; height: 18px; margin-top: 1px; border-radius: 6px; display: grid; place-items: center; font-size: 10px; font-weight: 700; color: #fff; }
    .l1-card.now .l1-ic { background: #22a06b; }
    .l1-card.next .l1-ic { background: #3bb4e5; }
    .l1-li-text { display: flex; flex-direction: column; gap: 1px; }
    .l1-li-en { font-size: 11px; font-weight: 600; color: #1f2d3d; line-height: 1.15; }
    .l1-li-ar { font-family: Tajawal, Tahoma, Arial, sans-serif; direction: rtl; unicode-bidi: embed; font-size: 10px; font-weight: 500; color: #6b7886; line-height: 1.15; text-align: right; }
    .l1-foot { margin-top: 14px; padding-top: 10px; border-top: 1px solid #e7edf2; display: flex; flex-direction: column; gap: 10px; }
    .l1-foot-top { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); align-items: end; gap: 16px; }
    .l1-sig { text-align: center; min-width: 0; }
    .l1-sig-script { position: relative; line-height: .9; color: #182636; margin-bottom: 4px; letter-spacing: .3px; white-space: nowrap; max-width: 100%; overflow: hidden; background: transparent; }
    .l1-sig-script.mentor { font-family: 'Birthstone Bounce', cursive; font-size: 38px; font-weight: 500; }
    .l1-sig-script.head { font-family: 'Great Vibes', cursive; font-size: 28px; max-width: 92%; margin-left: auto; margin-right: auto; }
    .l1-sig-script::after { display: none; }
    .l1-sig-name { font-family: Fraunces, Georgia, serif; font-weight: 600; font-size: 13px; color: #1f2d3d; margin-bottom: 4px; }
    .l1-sig-line { border-top: 1.5px solid #1f2d3d; padding-top: 6px; font-size: 10px; letter-spacing: 1.4px; color: #6b7886; text-transform: uppercase; }
    .l1-seal { width: 70px; height: 70px; border-radius: 50%; background: radial-gradient(circle at 35% 30%,#fbf3d6,#e3c766 60%,#c9a227); display: grid; place-items: center; text-align: center; box-shadow: 0 8px 18px -8px rgba(201,162,39,.7); border: 2px solid #fff; }
    .l1-seal span { font-family: Fraunces, Georgia, serif; font-weight: 900; font-size: 11px; color: #7a5e10; line-height: 1.1; letter-spacing: .5px; }
    .l1-date-row { text-align: center; }
    .l1-date-value { font-family: Fraunces, Georgia, serif; font-weight: 600; font-size: 14px; color: #1f2d3d; margin-bottom: 3px; }
    .l1-date-label { font-size: 10px; letter-spacing: 2px; color: #6b7886; text-transform: uppercase; }
  </style>
</head>
<body>
  ${showJourneyCertificate && journeyContent ? `<section class="level-one-certificate">
    <div class="l1-frame"></div>
    <div class="l1-inner">
      <div class="l1-brand">
        <div class="l1-logo"><span>8O</span><span>2O</span></div>
        <div class="l1-logo-word">Eighty Twenty</div>
        <div class="l1-logo-tag">Mentors Deliver English</div>
      </div>
      <div class="l1-cert-head">
        <div class="l1-cert-title">Certificate of Completion</div>
        <div class="l1-presented">Presented to</div>
        <div class="l1-name">${escapeHtml(data.student_name)}</div>
        <div class="l1-name-rule"></div>
        <div class="l1-reason">for successfully completing <strong>Level ${escapeHtml(data.class_level)}</strong> at Eighty Twenty.</div>
        <div class="l1-grade-chip"><span class="star">★</span> Final Grade: ${finalScore} - ${escapeHtml(finalGrade)}</div>
      </div>
      <div class="l1-journey">
        <div class="l1-bi">
          <span class="l1-ar">${JOURNEY_HEADING_AR}</span>
          <span class="l1-en-sub">${JOURNEY_HEADING_EN}</span>
        </div>
        <div class="l1-milestone">
          <div class="l1-ms-block">
            <div class="l1-ms-num">${escapeHtml(journeyContent.vocabularyCount)}</div>
            <div class="l1-ms-label-ar">كلمة تعلّمتها في المستوى ${escapeHtml(data.class_level)}</div>
            <div class="l1-ms-label-en">Words mastered · Level ${escapeHtml(data.class_level)}</div>
          </div>
          <div class="l1-ms-divider"></div>
          <div class="l1-ms-block">
            <div class="l1-ms-num">Level ${escapeHtml(journeyContent.nextLevel)}</div>
            <div class="l1-ms-label-ar">الفصل القادم ينتظرك</div>
            <div class="l1-ms-label-en">Your next chapter awaits</div>
          </div>
        </div>
        <div class="l1-cols">
          <div class="l1-card now">
            <span class="l1-card-tag">✓ Achieved</span>
            <div class="l1-card-head">
              <span class="l1-ar">${ACHIEVED_HEADING_AR}</span>
              <span class="l1-small">${ACHIEVED_HEADING_EN}</span>
            </div>
            <ul class="l1-list">${renderJourneyListHtml(journeyContent.achievedItems, '✓')}</ul>
          </div>
          <div class="l1-card next">
            <span class="l1-card-tag">→ Coming in Level ${escapeHtml(journeyContent.nextLevel)}</span>
            <div class="l1-card-head">
              <span class="l1-ar">${NEXT_HEADING_AR}</span>
              <span class="l1-small">${NEXT_HEADING_EN}</span>
            </div>
            <ul class="l1-list">${renderJourneyListHtml(journeyContent.nextItems, '→')}</ul>
          </div>
        </div>
      </div>
      <div class="l1-foot">
        <div class="l1-foot-top">
          <div class="l1-sig">
            <div class="l1-sig-script mentor">${escapeHtml(mentorSignatureName)}</div>
            <div class="l1-sig-name">${escapeHtml(mentorSignatureName)}</div>
            <div class="l1-sig-line">Class Mentor</div>
          </div>
          <div class="l1-seal"><span>LEVEL&nbsp;${escapeHtml(data.class_level)}<br>PASSED</span></div>
          <div class="l1-sig">
            <div class="l1-sig-script head">${escapeHtml(mentorHeadSignatureName)}</div>
            <div class="l1-sig-name">${escapeHtml(mentorHeadSignatureName)}</div>
            <div class="l1-sig-line">Mentor Head</div>
          </div>
        </div>
        <div class="l1-date-row">
          <div class="l1-date-value">${escapeHtml(completionDate)}</div>
          <div class="l1-date-label">Date of Completion</div>
        </div>
      </div>
    </div>
  </section>` : showCertificate ? `<section class="certificate">
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
        @import url('https://fonts.googleapis.com/css2?family=Birthstone+Bounce:wght@400;500&family=Fraunces:opsz,wght@9..144,400;9..144,600;9..144,900&family=Great+Vibes&family=Outfit:wght@300;400;500;600;700&family=Tajawal:wght@400;500;700;800&display=swap');
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
        .level-one-certificate {
          min-height: 1000px;
          box-sizing: border-box;
          font-family: Outfit, Arial, sans-serif;
          color: #2b3947;
          background: #fff;
          position: relative;
          overflow: hidden;
          padding: 18px;
          border: none;
        }
        .l1-frame { position: absolute; inset: 18px; border: 3px solid #c9a227; border-radius: 4px; pointer-events: none; box-shadow: inset 0 0 0 1px rgba(201,162,39,.25); }
        .l1-frame::after { content: ""; position: absolute; inset: 6px; border: 1px solid rgba(201,162,39,.35); border-radius: 3px; }
        .l1-inner { position: relative; padding: 38px 46px 34px; }
        .l1-brand { text-align: center; margin-bottom: 6px; }
        .l1-logo { font-family: Fraunces, Georgia, serif; font-weight: 900; font-size: 34px; line-height: .82; letter-spacing: -1px; color: #3bb4e5; display: inline-block; }
        .l1-logo span { display: block; }
        .l1-logo span:last-child { color: #1a8fc4; }
        .l1-logo-word { font-family: Fraunces, Georgia, serif; font-weight: 600; font-size: 13px; letter-spacing: 3px; color: #1a8fc4; margin-top: 4px; }
        .l1-logo-tag { font-size: 7.5px; letter-spacing: 3px; color: #6b7886; text-transform: uppercase; margin-top: 2px; }
        .l1-cert-head { text-align: center; margin-top: 22px; }
        .l1-cert-title { font-family: Fraunces, Georgia, serif; font-weight: 900; font-size: 48px; letter-spacing: -1.5px; color: #1f2d3d; line-height: 1; }
        .l1-presented { font-size: 15px; letter-spacing: 2px; text-transform: uppercase; color: #6b7886; margin-top: 18px; }
        .l1-name { font-family: Fraunces, Georgia, serif; font-weight: 600; font-size: 42px; color: #1f2d3d; margin-top: 10px; letter-spacing: -.5px; }
        .l1-name-rule { width: 200px; height: 2px; margin: 12px auto 0; background: linear-gradient(90deg, transparent, #c9a227, transparent); }
        .l1-reason { font-size: 16px; color: #3c4a5b; margin-top: 18px; text-align: center; }
        .l1-reason strong { color: #1f2d3d; font-weight: 600; }
        .l1-grade-chip { display: inline-flex; align-items: center; gap: 10px; margin-top: 18px; padding: 10px 24px; border-radius: 999px; background: linear-gradient(180deg,#fbf8ec,#f3ecd2); border: 1px solid #e3c766; font-weight: 700; color: #1f2d3d; font-size: 16px; letter-spacing: .5px; }
        .l1-grade-chip .star { color: #c9a227; }
        .l1-journey { margin-top: 34px; padding-top: 28px; border-top: 1px dashed #e7edf2; }
        .l1-bi { display: flex; flex-direction: column; align-items: center; gap: 4px; text-align: center; }
        .l1-ar { font-family: Tajawal, Tahoma, Arial, sans-serif; direction: rtl; unicode-bidi: embed; }
        .l1-bi .l1-ar { font-size: 26px; font-weight: 800; color: #1f2d3d; line-height: 1.3; }
        .l1-en-sub { font-size: 12px; text-transform: uppercase; letter-spacing: 3px; color: #6b7886; }
        .l1-milestone { display: flex; align-items: center; justify-content: center; gap: 30px; margin: 22px auto 24px; padding: 16px 26px; max-width: 580px; background: linear-gradient(120deg,#0f2231,#1f3a4f); border-radius: 14px; color: #fff; box-shadow: 0 16px 30px -16px rgba(15,34,49,.7); }
        .l1-ms-block { text-align: center; }
        .l1-ms-num { font-family: Fraunces, Georgia, serif; font-weight: 900; font-size: 36px; line-height: 1; color: #e3c766; }
        .l1-ms-label-ar { font-family: Tajawal, Tahoma, Arial, sans-serif; direction: rtl; font-size: 14px; font-weight: 700; color: #fff; margin-top: 5px; }
        .l1-ms-label-en { font-size: 9px; letter-spacing: 1px; color: #7fa8c0; margin-top: 2px; text-transform: uppercase; }
        .l1-ms-divider { width: 1px; height: 52px; background: rgba(255,255,255,.18); }
        .l1-cols { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; }
        .l1-card { border-radius: 16px; padding: 20px 22px 22px; border: 1px solid #e7edf2; }
        .l1-card.now { background: linear-gradient(180deg,#f1faf5,#ffffff); border-color: #cdeada; }
        .l1-card.next { background: linear-gradient(180deg,#eef7fc,#ffffff); border-color: #cfe9f6; }
        .l1-card-tag { display: inline-flex; align-items: center; gap: 8px; font-size: 11px; letter-spacing: 2px; text-transform: uppercase; font-weight: 600; padding: 6px 12px; border-radius: 999px; margin-bottom: 8px; }
        .l1-card.now .l1-card-tag { background: #d8f1e3; color: #157a4f; }
        .l1-card.next .l1-card-tag { background: #d6edf9; color: #1a8fc4; }
        .l1-card-head { margin-bottom: 14px; }
        .l1-card-head .l1-ar { font-size: 20px; font-weight: 800; color: #1f2d3d; line-height: 1.3; display: block; }
        .l1-card-head .l1-small { font-size: 12px; color: #6b7886; letter-spacing: 1px; display: block; margin-top: 3px; }
        .l1-list { list-style: none; display: flex; flex-direction: column; gap: 8px; margin: 0; padding: 0; }
        .l1-list li { display: flex; gap: 10px; align-items: flex-start; text-align: left; }
        .l1-ic { flex: 0 0 20px; width: 20px; height: 20px; margin-top: 2px; border-radius: 6px; display: grid; place-items: center; font-size: 11px; font-weight: 700; color: #fff; }
        .l1-card.now .l1-ic { background: #22a06b; }
        .l1-card.next .l1-ic { background: #3bb4e5; }
        .l1-li-text { display: flex; flex-direction: column; gap: 1px; }
        .l1-li-en { font-size: 13px; font-weight: 600; color: #1f2d3d; line-height: 1.25; }
        .l1-li-ar { font-family: Tajawal, Tahoma, Arial, sans-serif; direction: rtl; unicode-bidi: embed; font-size: 12px; font-weight: 500; color: #6b7886; line-height: 1.3; text-align: right; }
        .l1-foot { margin-top: 28px; padding-top: 18px; border-top: 1px solid #e7edf2; display: flex; flex-direction: column; gap: 18px; }
        .l1-foot-top { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); align-items: end; gap: 24px; }
        .l1-sig { text-align: center; min-width: 0; }
        .l1-sig-script { position: relative; line-height: .9; color: #182636; margin-bottom: 6px; letter-spacing: .4px; text-shadow: 0.6px 0 rgba(24,38,54,.35), -0.6px 0 rgba(24,38,54,.18), 0 1.6px 10px rgba(24,38,54,.08); white-space: nowrap; max-width: 100%; overflow: hidden; }
        .l1-sig-script.mentor { font-family: 'Birthstone Bounce', cursive; font-size: 48px; font-weight: 500; transform: rotate(-4deg) translateX(-4px); }
        .l1-sig-script.head { font-family: 'Great Vibes', cursive; font-size: 36px; transform: rotate(1deg); max-width: 92%; margin-left: auto; margin-right: auto; }
        .l1-sig-script::after { content: ""; position: absolute; left: 6%; right: 2%; bottom: 4px; height: 1px; background: linear-gradient(90deg, transparent, rgba(24,38,54,.18), transparent); filter: blur(.2px); }
        .l1-sig-name { font-family: Fraunces, Georgia, serif; font-weight: 600; font-size: 14px; color: #1f2d3d; margin-bottom: 6px; }
        .l1-sig-line { border-top: 1.5px solid #1f2d3d; padding-top: 8px; font-size: 11px; letter-spacing: 1.4px; color: #6b7886; text-transform: uppercase; }
        .l1-seal { width: 78px; height: 78px; border-radius: 50%; background: radial-gradient(circle at 35% 30%,#fbf3d6,#e3c766 60%,#c9a227); display: grid; place-items: center; text-align: center; box-shadow: 0 8px 18px -8px rgba(201,162,39,.7); border: 2px solid #fff; }
        .l1-seal span { font-family: Fraunces, Georgia, serif; font-weight: 900; font-size: 11px; color: #7a5e10; line-height: 1.1; letter-spacing: .5px; }
        .l1-date-row { text-align: center; }
        .l1-date-value { font-family: Fraunces, Georgia, serif; font-weight: 600; font-size: 16px; color: #1f2d3d; margin-bottom: 4px; }
        .l1-date-label { font-size: 11px; letter-spacing: 2px; color: #6b7886; text-transform: uppercase; }
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
          .report-main {
            display: none !important;
          }
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
          .level-one-certificate {
            page-break-before: always !important;
            break-before: page !important;
            height: calc(297mm - 20mm) !important;
            min-height: calc(297mm - 20mm) !important;
            padding: 12px !important;
            overflow: hidden !important;
          }
          .level-one-certificate .l1-frame {
            inset: 12px !important;
          }
          .level-one-certificate .l1-inner {
            padding: 18px 22px 14px !important;
            transform: none !important;
          }
          .level-one-certificate .l1-sig-script,
          .level-one-certificate .l1-sig-script.mentor,
          .level-one-certificate .l1-sig-script.head {
            transform: none !important;
            text-shadow: none !important;
            filter: none !important;
            background: transparent !important;
            mix-blend-mode: normal !important;
          }
          .metric-fill, .metric-bar, .score-box, .certificate-page, .level-one-certificate {
            -webkit-print-color-adjust: exact;
            print-color-adjust: exact;
          }
        }
      `}</style>

      <div className="report-shell">
        <div className="report-toolbar no-print">
          {showCertificate && (
            <button className="report-btn" onClick={handlePrint}>
              Print / Download Official Certificate
            </button>
          )}
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
              <span>{data.calculation.attendance_score.toFixed(2)}/50 ({data.calculation.absences} Absences)</span>
            </div>
            <div className="metric-bar"><div className="metric-fill" style={{ width: scoreBarWidth(data.calculation.attendance_score, 50) }} /></div>
          </div>

          <div className="metric">
            <div className="metric-head">
              <span>Tasks</span>
              <span>{data.calculation.task_score.toFixed(2)}/40 (Missed {data.calculation.missed_tasks} Tasks)</span>
            </div>
            <div className="metric-bar"><div className="metric-fill tasks" style={{ width: scoreBarWidth(data.calculation.task_score, 40) }} /></div>
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

        {showJourneyCertificate && journeyContent ? (
          <div className="report-page level-one-certificate">
            <div className="l1-frame"></div>
            <div className="l1-inner">
              <div className="l1-brand">
                <div className="l1-logo"><span>8O</span><span>2O</span></div>
                <div className="l1-logo-word">Eighty Twenty</div>
                <div className="l1-logo-tag">Mentors Deliver English</div>
              </div>

              <div className="l1-cert-head">
                <div className="l1-cert-title">Certificate of Completion</div>
                <div className="l1-presented">Presented to</div>
                <div className="l1-name">{data.student_name}</div>
                <div className="l1-name-rule"></div>
                <div className="l1-reason">for successfully completing <strong>Level {data.class_level}</strong> at Eighty Twenty.</div>
                <div className="l1-grade-chip"><span className="star">★</span> Final Grade: {finalScore} - {finalGrade}</div>
              </div>

              <div className="l1-journey">
                <div className="l1-bi">
                  <span className="l1-ar">{JOURNEY_HEADING_AR}</span>
                  <span className="l1-en-sub">{JOURNEY_HEADING_EN}</span>
                </div>

                <div className="l1-milestone">
                  <div className="l1-ms-block">
                    <div className="l1-ms-num">{journeyContent.vocabularyCount}</div>
                    <div className="l1-ms-label-ar">كلمة تعلّمتها في المستوى {data.class_level}</div>
                    <div className="l1-ms-label-en">Words mastered · Level {data.class_level}</div>
                  </div>
                  <div className="l1-ms-divider"></div>
                  <div className="l1-ms-block">
                    <div className="l1-ms-num">Level {journeyContent.nextLevel}</div>
                    <div className="l1-ms-label-ar">الفصل القادم ينتظرك</div>
                    <div className="l1-ms-label-en">Your next chapter awaits</div>
                  </div>
                </div>

                <div className="l1-cols">
                  <div className="l1-card now">
                    <span className="l1-card-tag">✓ Achieved</span>
                    <div className="l1-card-head">
                      <span className="l1-ar">{ACHIEVED_HEADING_AR}</span>
                      <span className="l1-small">{ACHIEVED_HEADING_EN}</span>
                    </div>
                    <ul className="l1-list">
                      {journeyContent.achievedItems.map((item) => (
                        <li key={`achieved-${item.en}`}>
                          <span className="l1-ic">✓</span>
                          <span className="l1-li-text">
                            <span className="l1-li-en">{item.en}</span>
                            {item.ar && <span className="l1-li-ar">{item.ar}</span>}
                          </span>
                        </li>
                      ))}
                    </ul>
                  </div>

                  <div className="l1-card next">
                    <span className="l1-card-tag">→ Coming in Level {journeyContent.nextLevel}</span>
                    <div className="l1-card-head">
                      <span className="l1-ar">{NEXT_HEADING_AR}</span>
                      <span className="l1-small">{NEXT_HEADING_EN}</span>
                    </div>
                    <ul className="l1-list">
                      {journeyContent.nextItems.map((item) => (
                        <li key={`next-${item.en}`}>
                          <span className="l1-ic">→</span>
                          <span className="l1-li-text">
                            <span className="l1-li-en">{item.en}</span>
                            {item.ar && <span className="l1-li-ar">{item.ar}</span>}
                          </span>
                        </li>
                      ))}
                    </ul>
                  </div>
                </div>
              </div>

              <div className="l1-foot">
                <div className="l1-foot-top">
                  <div className="l1-sig">
                    <div className="l1-sig-script mentor">{mentorSignatureName}</div>
                    <div className="l1-sig-name">{mentorSignatureName}</div>
                    <div className="l1-sig-line">Class Mentor</div>
                  </div>
                  <div className="l1-seal"><span>LEVEL&nbsp;{data.class_level}<br />PASSED</span></div>
                  <div className="l1-sig">
                    <div className="l1-sig-script head">{mentorHeadSignatureName}</div>
                    <div className="l1-sig-name">{mentorHeadSignatureName}</div>
                    <div className="l1-sig-line">Mentor Head</div>
                  </div>
                </div>
                <div className="l1-date-row">
                  <div className="l1-date-value">{completionDate}</div>
                  <div className="l1-date-label">Date of Completion</div>
                </div>
              </div>
            </div>
          </div>
        ) : showCertificate && (
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
