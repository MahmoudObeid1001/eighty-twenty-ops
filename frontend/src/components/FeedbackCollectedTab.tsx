import { useEffect, useState } from 'react'
import { api, type FeedbackCollectedUpload } from '../api/client'

type FeedbackCollectedStudent = {
  lead_id: string
  full_name: string
  phone: string
}

export default function FeedbackCollectedTab({
  classKey,
  students,
  canEdit,
}: {
  classKey: string
  students: FeedbackCollectedStudent[]
  canEdit: boolean
}) {
  const [uploads, setUploads] = useState<FeedbackCollectedUpload[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedFile, setSelectedFile] = useState<Record<string, File | null>>({})
  const [selectedSession, setSelectedSession] = useState<Record<string, string>>({})
  const [uploading, setUploading] = useState<Record<string, boolean>>({})
  const [deleting, setDeleting] = useState<Record<string, boolean>>({})
  const [noteModalStudent, setNoteModalStudent] = useState<FeedbackCollectedStudent | null>(null)
  const [noteModalSession, setNoteModalSession] = useState('')
  const [noteModalText, setNoteModalText] = useState('')
  const [savingNote, setSavingNote] = useState(false)

  useEffect(() => {
    loadUploads()
  }, [classKey])

  async function loadUploads() {
    try {
      setLoading(true)
      setError(null)
      const res = await api.getFeedbackCollected(classKey)
      setUploads(res.uploads || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load feedback uploads')
    } finally {
      setLoading(false)
    }
  }

  async function handleUpload(leadId: string) {
    if (!canEdit) return
    const file = selectedFile[leadId]
    if (!file) return

    setUploading((prev) => ({ ...prev, [leadId]: true }))
    setError(null)
    try {
      const formData = new FormData()
      formData.append('class_key', classKey)
      formData.append('lead_id', leadId)
      if (selectedSession[leadId]) formData.append('session_number', selectedSession[leadId])
      formData.append('file', file)

      await api.uploadFeedbackCollected(formData)
      setSelectedFile((prev) => ({ ...prev, [leadId]: null }))
      setSelectedSession((prev) => ({ ...prev, [leadId]: '' }))
      await loadUploads()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to upload feedback')
    } finally {
      setUploading((prev) => ({ ...prev, [leadId]: false }))
    }
  }

  function openNoteModal(student: FeedbackCollectedStudent) {
    setNoteModalStudent(student)
    setNoteModalSession('')
    setNoteModalText('')
    setError(null)
  }

  function closeNoteModal() {
    if (savingNote) return
    setNoteModalStudent(null)
    setNoteModalSession('')
    setNoteModalText('')
  }

  async function handleSaveNote() {
    if (!canEdit || !noteModalStudent) return
    const trimmed = noteModalText.trim()
    if (!trimmed) {
      setError('Feedback note cannot be empty.')
      return
    }

    setSavingNote(true)
    setError(null)
    try {
      const formData = new FormData()
      formData.append('class_key', classKey)
      formData.append('lead_id', noteModalStudent.lead_id)
      if (noteModalSession) formData.append('session_number', noteModalSession)
      formData.append('note', trimmed)

      await api.uploadFeedbackCollected(formData)
      closeNoteModal()
      await loadUploads()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save feedback note')
    } finally {
      setSavingNote(false)
    }
  }

  async function handleDelete(uploadId: string) {
    if (!canEdit) return
    if (!window.confirm('Remove this uploaded feedback?')) return
    setDeleting((prev) => ({ ...prev, [uploadId]: true }))
    setError(null)
    try {
      await api.deleteFeedbackCollected(uploadId)
      await loadUploads()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete feedback')
    } finally {
      setDeleting((prev) => ({ ...prev, [uploadId]: false }))
    }
  }

  const uploadsByLead = uploads.reduce((acc: Record<string, FeedbackCollectedUpload[]>, item) => {
    if (!acc[item.lead_id]) acc[item.lead_id] = []
    acc[item.lead_id].push(item)
    return acc
  }, {})

  return (
    <div style={{ background: 'white', borderRadius: '12px', border: '1px solid #dee2e6', overflow: 'hidden' }}>
      <div style={{ padding: '16px', borderBottom: '1px solid #eee', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2 style={{ fontSize: '18px', margin: 0 }}>Feedback Collected</h2>
        {!canEdit && <span style={{ fontSize: '12px', color: '#999' }}>Read-only</span>}
      </div>

      {error && (
        <div style={{ padding: '12px 16px', background: '#f8d7da', color: '#721c24', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{error}</span>
          <button onClick={() => setError(null)} style={{ background: 'none', border: 'none', fontSize: '18px', cursor: 'pointer', color: '#721c24' }}>
            ×
          </button>
        </div>
      )}

      {loading ? (
        <div style={{ padding: '24px', textAlign: 'center', color: '#666' }}>Loading uploads...</div>
      ) : (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '14px' }}>
            <thead>
              <tr style={{ textAlign: 'left', background: '#f8f9fa' }}>
                <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Student</th>
                <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Feedback</th>
                {canEdit && <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Add Upload</th>}
              </tr>
            </thead>
            <tbody>
              {students.map((student) => {
                const studentUploads = uploadsByLead[student.lead_id] || []

                return (
                  <tr key={student.lead_id} style={{ borderBottom: '1px solid #eee' }}>
                    <td style={{ padding: '12px', width: '240px' }}>
                      <div style={{ fontWeight: 600 }}>{student.full_name}</div>
                      <div style={{ fontSize: '12px', color: '#666' }}>{student.phone}</div>
                    </td>
                    <td style={{ padding: '12px' }}>
                      {studentUploads.length === 0 ? (
                        <span style={{ color: '#999' }}>No uploads yet</span>
                      ) : (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                          {studentUploads.map((upload) => (
                            <div
                              key={upload.id}
                              style={{
                                display: 'flex',
                                alignItems: 'flex-start',
                                gap: '10px',
                                padding: '10px 12px',
                                border: '1px solid #e5e7eb',
                                borderRadius: '8px',
                                background: '#fff',
                              }}
                            >
                              <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '4px' }}>
                                {upload.file_url ? (
                                  <a href={upload.file_url} target="_blank" rel="noreferrer" style={{ color: '#007bff', fontWeight: 600 }}>
                                    {upload.file_name}
                                  </a>
                                ) : (
                                  <span style={{ color: '#111827', fontWeight: 600 }}>Written feedback</span>
                                )}
                                <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                                  {upload.session_number && <span style={{ fontSize: '12px', color: '#666' }}>S{upload.session_number}</span>}
                                  {upload.uploaded_by && <span style={{ fontSize: '12px', color: '#666' }}>{upload.uploaded_by}</span>}
                                </div>
                                {upload.note && <span style={{ fontSize: '12px', color: '#374151', whiteSpace: 'pre-wrap' }}>{upload.note}</span>}
                              </div>
                              {canEdit && (
                                <button
                                  onClick={() => handleDelete(upload.id)}
                                  disabled={deleting[upload.id]}
                                  style={{ padding: '4px 8px', borderRadius: '4px', border: '1px solid #dc3545', background: 'white', color: '#dc3545', fontSize: '11px', cursor: 'pointer' }}
                                >
                                  {deleting[upload.id] ? 'Removing...' : 'Remove'}
                                </button>
                              )}
                            </div>
                          ))}
                        </div>
                      )}
                    </td>
                    {canEdit && (
                      <td style={{ padding: '12px', width: '320px' }}>
                        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                          <input
                            type="file"
                            onChange={(e) => setSelectedFile((prev) => ({ ...prev, [student.lead_id]: e.target.files?.[0] || null }))}
                          />
                          <div style={{ display: 'flex', gap: '8px' }}>
                            <select
                              value={selectedSession[student.lead_id] || ''}
                              onChange={(e) => setSelectedSession((prev) => ({ ...prev, [student.lead_id]: e.target.value }))}
                              style={{ padding: '6px 8px', borderRadius: '6px', border: '1px solid #ddd', flex: 1 }}
                            >
                              <option value="">Session</option>
                            {[4, 8].map((s) => (
                              <option key={s} value={s}>
                                S{s}
                              </option>
                            ))}
                            </select>
                          </div>
                          <div style={{ display: 'flex', gap: '8px' }}>
                            <button
                              onClick={() => handleUpload(student.lead_id)}
                              disabled={uploading[student.lead_id] || !selectedFile[student.lead_id]}
                              style={{
                                padding: '6px 10px',
                                borderRadius: '6px',
                                border: '1px solid #007bff',
                                background: uploading[student.lead_id] ? '#ccc' : '#007bff',
                                color: 'white',
                                fontWeight: 600,
                                cursor: uploading[student.lead_id] ? 'not-allowed' : 'pointer',
                                flex: 1,
                              }}
                            >
                              {uploading[student.lead_id] ? 'Uploading...' : 'Upload File'}
                            </button>
                            <button
                              onClick={() => openNoteModal(student)}
                              style={{
                                padding: '6px 10px',
                                borderRadius: '6px',
                                border: '1px solid #0f766e',
                                background: 'white',
                                color: '#0f766e',
                                fontWeight: 600,
                                cursor: 'pointer',
                                flex: 1,
                              }}
                            >
                              Add Note
                            </button>
                          </div>
                        </div>
                      </td>
                    )}
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {noteModalStudent && (
        <div
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(15, 23, 42, 0.45)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 1000,
            padding: '24px',
          }}
        >
          <div
            style={{
              width: '100%',
              maxWidth: '760px',
              background: 'white',
              borderRadius: '12px',
              border: '1px solid #d1d5db',
              boxShadow: '0 20px 40px rgba(15, 23, 42, 0.2)',
              overflow: 'hidden',
            }}
          >
            <div style={{ padding: '18px 20px', borderBottom: '1px solid #e5e7eb', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <div style={{ fontSize: '18px', fontWeight: 700 }}>Add Feedback Note</div>
                <div style={{ fontSize: '13px', color: '#6b7280', marginTop: '4px' }}>{noteModalStudent.full_name}</div>
              </div>
              <button
                type="button"
                onClick={closeNoteModal}
                disabled={savingNote}
                style={{ border: 'none', background: 'transparent', fontSize: '24px', cursor: savingNote ? 'not-allowed' : 'pointer', color: '#6b7280' }}
              >
                ×
              </button>
            </div>

            <div style={{ padding: '20px', display: 'grid', gap: '14px' }}>
              <div>
                <label style={{ display: 'block', fontSize: '13px', fontWeight: 600, marginBottom: '6px' }}>Session</label>
                <select
                  value={noteModalSession}
                  onChange={(e) => setNoteModalSession(e.target.value)}
                  style={{ width: '180px', padding: '10px 12px', borderRadius: '8px', border: '1px solid #d1d5db' }}
                >
                  <option value="">No session tag</option>
                  {[4, 8].map((s) => (
                    <option key={s} value={s}>
                      Session {s}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '13px', fontWeight: 600, marginBottom: '6px' }}>Feedback</label>
                <textarea
                  value={noteModalText}
                  onChange={(e) => setNoteModalText(e.target.value)}
                  placeholder="Paste the student comment or write the collected feedback here..."
                  rows={10}
                  style={{
                    width: '100%',
                    minHeight: '260px',
                    resize: 'vertical',
                    padding: '12px 14px',
                    borderRadius: '8px',
                    border: '1px solid #d1d5db',
                    fontSize: '14px',
                    lineHeight: 1.5,
                    boxSizing: 'border-box',
                  }}
                />
              </div>
            </div>

            <div style={{ padding: '16px 20px', borderTop: '1px solid #e5e7eb', display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
              <button
                type="button"
                onClick={closeNoteModal}
                disabled={savingNote}
                style={{ padding: '10px 14px', borderRadius: '8px', border: '1px solid #d1d5db', background: 'white', cursor: savingNote ? 'not-allowed' : 'pointer' }}
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleSaveNote}
                disabled={savingNote || !noteModalText.trim()}
                style={{
                  padding: '10px 14px',
                  borderRadius: '8px',
                  border: '1px solid #007bff',
                  background: savingNote || !noteModalText.trim() ? '#93c5fd' : '#007bff',
                  color: 'white',
                  fontWeight: 600,
                  cursor: savingNote || !noteModalText.trim() ? 'not-allowed' : 'pointer',
                }}
              >
                {savingNote ? 'Saving...' : 'Save Feedback'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
