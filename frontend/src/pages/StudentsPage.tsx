import StudentSearch from '../components/StudentSearch'

export default function StudentsPage() {
    return (
        <div style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            minHeight: '60vh',
            padding: '20px'
        }}>
            <div style={{ textAlign: 'center', marginBottom: '40px' }}>
                <h1 style={{ fontSize: '32px', fontWeight: 700, marginBottom: '12px' }}>Student Directory</h1>
                <p style={{ color: '#666', fontSize: '18px' }}>Search for any student to view their profile, academic history, and notes.</p>
            </div>

            <div style={{ width: '100%', maxWidth: '600px' }}>
                <StudentSearch />
            </div>

            <div style={{ marginTop: '60px', display: 'flex', gap: '40px', color: '#888', fontSize: '14px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <span style={{ fontSize: '20px' }}>🔍</span> Search by Name or Phone
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <span style={{ fontSize: '20px' }}>📜</span> View Academic History
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <span style={{ fontSize: '20px' }}>📝</span> Access Student Notes
                </div>
            </div>
        </div>
    )
}
