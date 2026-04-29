package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID                 uuid.UUID
	Email              string
	FullName           sql.NullString
	Phone              sql.NullString
	PasswordHash       string
	Role               string
	IsActive           bool
	MustChangePassword bool
	CreatedAt          time.Time
}

type Lead struct {
	ID                     uuid.UUID
	FullName               string
	Phone                  string
	Source                 sql.NullString
	Notes                  sql.NullString
	Status                 string
	OpsQueueReason         sql.NullString
	MentorHeadReturnReason sql.NullString
	SentToClasses          bool           // Whether student has been manually sent to classes board
	LevelsPurchasedTotal   sql.NullInt32  // Total levels purchased (from bundles)
	LevelsConsumed         sql.NullInt32  // Levels consumed (when rounds start)
	BundleType             sql.NullString // none, single, bundle2, bundle3, bundle4
	RemainingCredits       sql.NullInt32  // Remaining class credits (after-class pipeline)
	IsReturning            bool           // Flag for returning students (after-class pipeline)
	HighPriorityFollowUp   bool           // Set by mentor_head on round close for students with no remaining credits
	HighPriorityAbsence    bool           // Set automatically if 3+ absences
	HighPriorityReason     sql.NullString // Reason for high priority
	CreatedByUserID        sql.NullString
	OfferSentAt            sql.NullTime
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type PlacementTest struct {
	ID                         uuid.UUID
	LeadID                     uuid.UUID
	TestDate                   sql.NullTime
	TestTime                   sql.NullString
	TestType                   sql.NullString
	AssignedLevel              sql.NullInt32
	TestNotes                  sql.NullString
	RunByUserID                sql.NullString
	PlacementTestFee           sql.NullInt32
	PlacementTestFeePaid       sql.NullInt32
	PlacementTestPaymentDate   sql.NullTime
	PlacementTestPaymentMethod sql.NullString
	DiscountValue              sql.NullInt32
	DiscountType               sql.NullString
	UpdatedAt                  time.Time
}

type Offer struct {
	ID            uuid.UUID
	LeadID        uuid.UUID
	BundleLevels  sql.NullInt32
	BasePrice     sql.NullInt32
	DiscountValue sql.NullInt32
	DiscountType  sql.NullString
	FinalPrice    sql.NullInt32
	UpdatedAt     time.Time
}

type Booking struct {
	ID            uuid.UUID
	LeadID        uuid.UUID
	BookFormat    sql.NullString
	Address       sql.NullString
	City          sql.NullString
	DeliveryNotes sql.NullString
	UpdatedAt     time.Time
}

type Payment struct {
	ID               uuid.UUID
	LeadID           uuid.UUID
	PaymentType      sql.NullString
	AmountPaid       sql.NullInt32
	RemainingBalance sql.NullInt32
	PaymentDate      sql.NullTime
	UpdatedAt        time.Time
}

type Scheduling struct {
	ID              uuid.UUID
	LeadID          uuid.UUID
	ExpectedRound   sql.NullString
	ClassDays       sql.NullString
	ClassTime       sql.NullString
	StartDate       sql.NullTime
	StartTime       sql.NullString
	ClassGroupIndex sql.NullInt32 // Which class group (1, 2, 3...) for same level+days+time
	UpdatedAt       time.Time
}

type Shipping struct {
	ID             uuid.UUID
	LeadID         uuid.UUID
	ShipmentStatus sql.NullString
	ShipmentDate   sql.NullTime
	UpdatedAt      time.Time
}

type LeadDetail struct {
	Lead          *Lead
	PlacementTest *PlacementTest
	Offer         *Offer
	Booking       *Booking
	Payment       *Payment
	Scheduling    *Scheduling
	Shipping      *Shipping
}

type LeadListItem struct {
	Lead                  *Lead
	AssignedLevel         sql.NullInt32
	ClassDays             sql.NullString
	ClassTime             sql.NullString
	LastOutcome           sql.NullString // latest class_enrollments.outcome (promoted/repeated)
	LastFinalGrade        sql.NullString // latest class_enrollments.final_grade
	RefusedRenewal        bool           // latest renewal_refusals marker exists
	RefusedRenewalAt      sql.NullTime   // latest refusal timestamp
	PaymentStatus         string
	PaymentState          string // UNPAID, DEPOSIT, PAID_FULL
	NextAction            string
	WhatsAppURL           string
	DaysSinceLastProgress int
	HotLevel              string // "HOT", "WARM", "COOL", or ""
	FollowUpDue           bool
	OfferFollowUpStep     int
	OfferFollowUpLastStep int
	OfferFollowUpLastSent sql.NullTime
	OfferFollowUpDueAt    sql.NullTime
	OfferFollowUpDueNow   bool
	OfferReminderAt       sql.NullTime
	OfferReminderNote     sql.NullString
	OfferReminderDue      bool
	SnoozedUntil          sql.NullTime
	SnoozeNote            sql.NullString
	SnoozeDue             bool
	TestDate              sql.NullTime  // For computing days since progress
	AmountPaid            sql.NullInt32 // For checking if paid
	FinalPrice            sql.NullInt32 // For computing payment state
	RemainingBalance      sql.NullInt32 // For computing payment state
	SleepingLeadStep      int
	SleepingLeadLastStep  int
	SleepingLeadLastSent  sql.NullTime
	SleepingLeadDueAt     sql.NullTime
	SleepingLeadDueNow    bool
	SleepingReminderAt    sql.NullTime
	SleepingReminderNote  sql.NullString
	SleepingReminderDue   bool
}

type SleepingLeadReminder struct {
	LeadID            uuid.UUID
	FollowUpAt        time.Time
	Note              sql.NullString
	ScheduledByUserID sql.NullString
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type OfferSentReminder struct {
	LeadID            uuid.UUID
	FollowUpAt        time.Time
	Note              sql.NullString
	ScheduledByUserID sql.NullString
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type LeadSnooze struct {
	LeadID            uuid.UUID
	SnoozedUntil      time.Time
	Note              sql.NullString
	ScheduledByUserID sql.NullString
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type LastClassOutcome struct {
	Outcome     sql.NullString
	FinalGrade  sql.NullString
	CompletedAt sql.NullTime
}

type PlacementTestQueueItem struct {
	LeadID        uuid.UUID
	FullName      string
	Phone         string
	Status        string
	TestDate      sql.NullTime
	TestTime      sql.NullString
	TestType      sql.NullString
	AssignedLevel sql.NullInt32
	TestNotes     sql.NullString
}

// ClassGroup represents a group of students with same level+days+time
type ClassGroup struct {
	Level        int32
	ClassDays    string
	ClassTime    string
	GroupIndex   int32 // 1, 2, 3...
	StudentCount int
	Readiness    string // "LOCKED", "READY", "NOT READY"
	Students     []*ClassStudent
	ClassKey     string // Stable identifier: "L{level}|{days}|{time}|{index}"
	SentToMentor bool   // Whether this class has been sent to mentor head
	SentAt       sql.NullTime
	ReturnedAt   sql.NullTime
	RoundStatus  string // not_started | active | closed
	// Current session for active rounds (computed from completed sessions + 1)
	CurrentSession sql.NullInt32
}

// ClassGroupWorkflow tracks workflow state for a class group
type ClassGroupWorkflow struct {
	ClassKey           string
	Level              int32
	ClassDays          string
	ClassTime          string
	ClassNumber        int32
	CompletedSessions  int32
	SentToMentor       bool
	SentAt             sql.NullTime
	ReturnedAt         sql.NullTime
	UpdatedAt          time.Time
	HiddenInOps        bool
	HiddenAt           sql.NullTime
	HiddenBy           sql.NullString // user UUID
	RoundStatus        string         // not_started | active | closed
	RoundStartedAt     sql.NullTime
	RoundStartedBy     sql.NullString // user UUID
	ClosedMentorUserID sql.NullString // user UUID
	RoundClosedAt      sql.NullTime
	RoundClosedBy      sql.NullString // user UUID
}

// MentorReminder represents a reminder for a mentor to complete an action
type MentorReminder struct {
	Type          string `json:"type"` // "attendance" or "grading"
	ClassKey      string `json:"class_key"`
	Level         int32  `json:"level"`
	ClassDays     string `json:"class_days"`
	ClassTime     string `json:"class_time"`
	ClassNumber   int32  `json:"class_number"`
	SessionNumber int32  `json:"session_number"` // For attendance reminders
	Message       string `json:"message"`
}

// ClassStudent represents a student in a class group
type ClassStudent struct {
	LeadID                uuid.UUID         `json:"lead_id"`
	FullName              string            `json:"full_name"`
	Phone                 string            `json:"phone"`
	IsReturning           bool              `json:"is_returning"` // Flag for returning/promoted students
	RemainingCredits      int32             `json:"remaining_credits"`
	GroupIndex            sql.NullInt32     `json:"group_index"`
	AvailableGroups       []int32           `json:"available_groups"`         // Available group indices for move (computed in handler)
	AvailableClassOptions []MoveClassOption `json:"available_class_options"`  // Available class options for move (computed in handler)
	JoinedAtSessionNumber sql.NullInt32     `json:"joined_at_session_number"` // NEW: For late joiners
}

type MoveClassOption struct {
	Value string
	Label string
}

type ClassMembership struct {
	ID                     uuid.UUID
	LeadID                 uuid.UUID
	ClassKey               string
	JoinedAtSessionNumber  int32
	LeftAfterSessionNumber sql.NullInt32
	JoinReason             string
	LeaveReason            sql.NullString
	AddedByUserID          sql.NullString
	RemovedByUserID        sql.NullString
	RemovedAt              sql.NullTime
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type ClassTransferOption struct {
	ClassKey          string `json:"class_key"`
	Level             int32  `json:"level"`
	ClassDays         string `json:"class_days"`
	ClassTime         string `json:"class_time"`
	ClassNumber       int32  `json:"class_number"`
	RoundStatus       string `json:"round_status"`
	CurrentSession    int32  `json:"current_session"`
	CurrentEnrollment int32  `json:"current_enrollment"`
}

type ClassRosterChangeResult struct {
	LeadID                       uuid.UUID
	SourceClassKey               string
	TargetClassKey               sql.NullString
	SourceExitAfterSessionNumber int32
	TargetJoinedAtSessionNumber  sql.NullInt32
	Reason                       string
	OpsQueueReason               sql.NullString
}

// Transaction represents a financial transaction (IN or OUT)
type Transaction struct {
	ID              uuid.UUID
	TransactionDate time.Time
	TransactionType string // "IN" or "OUT"
	Category        string // placement_test, course_payment, teacher_salary, refund, ads, rent, software, moderator, content_creator, other
	Amount          int32
	PaymentMethod   sql.NullString // vodafone_cash, bank_transfer, paypal, other
	LeadID          sql.NullString // Optional: link to lead for income/refunds (stored as UUID in DB, but we use string for null handling)
	Notes           sql.NullString
	SourceKey       sql.NullString // Deprecated: use RefKey instead
	RefType         sql.NullString // "lead" or other reference type
	RefID           sql.NullString // Reference ID (e.g., lead ID)
	RefSubType      sql.NullString // "placement_test", "course_payment", etc.
	RefKey          sql.NullString // Unique key for updates: "lead:<id>:placement_test" or "lead:<id>:course_payment:<payment_id>"
	BundleLevels    sql.NullInt32  // For course payments: 1, 2, 3, or 4
	LevelsPurchased sql.NullInt32  // For course payments: how many levels this payment represents
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// LeadPayment represents a course payment record (supports multiple payments per lead)
type LeadPayment struct {
	ID            uuid.UUID
	LeadID        uuid.UUID
	Kind          string // "course"
	Amount        int32
	PaymentMethod string
	PaymentDate   time.Time
	Notes         sql.NullString
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RenewalRefusal struct {
	ID              uuid.UUID
	LeadID          uuid.UUID
	RefusedAt       time.Time
	RefusedByUserID sql.NullString
	Reason          string
	Notes           sql.NullString
	CreatedAt       time.Time
}

// PaymentMethodBalance holds IN/OUT/Net for a payment-method bucket (e.g. Cash vs Bank)
type PaymentMethodBalance struct {
	Label string
	In    int32
	Out   int32
	Net   int32
}

// FinanceSummary represents aggregated finance data
type FinanceSummary struct {
	TodayIN              int32
	TodayOUT             int32
	TodayNet             int32
	RangeIN              int32
	RangeOUT             int32
	RangeNet             int32
	INByCategory         map[string]int32
	OUTByCategory        map[string]int32
	TotalRemainingLevels int32
	StudentsWithCredits  int
	CreditsTracked       int
	CreditsBreakdown     map[string]int // "0", "1", "2", "3+"
}

// CancelledLeadSummary represents financial summary for a cancelled lead
type CancelledLeadSummary struct {
	LeadID            uuid.UUID
	FullName          string
	Phone             string
	CancelledAt       sql.NullTime
	PlacementTestPaid int32 // Not refundable
	CoursePaid        int32 // Total course payments
	Refunded          int32 // Total refunds issued
	NetMoney          int32 // Course paid - refunded (positive = we owe, negative = we kept)
}

// LedgerDayGroup represents a group of transactions for a single day with daily totals
type LedgerDayGroup struct {
	Date         time.Time
	DateLabel    string // "2026-01-24"
	InTotal      int32
	OutTotal     int32
	NetTotal     int32
	Transactions []*Transaction
}

// Milestone 2: Active Classes models

// ClassSession represents a single session (1-8) for a class
type ClassSession struct {
	ID               uuid.UUID
	ClassKey         string
	SessionNumber    int32
	ScheduledDate    time.Time
	ScheduledTime    sql.NullString
	ScheduledEndTime sql.NullString
	ActualDate       sql.NullTime
	ActualTime       sql.NullString
	ActualEndTime    sql.NullString
	Status           string       // 'scheduled', 'completed', 'cancelled'
	CompletedAt      sql.NullTime // Timestamp when marked completed (for refund rule)
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Attendance represents attendance record for a student in a session
type Attendance struct {
	ID             uuid.UUID
	SessionID      uuid.UUID
	LeadID         uuid.UUID
	Status         string // 'PRESENT', 'ABSENT', 'LATE'
	Notes          sql.NullString
	MarkedByUserID sql.NullString
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SessionPerformance stores per-session task + participation signals for grading automation.
type SessionPerformance struct {
	ID                 uuid.UUID
	ClassSessionID     uuid.UUID
	LeadID             uuid.UUID
	TaskCompleted      bool
	ParticipationScore int32 // 1..5 stars
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Grade represents a grade (A/B/C/F) assigned at session 8
type Grade struct {
	ID              uuid.UUID
	LeadID          uuid.UUID
	ClassKey        string
	SessionNumber   int32  // Always 8
	Grade           string // 'A', 'B', 'C', 'F'
	Notes           sql.NullString
	CreatedByUserID sql.NullString
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type GradePreview struct {
	LeadID                 uuid.UUID
	Absences               int
	CompletedTasks         int
	AttendedSessions       int
	AverageParticipation   float64
	AttendanceScore        float64
	TaskScore              float64
	ParticipationScore     float64
	TotalScore             float64
	CalculatedGrade        string
	UsedLegacyTaskFallback bool
}

// StudentNote represents a carry-over note for a student
type StudentNote struct {
	ID              uuid.UUID
	LeadID          uuid.UUID
	ClassKey        sql.NullString
	SessionNumber   sql.NullInt32
	NoteText        string
	IsPrivate       bool
	CreatedByUserID sql.NullString
	CreatedByEmail  sql.NullString // Email of the user who created the note
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// StudentSuccessFeedback represents feedback submitted at sessions 4 or 8
type StudentSuccessFeedback struct {
	ID               uuid.UUID
	LeadID           uuid.UUID
	ClassKey         string
	SessionNumber    int32 // 4 or 8
	FeedbackText     string
	FollowUpRequired bool
	Status           string // sent, received, removed
	CreatedByUserID  sql.NullString
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// FeedbackCollectedUpload represents an uploaded feedback file from a student.
type FeedbackCollectedUpload struct {
	ID             uuid.UUID
	LeadID         uuid.UUID
	ClassKey       string
	SessionNumber  sql.NullInt32
	FileName       string
	FileURL        string
	MimeType       sql.NullString
	SizeBytes      sql.NullInt32
	Note           sql.NullString
	UploadedByUser sql.NullString
	UploadedAt     time.Time
}

// AbsenceFollowUpLog represents a follow-up action logged for an absence
type AbsenceFollowUpLog struct {
	ID              uuid.UUID
	LeadID          uuid.UUID
	SessionID       sql.NullString
	MessageSent     bool
	Reason          sql.NullString
	StudentReply    sql.NullString
	ActionTaken     sql.NullString
	Notes           sql.NullString
	CreatedByUserID sql.NullString
	CreatedAt       time.Time
}

// MentorAssignment represents assignment of a mentor (user with role='mentor') to a class
type MentorAssignment struct {
	ID              uuid.UUID
	MentorUserID    uuid.UUID // References users.id
	ClassKey        string
	AssignedAt      time.Time
	CreatedByUserID sql.NullString
}

// MentorEvaluation represents KPI and attendance evaluation for a mentor
type MentorEvaluation struct {
	ID                   uuid.UUID
	MentorID             uuid.UUID
	ClassKey             string
	KPISessionQuality    int
	KPISessionQualityByS []int
	KPIStudentsFeedback  int
	TrelloSessionChecks  []bool
	EvaluatorID          uuid.UUID
	UpdatedAt            time.Time
}

type MentorEvaluationClassItem struct {
	ClassKey             string
	Level                int32
	ClassDays            string
	ClassTime            string
	ClassNumber          int32
	RoundStatus          string
	KPISessionQuality    int
	KPISessionQualityByS []int
	KPIStudentsFeedback  int
	TrelloSessionChecks  []bool
	AutoWhatsAppPercent  int
	AttendanceStatuses   []string
	AttendancePercent    int
	RecordedSessionCount int
	ClassCollectiveScore int
}

type MentorEvaluationMentorItem struct {
	User          *User
	ActiveClasses []MentorEvaluationClassItem
}

type MentorDirectoryItem struct {
	ID                 uuid.UUID
	Name               string
	Email              string
	Phone              string
	Status             string
	TotalClassesTaught int
}

type MentorClassHistoryItem struct {
	ClassKey        string
	Level           int32
	Days            string
	Time            string
	StartDate       sql.NullTime
	EndDate         sql.NullTime
	Duration        string
	EvaluationScore int
	ComplianceScore int
}

type MentorProfileStats struct {
	TotalClasses    int
	FirstClassDate  sql.NullTime
	LastClassDate   sql.NullTime
	AvgRating       int
	FeedbackMeter   int
	ComplianceScore int
}

type MentorTestimonial struct {
	ID              uuid.UUID
	MentorID        uuid.UUID
	ClassKey        string
	TestimonialText string
	CreatedByUserID uuid.UUID
	CreatedByEmail  string
	CreatedAt       time.Time
}

type MentorProfile struct {
	MentorDetails MentorDirectoryItem
	Stats         MentorProfileStats
	ClassHistory  []MentorClassHistoryItem
	Testimonials  []MentorTestimonial
}

// FollowUp represents a follow-up action for an absence
type FollowUp struct {
	ID               uuid.UUID      `json:"id"`
	ClassKey         string         `json:"class_key"`
	LeadID           uuid.UUID      `json:"lead_id"`
	SessionNumber    int32          `json:"session_number"`
	Note             sql.NullString `json:"note"`
	Status           string         `json:"status"` // none, contacted, replied, no_response
	CreatedBy        sql.NullString `json:"created_by"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	Resolved         bool           `json:"resolved"`
	ResolvedAt       sql.NullTime   `json:"resolved_at"`
	ResolvedByUserID sql.NullString `json:"resolved_by_user_id"`
}

// AbsenceFeedItem represents an item in the absence feed
type AbsenceFeedItem struct {
	SessionNumber         int32         `json:"sessionNumber"`
	SessionDate           string        `json:"sessionDate"`
	StartTime             string        `json:"startTime"`
	StudentID             uuid.UUID     `json:"studentId"`
	StudentName           string        `json:"studentName"`
	StudentPhone          string        `json:"studentPhone"`
	Status                string        `json:"status"` // PRESENT, ABSENT, LATE, EXCUSED
	MarkedBy              string        `json:"markedBy"`
	MarkedAt              time.Time     `json:"markedAt"`
	MentorNote            string        `json:"mentorNote"`
	JoinedAtSessionNumber *int32        `json:"joinedAtSessionNumber,omitempty"`
	FollowUp              *FollowUpInfo `json:"followUp"`
}

type FollowUpInfo struct {
	ID         uuid.UUID           `json:"id"`
	Status     string              `json:"status"`
	LastNote   string              `json:"lastNote"`
	UpdatedAt  time.Time           `json:"updatedAt"`
	Resolved   bool                `json:"resolved"`
	ResolvedAt *time.Time          `json:"resolvedAt,omitempty"`
	Notes      []*FollowUpCaseNote `json:"notes,omitempty"`
}

type FollowUpListItem struct {
	ID               uuid.UUID           `json:"id"`
	LeadID           uuid.UUID           `json:"lead_id"`
	StudentName      string              `json:"student_name"`
	StudentPhone     string              `json:"student_phone"`
	SessionNumber    int32               `json:"session_number"`
	AttendanceStatus string              `json:"attendance_status"`
	Note             string              `json:"note"`
	Status           string              `json:"status"`
	CreatedAt        time.Time           `json:"created_at"`
	Resolved         bool                `json:"resolved"`
	ResolvedAt       *time.Time          `json:"resolved_at,omitempty"`
	Notes            []*FollowUpCaseNote `json:"notes,omitempty"`
}

// ComplaintCase represents a complaint filed by Student Success
type ComplaintCase struct {
	ID               uuid.UUID      `json:"id"`
	Type             string         `json:"type"` // Always "complaint"
	ClassKey         string         `json:"class_key"`
	LeadID           sql.NullString `json:"lead_id"`
	StudentPhone     string         `json:"student_phone"`
	SessionNumber    sql.NullInt32  `json:"session_number"`
	Category         string         `json:"category"`
	ComplaintText    string         `json:"complaint_text"`
	Urgency          string         `json:"urgency"` // low, medium, high
	Status           string         `json:"status"`  // open, contacted, investigating, resolved
	CreatedBy        uuid.UUID      `json:"created_by"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	Resolved         bool           `json:"resolved"`
	ResolvedAt       sql.NullTime   `json:"resolved_at"`
	ResolvedByUserID sql.NullString `json:"resolved_by_user_id"`
	DeletedAt        sql.NullTime   `json:"deleted_at"`
	DeletedByUserID  sql.NullString `json:"deleted_by_user_id"`
	DeleteReason     sql.NullString `json:"delete_reason"`
}

// FollowUpCaseNote represents a note/action on a follow-up case
type FollowUpCaseNote struct {
	ID              uuid.UUID `json:"id"`
	CaseID          uuid.UUID `json:"case_id"`
	NoteText        string    `json:"note_text"`
	NoteType        string    `json:"note_type"` // comment, status_change, resolution, system
	CreatedAt       time.Time `json:"created_at"`
	CreatedByUserID uuid.UUID `json:"created_by_user_id"`
	CreatedByEmail  string    `json:"created_by_email,omitempty"`
}

// ComplaintListItem for API responses
type ComplaintListItem struct {
	ID            uuid.UUID           `json:"id"`
	ClassKey      string              `json:"class_key"`
	StudentName   string              `json:"student_name"`
	StudentPhone  string              `json:"student_phone"`
	Category      string              `json:"category"`
	Urgency       string              `json:"urgency"`
	Status        string              `json:"status"`
	ComplaintText string              `json:"complaint_text"`
	LastNote      string              `json:"last_note"`
	CreatedAt     time.Time           `json:"created_at"`
	Resolved      bool                `json:"resolved"`
	ResolvedAt    *time.Time          `json:"resolved_at,omitempty"`
	Notes         []*FollowUpCaseNote `json:"notes"`
}

// DailyReportPayload is the operational daily class report shown to Mentor Head and Manager.
type DailyReportPayload struct {
	ReportDate                 string                         `json:"report_date"`
	ReadyAt                    string                         `json:"ready_at"`
	GeneratedAt                string                         `json:"generated_at"`
	RankingFrom                string                         `json:"ranking_from"`
	RankingTo                  string                         `json:"ranking_to"`
	ClassesScheduled           int                            `json:"classes_scheduled"`
	ClassesTaught              int                            `json:"classes_taught"`
	ClassesMissingReport       int                            `json:"classes_missing_report"`
	ExpectedStudents           int                            `json:"expected_students"`
	AbsentStudents             int                            `json:"absent_students"`
	SessionsLiveNow            int                            `json:"sessions_live_now"`
	StudentsInClassesCount     int                            `json:"students_in_classes_count"`
	AbsentStudentsRanking      []ManagerOpsWeeklyMentorLeader `json:"absent_students_ranking"`
	LateStartsRanking          []ManagerOpsWeeklyMentorLeader `json:"late_starts_ranking"`
	StudentsOverAbsenceRanking []DailyReportStudentLeader     `json:"students_over_absence_ranking"`
	SessionRows                []*ManagerOpsSessionRow        `json:"session_rows"`
}

// DailyReportClassRow summarizes one class session expected on the report date.
type DailyReportClassRow struct {
	SessionID         uuid.UUID `json:"session_id"`
	ClassKey          string    `json:"class_key"`
	ClassLabel        string    `json:"class_label"`
	MentorID          uuid.UUID `json:"mentor_id"`
	MentorEmail       string    `json:"mentor_email"`
	SessionNumber     int32     `json:"session_number"`
	ScheduledDate     string    `json:"scheduled_date"`
	ScheduledTime     string    `json:"scheduled_time"`
	ActualTime        string    `json:"actual_time"`
	SessionStatus     string    `json:"session_status"`
	ReportStatus      string    `json:"report_status"`
	PunctualityStatus string    `json:"punctuality_status"`
	DelayMinutes      int       `json:"delay_minutes"`
	ComplianceChecked bool      `json:"compliance_checked"`
	MentorAbsent      bool      `json:"mentor_absent"`
	ExpectedStudents  int       `json:"expected_students"`
	AbsentStudents    int       `json:"absent_students"`
}

type DailyReportStudentLeader struct {
	LeadID       string `json:"lead_id"`
	StudentName  string `json:"student_name"`
	StudentPhone string `json:"student_phone"`
	MetricValue  int    `json:"metric_value"`
}

// ManagerOpsPayload is the manager-only daily operations view for a single Cairo business day.
type ManagerOpsPayload struct {
	ReportDate                 string                         `json:"report_date"`
	Timezone                   string                         `json:"timezone"`
	GeneratedAt                string                         `json:"generated_at"`
	RankingFrom                string                         `json:"ranking_from"`
	RankingTo                  string                         `json:"ranking_to"`
	Summary                    ManagerOpsSummary              `json:"summary"`
	WeeklySummary              ManagerOpsWeeklySummary        `json:"weekly_summary"`
	AbsentStudentsRanking      []ManagerOpsWeeklyMentorLeader `json:"absent_students_ranking"`
	LateStartsRanking          []ManagerOpsWeeklyMentorLeader `json:"late_starts_ranking"`
	StudentsOverAbsenceRanking []DailyReportStudentLeader     `json:"students_over_absence_ranking"`
	SessionRows                []*ManagerOpsSessionRow        `json:"session_rows"`
}

type ManagerOverviewPayload struct {
	Timezone                  string                           `json:"timezone"`
	GeneratedAt               string                           `json:"generated_at"`
	Summary                   ManagerOverviewSummary           `json:"summary"`
	PreEnrolmentStatusBuckets []ManagerOverviewStatusBreakdown `json:"pre_enrolment_status_buckets"`
	WaitingListLevelBuckets   []ManagerWaitingListLevelBucket  `json:"waiting_list_level_buckets"`
}

type ManagerOverviewSummary struct {
	StudentsInClassesCount int   `json:"students_in_classes_count"`
	PreEnrolmentCount      int   `json:"pre_enrolment_count"`
	WaitingListCount       int   `json:"waiting_list_count"`
	CurrentCashBalance     int32 `json:"current_cash_balance"`
	RunningClassesCount    int   `json:"running_classes_count"`
	ActiveMentorsCount     int   `json:"active_mentors_count"`
}

type ManagerOverviewStatusBreakdown struct {
	StatusKey string `json:"status_key"`
	Label     string `json:"label"`
	Count     int    `json:"count"`
}

type ManagerWaitingListLevelBucket struct {
	Level int32 `json:"level"`
	Count int   `json:"count"`
}

type ManagerOpsSummary struct {
	SessionsScheduled         int   `json:"sessions_scheduled"`
	SessionsLiveNow           int   `json:"sessions_live_now"`
	SessionsCompleted         int   `json:"sessions_completed"`
	SessionsAttendanceDone    int   `json:"sessions_attendance_done"`
	SessionsAttendancePending int   `json:"sessions_attendance_pending"`
	ExpectedStudents          int   `json:"expected_students"`
	AttendedStudents          int   `json:"attended_students"`
	StudentsInClassesCount    int   `json:"students_in_classes_count"`
	PreEnrolmentStudentsCount int   `json:"pre_enrolment_students_count"`
	TodayRevenue              int32 `json:"today_revenue"`
	PayingLeadsCount          int   `json:"paying_leads_count"`
	PlacementTestsScheduled   int   `json:"placement_tests_scheduled"`
	PlacementTestsCompleted   int   `json:"placement_tests_completed"`
	PlacementTestsPending     int   `json:"placement_tests_pending"`
	LateMentorSessions        int   `json:"late_mentor_sessions"`
	AbsentMentorSessions      int   `json:"absent_mentor_sessions"`
	UncheckedMentorSessions   int   `json:"unchecked_mentor_sessions"`
}

type ManagerOpsWeeklySummary struct {
	Label                     string                         `json:"label"`
	WeekStart                 string                         `json:"week_start"`
	WeekEnd                   string                         `json:"week_end"`
	SessionsScheduled         int                            `json:"sessions_scheduled"`
	SessionsCompleted         int                            `json:"sessions_completed"`
	SessionsAttendanceDone    int                            `json:"sessions_attendance_done"`
	SessionsAttendancePending int                            `json:"sessions_attendance_pending"`
	ExpectedStudents          int                            `json:"expected_students"`
	AttendedStudents          int                            `json:"attended_students"`
	Revenue                   int32                          `json:"revenue"`
	PayingLeadsCount          int                            `json:"paying_leads_count"`
	PlacementTestsScheduled   int                            `json:"placement_tests_scheduled"`
	PlacementTestsCompleted   int                            `json:"placement_tests_completed"`
	PlacementTestsPending     int                            `json:"placement_tests_pending"`
	LateMentorSessions        int                            `json:"late_mentor_sessions"`
	AbsentMentorSessions      int                            `json:"absent_mentor_sessions"`
	UncheckedMentorSessions   int                            `json:"unchecked_mentor_sessions"`
	TransferEvents            int                            `json:"transfer_events"`
	ReturnsToAdmin            int                            `json:"returns_to_admin"`
	TopAbsentStudentsMentor   ManagerOpsWeeklyMentorLeader   `json:"top_absent_students_mentor"`
	TopLateStartsMentor       ManagerOpsWeeklyMentorLeader   `json:"top_late_starts_mentor"`
	AbsentStudentsRanking     []ManagerOpsWeeklyMentorLeader `json:"absent_students_ranking"`
	LateStartsRanking         []ManagerOpsWeeklyMentorLeader `json:"late_starts_ranking"`
}

type ManagerOpsWeeklyMentorLeader struct {
	MentorID    string `json:"mentor_id"`
	MentorName  string `json:"mentor_name"`
	MentorEmail string `json:"mentor_email"`
	MetricValue int    `json:"metric_value"`
}

type ManagerOpsSessionRow struct {
	SessionID         uuid.UUID `json:"session_id"`
	ClassKey          string    `json:"class_key"`
	ClassLabel        string    `json:"class_label"`
	MentorID          uuid.UUID `json:"mentor_id"`
	MentorName        string    `json:"mentor_name"`
	MentorEmail       string    `json:"mentor_email"`
	SessionNumber     int32     `json:"session_number"`
	ScheduledDate     string    `json:"scheduled_date"`
	ScheduledTime     string    `json:"scheduled_time"`
	ActualTime        string    `json:"actual_time"`
	SessionStatus     string    `json:"session_status"`
	SessionPhase      string    `json:"session_phase"`
	MentorStatus      string    `json:"mentor_status"`
	DelayMinutes      int       `json:"delay_minutes"`
	ComplianceChecked bool      `json:"compliance_checked"`
	MentorAbsent      bool      `json:"mentor_absent"`
	ExpectedStudents  int       `json:"expected_students"`
	AttendanceMarked  int       `json:"attendance_marked"`
	AttendedStudents  int       `json:"attended_students"`
	AbsentStudents    int       `json:"absent_students"`
	AttendanceStatus  string    `json:"attendance_status"`
	WasRescheduled    bool      `json:"was_rescheduled"`
	PreviousDate      string    `json:"previous_date"`
	PreviousTime      string    `json:"previous_time"`
	RescheduledAt     string    `json:"rescheduled_at"`
	RescheduledBy     string    `json:"rescheduled_by"`
}

// OpsNotificationSummary contains the Mentor Head/Manager banners that still need user attention.
type OpsNotificationSummary struct {
	DailyReport *DailyReportNotification `json:"daily_report,omitempty"`
	Complaint   *ComplaintNotification   `json:"complaint,omitempty"`
}

type DailyReportNotification struct {
	ReportDate           string `json:"report_date"`
	ReadyAt              string `json:"ready_at"`
	ClassesScheduled     int    `json:"classes_scheduled"`
	ClassesTaught        int    `json:"classes_taught"`
	ClassesMissingReport int    `json:"classes_missing_report"`
	AbsentStudents       int    `json:"absent_students"`
	ExpectedStudents     int    `json:"expected_students"`
}

type ComplaintNotification struct {
	ID           uuid.UUID `json:"id"`
	ClassKey     string    `json:"class_key"`
	StudentName  string    `json:"student_name"`
	StudentPhone string    `json:"student_phone"`
	Urgency      string    `json:"urgency"`
	CreatedAt    time.Time `json:"created_at"`
	UnreadCount  int       `json:"unread_count"`
}

// LateJoiner represents an audit record for a student who joined a class after session 1
type LateJoiner struct {
	ID                      uuid.UUID      `json:"id"`
	LeadID                  uuid.UUID      `json:"lead_id"`
	ClassKey                string         `json:"class_key"`
	JoinedAtSessionNumber   int32          `json:"joined_at_session_number"`
	Reason                  string         `json:"reason"`
	AddedByUserID           uuid.UUID      `json:"added_by_user_id"`
	CreatedAt               time.Time      `json:"created_at"`
	PreviousClassDays       sql.NullString `json:"previous_class_days"`
	PreviousClassTime       sql.NullString `json:"previous_class_time"`
	PreviousClassGroupIndex sql.NullInt32  `json:"previous_class_group_index"`
}

// EligibleClass represents a class that a lead can join as a late joiner
type EligibleClass struct {
	ClassKey          string `json:"class_key"`
	Level             int32  `json:"level"`
	ClassDays         string `json:"class_days"`
	ClassTime         string `json:"class_time"`
	CurrentSession    int32  `json:"current_session"`
	CurrentEnrollment int32  `json:"current_enrollment"`
}

// ClassEnrollment represents a historical class enrollment record
type ClassEnrollment struct {
	ID          uuid.UUID
	LeadID      uuid.UUID
	ClassKey    string
	Level       int32
	ClassDays   string
	ClassTime   string
	MentorName  sql.NullString
	FinalGrade  sql.NullString // A, B, C, F
	Outcome     sql.NullString // promoted, repeated
	EnrolledAt  time.Time
	CompletedAt sql.NullTime
}
