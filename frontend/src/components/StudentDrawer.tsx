import { useEffect, useState } from 'react'
import { api, Student, Note } from '../api/client'
import NotesSection from './NotesSection'

interface StudentDrawerProps {
  student: Student | null
  classKey: string
  sessionsCount: number
  onClose: () => void
}

export default function StudentDrawer({ student, classKey, sessionsCount, onClose }: StudentDrawerProps) {
  const [notes, setNotes] = useState<Note[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (student) {
      loadNotes()
    } else {
      setNotes([])
    }
  }, [student, classKey])

  async function loadNotes() {
    if (!student) return
    try {
      setLoading(true)
      setError(null)
      const data = await api.getNotes(student.lead_id, classKey)
      setNotes(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load notes')
    } finally {
      setLoading(false)
    }
  }

  async function handleAddNote(text: string) {
    if (!student) return
    try {
      const newNote = await api.createNote(student.lead_id, classKey, text)
      setNotes([newNote, ...notes])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add note')
      throw err
    }
  }

  async function handleDeleteNote(noteId: string) {
    try {
      await api.deleteNote(noteId)
      setNotes(notes.filter((n) => n.id !== noteId))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete note')
    }
  }

  if (!student) return null

  return (
    <div
      style={{
        position: 'fixed',
        right: 0,
        top: 0,
        bottom: 0,
        width: '400px',
        background: 'white',
        boxShadow: '-2px 0 8px rgba(0,0,0,0.1)',
        zIndex: 1000,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        borderLeft: '1px solid #ddd',
      }}
    >
      <div style={{ padding: '20px', borderBottom: '1px solid #ddd', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h2 style={{ fontSize: '18px', marginBottom: '4px' }}>{student.full_name}</h2>
          <p style={{ fontSize: '14px', color: '#666' }}>{student.phone}</p>
        </div>
        <button
          onClick={onClose}
          style={{
            background: 'none',
            border: 'none',
            fontSize: '24px',
            cursor: 'pointer',
            color: '#666',
            padding: '0 8px',
          }}
        >
          ×
        </button>
      </div>

      <div style={{ flex: 1, overflowY: 'auto', padding: '20px' }}>
        {sessionsCount === 0 && (
          <div style={{ marginBottom: '20px', padding: '12px', background: '#fff3cd', borderRadius: '6px', border: '1px solid #ffc107' }}>
            <p style={{ fontSize: '13px', color: '#856404', margin: 0 }}>
              Round not started yet
            </p>
          </div>
        )}

        {error && (
          <div style={{ marginBottom: '16px', padding: '12px', background: '#fee', borderRadius: '6px', color: '#c33' }}>
            <p style={{ fontSize: '13px', margin: 0 }}>{error}</p>
          </div>
        )}

        <NotesSection
          notes={notes}
          loading={loading}
          onAddNote={handleAddNote}
          onDeleteNote={handleDeleteNote}
        />
      </div>
    </div>
  )
}
