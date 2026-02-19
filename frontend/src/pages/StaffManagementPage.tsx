import { CSSProperties, FormEvent, useEffect, useState } from 'react'
import { api, StaffUser } from '../api/client'

const roleOptions = [
  { value: 'admin', label: 'Admin' },
  { value: 'mentor_head', label: 'Mentor Head' },
  { value: 'mentor', label: 'Mentor' },
  { value: 'hr', label: 'HR' },
  { value: 'student_success', label: 'Student Success' },
  { value: 'moderator', label: 'Moderator' },
]

export default function StaffManagementPage() {
  const [users, setUsers] = useState<StaffUser[]>([])
  const [currentUserId, setCurrentUserId] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [showModal, setShowModal] = useState(false)
  const [deletingUserId, setDeletingUserId] = useState<string | null>(null)

  const [fullName, setFullName] = useState('')
  const [email, setEmail] = useState('')
  const [role, setRole] = useState('admin')
  const [temporaryPassword, setTemporaryPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    void loadUsers()
    void loadCurrentUser()
  }, [])

  async function loadCurrentUser() {
    try {
      const me = await api.getMe()
      setCurrentUserId(me.id)
    } catch {
      setCurrentUserId('')
    }
  }

  async function loadUsers() {
    try {
      setLoading(true)
      setError(null)
      const res = await api.getStaffUsers()
      setUsers(res.users || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load users')
    } finally {
      setLoading(false)
    }
  }

  async function createUser(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSuccess(null)

    if (!fullName.trim() || !email.trim() || !temporaryPassword.trim()) {
      setError('Name, email, and temporary password are required.')
      return
    }

    try {
      setSubmitting(true)
      await api.createStaffUser({
        full_name: fullName.trim(),
        email: email.trim(),
        role,
        temporary_password: temporaryPassword,
      })
      setShowModal(false)
      setFullName('')
      setEmail('')
      setTemporaryPassword('')
      setRole('admin')
      setSuccess('User created. They must change their password on first login.')
      await loadUsers()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create user')
    } finally {
      setSubmitting(false)
    }
  }

  async function removeUser(user: StaffUser) {
    const confirmed = window.confirm(`Remove user \"${user.full_name}\" (${user.email})?`)
    if (!confirmed) return
    try {
      setDeletingUserId(user.id)
      setError(null)
      setSuccess(null)
      await api.deleteStaffUser(user.id)
      setSuccess('User removed successfully.')
      await loadUsers()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove user')
    } finally {
      setDeletingUserId(null)
    }
  }

  return (
    <div style={{ padding: '24px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
        <h1 style={{ margin: 0, fontSize: 32, fontWeight: 700 }}>Staff Management</h1>
        <button
          onClick={() => setShowModal(true)}
          style={{ padding: '10px 14px', borderRadius: 8, border: '1px solid #0d6efd', background: '#0d6efd', color: '#fff', fontWeight: 700 }}
        >
          Create User
        </button>
      </div>
      <p style={{ marginTop: 0, color: '#666', marginBottom: 16 }}>Manager-only user directory and account provisioning.</p>

      {error && <div style={{ background: '#f8d7da', color: '#842029', border: '1px solid #f5c2c7', borderRadius: 8, padding: 10, marginBottom: 12 }}>{error}</div>}
      {success && <div style={{ background: '#d1e7dd', color: '#0f5132', border: '1px solid #badbcc', borderRadius: 8, padding: 10, marginBottom: 12 }}>{success}</div>}

      {loading ? (
        <div>Loading users...</div>
      ) : (
        <div style={{ background: '#fff', border: '1px solid #dee2e6', borderRadius: 8, overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: 720 }}>
            <thead>
              <tr style={{ background: '#f8f9fa', borderBottom: '1px solid #dee2e6' }}>
                <th style={thStyle}>Name</th>
                <th style={thStyle}>Email</th>
                <th style={thStyle}>Role</th>
                <th style={thStyle}>Password State</th>
                <th style={thStyle}>Created</th>
                <th style={thStyle}>Action</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id} style={{ borderBottom: '1px solid #f1f3f5' }}>
                  <td style={tdStyle}>{u.full_name}</td>
                  <td style={tdStyle}>{u.email}</td>
                  <td style={tdStyle}>{u.role}</td>
                  <td style={tdStyle}>{u.must_change_password ? 'Must change on first login' : 'Active'}</td>
                  <td style={tdStyle}>{new Date(u.created_at).toLocaleString()}</td>
                  <td style={tdStyle}>
                    <button
                      type="button"
                      disabled={u.id === currentUserId || deletingUserId === u.id}
                      onClick={() => void removeUser(u)}
                      style={{
                        padding: '6px 10px',
                        borderRadius: 6,
                        border: '1px solid #dc3545',
                        background: '#fff',
                        color: '#dc3545',
                        cursor: u.id === currentUserId || deletingUserId === u.id ? 'not-allowed' : 'pointer',
                      }}
                    >
                      {deletingUserId === u.id ? 'Removing...' : (u.id === currentUserId ? 'Current User' : 'Remove')}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showModal && (
        <div onClick={() => setShowModal(false)} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.45)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16 }}>
          <div onClick={(e) => e.stopPropagation()} style={{ width: '100%', maxWidth: 520, background: '#fff', borderRadius: 10, padding: 18 }}>
            <h3 style={{ marginTop: 0 }}>Create Staff User</h3>
            <form onSubmit={createUser}>
              <label style={labelStyle}>Name</label>
              <input value={fullName} onChange={(e) => setFullName(e.target.value)} required style={inputStyle} />

              <label style={labelStyle}>Email</label>
              <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required style={inputStyle} />

              <label style={labelStyle}>Role</label>
              <select value={role} onChange={(e) => setRole(e.target.value)} style={inputStyle}>
                {roleOptions.map((opt) => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>

              <label style={labelStyle}>Temporary Password</label>
              <input type="password" value={temporaryPassword} onChange={(e) => setTemporaryPassword(e.target.value)} required style={inputStyle} />

              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 12 }}>
                <button type="button" onClick={() => setShowModal(false)} style={{ padding: '10px 12px', borderRadius: 8, border: '1px solid #adb5bd', background: '#fff' }}>Cancel</button>
                <button type="submit" disabled={submitting} style={{ padding: '10px 12px', borderRadius: 8, border: '1px solid #0d6efd', background: '#0d6efd', color: '#fff', fontWeight: 700 }}>
                  {submitting ? 'Creating...' : 'Create'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}

const thStyle: CSSProperties = {
  textAlign: 'left',
  padding: '10px 12px',
  fontSize: 13,
  fontWeight: 700,
}

const tdStyle: CSSProperties = {
  padding: '10px 12px',
  fontSize: 14,
}

const labelStyle: CSSProperties = {
  display: 'block',
  fontWeight: 600,
  marginBottom: 6,
  marginTop: 8,
}

const inputStyle: CSSProperties = {
  width: '100%',
  padding: '10px 12px',
  borderRadius: 8,
  border: '1px solid #ced4da',
}
