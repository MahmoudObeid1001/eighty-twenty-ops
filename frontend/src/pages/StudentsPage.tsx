import type { CSSProperties } from 'react'
import StudentSearch from '../components/StudentSearch'

export default function StudentsPage() {
    return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
            <section
                style={{
                    background: 'linear-gradient(135deg, #f7fbff 0%, #ffffff 100%)',
                    border: '1px solid #d7e6ef',
                    borderRadius: '18px',
                    padding: '28px',
                    boxShadow: '0 10px 30px rgba(47, 164, 199, 0.08)',
                }}
            >
                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                    <div style={{ display: 'inline-flex', alignItems: 'center', gap: '8px', width: 'fit-content', padding: '6px 12px', borderRadius: '999px', background: '#e7f6fb', color: '#11647b', fontSize: '13px', fontWeight: 700 }}>
                        Student workspace
                    </div>
                    <div>
                        <h1 style={{ fontSize: '34px', fontWeight: 800, letterSpacing: '-0.03em', marginBottom: '10px', color: '#1f2937' }}>
                            Students
                        </h1>
                        <p style={{ color: '#5f6b76', fontSize: '17px', maxWidth: '760px', lineHeight: 1.7 }}>
                            Search by student name or phone number, review their status in one place, then open the full profile to check history, report cards, and notes.
                        </p>
                    </div>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: '10px', marginTop: '4px' }}>
                        <div style={featurePillStyle}>Fast search for admin calls</div>
                        <div style={featurePillStyle}>Visible table for quick scanning</div>
                        <div style={featurePillStyle}>One click to open full student profile</div>
                    </div>
                </div>
            </section>

            <section
                style={{
                    background: '#fff',
                    border: '1px solid #e5e7eb',
                    borderRadius: '18px',
                    padding: '24px',
                    boxShadow: '0 8px 24px rgba(15, 23, 42, 0.06)',
                }}
            >
                <StudentSearch />
            </section>
        </div>
    )
}

const featurePillStyle: CSSProperties = {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '8px',
    padding: '10px 14px',
    borderRadius: '999px',
    background: '#f8fafc',
    border: '1px solid #e2e8f0',
    color: '#334155',
    fontSize: '14px',
    fontWeight: 600,
}
