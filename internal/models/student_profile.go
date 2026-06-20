package models

import (
	"time"

	"github.com/google/uuid"
)

// Student Profile Models for Universal Student Profile (Milestone 4)

// StudentSearchResult represents a search result item
type StudentSearchResult struct {
	LeadID       uuid.UUID `json:"lead_id"`
	FullName     string    `json:"full_name"`
	Phone        string    `json:"phone"`
	CurrentLevel int32     `json:"current_level"`
	Status       string    `json:"status"`
}

// StudentProfile represents the header information for a student
type StudentProfile struct {
	LeadID           uuid.UUID `json:"lead_id"`
	FullName         string    `json:"full_name"`
	Gender           string    `json:"gender"`
	Phone            string    `json:"phone"`
	CurrentLevel     int32     `json:"current_level"`
	RemainingCredits int32     `json:"remaining_credits"`
	Status           string    `json:"status"`
	IsReturning      bool      `json:"is_returning"`
}

// AcademicHistoryItem represents one enrollment record from class_enrollments
type AcademicHistoryItem struct {
	ID          uuid.UUID  `json:"id"`
	Level       int32      `json:"level"`
	ClassDays   string     `json:"class_days"`
	ClassTime   string     `json:"class_time"`
	MentorName  string     `json:"mentor_name"`
	FinalGrade  string     `json:"final_grade"` // A, B, C, F
	Outcome     string     `json:"outcome"`     // promoted, repeated
	EnrolledAt  time.Time  `json:"enrolled_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// CurrentClassStatus represents the current class status for a student in_classes
type CurrentClassStatus struct {
	ClassKey        string              `json:"class_key"`
	Level           int32               `json:"level"`
	ClassDays       string              `json:"class_days"`
	ClassTime       string              `json:"class_time"`
	MentorName      string              `json:"mentor_name"`
	CurrentSession  int32               `json:"current_session"`
	AttendanceStats AttendanceStats     `json:"attendance_stats"`
	SessionDetails  []SessionAttendance `json:"session_details"`
}

// AttendanceStats represents aggregated attendance statistics
type AttendanceStats struct {
	Present int32 `json:"present"`
	Absent  int32 `json:"absent"`
	Late    int32 `json:"late"`
	Total   int32 `json:"total"`
}

// SessionAttendance represents attendance for a single session
type SessionAttendance struct {
	SessionNumber int32  `json:"session_number"`
	Status        string `json:"status"` // PRESENT, ABSENT, LATE
	Date          string `json:"date"`
}

// TimelineItem represents a note or followup in the student timeline
type TimelineItem struct {
	ID             uuid.UUID `json:"id"`
	Type           string    `json:"type"` // "note", "followup", or "grade_note"
	Text           string    `json:"text"`
	TranslatedText string    `json:"translated_text,omitempty"`
	ClassKey       string    `json:"class_key"`  // nullable
	Session        int32     `json:"session"`    // nullable
	IsPrivate      bool      `json:"is_private"` // for notes only
	CreatedBy      string    `json:"created_by"` // email
	CreatedAt      time.Time `json:"created_at"`
}

// StudentPaymentHistoryItem is a normalized payment/refund row for the student profile.
type StudentPaymentHistoryItem struct {
	ID            uuid.UUID `json:"id"`
	Type          string    `json:"type"`
	Direction     string    `json:"direction"` // "in" or "out"
	Amount        int32     `json:"amount"`
	PaymentMethod string    `json:"payment_method"`
	PaymentDate   string    `json:"payment_date"`
	Notes         string    `json:"notes"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"created_at"`
}
