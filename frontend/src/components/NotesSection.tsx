import { useState } from 'react'
import { Note } from '../api/client'

interface NotesSectionProps {
  notes: Note[]
  loading: boolean
  onAddNote: (text: string) => Promise<void>
  onDeleteNote: (noteId: string) => Promise<void>
}

export default function NotesSection({ notes, loading, onAddNote, onDeleteNote }: NotesSectionProps) {
  const [showHistory, setShowHistory] = useState(false)
  const [noteText, setNoteText] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!noteText.trim() || submitting) return
    try {
      setSubmitting(true)
      await onAddNote(noteText.trim())
      setNoteText('')
    } catch (err) {
      // Error handled by parent
    } finally {
      setSubmitting(false)
    }
  }

  const latestNote = notes.length > 0 ? notes[0] : null
  const historyNotes = notes.slice(1)

  return (
    <div>
      <h3 style={{ fontSize: '16px', marginBottom: '16px', color: '#333' }}>Notes</h3>

      {loading && (
        <p style={{ color: '#666', fontStyle: 'italic', marginBottom: '12px' }}>Loading notes...</p>
      )}

      {!loading && latestNote ? (
        <>
          <div
            style={{
              background: '#f8f9fa',
              padding: '12px',
              borderRadius: '6px',
              marginBottom: '12px',
              borderLeft: '3px solid #007bff',
            }}
          >
            <div style={{ fontSize: '11px', color: '#666', marginBottom: '4px', textTransform: 'uppercase', fontWeight: 600 }}>
              Latest Note
            </div>
            <div style={{ fontSize: '13px', color: '#333', marginBottom: '6px' }}>{latestNote.text}</div>
            <div style={{ fontSize: '11px', color: '#666' }}>
              {latestNote.created_by_email} · {new Date(latestNote.created_at).toLocaleString()}
            </div>
            <button
              onClick={() => onDeleteNote(latestNote.id)}
              style={{
                marginTop: '8px',
                padding: '4px 8px',
                background: '#dc3545',
                color: 'white',
                border: 'none',
                borderRadius: '4px',
                cursor: 'pointer',
                fontSize: '11px',
              }}
            >
              Delete
            </button>
          </div>

          {historyNotes.length > 0 && (
            <div style={{ marginBottom: '12px' }}>
              <button
                onClick={() => setShowHistory(!showHistory)}
                style={{
                  background: '#f8f9fa',
                  border: '1px solid #dee2e6',
                  padding: '8px 12px',
                  borderRadius: '6px',
                  fontSize: '13px',
                  color: '#495057',
                  cursor: 'pointer',
                }}
              >
                {showHistory ? `Hide history (${historyNotes.length} older)` : `View all notes (${notes.length})`}
              </button>
            </div>
          )}

          {showHistory && historyNotes.length > 0 && (
            <div
              style={{
                maxHeight: '300px',
                overflowY: 'auto',
                border: '1px solid #dee2e6',
                borderRadius: '8px',
                padding: '12px',
                background: '#f8f9fa',
                marginBottom: '12px',
              }}
            >
              {historyNotes.map((note) => (
                <div
                  key={note.id}
                  style={{
                    background: 'white',
                    padding: '12px',
                    borderRadius: '6px',
                    marginBottom: '8px',
                    borderLeft: '3px solid #007bff',
                  }}
                >
                  <div style={{ fontSize: '13px', color: '#333', marginBottom: '6px' }}>{note.text}</div>
                  <div style={{ fontSize: '11px', color: '#666' }}>
                    {note.created_by_email} · {new Date(note.created_at).toLocaleString()}
                  </div>
                  <button
                    onClick={() => onDeleteNote(note.id)}
                    style={{
                      marginTop: '8px',
                      padding: '4px 8px',
                      background: '#dc3545',
                      color: 'white',
                      border: 'none',
                      borderRadius: '4px',
                      cursor: 'pointer',
                      fontSize: '11px',
                    }}
                  >
                    Delete
                  </button>
                </div>
              ))}
            </div>
          )}
        </>
      ) : !loading ? (
        <p style={{ color: '#666', fontStyle: 'italic', marginBottom: '12px' }}>No notes yet.</p>
      ) : null}

      <form onSubmit={handleSubmit} style={{ marginTop: '12px' }}>
        <div style={{ display: 'flex', gap: '8px' }}>
          <input
            type="text"
            value={noteText}
            onChange={(e) => setNoteText(e.target.value)}
            placeholder="Add a new note..."
            disabled={submitting}
            style={{
              flex: 1,
              padding: '10px 12px',
              border: '1px solid #ced4da',
              borderRadius: '6px',
              fontSize: '14px',
            }}
          />
          <button
            type="submit"
            disabled={submitting || !noteText.trim()}
            style={{
              padding: '10px 20px',
              background: submitting ? '#ccc' : '#007bff',
              color: 'white',
              border: 'none',
              borderRadius: '6px',
              cursor: submitting ? 'not-allowed' : 'pointer',
              fontSize: '14px',
            }}
          >
            {submitting ? 'Adding...' : 'Add Note'}
          </button>
        </div>
      </form>
    </div>
  )
}
