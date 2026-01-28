import { useEffect, useState, useRef } from 'react'
import { api, Student, Note, StudentProfile } from '../api/client'
import NotesSection from './NotesSection'

interface StudentModalProps {
  student: Student | null
  classKey: string
  sessionsCount: number
  onClose: () => void
}

export default function StudentModal({ student, classKey, sessionsCount, onClose }: StudentModalProps) {
  const [profile, setProfile] = useState<StudentProfile | null>(null)
  const [notes, setNotes] = useState<Note[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const modalRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (student) {
      loadData()
    } else {
      setProfile(null)
      setNotes([])
    }
  }, [student, classKey])

  useEffect(() => {
    // Handle ESC key
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose()
      }
    }
    if (student) {
      document.addEventListener('keydown', handleEsc)
      return () => document.removeEventListener('keydown', handleEsc)
    }
  }, [student, onClose])

  async function loadData() {
    if (!student) return
    try {
      setLoading(true)
      setError(null)
      const [profileData, notesData] = await Promise.all([
        api.getStudent(student.lead_id, classKey),
        api.getNotes(student.lead_id, classKey),
      ])
      setProfile(profileData)
      setNotes(notesData)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load data')
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
    <>
      {/* Backdrop */}
      <div
        style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'rgba(0, 0, 0, 0.5)',
          zIndex: 1000,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
        onClick={(e) => {
          if (e.target === e.currentTarget) {
            onClose()
          }
        }}
      >
        {/* Modal */}
        <div
          ref={modalRef}
          style={{
            background: 'white',
            borderRadius: '8px',
            width: '90%',
            maxWidth: '600px',
            maxHeight: '90vh',
            overflow: 'hidden',
            display: 'flex',
            flexDirection: 'column',
            boxShadow: '0 4px 20px rgba(0,0,0,0.3)',
          }}
          onClick={(e) => e.stopPropagation()}
        >
          {/* Header */}
          <div style={{ padding: '24px', borderBottom: '1px solid #ddd', background: '#f8f9fa' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '12px' }}>
              <div>
                <h2 style={{ fontSize: '24px', marginBottom: '4px', color: '#333' }}>{profile?.name || student.full_name}</h2>
                <p style={{ fontSize: '14px', color: '#666', marginBottom: '4px' }}>{profile?.phone || student.phone}</p>
                <p style={{ fontSize: '12px', color: '#999' }}>ID: {student.lead_id.substring(0, 8)}...</p>
              </div>
              <button
                onClick={onClose}
                style={{
                  background: 'none',
                  border: 'none',
                  fontSize: '28px',
                  cursor: 'pointer',
                  color: '#666',
                  padding: '0',
                  width: '32px',
                  height: '32px',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                }}
              >
                ×
              </button>
            </div>
          </div>

          {/* Body */}
          <div style={{ flex: 1, overflowY: 'auto', padding: '24px' }}>
            {loading && (
              <div style={{ textAlign: 'center', padding: '20px' }}>
                <p>Loading...</p>
              </div>
            )}

            {error && (
              <div style={{ marginBottom: '16px', padding: '12px', background: '#fee', borderRadius: '6px', color: '#c33' }}>
                <p style={{ fontSize: '13px', margin: 0 }}>{error}</p>
              </div>
            )}

            {!loading && profile && (
              <>
                {/* Stats blocks (ID card style) */}
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: '12px', marginBottom: '24px' }}>
                  <div style={{ background: '#f8f9fa', padding: '16px', borderRadius: '6px', border: '1px solid #dee2e6' }}>
                    <div style={{ fontSize: '11px', color: '#666', marginBottom: '4px', textTransform: 'uppercase' }}>Levels Finished</div>
                    <div style={{ fontSize: '24px', fontWeight: 600, color: '#333' }}>{profile.levelsFinished}</div>
                  </div>
                  <div style={{ background: '#f8f9fa', padding: '16px', borderRadius: '6px', border: '1px solid #dee2e6' }}>
                    <div style={{ fontSize: '11px', color: '#666', marginBottom: '4px', textTransform: 'uppercase' }}>Levels Left</div>
                    <div style={{ fontSize: '24px', fontWeight: 600, color: '#333' }}>{profile.levelsLeft}</div>
                  </div>
                  <div style={{ background: '#f8f9fa', padding: '16px', borderRadius: '6px', border: '1px solid #dee2e6' }}>
                    <div style={{ fontSize: '11px', color: '#666', marginBottom: '4px', textTransform: 'uppercase' }}>Last Level Grade</div>
                    <div style={{ fontSize: '24px', fontWeight: 600, color: '#333' }}>{profile.lastLevelGrade || '—'}</div>
                  </div>
                  <div style={{ background: '#f8f9fa', padding: '16px', borderRadius: '6px', border: '1px solid #dee2e6' }}>
                    <div style={{ fontSize: '11px', color: '#666', marginBottom: '4px', textTransform: 'uppercase' }}>Phone</div>
                    <div style={{ fontSize: '16px', fontWeight: 600, color: '#333' }}>{profile.phone}</div>
                  </div>
                </div>

                {!loading && sessionsCount === 0 && (
                  <div style={{ marginBottom: '20px', padding: '12px', background: '#fff3cd', borderRadius: '6px', border: '1px solid #ffc107' }}>
                    <p style={{ fontSize: '13px', color: '#856404', margin: 0 }}>
                      Round not started yet
                    </p>
                  </div>
                )}
              </>
            )}

            {/* Notes section */}
            <NotesSection
              notes={notes}
              loading={loading}
              onAddNote={handleAddNote}
              onDeleteNote={handleDeleteNote}
            />
          </div>
        </div>
      </div>
    </>
  )
}
