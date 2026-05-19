import { useEffect, useState } from 'react'
import { api, MentorAvailabilityWindow, MentorDirectoryItem, MentorProfileResponse } from '../api/client'

function formatDate(value: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleDateString()
}

function currentMonth() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

export default function MentorsPage() {
  const [mentors, setMentors] = useState<MentorDirectoryItem[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [profile, setProfile] = useState<MentorProfileResponse | null>(null)
  const [loadingProfile, setLoadingProfile] = useState(false)
  const [profileError, setProfileError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'history' | 'availability' | 'testimonials'>('history')
  const [availabilityMonth, setAvailabilityMonth] = useState(currentMonth())
  const [availabilityWindows, setAvailabilityWindows] = useState<MentorAvailabilityWindow[]>([])
  const [availabilityLoading, setAvailabilityLoading] = useState(false)
  const [availabilityError, setAvailabilityError] = useState<string | null>(null)

  useEffect(() => {
    void loadMentors()
  }, [])

  async function loadMentors() {
    try {
      setLoading(true)
      setError(null)
      const data = await api.getMentors()
      setMentors(data.mentors || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load mentors')
    } finally {
      setLoading(false)
    }
  }

  async function openProfile(mentorId: string) {
    try {
      setLoadingProfile(true)
      setProfileError(null)
      setAvailabilityError(null)
      setAvailabilityWindows([])
      setActiveTab('history')
      const data = await api.getMentorProfile(mentorId)
      setProfile(data)
      void loadAvailability(mentorId, availabilityMonth)
    } catch (err) {
      setProfileError(err instanceof Error ? err.message : 'Failed to load mentor profile')
    } finally {
      setLoadingProfile(false)
    }
  }

  async function loadAvailability(mentorId: string, month: string) {
    try {
      setAvailabilityLoading(true)
      setAvailabilityError(null)
      const data = await api.getMentorAvailability(mentorId, month)
      setAvailabilityWindows(data.windows || [])
    } catch (err) {
      setAvailabilityError(err instanceof Error ? err.message : 'Failed to load mentor availability')
    } finally {
      setAvailabilityLoading(false)
    }
  }

  function changeAvailabilityMonth(month: string) {
    const nextMonth = month || currentMonth()
    setAvailabilityMonth(nextMonth)
    if (profile?.mentor_details.id) {
      void loadAvailability(profile.mentor_details.id, nextMonth)
    }
  }

  const filteredMentors = mentors.filter((m) => {
    const q = search.trim().toLowerCase()
    if (!q) return true
    return (
      m.name.toLowerCase().includes(q) ||
      m.email.toLowerCase().includes(q) ||
      (m.phone || '').toLowerCase().includes(q)
    )
  })

  return (
    <div style={{ padding: '24px' }}>
      <h1 style={{ marginBottom: '10px', fontSize: '32px', fontWeight: 700 }}>Mentor Directory</h1>
      <p style={{ color: '#666', marginTop: 0, marginBottom: '18px' }}>
        Browse all mentors and open detailed profile history.
      </p>

      <div style={{ marginBottom: '12px', maxWidth: '520px' }}>
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by name, phone, or email..."
          style={{
            width: '100%',
            padding: '10px 12px',
            border: '1px solid #ced4da',
            borderRadius: '6px',
            fontSize: '14px',
            background: '#fff',
          }}
        />
      </div>

      {loading ? (
        <p>Loading mentors...</p>
      ) : error ? (
        <div style={{ padding: '12px', borderRadius: '6px', background: '#f8d7da', color: '#721c24' }}>{error}</div>
      ) : (
        <div style={{ background: '#fff', border: '1px solid #dee2e6', borderRadius: '8px', overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ background: '#f8f9fa', borderBottom: '1px solid #dee2e6' }}>
                <th style={thStyle}>Name</th>
                <th style={thStyle}>Phone</th>
                <th style={thStyle}>Status</th>
                <th style={thStyle}>Total Classes Taught</th>
              </tr>
            </thead>
            <tbody>
              {filteredMentors.map((m) => (
                <tr
                  key={m.id}
                  onClick={() => void openProfile(m.id)}
                  style={{ borderBottom: '1px solid #f1f3f5', cursor: 'pointer' }}
                >
                  <td style={tdStyle}>
                    <div style={{ fontWeight: 600 }}>{m.name}</div>
                    <div style={{ color: '#666', fontSize: '12px' }}>{m.email}</div>
                  </td>
                  <td style={tdStyle}>{m.phone || '-'}</td>
                  <td style={tdStyle}>
                    <span
                      style={{
                        display: 'inline-block',
                        padding: '3px 8px',
                        borderRadius: '12px',
                        fontSize: '12px',
                        fontWeight: 700,
                        background: m.status === 'active' ? '#d3f9d8' : '#f1f3f5',
                        color: m.status === 'active' ? '#2b8a3e' : '#495057',
                      }}
                    >
                      {m.status === 'active' ? 'Active' : 'Inactive'}
                    </span>
                  </td>
                  <td style={tdStyle}>{m.total_classes_taught}</td>
                </tr>
              ))}
              {filteredMentors.length === 0 && (
                <tr>
                  <td style={{ ...tdStyle, color: '#666' }} colSpan={4}>
                    No mentors match your search.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}

      {profile && (
        <div
          onClick={() => {
            setProfile(null)
            setProfileError(null)
          }}
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(0,0,0,0.45)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 1000,
          }}
        >
          <div
            onClick={(e) => e.stopPropagation()}
            style={{
              width: '94%',
              maxWidth: '980px',
              maxHeight: '90vh',
              overflowY: 'auto',
              background: '#fff',
              borderRadius: '8px',
              padding: '18px',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h2 style={{ margin: 0, fontSize: '28px' }}>{profile.mentor_details.name}</h2>
              <button onClick={() => setProfile(null)} style={{ border: 'none', background: 'transparent', fontSize: '24px', cursor: 'pointer' }}>×</button>
            </div>

            <div style={{ color: '#666', marginTop: '6px' }}>
              {profile.mentor_details.email} • {profile.mentor_details.phone || '-'} • {profile.mentor_details.status === 'active' ? 'Active' : 'Inactive'}
            </div>

            <div style={{ display: 'flex', gap: '16px', marginTop: '14px', flexWrap: 'wrap' }}>
              <div style={metaPill}>First Class: {formatDate(profile.stats.first_class_date)}</div>
              <div style={metaPill}>Last Class: {formatDate(profile.stats.last_class_date)}</div>
            </div>

            {loadingProfile && <p style={{ marginTop: '14px' }}>Loading profile...</p>}
            {profileError && <div style={{ marginTop: '10px', padding: '10px', background: '#f8d7da', color: '#721c24', borderRadius: '6px' }}>{profileError}</div>}

            <div style={{ marginTop: '16px', display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '10px' }}>
              <KpiCard label="Total Classes" value={String(profile.stats.total_classes)} />
              <KpiCard label="Feedback Meter (MH)" value={`${profile.stats.feedback_meter}%`} />
              <KpiCard label="Overall KPI (All Classes)" value={`${profile.stats.avg_rating}%`} />
            </div>

            <div style={{ display: 'flex', gap: '8px', marginTop: '20px' }}>
              <button
                onClick={() => setActiveTab('history')}
                style={{ ...tabBtn, ...(activeTab === 'history' ? activeTabBtn : {}) }}
              >
                Class History
              </button>
              <button
                onClick={() => setActiveTab('availability')}
                style={{ ...tabBtn, ...(activeTab === 'availability' ? activeTabBtn : {}) }}
              >
                Availability
              </button>
              <button
                onClick={() => setActiveTab('testimonials')}
                style={{ ...tabBtn, ...(activeTab === 'testimonials' ? activeTabBtn : {}) }}
              >
                Testimonials
              </button>
            </div>

            {activeTab === 'history' ? (
              <div style={{ border: '1px solid #dee2e6', borderRadius: '6px', overflow: 'hidden', marginTop: '8px' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ background: '#f8f9fa', borderBottom: '1px solid #dee2e6' }}>
                      <th style={thStyle}>Class</th>
                      <th style={thStyle}>Start Date</th>
                      <th style={thStyle}>End Date</th>
                      <th style={thStyle}>Duration</th>
                      <th style={thStyle}>Evaluation Score</th>
                    </tr>
                  </thead>
                  <tbody>
                    {profile.class_history.map((h) => (
                      <tr key={h.class_key} style={{ borderBottom: '1px solid #f1f3f5' }}>
                        <td style={tdStyle}>
                          <div style={{ fontWeight: 600 }}>
                            Level {h.level} {h.days} {h.time}
                          </div>
                          <div style={{ color: '#666', fontSize: '12px' }}>{h.class_key}</div>
                        </td>
                        <td style={tdStyle}>{formatDate(h.start_date)}</td>
                        <td style={tdStyle}>{formatDate(h.end_date)}</td>
                        <td style={tdStyle}>{h.duration}</td>
                        <td style={tdStyle}>{h.evaluation_score}%</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : activeTab === 'availability' ? (
              <div style={{ marginTop: '8px', border: '1px solid #dee2e6', borderRadius: '6px', background: '#fff', overflow: 'hidden' }}>
                <div style={{ padding: '12px 14px', borderBottom: '1px solid #dee2e6', display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
                  <strong>Submitted Availability</strong>
                  <input
                    type="month"
                    value={availabilityMonth}
                    onChange={(e) => changeAvailabilityMonth(e.target.value)}
                    style={{ padding: '7px 10px', border: '1px solid #ced4da', borderRadius: '6px' }}
                  />
                </div>
                {availabilityLoading ? (
                  <div style={{ padding: '14px', color: '#666' }}>Loading availability...</div>
                ) : availabilityError ? (
                  <div style={{ padding: '14px', color: '#721c24', background: '#f8d7da' }}>{availabilityError}</div>
                ) : availabilityWindows.length === 0 ? (
                  <div style={{ padding: '14px', color: '#666' }}>No availability submitted for {availabilityMonth}.</div>
                ) : (
                  <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                    <thead>
                      <tr style={{ background: '#f8f9fa', borderBottom: '1px solid #dee2e6' }}>
                        <th style={thStyle}>Date</th>
                        <th style={thStyle}>Time</th>
                        <th style={thStyle}>Note</th>
                      </tr>
                    </thead>
                    <tbody>
                      {availabilityWindows.map((window) => (
                        <tr key={window.id || `${window.available_date}:${window.start_time}:${window.end_time}`} style={{ borderBottom: '1px solid #f1f3f5' }}>
                          <td style={tdStyle}>{window.available_date}</td>
                          <td style={tdStyle}>
                            {window.start_time} - {window.end_time}
                          </td>
                          <td style={tdStyle}>{window.note || '-'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            ) : (
              <div style={{ marginTop: '8px', border: '1px solid #dee2e6', borderRadius: '6px', background: '#fff' }}>
                {profile.testimonials.length === 0 ? (
                  <div style={{ padding: '14px', color: '#666' }}>No testimonials yet.</div>
                ) : (
                  profile.testimonials.map((t) => (
                    <div key={t.id} style={{ padding: '12px 14px', borderBottom: '1px solid #f1f3f5' }}>
                      <div style={{ fontSize: '12px', color: '#666', marginBottom: '6px' }}>
                        {t.class_key} • by {t.created_by || '-'} • {formatDate(t.created_at)}
                      </div>
                      <div style={{ whiteSpace: 'pre-wrap' }}>{t.testimonial_text}</div>
                    </div>
                  ))
                )}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function KpiCard({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ border: '1px solid #dee2e6', borderRadius: '8px', padding: '12px', background: '#fff' }}>
      <div style={{ fontSize: '12px', color: '#666' }}>{label}</div>
      <div style={{ fontSize: '24px', fontWeight: 700, marginTop: '4px' }}>{value}</div>
    </div>
  )
}

const thStyle: React.CSSProperties = {
  textAlign: 'left',
  padding: '10px 12px',
  fontSize: '13px',
  fontWeight: 700,
}

const tdStyle: React.CSSProperties = {
  padding: '10px 12px',
  fontSize: '14px',
}

const metaPill: React.CSSProperties = {
  background: '#f1f3f5',
  borderRadius: '999px',
  padding: '6px 10px',
  fontSize: '13px',
}

const tabBtn: React.CSSProperties = {
  border: '1px solid #dee2e6',
  borderRadius: '6px',
  background: '#fff',
  padding: '8px 12px',
  cursor: 'pointer',
  fontWeight: 600,
}

const activeTabBtn: React.CSSProperties = {
  background: '#e7f5ff',
  borderColor: '#74c0fc',
  color: '#1864ab',
}
