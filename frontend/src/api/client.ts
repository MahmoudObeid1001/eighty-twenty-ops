// API client with cookie-based authentication
const API_BASE = '/api'

async function fetchAPI(endpoint: string, options: RequestInit = {}) {
  const url = `${API_BASE}${endpoint}`
  // console.log(`[DEBUG] fetchAPI: ${url}`)
  const isFormData = typeof FormData !== 'undefined' && options.body instanceof FormData
  const response = await fetch(url, {
    ...options,
    credentials: 'include', // Send cookies
    headers: {
      ...(isFormData ? {} : { 'Content-Type': 'application/json' }),
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
  must_change_password?: boolean
}

export interface MentorDirectoryItem {
  id: string
  name: string
  email: string
  phone: string
  status: 'active' | 'inactive'
  total_classes_taught: number
}

export interface MentorProfileStats {
  total_classes: number
  first_class_date: string | null
  last_class_date: string | null
  avg_rating: number
  feedback_meter: number
  compliance_score: number
}

export interface MentorProfileClassHistory {
  class_key: string
  level: number
  days: string
  time: string
  start_date: string | null
  end_date: string | null
  duration: string
  evaluation_score: number
  compliance_score: number
}

export interface MentorProfileResponse {
  mentor_details: {
    id: string
    name: string
    email: string
    phone: string
    status: 'active' | 'inactive'
  }
  stats: MentorProfileStats
  class_history: MentorProfileClassHistory[]
  testimonials: MentorTestimonial[]
}

export interface MentorTestimonial {
  id: string
  class_key: string
  testimonial_text: string
  created_by: string
  created_at: string
}

export interface Class {
  class_key: string
  level: number
  days: string
  time: string
  class_number: number
  student_count: number
}

export interface MentorReminder {
  type: 'attendance' | 'grading'
  class_key: string
  level: number
  class_days: string
  class_time: string
  class_number: number
  session_number: number
  message: string
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
  all_graded?: boolean
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
  session_performance?: Record<string, { task_completed: boolean; participation_score: number }>
  joined_at_session_number?: number // NEW
}

export interface Note {
  id: string
  text: string
  is_private?: boolean
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

export interface ClassTransferOption {
  class_key: string
  level: number
  class_days: string
  class_time: string
  class_number: number
  round_status: string
  current_session: number
  current_enrollment: number
}

export interface StudentProfile {
  id: string
  name: string
  phone: string
  levelsFinished: number
  levelsLeft: number
  lastLevelGrade: string | null
  highPriority?: boolean
  highPriorityReason?: string
}

export interface StudentReportCardData {
  class_key: string
  class_level: number
  student_name: string
  student_phone: string
  generated_at: string
  final_grade: string
  mentor_comment: string
  session_evidence: Array<{
    session_number: number
    attendance_status: string
    task_completed?: boolean
    participation_score?: number
    participation_stars: string
    task_display: string
    attendance_display: string
    participation_symbol: string
  }>
  calculation: {
    attendance_score: number
    task_score: number
    participation_score: number
    total_score: number
    absences: number
    completed_tasks: number
    missed_tasks: number
    average_stars: number
    calculated_grade: string
    used_legacy_task_safe: boolean
  }
}

export interface StudentProfileResponse extends StudentProfile {
  report_card?: StudentReportCardData
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
  has_high_priority?: boolean
  high_priority_reason?: string
  mid_round_required?: boolean
  end_round_required?: boolean
  compliance_required?: boolean
  compliance_done?: number
  compliance_total?: number
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
  students: Array<{ lead_id: string; full_name: string; phone: string; missed_count: number; missed_sessions: number[]; joined_at_session_number?: number }>
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
    phone: string
    s4?: { session_number: number; status: string; feedback_text?: string; follow_up_required: boolean }
    s8?: { session_number: number; status: string; feedback_text?: string; follow_up_required: boolean }
  }>
  milestones: {
    midRound: { reached: boolean; complete: boolean }
    endRound: { reached: boolean; complete: boolean }
  }
}

export interface FeedbackCollectedUpload {
  id: string
  lead_id: string
  class_key: string
  session_number?: number
  file_name: string
  file_url: string
  mime_type?: string
  size_bytes?: number
  note?: string
  uploaded_by?: string
  uploaded_at: string
}

export interface PlacementTestQueueItem {
  lead_id: string
  full_name: string
  phone: string
  status: string
  test_date: string
  test_time: string
  test_type: string
  assigned_level?: number
  test_notes?: string
}

export interface MentorArchiveGroup {
  mentor: {
    id: string
    email: string
    name: string
  }
  classes: Array<{
    class_key: string
    level: number
    days: string
    time: string
    class_number: number
    student_count: number
    closed_at: string
    completed_sessions_count: number
  }>
}

export type SubmitFeedbackRequest = {
  lead_id: string
  class_key: string
  session_number: number
  feedback_text: string
  follow_up_required: boolean
}

export interface Grade {
  id: string
  lead_id: string
  class_key: string
  session_number: number
  grade: string
  notes: string
  created_by_user_id: string
  created_at: string
}

export interface GradePreview {
  lead_id: string
  absences: number
  completed_tasks: number
  attended_sessions: number
  average_stars: number
  attendance_score: number
  task_score: number
  participation_score: number
  total_score: number
  calculated_grade: string
  used_legacy_task_safety: boolean
}

export interface ComplianceCheck {
  id: string
  class_session_id: string
  checked_by_user_id: string
  checked_by_email?: string
  reminder_1d: boolean
  reminder_1h: boolean
  reminder_tasks: boolean
  delay_minutes: number
  is_absent: boolean
  created_at: string
  updated_at: string
}

export interface ComplianceClassSession {
  session_number: number
  class_session_id?: string
  status?: string
  scheduled_date?: string
  scheduled_time?: string
  check?: ComplianceCheck
}

export interface MentorReportItem {
  mentor_id: string
  mentor_email: string
  classes_count: number
  sessions_count: number
  checks_count: number
  compliance_score: number
  avg_delay_minutes: number
  absence_count: number
  complaints_count: number
}

export interface MentorReportChecklistItem {
  class_key: string
  class_days: string
  class_time: string
  session_number: number
  scheduled_date: string
  scheduled_time: string
  session_status: string
  reminder_1d: boolean
  reminder_1h: boolean
  reminder_tasks: boolean
  delay_minutes: number
  is_absent: boolean
  checked_by?: string
}

export interface MentorClassReportItem {
  mentor_id: string
  mentor_email: string
  class_key: string
  level: number
  class_days: string
  class_time: string
  class_number: number
  sessions_count: number
  checks_count: number
  compliance_score: number
  avg_delay_minutes: number
  absence_count: number
  complaints_count: number
}

export interface BIBottleneckLead {
  lead_id: string
  full_name: string
  phone: string
  status: string
  days_in_status: number
}

export interface BIGhostStudent {
  lead_id: string
  full_name: string
  phone: string
  offer_price: number
  total_paid: number
  shortfall: number
  class_status: string
}

export interface BIRetentionStudent {
  lead_id: string
  full_name: string
  phone: string
  status: string
  remaining_credits: number
  last_level: number
  last_completed_at: string
}

export interface BIActiveClassesMonth {
  month: string
  classes_count: number
}

export interface BIReportPayload {
  generated_at: string
  filters: {
    from: string
    to: string
  }
  report1: {
    conversion: {
      test_booked_count: number
      converted_count: number
      conversion_rate: number
    }
    bottleneck: BIBottleneckLead[]
    renewal: {
      returning_count: number
      renewed_count: number
      renewal_rate: number
    }
  }
  report2: {
    ghost_students: BIGhostStudent[]
    refund_liability: {
      students_count: number
      total_value: number
      pricing_model: string
    }
    revenue_pulse: {
      active_round_start?: string
      total_collected: number
    }
  }
  report3: {
    lost: BIRetentionStudent[]
    stalled: BIRetentionStudent[]
  }
  report4: {
    active_classes_by_month: BIActiveClassesMonth[]
    started_learners: number
    finished_learners: number
  }
}

export interface DailyReportClassRow {
  session_id: string
  class_key: string
  class_label: string
  mentor_id: string
  mentor_email: string
  session_number: number
  scheduled_date: string
  scheduled_time: string
  actual_time: string
  session_status: string
  report_status: string
  punctuality_status: string
  delay_minutes: number
  compliance_checked: boolean
  mentor_absent: boolean
  expected_students: number
  absent_students: number
}

export interface DailyReportPayload {
  report_date: string
  ready_at: string
  generated_at: string
  classes_scheduled: number
  classes_taught: number
  classes_missing_report: number
  expected_students: number
  absent_students: number
  class_rows: DailyReportClassRow[]
}

export interface ManagerOpsSummary {
  sessions_scheduled: number
  sessions_live_now: number
  sessions_completed: number
  sessions_attendance_done: number
  sessions_attendance_pending: number
  expected_students: number
  attended_students: number
  today_revenue: number
  paying_leads_count: number
  placement_tests_scheduled: number
  placement_tests_completed: number
  placement_tests_pending: number
  late_mentor_sessions: number
  absent_mentor_sessions: number
  unchecked_mentor_sessions: number
}

export interface ManagerOpsWeeklySummary {
  label: string
  week_start: string
  week_end: string
  sessions_scheduled: number
  sessions_completed: number
  sessions_attendance_done: number
  sessions_attendance_pending: number
  expected_students: number
  attended_students: number
  revenue: number
  paying_leads_count: number
  placement_tests_scheduled: number
  placement_tests_completed: number
  placement_tests_pending: number
  late_mentor_sessions: number
  absent_mentor_sessions: number
  unchecked_mentor_sessions: number
  transfer_events: number
  returns_to_admin: number
}

export interface ManagerOpsSessionRow {
  session_id: string
  class_key: string
  class_label: string
  mentor_id: string
  mentor_name: string
  mentor_email: string
  session_number: number
  scheduled_date: string
  scheduled_time: string
  actual_time: string
  session_status: string
  session_phase: string
  mentor_status: string
  delay_minutes: number
  compliance_checked: boolean
  mentor_absent: boolean
  expected_students: number
  attendance_marked: number
  attended_students: number
  absent_students: number
  attendance_status: string
}

export interface ManagerOpsPayload {
  report_date: string
  timezone: string
  generated_at: string
  summary: ManagerOpsSummary
  weekly_summary: ManagerOpsWeeklySummary
  session_rows: ManagerOpsSessionRow[]
}

export interface DailyReportNotification {
  report_date: string
  ready_at: string
  classes_scheduled: number
  classes_taught: number
  classes_missing_report: number
  absent_students: number
  expected_students: number
}

export interface ComplaintNotification {
  id: string
  class_key: string
  student_name: string
  student_phone: string
  urgency: string
  created_at: string
  unread_count: number
}

export interface OpsNotificationSummary {
  daily_report?: DailyReportNotification
  complaint?: ComplaintNotification
}

export const api = {
  getMe: (): Promise<User> => fetchAPI('/me'),

  getMentorClasses: (): Promise<Class[]> => fetchAPI('/mentor/classes'),

  getMentorReminders: (): Promise<{ reminders: MentorReminder[] }> => fetchAPI('/mentor/reminders'),

  getMentorHeadMentors: (): Promise<Mentor[]> => fetchAPI('/mentor-head/mentors'),

  getMentorHeadClasses: (): Promise<MentorGroup[]> => fetchAPI('/mentor-head/classes'),

  getMentorHeadArchive: (
    sort: string = 'oldest',
    from?: string,
    to?: string,
  ): Promise<MentorArchiveGroup[]> => {
    const params = new URLSearchParams()
    if (sort) {
      params.set('sort', sort)
    }
    if (from) {
      params.set('from', from)
    }
    if (to) {
      params.set('to', to)
    }
    const qs = params.toString()
    return fetchAPI(`/mentor-head/archive${qs ? `?${qs}` : ''}`)
  },

  getClassWorkspace: (classKey: string): Promise<ClassDetail> =>
    fetchAPI(`/class-workspace?class_key=${encodeURIComponent(classKey)}`),

  getClassTransferOptions: (leadId: string, sourceClassKey: string): Promise<{ options: ClassTransferOption[] }> =>
    fetchAPI(`/classes/transfer-options?lead_id=${encodeURIComponent(leadId)}&source_class_key=${encodeURIComponent(sourceClassKey)}`),

  transferClassStudent: (payload: {
    lead_id: string
    source_class_key: string
    target_class_key: string
    reason: string
    notes?: string
  }): Promise<{
    ok: boolean
    lead_id: string
    source_class_key: string
    source_exit_after_session_number: number
    target_class_key?: string
    target_joined_at_session_number?: number
    reason: string
  }> =>
    fetchAPI('/classes/transfer', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  returnClassStudentToAdmin: (payload: {
    lead_id: string
    source_class_key: string
    reason: string
    notes?: string
  }): Promise<{
    ok: boolean
    lead_id: string
    source_class_key: string
    source_exit_after_session_number: number
    reason: string
    ops_queue_reason?: string
  }> =>
    fetchAPI('/classes/return-to-admin', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

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

  shiftRoundStartDate: (classKey: string, newStartDate: string): Promise<{ ok: boolean; class_key: string; new_start_date: string }> =>
    fetchAPI('/mentor-head/shift-start-date', {
      method: 'POST',
      body: JSON.stringify({ class_key: classKey, new_start_date: newStartDate }),
    }),

  rescheduleSession: (
    classKey: string,
    sessionId: string,
    newDate: string,
    newTime: string,
  ): Promise<{ ok: boolean; class_key: string; session_id: string; new_date: string; new_time: string }> =>
    fetchAPI('/mentor-head/reschedule-session', {
      method: 'POST',
      body: JSON.stringify({ class_key: classKey, session_id: sessionId, new_date: newDate, new_time: newTime }),
    }),

  closeRound: (classKey: string): Promise<{ ok: boolean }> =>
    fetchAPI('/mentor-head/close-round', {
      method: 'POST',
      body: JSON.stringify({ class_key: classKey }),
    }),

  reopenRound: (classKey: string): Promise<{ ok: boolean }> =>
    fetchAPI('/mentor-head/reopen-round', {
      method: 'POST',
      body: JSON.stringify({ class_key: classKey }),
    }),

  getStudent: (studentId: string, classKey: string): Promise<StudentProfileResponse> =>
    fetchAPI(`/student?student_id=${encodeURIComponent(studentId)}&class_key=${encodeURIComponent(classKey)}`),

  getStudentReportCard: (leadId: string, classKey: string): Promise<StudentProfileResponse> =>
    fetchAPI(`/student?lead_id=${encodeURIComponent(leadId)}&class_key=${encodeURIComponent(classKey)}`),

  getMentorEvaluations: (scope: 'active' | 'closed' = 'active', filters?: {
    q?: string
    from?: string
    to?: string
  }): Promise<{
    scope: 'active' | 'closed'
    filters?: {
      q?: string
      from?: string
      to?: string
    }
    mentors: Array<{
      id: string
      email: string
      name: string
      activeClassCount: number
      classes: Array<{
        classKey: string
        level: number
        days: string
        time: string
        classNumber: number
        roundStatus: 'active' | 'closed'
        manual: {
          sessionQuality: number
          studentsFeedback: number
          trelloSessionChecks: boolean[]
          trelloCompliancePercent: number
        }
        automatic: {
          whatsAppManagementPercent: number
          attendancePunctualityPercent: number
          attendanceStatuses: string[]
        }
      }>
    }>
  }> => {
    const params = new URLSearchParams()
    params.set('scope', scope)
    if (filters?.q) params.set('q', filters.q)
    if (filters?.from) params.set('from', filters.from)
    if (filters?.to) params.set('to', filters.to)
    return fetchAPI(`/mentor-head/evaluations?${params.toString()}`)
  },

  updateMentorEvaluation: (mentorId: string, data: {
    classKey: string
    manual: {
      sessionQuality: number
      studentsFeedback: number
      trelloSessionChecks: boolean[]
    }
  }): Promise<{
    id: string
    classKey: string
    manual: {
      sessionQuality: number
      studentsFeedback: number
      trelloSessionChecks: boolean[]
      trelloCompliancePct: number
    }
  }> => fetchAPI(`/mentor-head/evaluations/${encodeURIComponent(mentorId)}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),

  createMentorTestimonial: (mentorId: string, data: {
    class_key: string
    testimonial_text: string
  }): Promise<MentorTestimonial> => fetchAPI(`/mentor-head/mentors/${encodeURIComponent(mentorId)}/testimonials`, {
    method: 'POST',
    body: JSON.stringify(data),
  }),

  // Student Success endpoints
  getStudentSuccessClasses: (): Promise<{ classes: StudentSuccessClass[] }> =>
    fetchAPI('/student-success/classes'),

  getStudentSuccessPlacementTests: (showCompleted: boolean = false): Promise<{ placement_tests: PlacementTestQueueItem[] }> =>
    fetchAPI(`/student-success/placement-tests?show_completed=${showCompleted ? '1' : '0'}`),

  completePlacementTest: (data: { lead_id: string; assigned_level: number; test_notes: string }): Promise<{ ok: boolean }> =>
    fetchAPI('/student-success/placement-tests/complete', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  getStudentSuccessClass: (classKey: string): Promise<StudentSuccessClassDetail> =>
    fetchAPI(`/student-success/class?class_key=${encodeURIComponent(classKey)}`),

  submitFeedback: (req: SubmitFeedbackRequest): Promise<{ status: string }> =>
    fetchAPI('/student-success/feedback', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  updateFeedbackStatus: (leadID: string, classKey: string, sessionNumber: number, status: 'received' | 'removed'): Promise<{ status: string }> =>
    fetchAPI('/student-success/feedback/status', {
      method: 'POST',
      body: JSON.stringify({ lead_id: leadID, class_key: classKey, session_number: sessionNumber, status }),
    }),

  getFeedbackCollected: (classKey: string): Promise<{ uploads: FeedbackCollectedUpload[] }> =>
    fetchAPI(`/student-success/feedback-collected?class_key=${encodeURIComponent(classKey)}`),

  uploadFeedbackCollected: (data: FormData): Promise<{ id: string; file_url: string; file_name: string }> =>
    fetchAPI('/student-success/feedback-collected', {
      method: 'POST',
      body: data,
      headers: {}, // Let browser set multipart boundary
    }),

  deleteFeedbackCollected: (uploadId: string): Promise<{ ok: boolean }> =>
    fetchAPI(`/student-success/feedback-collected/${encodeURIComponent(uploadId)}`, {
      method: 'DELETE',
    }),

  markAttendance: (
    sessionId: string,
    leadId: string,
    status: string,
    classKey: string,
    notes: string = '',
    taskCompleted?: boolean,
    participationScore?: number
  ): Promise<{ ok: boolean }> =>
    fetchAPI('/attendance', {
      method: 'POST',
      body: JSON.stringify({
        session_id: sessionId,
        lead_id: leadId,
        status,
        class_key: classKey,
        notes,
        task_completed: taskCompleted,
        participation_score: participationScore,
      }),
    }),

  completeSession: (sessionId: string, classKey: string): Promise<{ ok: boolean }> =>
    fetchAPI('/session/complete', {
      method: 'POST',
      body: JSON.stringify({ session_id: sessionId, class_key: classKey }),
    }),

  getAbsenceFeed: (classKey: string, filter: string = '', search: string = ''): Promise<AbsenceFeedItem[]> =>
    fetchAPI(`/student-success/class/absence-feed?class_key=${encodeURIComponent(classKey)}&filter=${encodeURIComponent(filter)}&search=${encodeURIComponent(search)}`),

  getFollowUps: (classKey: string, resolved: boolean = false): Promise<any[]> =>
    fetchAPI(`/student-success/followups?class_key=${encodeURIComponent(classKey)}&resolved=${resolved}`),

  resolveAbsence: (data: { class_key: string; lead_id: string; session_number: number; note?: string; status?: string }): Promise<{ ok: boolean }> =>
    fetchAPI('/student-success/resolve-absence', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

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
    fetchAPI(`/student-success/followups/update`, {
      method: 'POST',
      body: JSON.stringify({ id, status }),
    }),

  resolveFollowUp: (id: string): Promise<{ ok: boolean }> =>
    fetchAPI(`/absence-cases/${encodeURIComponent(id)}/resolve`, {
      method: 'POST',
    }),

  updateFollowUp: (id: string, data: { status: string; note: string; resolved: boolean }): Promise<{ ok: boolean }> =>
    fetchAPI(`/absence-cases/${encodeURIComponent(id)}/follow-up`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Complaints endpoints
  createComplaint: (data: {
    class_key: string
    student_phone: string
    category: string
    complaint_text: string
    urgency: string
  }): Promise<any> =>
    fetchAPI('/student-success/complaints', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Mentor Head - Complaints
  getMentorHeadComplaints: (showResolved: boolean = false): Promise<{ complaints: any[] }> =>
    fetchAPI(`/mentor-head/complaints?show_resolved=${showResolved ? '1' : '0'}`),

  updateComplaintStatus: (id: string, status: string, note: string): Promise<any> =>
    fetchAPI(`/mentor-head/complaints/${id}/update`, {
      method: 'POST',
      body: JSON.stringify({ status, note }),
    }),

  resolveComplaint: (id: string, note: string): Promise<any> =>
    fetchAPI(`/mentor-head/complaints/${id}/resolve`, {
      method: 'POST',
      body: JSON.stringify({ resolution_note: note }),
    }),

  // Late Joiner Notifications
  getLateJoinerNotifications: (): Promise<LateJoinerNotification[]> =>
    fetchAPI('/notifications/late-join'),

  acknowledgeLateJoinerNotification: (notificationId: string): Promise<{ ok: boolean }> =>
    fetchAPI(`/notifications/late-join/${encodeURIComponent(notificationId)}/acknowledge`, {
      method: 'POST',
    }),

  getOpsNotifications: (): Promise<OpsNotificationSummary> =>
    fetchAPI('/notifications/ops'),

  markComplaintRead: (complaintId: string): Promise<{ ok: boolean }> =>
    fetchAPI(`/notifications/complaints/${encodeURIComponent(complaintId)}/read`, {
      method: 'POST',
    }),

  // Mentor Directory API (Mentor Head/Admin)
  getMentors: (): Promise<{ mentors: MentorDirectoryItem[] }> =>
    fetchAPI('/mentors'),

  getMentorProfile: (mentorId: string): Promise<MentorProfileResponse> =>
    fetchAPI(`/mentors/${encodeURIComponent(mentorId)}/profile`),

  searchStudents: (query: string): Promise<StudentSearchResult[]> =>
    fetchAPI(`/students/search?q=${encodeURIComponent(query)}`),

  getStudentProfile: (studentId: string): Promise<UniversalStudentProfile> =>
    fetchAPI(`/students/${encodeURIComponent(studentId)}/profile`),

  getStudentHistory: (studentId: string): Promise<AcademicHistoryItem[]> =>
    fetchAPI(`/students/${encodeURIComponent(studentId)}/history`),

  getStudentCurrentStatus: (studentId: string): Promise<CurrentClassStatus | null> =>
    fetchAPI(`/students/${encodeURIComponent(studentId)}/current-status`),

  getStudentNotes: (studentId: string): Promise<TimelineItem[]> =>
    fetchAPI(`/students/${encodeURIComponent(studentId)}/notes`),

  getGrades: (classKey: string): Promise<{ grades: Grade[] }> =>
    fetchAPI(`/grades?class_key=${encodeURIComponent(classKey)}`),

  getGradePreview: (classKey: string): Promise<{ previews: GradePreview[] }> =>
    fetchAPI(`/grades/preview?class_key=${encodeURIComponent(classKey)}`),

  createGrade: (data: { lead_id: string; class_key: string; grade: string; notes: string }): Promise<{ ok: boolean; grade_id: string }> =>
    fetchAPI('/grades', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  deleteGrade: (leadId: string, classKey: string): Promise<{ ok: boolean }> =>
    fetchAPI(`/grades?lead_id=${encodeURIComponent(leadId)}&class_key=${encodeURIComponent(classKey)}`, {
      method: 'DELETE',
    }),

  upsertComplianceCheck: (data: {
    class_session_id: string
    reminder_1d: boolean
    reminder_1h: boolean
    reminder_tasks: boolean
    delay_minutes: number
    is_absent: boolean
  }): Promise<{ success: boolean; check: ComplianceCheck }> =>
    fetchAPI('/compliance/check', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  getComplianceByClass: (classKey: string): Promise<{ class_key: string; sessions: ComplianceClassSession[] }> =>
    fetchAPI(`/compliance/class/${encodeURIComponent(classKey)}`),

  getMentorReports: (params: { round_status?: 'active' | 'closed'; mentor_id?: string } = {}): Promise<{ items: MentorReportItem[] }> => {
    const qs = new URLSearchParams()
    if (params.round_status) qs.set('round_status', params.round_status)
    if (params.mentor_id) qs.set('mentor_id', params.mentor_id)
    const query = qs.toString()
    return fetchAPI(`/reports/mentors${query ? `?${query}` : ''}`)
  },

  getMentorReportChecklist: (params: { mentor_id: string; round_status?: 'active' | 'closed' }): Promise<{ items: MentorReportChecklistItem[] }> => {
    const qs = new URLSearchParams()
    qs.set('mentor_id', params.mentor_id)
    if (params.round_status) qs.set('round_status', params.round_status)
    return fetchAPI(`/reports/mentors/checklist?${qs.toString()}`)
  },

  getMentorClassReports: (params: { round_status?: 'active' | 'closed'; mentor_id?: string } = {}): Promise<{ items: MentorClassReportItem[] }> => {
    const qs = new URLSearchParams()
    if (params.round_status) qs.set('round_status', params.round_status)
    if (params.mentor_id) qs.set('mentor_id', params.mentor_id)
    const query = qs.toString()
    return fetchAPI(`/reports/mentors/classes${query ? `?${query}` : ''}`)
  },

  excludeMentorReportRow: (data: { mentor_id: string; round_status?: 'all' | 'active' | 'closed'; reason?: string }): Promise<{ success: boolean }> =>
    fetchAPI('/reports/mentors/exclude', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  getBIReports: (params: { from?: string; to?: string } = {}): Promise<BIReportPayload> => {
    const qs = new URLSearchParams()
    if (params.from) qs.set('from', params.from)
    if (params.to) qs.set('to', params.to)
    const query = qs.toString()
    return fetchAPI(`/reports/bi${query ? `?${query}` : ''}`)
  },

  getDailyReport: (date?: string): Promise<DailyReportPayload> => {
    const qs = new URLSearchParams()
    if (date) qs.set('date', date)
    const query = qs.toString()
    return fetchAPI(`/reports/daily${query ? `?${query}` : ''}`)
  },

  getManagerOpsReport: (date?: string): Promise<ManagerOpsPayload> => {
    const qs = new URLSearchParams()
    if (date) qs.set('date', date)
    const query = qs.toString()
    return fetchAPI(`/reports/manager-ops${query ? `?${query}` : ''}`)
  },

  markDailyReportRead: (reportDate: string): Promise<{ ok: boolean }> =>
    fetchAPI('/reports/daily/read', {
      method: 'POST',
      body: JSON.stringify({ report_date: reportDate }),
    }),

  forceChangePassword: (newPassword: string): Promise<{ ok: boolean; redirect: string }> =>
    fetchAPI('/auth/force-change-password', {
      method: 'POST',
      body: JSON.stringify({ new_password: newPassword }),
    }),

  getStaffUsers: (): Promise<{ users: StaffUser[] }> => fetchAPI('/manager/users'),

  createStaffUser: (payload: {
    full_name: string
    email: string
    role: string
    temporary_password: string
  }): Promise<{
    id: string
    email: string
    full_name: string
    role: string
    must_change_password: boolean
  }> =>
    fetchAPI('/manager/users', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  deleteStaffUser: (userId: string): Promise<{ ok: boolean }> =>
    fetchAPI(`/manager/users/${encodeURIComponent(userId)}`, {
      method: 'DELETE',
    }),

  deactivateStaffUser: (userId: string): Promise<{ ok: boolean }> =>
    fetchAPI(`/manager/users/${encodeURIComponent(userId)}/deactivate`, {
      method: 'POST',
    }),
}

// Student Profile Types (Milestone 4)
export interface StudentSearchResult {
  lead_id: string
  full_name: string
  phone: string
  current_level: number
  status: string
}

export interface UniversalStudentProfile {
  lead_id: string
  full_name: string
  phone: string
  current_level: number
  remaining_credits: number
  status: string
  is_returning: boolean
}

export interface AcademicHistoryItem {
  id: string
  level: number
  class_days: string
  class_time: string
  mentor_name: string
  final_grade: string
  outcome: string
  enrolled_at: string
  completed_at: string | null
}

export interface CurrentClassStatus {
  class_key: string
  level: number
  class_days: string
  class_time: string
  mentor_name: string
  current_session: number
  attendance_stats: {
    present: number
    absent: number
    late: number
    total: number
  }
  session_details: Array<{
    session_number: number
    status: string
    date: string
  }>
}

export interface TimelineItem {
  id: string
  type: 'note' | 'followup' | 'grade_note'
  text: string
  class_key: string
  session: number
  is_private: boolean
  created_by: string
  created_at: string
}

export interface LateJoinerNotification {
  id: string
  lead_id: string
  full_name: string
  class_key: string
  joined_at_session_number: number
  created_at: string
}

export interface StaffUser {
  id: string
  full_name: string
  email: string
  role: string
  is_active: boolean
  must_change_password: boolean
  created_at: string
}

export interface FollowUpCaseNote {
  id: string
  case_id: string
  note_text: string
  note_type: string // comment, status_change, resolution, system
  created_at: string
  created_by_user_id: string
  created_by_email?: string
}

export interface ComplaintListItem {
  id: string
  class_key: string
  student_name: string
  student_phone: string
  category: string
  urgency: string
  status: string
  complaint_text: string
  last_note: string
  created_at: string
  resolved: boolean
  resolved_at?: string
  notes?: FollowUpCaseNote[]
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
  joinedAtSessionNumber?: number // NEW
  followUp?: {
    id: string
    status: string
    lastNote: string
    updatedAt: string
    resolved: boolean
    resolvedAt?: string
    notes?: FollowUpCaseNote[]
  }
}
