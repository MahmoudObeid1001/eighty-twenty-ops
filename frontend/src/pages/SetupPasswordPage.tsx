import { FormEvent, useEffect, useState } from 'react'
import { api } from '../api/client'

function roleHomePath(role: string): string {
  switch (role) {
    case 'manager':
      return '/app/staff'
    case 'admin':
    case 'moderator':
      return '/pre-enrolment'
    case 'mentor_head':
      return '/mentor-head'
    case 'mentor':
      return '/mentor'
    case 'student_success':
      return '/app/student-success'
    case 'hr':
      return '/hr/mentors'
    default:
      return '/pre-enrolment'
  }
}

export default function SetupPasswordPage() {
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void ensureSetupRequired()
  }, [])

  async function ensureSetupRequired() {
    try {
      const me = await api.getMe()
      if (!me.must_change_password) {
        window.location.href = roleHomePath(me.role)
      }
    } catch {
      window.location.href = '/login'
    }
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)

    if (newPassword.length < 6) {
      setError('Password must be at least 6 characters.')
      return
    }
    if (newPassword !== confirmPassword) {
      setError('Passwords do not match.')
      return
    }

    try {
      setLoading(true)
      const res = await api.forceChangePassword(newPassword)
      window.location.href = res.redirect || '/pre-enrolment'
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to set password')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20, background: '#f4f6f8' }}>
      <div style={{ width: '100%', maxWidth: 520, background: '#fff', border: '1px solid #e5e7eb', borderRadius: 12, padding: 24 }}>
        <h1 style={{ marginTop: 0, marginBottom: 12 }}>Welcome</h1>
        <p style={{ marginTop: 0, color: '#555', lineHeight: 1.5 }}>
          For security, please set your permanent password to continue.
        </p>

        {error && (
          <div style={{ marginBottom: 12, background: '#f8d7da', color: '#842029', border: '1px solid #f5c2c7', borderRadius: 8, padding: 10 }}>
            {error}
          </div>
        )}

        <form onSubmit={onSubmit}>
          <div style={{ marginBottom: 12 }}>
            <label style={{ display: 'block', fontWeight: 600, marginBottom: 6 }}>New Password</label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              style={{ width: '100%', padding: '10px 12px', borderRadius: 8, border: '1px solid #ced4da' }}
            />
          </div>

          <div style={{ marginBottom: 16 }}>
            <label style={{ display: 'block', fontWeight: 600, marginBottom: 6 }}>Confirm Password</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              style={{ width: '100%', padding: '10px 12px', borderRadius: 8, border: '1px solid #ced4da' }}
            />
          </div>

          <button
            type="submit"
            disabled={loading}
            style={{ width: '100%', padding: '12px 14px', borderRadius: 8, border: 0, background: '#0d6efd', color: '#fff', fontWeight: 700, cursor: loading ? 'not-allowed' : 'pointer' }}
          >
            {loading ? 'Saving...' : 'Set Password'}
          </button>
        </form>
      </div>
    </div>
  )
}
