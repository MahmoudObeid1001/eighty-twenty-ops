// API client with cookie-based authentication
const API_BASE = '/api'

async function fetchAPI(endpoint: string, options: RequestInit = {}) {
  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    credentials: 'include', // Send cookies
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
  })

  if (!response.ok) {
    let errorMessage = `HTTP ${response.status}`
    try {
      const errorData = await response.json()
      if (errorData.error) {
        errorMessage = errorData.error
      } else if (typeof errorData === 'string') {
        errorMessage = errorData
      }
    } catch {
      // If JSON parse fails, use status text
      errorMessage = response.statusText || `HTTP ${response.status}`
    }
    throw new Error(errorMessage)
  }

  return response.json()
}

export interface User {
  id: string
  email: string
  name: string
  role: string
}

export interface Class {
  class_key: string
  level: number
  days: string
  time: string
  class_number: number
  student_count: number
}

export interface Mentor {
  id: string
  email: string
}

export interface MentorGroup {
  mentor_id?: string
  mentor_email?: string
  classes: Class[]
}

export interface MentorHeadClass {
  class_key: string
  level: number
  days: string
  time: string
  class_number: number
  student_count: number
  readiness: string
  mentor_user_id?: string
  mentor_email?: string
  sent_to_mentor: boolean
}

export interface MentorHeadDashboard {
  classes: MentorHeadClass[]
  mentors: Mentor[]
}

export interface Student {
  lead_id: string
  full_name: string
  phone: string
  missed_count?: number
  attendance?: Record<string, string> // session_id -> status
}

export interface Note {
  id: string
  text: string
  created_at: string
  created_by_email: string
}

export interface Session {
  id: string
  session_number: number
  scheduled_date: string
  scheduled_time: string
  status: string
}

export interface ClassDetail {
  class: {
    class_key: string
    level: number
    days: string
    time: string
    class_number: number
    round_status?: string
  }
  sessionsCount: number
  totalSessions: number
  students: Student[]
  sessions: Session[]
}

export interface StudentProfile {
  id: string
  name: string
  phone: string
  levelsFinished: number
  levelsLeft: number
  lastLevelGrade: string | null
}

export interface StudentSuccessClass {
  class_key: string
  level: number
  days: string
  time: string
  class_number: number
  mentor_email: string
  mentor_name: string
  mentor_user_id?: string
  student_count: number
}

export interface StudentSuccessClassDetail {
  class: {
    class_key: string
    level: number
    days: string
    time: string
    class_number: number
    round_status: string
  }
  students: Array<{ lead_id: string; full_name: string; phone: string; missed_count: number; missed_sessions: number[] }>
  sessions: Array<{
    id: string
    session_number: number
  }>
  sessionsCount: number
  completedSessionsCount: number
  totalSessions: number
  feedback: Array<{
    lead_id: string
    full_name: string
    s4?: { session_number: number; status: string; feedback_text?: string; follow_up_required: boolean }
    s8?: { session_number: number; status: string; feedback_text?: string; follow_up_required: boolean }
  }>
  milestones: {
    midRound: { reached: boolean; complete: boolean }
    endRound: { reached: boolean; complete: boolean }
  }
}

export type SubmitFeedbackRequest = {
  lead_id: string
  class_key: string
  session_number: number
  feedback_text: string
  follow_up_required: boolean
}

export const api = {
  getMe: (): Promise<User> => fetchAPI('/me'),

  getMentorClasses: (): Promise<Class[]> => fetchAPI('/mentor/classes'),

  getMentors: (): Promise<Mentor[]> => fetchAPI('/mentor-head/mentors'),

  getMentorHeadClasses: (): Promise<MentorGroup[]> => fetchAPI('/mentor-head/classes'),

  getClassWorkspace: (classKey: string): Promise<ClassDetail> =>
    fetchAPI(`/class-workspace?class_key=${encodeURIComponent(classKey)}`),

  getClass: (classKey: string): Promise<ClassDetail> =>
    fetchAPI(`/class?class_key=${encodeURIComponent(classKey)}`),

  getNotes: (studentId: string, classKey: string): Promise<Note[]> =>
    fetchAPI(`/notes?student_id=${encodeURIComponent(studentId)}&class_key=${encodeURIComponent(classKey)}`),

  createNote: (studentId: string, classKey: string, text: string): Promise<Note> =>
    fetchAPI('/notes', {
      method: 'POST',
      body: JSON.stringify({ student_id: studentId, class_key: classKey, text }),
    }),

  deleteNote: (noteId: string): Promise<{ ok: boolean }> =>
    fetchAPI(`/notes?id=${encodeURIComponent(noteId)}`, {
      method: 'DELETE',
    }),

  // Mentor Head endpoints
  getMentorHeadDashboard: (): Promise<MentorHeadDashboard> =>
    fetchAPI('/mentor-head/dashboard'),

  assignMentor: (classKey: string, mentorEmail: string): Promise<{ ok: boolean }> =>
    fetchAPI('/mentor-head/assign-mentor', {
      method: 'POST',
      body: JSON.stringify({ class_key: classKey, mentor_email: mentorEmail }),
    }),

  unassignMentor: (classKey: string) =>
    fetchAPI('/mentor-head/unassign', {
      method: 'POST',
      body: JSON.stringify({ class_key: classKey }),
    }),

  returnToOps: (classKey: string): Promise<{ ok: boolean }> =>
    fetchAPI('/mentor-head/return-to-ops', {
      method: 'POST',
      body: JSON.stringify({ class_key: classKey }),
    }),

  startRound: (classKey: string): Promise<{ ok: boolean }> =>
    fetchAPI('/mentor-head/start-round', {
      method: 'POST',
      body: JSON.stringify({ class_key: classKey }),
    }),

  closeRound: (classKey: string): Promise<{ ok: boolean }> =>
    fetchAPI('/mentor-head/close-round', {
      method: 'POST',
      body: JSON.stringify({ class_key: classKey }),
    }),

  getStudent: (studentId: string, classKey: string): Promise<StudentProfile> =>
    fetchAPI(`/student?student_id=${encodeURIComponent(studentId)}&class_key=${encodeURIComponent(classKey)}`),

  getMentorEvaluations: (): Promise<{
    mentors: Array<{
      id: string
      email: string
      name: string
      assignedClassCount: number
      kpis: {
        sessionQuality: number
        trelloCompliance: number
        whatsappManagement: number
        studentsFeedback: number
      }
      attendance: {
        sessionsTotal: number
        statuses: string[]
        onTimePercent: number
      }
    }>
  }> => fetchAPI('/mentor-head/evaluations'),

  updateMentorEvaluation: (mentorId: string, data: {
    kpis: {
      sessionQuality: number
      trelloCompliance: number
      whatsappManagement: number
      studentsFeedback: number
    }
    attendance: {
      statuses: string[]
    }
  }): Promise<{
    id: string
    kpis: {
      sessionQuality: number
      trelloCompliance: number
      whatsappManagement: number
      studentsFeedback: number
    }
    attendance: {
      sessionsTotal: number
      statuses: string[]
      onTimePercent: number
    }
  }> => fetchAPI(`/mentor-head/evaluations/${encodeURIComponent(mentorId)}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),

  // Student Success endpoints
  getStudentSuccessClasses: (): Promise<{ classes: StudentSuccessClass[] }> =>
    fetchAPI('/student-success/classes'),

  getStudentSuccessClass: (classKey: string): Promise<StudentSuccessClassDetail> =>
    fetchAPI(`/student-success/class?class_key=${encodeURIComponent(classKey)}`),

  submitFeedback: (req: SubmitFeedbackRequest): Promise<{ status: string }> =>
    fetchAPI('/student-success/feedback', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  markAttendance: (
    sessionId: string,
    leadId: string,
    status: string,
    classKey: string,
    notes: string = ''
  ): Promise<{ ok: boolean }> =>
    fetchAPI('/attendance', {
      method: 'POST',
      body: JSON.stringify({ session_id: sessionId, lead_id: leadId, status, class_key: classKey, notes }),
    }),

  completeSession: (sessionId: string, classKey: string): Promise<{ ok: boolean }> =>
    fetchAPI('/session/complete', {
      method: 'POST',
      body: JSON.stringify({ session_id: sessionId, class_key: classKey }),
    }),

  getAbsenceFeed: (classKey: string, filter: string = '', search: string = ''): Promise<AbsenceFeedItem[]> =>
    fetchAPI(`/student-success/class/absence-feed?class_key=${encodeURIComponent(classKey)}&filter=${encodeURIComponent(filter)}&search=${encodeURIComponent(search)}`),

  addFollowUp: (data: {
    class_key: string
    lead_id: string
    session_number: number
    note: string
    status: string
  }): Promise<{ ok: boolean }> =>
    fetchAPI('/student-success/followups', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateFollowUpStatus: (id: string, status: string): Promise<{ ok: boolean }> =>
    fetchAPI('/student-success/followups/update', {
      method: 'POST',
      body: JSON.stringify({ id, status }),
    }),

  resolveFollowUp: (id: string): Promise<{ ok: boolean }> =>
    fetchAPI(`/api/absence-cases/${encodeURIComponent(id)}/resolve`, {
      method: 'POST',
    }),

  updateFollowUp: (id: string, data: { status: string; note: string; resolved: boolean }): Promise<{ ok: boolean }> =>
    fetchAPI(`/api/absence-cases/${encodeURIComponent(id)}/follow-up`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
}

export interface AbsenceFeedItem {
  sessionNumber: number
  sessionDate: string
  startTime: string
  studentId: string
  studentName: string
  studentPhone: string
  status: string
  markedBy: string
  markedAt: string
  mentorNote?: string
  followUp?: {
    id: string
    status: string
    lastNote: string
    updatedAt: string
    resolved: boolean
    resolvedAt?: string
  }
}
