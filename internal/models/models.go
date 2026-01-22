package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

type Lead struct {
	ID              uuid.UUID
	FullName        string
	Phone           string
	Source          sql.NullString
	Notes           sql.NullString
	Status          string
	SentToClasses   bool // Whether student has been manually sent to classes board
	CreatedByUserID sql.NullString
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type PlacementTest struct {
	ID                 uuid.UUID
	LeadID             uuid.UUID
	TestDate           sql.NullTime
	TestTime           sql.NullString
	TestType           sql.NullString
	AssignedLevel      sql.NullInt32
	TestNotes          sql.NullString
	RunByUserID        sql.NullString
	PlacementTestFee   sql.NullInt32
	PlacementTestFeePaid sql.NullInt32
	UpdatedAt          time.Time
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
	Lead                *Lead
	AssignedLevel       sql.NullInt32
	PaymentStatus       string
	PaymentState        string // UNPAID, DEPOSIT, PAID_FULL
	NextAction          string
	DaysSinceLastProgress int
	HotLevel            string // "HOT", "WARM", "COOL", or ""
	FollowUpDue         bool
	TestDate            sql.NullTime // For computing days since progress
	AmountPaid          sql.NullInt32 // For checking if paid
	FinalPrice          sql.NullInt32 // For computing payment state
	RemainingBalance    sql.NullInt32 // For computing payment state
}

// ClassGroup represents a group of students with same level+days+time
type ClassGroup struct {
	Level         int32
	ClassDays     string
	ClassTime     string
	GroupIndex    int32 // 1, 2, 3...
	StudentCount  int
	Readiness     string // "LOCKED", "READY", "NOT READY"
	Students      []*ClassStudent
	ClassKey      string // Stable identifier: "L{level}|{days}|{time}|{index}"
	SentToMentor  bool   // Whether this class has been sent to mentor head
	SentAt         sql.NullTime
	ReturnedAt     sql.NullTime
}

// ClassGroupWorkflow tracks workflow state for a class group
type ClassGroupWorkflow struct {
	ClassKey     string
	Level        int32
	ClassDays    string
	ClassTime    string
	ClassNumber  int32
	SentToMentor bool
	SentAt       sql.NullTime
	ReturnedAt   sql.NullTime
	UpdatedAt    time.Time
}

// ClassStudent represents a student in a class group
type ClassStudent struct {
	LeadID         uuid.UUID
	FullName       string
	Phone          string
	GroupIndex     sql.NullInt32
	AvailableGroups []int32 // Available group indices for move (computed in handler)
}
