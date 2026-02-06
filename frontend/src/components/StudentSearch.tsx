import { useState, useEffect, useRef } from 'react'
import { api, StudentSearchResult } from '../api/client'
import StudentProfileModal from './StudentProfileModal' // Clean import to resolve TS errors

export default function StudentSearch() {
    const [query, setQuery] = useState('')
    const [results, setResults] = useState<StudentSearchResult[]>([])
    const [loading, setLoading] = useState(false)
    const [showResults, setShowResults] = useState(false)
    const [selectedStudentId, setSelectedStudentId] = useState<string | null>(null)
    const searchRef = useRef<HTMLDivElement>(null)

    // Debounced search
    useEffect(() => {
        if (query.length < 2) {
            setResults([])
            setShowResults(false)
            return
        }

        const timer = setTimeout(async () => {
            setLoading(true)
            try {
                const data = await api.searchStudents(query)
                setResults(data || [])
                setShowResults(true)
            } catch (err) {
                console.error('Search failed:', err)
            } finally {
                setLoading(false)
            }
        }, 300)

        return () => clearTimeout(timer)
    }, [query])

    // Close dropdown when clicking outside
    useEffect(() => {
        function handleClickOutside(event: MouseEvent) {
            if (searchRef.current && !searchRef.current.contains(event.target as Node)) {
                setShowResults(false)
            }
        }
        document.addEventListener('mousedown', handleClickOutside)
        return () => document.removeEventListener('mousedown', handleClickOutside)
    }, [])

    return (
        <>
            <div ref={searchRef} style={{ position: 'relative', width: '400px' }}>
                <input
                    type="text"
                    placeholder="Search students by name or phone..."
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    style={{
                        width: '100%',
                        padding: '10px 40px 10px 12px',
                        border: '1px solid #ddd',
                        borderRadius: '6px',
                        fontSize: '14px',
                    }}
                />
                {loading && (
                    <span style={{ position: 'absolute', right: '12px', top: '12px', color: '#666' }}>
                        🔍
                    </span>
                )}

                {showResults && results.length > 0 && (
                    <div style={{
                        position: 'absolute',
                        top: '100%',
                        left: 0,
                        right: 0,
                        marginTop: '4px',
                        background: 'white',
                        border: '1px solid #ddd',
                        borderRadius: '6px',
                        boxShadow: '0 4px 12px rgba(0,0,0,0.1)',
                        maxHeight: '400px',
                        overflowY: 'auto',
                        zIndex: 1000,
                    }}>
                        {results.map((result) => (
                            <div
                                key={result.lead_id}
                                onClick={() => {
                                    setSelectedStudentId(result.lead_id)
                                    setShowResults(false)
                                    setQuery('')
                                }}
                                style={{
                                    padding: '12px',
                                    borderBottom: '1px solid #f0f0f0',
                                    cursor: 'pointer',
                                    transition: 'background 0.2s',
                                }}
                                onMouseEnter={(e) => e.currentTarget.style.background = '#f8f9fa'}
                                onMouseLeave={(e) => e.currentTarget.style.background = 'white'}
                            >
                                <div style={{ fontWeight: 600, marginBottom: '4px' }}>{result.full_name}</div>
                                <div style={{ fontSize: '12px', color: '#666' }}>
                                    {result.phone} • Level {result.current_level} • {result.status}
                                </div>
                            </div>
                        ))}
                    </div>
                )}

                {showResults && results.length === 0 && !loading && query.length >= 2 && (
                    <div style={{
                        position: 'absolute',
                        top: '100%',
                        left: 0,
                        right: 0,
                        marginTop: '4px',
                        background: 'white',
                        border: '1px solid #ddd',
                        borderRadius: '6px',
                        boxShadow: '0 4px 12px rgba(0,0,0,0.1)',
                        padding: '12px',
                        color: '#666',
                        fontSize: '14px',
                        zIndex: 1000,
                    }}>
                        No students found
                    </div>
                )}
            </div>

            {selectedStudentId && (
                <StudentProfileModal
                    studentId={selectedStudentId}
                    onClose={() => setSelectedStudentId(null)}
                />
            )}
        </>
    )
}
