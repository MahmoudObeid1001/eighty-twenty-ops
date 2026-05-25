import { useEffect, useState } from 'react'

const BANNER_KEY = 'eid-al-adha-break-2026-05-26-to-2026-05-31-v2'
const START_AT = new Date('2026-05-25T00:00:00+03:00').getTime()
const END_AT = new Date('2026-06-01T00:00:00+03:00').getTime()

export default function GlobalAnnouncementBanner() {
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    const now = Date.now()
    if (now < START_AT || now >= END_AT) {
      return
    }
    if (window.localStorage.getItem(BANNER_KEY) === 'dismissed') {
      return
    }
    setVisible(true)
  }, [])

  function dismiss() {
    window.localStorage.setItem(BANNER_KEY, 'dismissed')
    setVisible(false)
  }

  if (!visible) {
    return null
  }

  return (
    <div style={bannerStyle}>
      <div style={messageWrapStyle}>
        <div style={titleStyle}>Eid El Adha Break</div>
        <div style={messageStyle}>
          We will be on Eid El Adha break from Tuesday, May 26, 2026 through Sunday, May 31, 2026.
          Monday, June 1, 2026 will be the first day back. Happy Eid, and wishing you all the best.
        </div>
      </div>
      <button type="button" onClick={dismiss} aria-label="Dismiss announcement" style={dismissButtonStyle}>
        ×
      </button>
    </div>
  )
}

const bannerStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'flex-start',
  justifyContent: 'space-between',
  gap: '12px',
  marginBottom: '16px',
  padding: '16px 18px',
  borderRadius: '12px',
  border: '1px solid #f5d28a',
  background: 'linear-gradient(135deg, #fff8e8 0%, #fff1c9 100%)',
  boxShadow: '0 8px 20px rgba(180, 126, 0, 0.08)',
}

const messageWrapStyle: React.CSSProperties = {
  minWidth: 0,
}

const titleStyle: React.CSSProperties = {
  fontSize: '18px',
  fontWeight: 700,
  color: '#7a4a00',
  marginBottom: '4px',
}

const messageStyle: React.CSSProperties = {
  fontSize: '14px',
  lineHeight: 1.5,
  color: '#6a5320',
}

const dismissButtonStyle: React.CSSProperties = {
  flex: '0 0 auto',
  width: '32px',
  height: '32px',
  borderRadius: '50%',
  border: '1px solid #e2be68',
  background: 'rgba(255,255,255,0.72)',
  color: '#8a5a00',
  fontSize: '22px',
  lineHeight: 1,
  cursor: 'pointer',
}
