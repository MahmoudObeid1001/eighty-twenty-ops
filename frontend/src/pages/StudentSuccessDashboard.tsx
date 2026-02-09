import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type StudentSuccessClass, type PlacementTestQueueItem } from '../api/client'

interface Group {
  mentor_id?: string
  mentor_email?: string
  mentor_name?: string
  classes: StudentSuccessClass[]
}

export default function StudentSuccessDashboard() {
  const [classes, setClasses] = useState<StudentSuccessClass[]>([])
  const [placementTests, setPlacementTests] = useState<PlacementTestQueueItem[]>([])
  const [loading, setLoading] = useState(true)
  const [placementLoading, setPlacementLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [placementError, setPlacementError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'classes' | 'placement_tests'>('classes')
  const [showCompletedTests, setShowCompletedTests] = useState(false)
  const [pendingPlacementCount, setPendingPlacementCount] = useState(0)
  const [resultModal, setResultModal] = useState<{
    open: boolean
    item: PlacementTestQueueItem | null
    error?: string
  }>({ open: false, item: null })
  const [assignedLevel, setAssignedLevel] = useState<number | ''>('')
  const [testNotes, setTestNotes] = useState('')
  const navigate = useNavigate()

  useEffect(() => {
    loadData()
  }, [])

  async function loadData() {
    try {
      setLoading(true)
      setError(null)
      const me = await api.getMe()
      if (me.role !== 'student_success') {
        setError('No access. Student Success only.')
        setLoading(false)
        return
      }
      const data = await api.getStudentSuccessClasses()
      setClasses(data.classes)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load classes')
    } finally {
      setLoading(false)
    }
  }

  async function loadPlacementTestsCount() {
    try {
      const data = await api.getStudentSuccessPlacementTests(false)
      setPendingPlacementCount(data.placement_tests?.length || 0)
    } catch {
      // Silent failure: don't block dashboard
      setPendingPlacementCount(0)
    }
  }

  async function loadPlacementTests() {
    try {
      setPlacementLoading(true)
      setPlacementError(null)
      const data = await api.getStudentSuccessPlacementTests(showCompletedTests)
      setPlacementTests(data.placement_tests || [])
      if (!showCompletedTests) {
        setPendingPlacementCount(data.placement_tests?.length || 0)
      }
    } catch (err) {
      setPlacementError(err instanceof Error ? err.message : 'Failed to load placement tests')
    } finally {
      setPlacementLoading(false)
    }
  }

  useEffect(() => {
    loadPlacementTestsCount()
  }, [])

  useEffect(() => {
    if (activeTab === 'placement_tests') {
      loadPlacementTests()
    }
  }, [activeTab, showCompletedTests])

  function groupByMentor(list: StudentSuccessClass[]): Group[] {
    const mentorMap = new Map<string, Group>()
    const unassigned: Group = { classes: [] }

    for (const c of list) {
      if (c.mentor_user_id && (c.mentor_email || c.mentor_name)) {
        const key = c.mentor_user_id
        if (!mentorMap.has(key)) {
          mentorMap.set(key, {
            mentor_id: key,
            mentor_email: c.mentor_email,
            mentor_name: c.mentor_name,
            classes: [],
          })
        }
        mentorMap.get(key)!.classes.push(c)
      } else {
        unassigned.classes.push(c)
      }
    }

    const out: Group[] = []
    if (unassigned.classes.length > 0) out.push(unassigned)
    mentorMap.forEach((g) => out.push(g))
    return out
  }

  if (loading) {
    return (
      <div style={{ padding: '40px', textAlign: 'center' }}>
        <p>Loading...</p>
      </div>
    )
  }

  if (error) {
    return (
      <div style={{ padding: '40px' }}>
        <div style={{ background: '#f8d7da', padding: '16px', borderRadius: '8px', color: '#721c24' }}>
          <strong>Error:</strong> {error}
        </div>
      </div>
    )
  }

  const groups = groupByMentor(classes)
  const midRoundClasses = classes.filter((c) => c.mid_round_required)
  const endRoundClasses = classes.filter((c) => c.end_round_required)
  const complianceDueClasses = classes.filter((c) => c.compliance_required)

  return (
    <div>
      <div className="header content-header">
        <img src="/static/logo/eighty-twenty-logo.png" alt="" className="app-logo" />
        <h1>Student Success Dashboard</h1>
      </div>

      {midRoundClasses.length > 0 && (
        <div style={{ background: '#fff3cd', border: '1px solid #ffeeba', color: '#856404', padding: '16px', borderRadius: '8px', marginBottom: '16px' }}>
          <strong style={{ display: 'block', fontSize: '16px' }}>Mid-Round Feedback Required!</strong>
          <div style={{ fontSize: '14px', marginTop: '4px' }}>
            {midRoundClasses.length} class{midRoundClasses.length !== 1 ? 'es' : ''} reached Session 4 and still need mid-round feedback.
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginTop: '12px' }}>
            {midRoundClasses.map((cls) => (
              <button
                key={cls.class_key}
                onClick={() => navigate(`/student-success/class?class_key=${encodeURIComponent(cls.class_key)}&tab=feedback`)}
                style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #856404', background: '#fff', color: '#856404', cursor: 'pointer', fontSize: '12px', fontWeight: 600 }}
              >
                Level {cls.level} · Class {cls.class_number} · Go to Feedbacks
              </button>
            ))}
          </div>
        </div>
      )}

      {endRoundClasses.length > 0 && (
        <div style={{ background: '#fff3cd', border: '1px solid #ffeeba', color: '#856404', padding: '16px', borderRadius: '8px', marginBottom: '16px' }}>
          <strong style={{ display: 'block', fontSize: '16px' }}>End-of-Round Feedback Required!</strong>
          <div style={{ fontSize: '14px', marginTop: '4px' }}>
            {endRoundClasses.length} class{endRoundClasses.length !== 1 ? 'es' : ''} reached Session 8 and still need final feedback.
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginTop: '12px' }}>
            {endRoundClasses.map((cls) => (
              <button
                key={cls.class_key}
                onClick={() => navigate(`/student-success/class?class_key=${encodeURIComponent(cls.class_key)}&tab=feedback`)}
                style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #856404', background: '#fff', color: '#856404', cursor: 'pointer', fontSize: '12px', fontWeight: 600 }}
              >
                Level {cls.level} · Class {cls.class_number} · Go to Feedbacks
              </button>
            ))}
          </div>
        </div>
      )}

      {complianceDueClasses.length > 0 && (
        <div style={{ background: '#fde2e2', border: '1px solid #f5b5b5', color: '#8a1f1f', padding: '16px', borderRadius: '8px', marginBottom: '16px' }}>
          <strong style={{ display: 'block', fontSize: '16px' }}>Compliance Checklist Required!</strong>
          <div style={{ fontSize: '14px', marginTop: '4px' }}>
            {complianceDueClasses.length} class{complianceDueClasses.length !== 1 ? 'es' : ''} finished Session 8 and still have incomplete compliance checks.
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginTop: '12px' }}>
            {complianceDueClasses.map((cls) => (
              <button
                key={cls.class_key}
                onClick={() => navigate(`/student-success/class?class_key=${encodeURIComponent(cls.class_key)}&open_compliance=1`)}
                style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #8a1f1f', background: '#fff', color: '#8a1f1f', cursor: 'pointer', fontSize: '12px', fontWeight: 600 }}
              >
                Level {cls.level} · Class {cls.class_number} · Complete Checklist ({cls.compliance_done ?? 0}/{cls.compliance_total ?? 8})
              </button>
            ))}
          </div>
        </div>
      )}

      {pendingPlacementCount > 0 && (
        <div style={{ background: '#E6F7FF', border: '2px solid #4EC6E0', color: '#0052A3', padding: '12px 16px', borderRadius: '8px', marginBottom: '16px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '12px' }}>
          <div>
            <strong>{pendingPlacementCount}</strong> new placement test{pendingPlacementCount !== 1 ? 's' : ''} need results.
          </div>
          <button
            onClick={() => setActiveTab('placement_tests')}
            style={{ padding: '6px 12px', borderRadius: '6px', border: '1px solid #0052A3', background: '#fff', color: '#0052A3', cursor: 'pointer', fontWeight: 600 }}
          >
            View Placement Tests
          </button>
        </div>
      )}

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px', gap: '16px', flexWrap: 'wrap' }}>
        <p style={{ margin: 0, color: '#666' }}>
          {activeTab === 'classes'
            ? 'Active classes only (round started). Grouped by mentor.'
            : 'Leads with placement tests scheduled by Ops. Record level and notes after the test.'}
        </p>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button
            onClick={() => setActiveTab('classes')}
            style={{
              padding: '8px 14px',
              borderRadius: '6px',
              border: activeTab === 'classes' ? '1px solid #007bff' : '1px solid #ddd',
              background: activeTab === 'classes' ? '#e7f1ff' : '#fff',
              color: activeTab === 'classes' ? '#007bff' : '#333',
              cursor: 'pointer',
              fontWeight: 600,
            }}
          >
            Classes
          </button>
          <button
            onClick={() => setActiveTab('placement_tests')}
            style={{
              padding: '8px 14px',
              borderRadius: '6px',
              border: activeTab === 'placement_tests' ? '1px solid #007bff' : '1px solid #ddd',
              background: activeTab === 'placement_tests' ? '#e7f1ff' : '#fff',
              color: activeTab === 'placement_tests' ? '#007bff' : '#333',
              cursor: 'pointer',
              fontWeight: 600,
            }}
          >
            Placement Tests
          </button>
        </div>
      </div>

      <div>
        {activeTab === 'classes' && (
          <>
            {groups.length === 0 ? (
              <div style={{ padding: '24px', background: '#f9f9f9', borderRadius: '8px', textAlign: 'center' }}>
                <p>No active classes.</p>
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
                {groups.map((grp) => (
                  <div key={grp.mentor_id ?? 'unassigned'}>
                    <h2 style={{ fontSize: '16px', marginBottom: '12px', color: '#333' }}>
                      {grp.mentor_email ?? grp.mentor_name ?? 'Unassigned'}
                    </h2>
                    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: '16px' }}>
                      {grp.classes.map((cls) => (
                        <div
                          key={cls.class_key}
                          style={{
                            padding: '16px',
                            borderRadius: '6px',
                            border: cls.has_high_priority ? '1px solid #dc3545' : '1px solid #eee',
                            background: cls.has_high_priority ? '#fffafa' : '#fff',
                          }}
                        >
                          <div style={{ marginBottom: '12px' }}>
                            <h3 style={{ fontSize: '16px', marginBottom: '6px' }}>
                              Level {cls.level} · Class {cls.class_number}
                            </h3>
                            <p style={{ color: '#666', fontSize: '13px', marginBottom: '4px' }}>
                              {cls.days} · {cls.time}
                            </p>
                            <p style={{ color: '#666', fontSize: '13px', marginBottom: '8px' }}>
                              {cls.student_count} student{cls.student_count !== 1 ? 's' : ''}
                            </p>
                            <span
                              style={{
                                display: 'inline-block',
                                padding: '4px 10px',
                                borderRadius: '12px',
                                fontSize: '12px',
                                fontWeight: 600,
                                background: '#d4edda',
                                color: '#155724',
                                marginRight: '8px',
                              }}
                            >
                              ACTIVE
                            </span>
                            {cls.mid_round_required && (
                              <span
                                style={{
                                  display: 'inline-block',
                                  padding: '4px 10px',
                                  borderRadius: '12px',
                                  fontSize: '12px',
                                  fontWeight: 600,
                                  background: '#fff3cd',
                                  color: '#856404',
                                  marginRight: '8px',
                                }}
                              >
                                MID FEEDBACK DUE
                              </span>
                            )}
                            {cls.end_round_required && (
                              <span
                                style={{
                                  display: 'inline-block',
                                  padding: '4px 10px',
                                  borderRadius: '12px',
                                  fontSize: '12px',
                                  fontWeight: 600,
                                  background: '#ffe8a1',
                                  color: '#856404',
                                  marginRight: '8px',
                                }}
                              >
                                FINAL FEEDBACK DUE
                              </span>
                            )}
                            {cls.compliance_required && (
                              <span
                                style={{
                                  display: 'inline-block',
                                  padding: '4px 10px',
                                  borderRadius: '12px',
                                  fontSize: '12px',
                                  fontWeight: 600,
                                  background: '#fde2e2',
                                  color: '#8a1f1f',
                                  marginRight: '8px',
                                }}
                              >
                                COMPLIANCE DUE ({cls.compliance_done ?? 0}/{cls.compliance_total ?? 8})
                              </span>
                            )}
                            {cls.has_high_priority && (
                              <span
                                style={{
                                  display: 'inline-block',
                                  padding: '4px 10px',
                                  borderRadius: '12px',
                                  fontSize: '12px',
                                  fontWeight: 600,
                                  background: '#f8d7da',
                                  color: '#721c24',
                                }}
                                title={cls.high_priority_reason}
                              >
                                🚩 AT RISK
                              </span>
                            )}
                          </div>
                          <button
                            onClick={() => navigate(`/student-success/class?class_key=${encodeURIComponent(cls.class_key)}`)}
                            style={{
                              width: '100%',
                              padding: '8px',
                              background: '#007bff',
                              color: 'white',
                              border: 'none',
                              borderRadius: '4px',
                              cursor: 'pointer',
                              fontSize: '13px',
                            }}
                          >
                            Open Class
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </>
        )}

        {activeTab === 'placement_tests' && (
          <>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
              <div />
              <label style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '14px', color: '#333' }}>
                <input
                  type="checkbox"
                  checked={showCompletedTests}
                  onChange={(e) => setShowCompletedTests(e.target.checked)}
                />
                Show completed tests
              </label>
            </div>
            {placementError && (
              <div style={{ padding: '12px 16px', background: '#f8d7da', color: '#721c24', borderRadius: '8px', marginBottom: '16px' }}>
                <strong>Error:</strong> {placementError}
              </div>
            )}

            {placementLoading ? (
              <div style={{ padding: '24px', textAlign: 'center' }}>Loading placement tests...</div>
            ) : placementTests.length === 0 ? (
              <div style={{ padding: '24px', background: '#f9f9f9', borderRadius: '8px', textAlign: 'center' }}>
                <p>{showCompletedTests ? 'No completed placement tests.' : 'No placement tests waiting.'}</p>
              </div>
            ) : (
              <div style={{ background: '#fff', border: '1px solid #eee', borderRadius: '8px', overflow: 'hidden' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '14px' }}>
                  <thead>
                    <tr style={{ textAlign: 'left', background: '#f8f9fa' }}>
                      <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Lead</th>
                      <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Test Date</th>
                      <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Test Time</th>
                      <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Type</th>
                      <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {placementTests.map((item) => (
                      <tr key={item.lead_id} style={{ borderBottom: '1px solid #eee' }}>
                        <td style={{ padding: '12px' }}>
                          <div style={{ fontWeight: 600 }}>{item.full_name}</div>
                          <div style={{ fontSize: '12px', color: '#666' }}>{item.phone}</div>
                        </td>
                        <td style={{ padding: '12px' }}>{item.test_date || '-'}</td>
                        <td style={{ padding: '12px' }}>{item.test_time || '-'}</td>
                        <td style={{ padding: '12px' }}>{item.test_type || '-'}</td>
                        <td style={{ padding: '12px' }}>
                          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                            <button
                              onClick={() => {
                                setResultModal({ open: true, item, error: undefined })
                                setAssignedLevel(item.assigned_level ?? '')
                                setTestNotes(item.test_notes ?? '')
                              }}
                              style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #007bff', background: '#fff', color: '#007bff', cursor: 'pointer', fontSize: '12px' }}
                            >
                              Record Result
                            </button>
                            <a
                              href={`/pre-enrolment/${item.lead_id}`}
                              target="_blank"
                              rel="noopener noreferrer"
                              style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid #6c757d', background: '#fff', color: '#6c757d', cursor: 'pointer', fontSize: '12px', textDecoration: 'none', display: 'inline-flex', alignItems: 'center' }}
                            >
                              Open Lead
                            </a>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </div>

      {resultModal.open && resultModal.item && (
        <div
          style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 3000 }}
          onClick={() => setResultModal({ open: false, item: null })}
        >
          <div
            style={{ background: 'white', padding: '24px', borderRadius: '12px', width: '520px', maxWidth: '90%' }}
            onClick={(e) => e.stopPropagation()}
          >
            <h3 style={{ marginBottom: '16px' }}>Record Placement Test Result</h3>
            <p style={{ fontSize: '14px', color: '#666', marginBottom: '16px' }}>
              Lead: <strong>{resultModal.item.full_name}</strong> ({resultModal.item.phone})
            </p>

            {resultModal.error && (
              <div style={{ color: '#721c24', background: '#f8d7da', padding: '8px 12px', borderRadius: '6px', marginBottom: '12px', fontSize: '13px' }}>
                {resultModal.error}
              </div>
            )}

            <div style={{ marginBottom: '16px' }}>
              <label style={{ display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '6px' }}>Assigned Level *</label>
              <select
                value={assignedLevel}
                onChange={(e) => setAssignedLevel(e.target.value ? Number(e.target.value) : '')}
                style={{ width: '100%', padding: '10px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px' }}
              >
                <option value="">Select level</option>
                {[1, 2, 3, 4, 5, 6, 7, 8].map((lvl) => (
                  <option key={lvl} value={lvl}>Level {lvl}</option>
                ))}
              </select>
            </div>

            <div style={{ marginBottom: '20px' }}>
              <label style={{ display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '6px' }}>Test Notes</label>
              <textarea
                value={testNotes}
                onChange={(e) => setTestNotes(e.target.value)}
                placeholder="Add observations and results..."
                style={{ width: '100%', height: '120px', padding: '12px', borderRadius: '6px', border: '1px solid #ddd', fontSize: '14px', resize: 'vertical' }}
              />
            </div>

            <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
              <button
                onClick={() => setResultModal({ open: false, item: null })}
                style={{ padding: '8px 16px', borderRadius: '6px', border: '1px solid #ddd', background: '#fff', cursor: 'pointer' }}
              >
                Cancel
              </button>
              <button
                onClick={async () => {
                  if (!resultModal.item) return
                  if (!assignedLevel) {
                    setResultModal((prev) => ({ ...prev, error: 'Please select an assigned level.' }))
                    return
                  }
                  try {
                    await api.completePlacementTest({
                      lead_id: resultModal.item.lead_id,
                      assigned_level: assignedLevel,
                      test_notes: testNotes,
                    })
                    setResultModal({ open: false, item: null })
                    setAssignedLevel('')
                    setTestNotes('')
                    loadPlacementTests()
                  } catch (err) {
                    setResultModal((prev) => ({ ...prev, error: err instanceof Error ? err.message : 'Failed to save result' }))
                  }
                }}
                style={{ padding: '8px 16px', borderRadius: '6px', border: 'none', background: '#28a745', color: '#fff', cursor: 'pointer', fontWeight: 600 }}
              >
                Save Result
              </button>
            </div>
          </div>
        </div>
      )}

    </div>
  )
}
