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
	ID                  uuid.UUID
	FullName            string
	Phone               string
	Source              sql.NullString
	Notes               sql.NullString
	Status              string
	SentToClasses       bool // Whether student has been manually sent to classes board
	LevelsPurchasedTotal sql.NullInt32 // Total levels purchased (from bundles)
	LevelsConsumed      sql.NullInt32  // Levels consumed (when rounds start)
	BundleType          sql.NullString // none, single, bundle2, bundle3, bundle4
	CreatedByUserID     sql.NullString
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type PlacementTest struct {
	ID                      uuid.UUID
	LeadID                  uuid.UUID
	TestDate                sql.NullTime
	TestTime                sql.NullString
	TestType                sql.NullString
	AssignedLevel           sql.NullInt32
	TestNotes               sql.NullString
	RunByUserID             sql.NullString
	PlacementTestFee        sql.NullInt32
	PlacementTestFeePaid    sql.NullInt32
	PlacementTestPaymentDate sql.NullTime
	PlacementTestPaymentMethod sql.NullString
	UpdatedAt               time.Time
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
	LevelsPurchased sql.NullInt32 // For course payments: how many levels this payment represents
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

// FinanceSummary represents aggregated finance data
type FinanceSummary struct {
	TodayIN      int32
	TodayOUT     int32
	TodayNet     int32
	RangeIN      int32
	RangeOUT     int32
	RangeNet     int32
	INByCategory map[string]int32
	OUTByCategory map[string]int32
	TotalRemainingLevels int32
	CreditsBreakdown    map[string]int // "0", "1", "2", "3+"
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
