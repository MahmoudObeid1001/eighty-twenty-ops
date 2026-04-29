type GradeNotesModalProps = {
  open: boolean
  studentName: string
  value: string
  onChange: (value: string) => void
  onClose: () => void
  onSave?: () => void | Promise<void>
  saving?: boolean
  canEdit?: boolean
}

function countWords(value: string) {
  return value.trim() ? value.trim().split(/\s+/).length : 0
}

export default function GradeNotesModal({
  open,
  studentName,
  value,
  onChange,
  onClose,
  onSave,
  saving = false,
  canEdit = true,
}: GradeNotesModalProps) {
  if (!open) return null

  const wordCount = countWords(value)

  return (
    <div
      onClick={() => {
        if (saving) return
        onClose()
      }}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(0,0,0,0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 5000,
        padding: '16px',
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '760px',
          maxWidth: '100%',
          maxHeight: '90vh',
          overflow: 'auto',
          background: '#fff',
          borderRadius: '14px',
          boxShadow: '0 20px 45px rgba(0,0,0,0.22)',
          border: '1px solid #e5e7eb',
        }}
      >
        <div style={{ padding: '20px 24px', borderBottom: '1px solid #eee', display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px' }}>
          <div>
            <h3 style={{ margin: 0, fontSize: '22px' }}>Final Notes</h3>
            <p style={{ margin: '8px 0 0 0', color: '#666', fontSize: '14px' }}>
              {studentName} · Large writing space for detailed mentor notes.
            </p>
          </div>
          <button
            onClick={onClose}
            disabled={saving}
            style={{ border: 'none', background: 'transparent', fontSize: '24px', lineHeight: 1, cursor: saving ? 'not-allowed' : 'pointer', color: '#666' }}
            aria-label="Close notes modal"
          >
            ×
          </button>
        </div>

        <div style={{ padding: '20px 24px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '10px', fontSize: '13px', color: '#666', flexWrap: 'wrap' }}>
            <span>Target: enough room for 300+ words.</span>
            <span>{wordCount} words</span>
          </div>

          <textarea
            value={value}
            onChange={(e) => onChange(e.target.value)}
            disabled={!canEdit || saving}
            placeholder="Write final assessment notes for this student..."
            style={{
              width: '100%',
              minHeight: '360px',
              resize: 'vertical',
              padding: '16px',
              borderRadius: '10px',
              border: '1px solid #cbd5e1',
              fontSize: '15px',
              lineHeight: 1.6,
              background: canEdit ? '#fff' : '#f8f9fa',
              color: '#1f2937',
              boxSizing: 'border-box',
            }}
          />
        </div>

        <div style={{ padding: '16px 24px 24px 24px', display: 'flex', justifyContent: 'flex-end', gap: '12px' }}>
          <button
            onClick={onClose}
            disabled={saving}
            style={{ padding: '10px 16px', borderRadius: '8px', border: '1px solid #cbd5e1', background: '#fff', cursor: saving ? 'not-allowed' : 'pointer', fontWeight: 600 }}
          >
            {onSave && canEdit ? 'Cancel' : 'Close'}
          </button>
          {onSave && canEdit && (
            <button
              onClick={onSave}
              disabled={saving}
              style={{ padding: '10px 18px', borderRadius: '8px', border: 'none', background: '#0d6efd', color: '#fff', cursor: saving ? 'not-allowed' : 'pointer', fontWeight: 700 }}
            >
              {saving ? 'Saving...' : 'Save Notes'}
            </button>
          )}
          {!onSave && canEdit && (
            <button
              onClick={onClose}
              style={{ padding: '10px 18px', borderRadius: '8px', border: 'none', background: '#0d6efd', color: '#fff', cursor: 'pointer', fontWeight: 700 }}
            >
              Done
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
