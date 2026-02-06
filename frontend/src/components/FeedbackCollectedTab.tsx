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
  const [note, setNote] = useState<Record<string, string>>({})
  const [uploading, setUploading] = useState<Record<string, boolean>>({})
  const [deleting, setDeleting] = useState<Record<string, boolean>>({})

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
      if (note[leadId]) formData.append('note', note[leadId])
      formData.append('file', file)

      await api.uploadFeedbackCollected(formData)
      setSelectedFile((prev) => ({ ...prev, [leadId]: null }))
      setNote((prev) => ({ ...prev, [leadId]: '' }))
      setSelectedSession((prev) => ({ ...prev, [leadId]: '' }))
      await loadUploads()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to upload feedback')
    } finally {
      setUploading((prev) => ({ ...prev, [leadId]: false }))
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
                <th style={{ padding: '12px', borderBottom: '1px solid #eee' }}>Uploads</th>
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
                            <div key={upload.id} style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                              <a href={upload.file_url} target="_blank" rel="noreferrer" style={{ color: '#007bff', fontWeight: 600 }}>
                                {upload.file_name}
                              </a>
                              {upload.session_number && <span style={{ fontSize: '12px', color: '#666' }}>S{upload.session_number}</span>}
                              {upload.note && <span style={{ fontSize: '12px', color: '#666' }}>{upload.note}</span>}
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
                            <input
                              type="text"
                              placeholder="Note (optional)"
                              value={note[student.lead_id] || ''}
                              onChange={(e) => setNote((prev) => ({ ...prev, [student.lead_id]: e.target.value }))}
                              style={{ padding: '6px 8px', borderRadius: '6px', border: '1px solid #ddd', flex: 2 }}
                            />
                          </div>
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
                            }}
                          >
                            {uploading[student.lead_id] ? 'Uploading...' : 'Upload'}
                          </button>
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
    </div>
  )
}
