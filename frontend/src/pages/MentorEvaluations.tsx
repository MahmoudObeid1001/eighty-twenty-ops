import { useEffect, useMemo, useState } from 'react'
import { api } from '../api/client'

type AttendanceStatus = 'on-time' | 'late' | 'absent' | 'unknown'

interface MentorClassEvaluation {
  classKey: string
  level: number
  days: string
  time: string
  classNumber: number
  manual: {
    sessionQuality: number
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

function computeCollectiveKPI(classItem: MentorClassEvaluation): number {
  const punctuality = classItem.automatic.attendancePunctualityPercent
  const sessionQuality = classItem.manual.sessionQuality * 10
  const feedback = classItem.manual.studentsFeedback * 10
  const whatsapp = classItem.automatic.whatsAppManagementPercent
  const trello = classItem.manual.trelloCompliancePercent

  const weighted =
    punctuality*0.25 +
    sessionQuality*0.25 +
    feedback*0.20 +
    whatsapp*0.10 +
    trello*0.20

  return Math.round(weighted)
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

export default function MentorEvaluations() {
  const [mentors, setMentors] = useState<MentorEvaluationItem[]>([])
  const [expandedMentors, setExpandedMentors] = useState<Record<string, boolean>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [editing, setEditing] = useState<EditTarget | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [testimonialTarget, setTestimonialTarget] = useState<TestimonialTarget | null>(null)
  const [savingTestimonial, setSavingTestimonial] = useState(false)
  const [testimonialError, setTestimonialError] = useState<string | null>(null)
  const [testimonialSuccess, setTestimonialSuccess] = useState<string | null>(null)

  useEffect(() => {
    void load()
  }, [])

  async function load() {
    try {
      setLoading(true)
      setError(null)
      const data = await api.getMentorEvaluations()
      setMentors(data.mentors as MentorEvaluationItem[])
      const openByDefault: Record<string, boolean> = {}
      for (const mentor of data.mentors) {
        openByDefault[mentor.id] = true
      }
      setExpandedMentors(openByDefault)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load mentor evaluations')
    } finally {
      setLoading(false)
    }
  }

  function toggleMentor(mentorId: string) {
    setExpandedMentors((prev) => ({ ...prev, [mentorId]: !prev[mentorId] }))
  }

  async function saveClassManualEvaluation(payload: {
    mentorId: string
    classKey: string
    manual: { sessionQuality: number; studentsFeedback: number; trelloSessionChecks: boolean[] }
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
                classes: mentor.classes.map((cls) =>
                  cls.classKey !== payload.classKey
                    ? cls
                    : {
                        ...cls,
                        manual: {
                          ...cls.manual,
                          sessionQuality: res.manual.sessionQuality,
                          studentsFeedback: res.manual.studentsFeedback,
                          trelloSessionChecks: res.manual.trelloSessionChecks,
                          trelloCompliancePercent: res.manual.trelloCompliancePct,
                        },
                      }
                ),
              }
        )
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
        Evaluations are scoped per active class. Closed rounds are not shown here.
      </p>

      {testimonialSuccess && (
        <div style={{ marginBottom: '12px', padding: '10px 12px', borderRadius: '6px', background: '#d4edda', color: '#155724', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{testimonialSuccess}</span>
          <button onClick={() => setTestimonialSuccess(null)} style={{ background: 'none', border: 'none', fontSize: '18px', cursor: 'pointer', color: '#155724' }}>×</button>
        </div>
      )}

      {mentors.length === 0 ? (
        <p style={{ color: '#666' }}>No mentors with active classes.</p>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
          {mentors.map((mentor) => (
            <div key={mentor.id} style={{ border: '1px solid #dee2e6', borderRadius: '8px', background: '#fff' }}>
              {(() => {
                const classScores = mentor.classes.map(computeCollectiveKPI)
                const mentorCollective =
                  classScores.length > 0
                    ? Math.round(classScores.reduce((acc, s) => acc + s, 0) / classScores.length)
                    : 0
                return (
              <div
                style={{
                  width: '100%',
                  textAlign: 'left',
                  background: '#fff',
                  padding: '14px 16px',
                  borderRadius: '8px',
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                }}
              >
                <div onClick={() => toggleMentor(mentor.id)} style={{ cursor: 'pointer' }}>
                  <div style={{ fontWeight: 700 }}>{mentor.name}</div>
                  <div style={{ color: '#666', fontSize: '13px' }}>{mentor.email}</div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                  <span
                    style={{
                      padding: '4px 8px',
                      borderRadius: '4px',
                      background: '#e7f5ff',
                      color: '#1864ab',
                      fontSize: '12px',
                      fontWeight: 700,
                    }}
                  >
                    Collective KPI {mentorCollective}%
                  </span>
                  <span
                    style={{
                      padding: '4px 8px',
                      borderRadius: '4px',
                      background: '#d4edda',
                      color: '#155724',
                      fontSize: '12px',
                      fontWeight: 700,
                    }}
                  >
                    {mentor.activeClassCount} active {mentor.activeClassCount === 1 ? 'class' : 'classes'}
                  </span>
                  <button
                    onClick={() => {
                      setTestimonialError(null)
                      setTestimonialTarget({ mentorId: mentor.id, mentorName: mentor.name, classes: mentor.classes })
                    }}
                    style={{
                      border: '1px solid #1c7ed6',
                      borderRadius: '4px',
                      background: '#fff',
                      color: '#1c7ed6',
                      padding: '6px 10px',
                      cursor: 'pointer',
                      fontWeight: 600,
                    }}
                  >
                    Testimonials
                  </button>
                  <span onClick={() => toggleMentor(mentor.id)} style={{ color: '#666', cursor: 'pointer' }}>{expandedMentors[mentor.id] ? '▲' : '▼'}</span>
                </div>
              </div>
                )
              })()}

              {expandedMentors[mentor.id] && (
                <div style={{ padding: '0 16px 16px', display: 'grid', gap: '12px' }}>
                  {mentor.classes.map((cls) => (
                    <ClassCard
                      key={`${mentor.id}-${cls.classKey}`}
                      classItem={cls}
                      onEdit={() => setEditing({ mentorId: mentor.id, mentorName: mentor.name, classItem: cls })}
                    />
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
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

function ClassCard({ classItem, onEdit }: { classItem: MentorClassEvaluation; onEdit: () => void }) {
  const collective = computeCollectiveKPI(classItem)
  return (
    <div style={{ border: '1px solid #e9ecef', borderRadius: '8px', padding: '12px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
        <div>
          <strong>
            Level {classItem.level} • {classItem.days} • {classItem.time} • Class #{classItem.classNumber}
          </strong>
          <div style={{ color: '#666', fontSize: '12px' }}>{classItem.classKey}</div>
        </div>
        <button
          onClick={onEdit}
          style={{
            border: 'none',
            borderRadius: '4px',
            background: '#1c7ed6',
            color: '#fff',
            padding: '6px 10px',
            cursor: 'pointer',
          }}
        >
          Edit Class
        </button>
      </div>

      <div style={{ marginBottom: '10px' }}>
        <PercentBar label="Collective KPI Ratio (Weighted)" percent={collective} />
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '10px' }}>
        <ScoreBar label="Session Quality (Manual)" score={classItem.manual.sessionQuality} />
        <ScoreBar label="Students Feedback (Manual)" score={classItem.manual.studentsFeedback} />
        <PercentBar label="Trello Compliance (Manual)" percent={classItem.manual.trelloCompliancePercent} />
        <PercentBar label="WhatsApp Groups Management (Auto)" percent={classItem.automatic.whatsAppManagementPercent} />
        <PercentBar label="Attendance Punctuality (Auto)" percent={classItem.automatic.attendancePunctualityPercent} />
      </div>

      <div style={{ marginTop: '10px' }}>
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
  onSave: (manual: { sessionQuality: number; studentsFeedback: number; trelloSessionChecks: boolean[] }) => void
}) {
  const [sessionQuality, setSessionQuality] = useState(Math.max(1, classItem.manual.sessionQuality || 1))
  const [studentsFeedback, setStudentsFeedback] = useState(Math.max(1, classItem.manual.studentsFeedback || 1))
  const [trelloChecks, setTrelloChecks] = useState<boolean[]>(() => {
    const fixed = new Array(8).fill(false) as boolean[]
    const src = classItem.manual.trelloSessionChecks || []
    for (let i = 0; i < Math.min(8, src.length); i++) fixed[i] = !!src[i]
    return fixed
  })

  const trelloPercent = useMemo(
    () => Math.round((trelloChecks.filter(Boolean).length / 8) * 100),
    [trelloChecks]
  )
  const collectivePreview = useMemo(() => {
    const punctuality = classItem.automatic.attendancePunctualityPercent
    const sessionQualityPct = sessionQuality * 10
    const feedbackPct = studentsFeedback * 10
    const whatsapp = classItem.automatic.whatsAppManagementPercent
    return Math.round(
      punctuality*0.25 +
      sessionQualityPct*0.25 +
      feedbackPct*0.20 +
      whatsapp*0.10 +
      trelloPercent*0.20
    )
  }, [classItem.automatic.attendancePunctualityPercent, classItem.automatic.whatsAppManagementPercent, sessionQuality, studentsFeedback, trelloPercent])

  function toggleSession(index: number) {
    setTrelloChecks((prev) => prev.map((v, i) => (i === index ? !v : v)))
  }

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
        style={{ width: '92%', maxWidth: '680px', background: '#fff', borderRadius: '8px', padding: '16px' }}
      >
        <div style={{ marginBottom: '10px' }}>
          <h3 style={{ margin: 0 }}>Edit Class Evaluation</h3>
          <div style={{ color: '#666', fontSize: '13px', marginTop: '4px' }}>
            {mentorName} • Level {classItem.level} • {classItem.days} • {classItem.time} • Class #{classItem.classNumber}
          </div>
        </div>

        {error && <div style={{ color: '#721c24', background: '#f8d7da', padding: '8px', borderRadius: '4px' }}>{error}</div>}

        <div style={{ marginTop: '12px', display: 'grid', gap: '14px' }}>
          <div>
            <PercentBar label="Collective KPI Ratio (Preview)" percent={collectivePreview} />
          </div>
          <div>
            <label style={{ display: 'block', marginBottom: '6px' }}>Session Quality (1-10): {sessionQuality}</label>
            <input type="range" min={1} max={10} value={sessionQuality} onChange={(e) => setSessionQuality(parseInt(e.target.value, 10))} style={{ width: '100%' }} />
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
          <button onClick={onClose} disabled={saving} style={{ padding: '8px 12px' }}>
            Cancel
          </button>
          <button
            disabled={saving}
            onClick={() => onSave({ sessionQuality, studentsFeedback, trelloSessionChecks: trelloChecks })}
            style={{ padding: '8px 12px', background: '#1c7ed6', border: 'none', color: '#fff', borderRadius: '4px' }}
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
        <div style={{ color: '#666', fontSize: '13px', marginBottom: '12px' }}>
          {mentorName}
        </div>

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
          <button onClick={onClose} disabled={saving} style={{ padding: '8px 12px' }}>
            Cancel
          </button>
          <button
            disabled={saving || !classKey || !testimonialText.trim()}
            onClick={() => onSave({ classKey, testimonialText })}
            style={{ padding: '8px 12px', background: '#1c7ed6', border: 'none', color: '#fff', borderRadius: '4px' }}
          >
            {saving ? 'Saving...' : 'Save Testimonial'}
          </button>
        </div>
      </div>
    </div>
  )
}
