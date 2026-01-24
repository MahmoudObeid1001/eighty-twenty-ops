package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"
	"eighty-twenty-ops/internal/util"

	"github.com/google/uuid"
)

type FinanceHandler struct {
	cfg *config.Config
}

func NewFinanceHandler(cfg *config.Config) *FinanceHandler {
	return &FinanceHandler{cfg: cfg}
}

// Dashboard renders the finance dashboard
func (h *FinanceHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	// Parse date filters
	var dateFrom, dateTo sql.NullTime
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			dateFrom = sql.NullTime{Time: t, Valid: true}
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			dateTo = sql.NullTime{Time: t, Valid: true}
		}
	}

	// Get filters
	categoryFilter := r.URL.Query().Get("category")
	paymentMethodFilter := r.URL.Query().Get("payment_method")
	transactionTypeFilter := r.URL.Query().Get("type")

	// Current cash balance (full history, ignores date filters)
	currentBalance, err := models.GetCurrentCashBalance()
	if err != nil {
		log.Printf("ERROR: Failed to get current cash balance: %v", err)
		currentBalance = 0
	}
	balanceByMethod, _ := models.GetCurrentCashBalanceByPaymentMethod()

	// Get summary
	summary, err := models.GetFinanceSummary(dateFrom, dateTo)
	if err != nil {
		log.Printf("ERROR: Failed to get finance summary: %v", err)
		http.Error(w, fmt.Sprintf("Failed to load finance summary: %v", err), http.StatusInternalServerError)
		return
	}

	// Get transactions
	transactions, err := models.GetTransactions(dateFrom, dateTo, transactionTypeFilter, categoryFilter, paymentMethodFilter, 100, 0)
	if err != nil {
		log.Printf("ERROR: Failed to get transactions: %v", err)
		http.Error(w, fmt.Sprintf("Failed to load transactions: %v", err), http.StatusInternalServerError)
		return
	}

	// Group transactions by day with daily totals
	ledgerGroups := models.GroupTransactionsByDay(transactions)

	// Get cancelled leads summary
	cancelledLeads, err := models.GetCancelledLeadsSummary()
	if err != nil {
		log.Printf("ERROR: Failed to get cancelled leads summary: %v", err)
		// Don't fail the whole page, just log the error
		cancelledLeads = []*models.CancelledLeadSummary{}
	}

	// Calculate cancelled leads counts for summary
	cancelledCount := len(cancelledLeads)
	cancelledBalancedCount := 0
	cancelledErrorCount := 0
	for _, lead := range cancelledLeads {
		if lead.NetMoney == 0 {
			cancelledBalancedCount++
		} else if lead.NetMoney < 0 {
			// Error: refunded more than paid
			cancelledErrorCount++
		}
	}

	// Get cancelled leads totals
	var cancelledTotals struct {
		TotalPlacementTest int32
		TotalCoursePaid    int32
		TotalRefunded      int32
		NetOutstanding     int32
	}
	if len(cancelledLeads) > 0 {
		totalPT, totalCP, totalRef, netOut, err := models.GetCancelledLeadsTotals()
		if err != nil {
			log.Printf("ERROR: Failed to get cancelled leads totals: %v", err)
		} else {
			cancelledTotals.TotalPlacementTest = totalPT
			cancelledTotals.TotalCoursePaid = totalCP
			cancelledTotals.TotalRefunded = totalRef
			cancelledTotals.NetOutstanding = netOut
		}
	}

	// Check for flash messages
	flashMessage := ""
	if r.URL.Query().Get("expense_created") == "1" {
		flashMessage = "Expense created successfully"
	} else if r.URL.Query().Get("error") == "future_date" {
		flashMessage = "Payment date cannot be in the future"
	}

	data := map[string]interface{}{
		"Title":                  "Finance - Eighty Twenty",
		"CurrentCashBalance":     currentBalance,
		"BalanceByPaymentMethod": balanceByMethod,
		"Summary":                summary,
		"Transactions":           transactions, // Keep for backward compatibility if needed
		"LedgerGroups":           ledgerGroups, // New grouped data
		"CancelledLeads":         cancelledLeads,
		"CancelledCount":         cancelledCount,
		"CancelledBalancedCount": cancelledBalancedCount,
		"CancelledErrorCount":    cancelledErrorCount,
		"CancelledTotals":        cancelledTotals,
		"DateFrom":               dateFrom,
		"DateTo":                 dateTo,
		"CategoryFilter":         categoryFilter,
		"PaymentMethodFilter":    paymentMethodFilter,
		"TransactionTypeFilter":  transactionTypeFilter,
		"UserRole":               userRole,
		"FlashMessage":           flashMessage,
	}
	renderTemplate(w, "finance.html", data)
}

// NewExpenseForm renders the new expense form
func (h *FinanceHandler) NewExpenseForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	today := time.Now().Format("2006-01-02")
	
	// Check for error messages
	errorMsg := ""
	if r.URL.Query().Get("error") == "future_date" {
		errorMsg = "Payment date cannot be in the future"
	}
	
	data := map[string]interface{}{
		"Title":    "New Expense - Finance",
		"Today":     today,
		"UserRole":  userRole,
		"Error":     errorMsg,
	}
	renderTemplate(w, "finance_new_expense.html", data)
}

// CreateExpense handles POST to create a new expense
func (h *FinanceHandler) CreateExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	// Parse form
	category := r.FormValue("category")
	amountStr := r.FormValue("amount")
	paymentMethod := r.FormValue("payment_method")
	dateStr := r.FormValue("transaction_date")
	notes := r.FormValue("notes")

	if category == "" || amountStr == "" || paymentMethod == "" {
		http.Error(w, "Category, amount, and payment method are required", http.StatusBadRequest)
		return
	}

	amount, err := strconv.Atoi(amountStr)
	if err != nil || amount <= 0 {
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}

	transactionDate := time.Now()
	if dateStr != "" {
		if t, err := util.ParseDateLocal(dateStr); err == nil {
			transactionDate = t
		}
	}

	_, err = models.CreateExpense(category, int32(amount), paymentMethod, transactionDate, notes)
	if err != nil {
		log.Printf("ERROR: Failed to create expense: %v", err)
		// Check if it's a validation error (future date)
		if err.Error() == "payment date cannot be in the future" {
			http.Redirect(w, r, "/finance/new-expense?error=future_date", http.StatusFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to create expense: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/finance?expense_created=1", http.StatusFound)
}

// CreateRefund handles POST to create a refund for a lead
func (h *FinanceHandler) CreateRefund(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	// Parse lead ID from path: /finance/refund/{leadID}
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 || pathParts[2] != "refund" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[3])
	if err != nil {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	// Parse form
	amountStr := r.FormValue("amount")
	paymentMethod := r.FormValue("payment_method")
	dateStr := r.FormValue("transaction_date")
	notes := r.FormValue("notes")

	if amountStr == "" || paymentMethod == "" {
		http.Error(w, "Amount and payment method are required", http.StatusBadRequest)
		return
	}

	amount, err := strconv.Atoi(amountStr)
	if err != nil || amount <= 0 {
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}

	transactionDate := time.Now()
	if dateStr != "" {
		if t, err := util.ParseDateLocal(dateStr); err == nil {
			transactionDate = t
		}
	}

	_, err = models.CreateRefund(leadID, int32(amount), paymentMethod, transactionDate, notes)
	if err != nil {
		log.Printf("ERROR: Failed to create refund: %v", err)
		// Check if it's a validation error (future date)
		if err.Error() == "payment date cannot be in the future" {
			http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?error=future_date", leadID.String()), http.StatusFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to create refund: %v", err), http.StatusInternalServerError)
		return
	}

	// Validate refund doesn't exceed total paid
	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
		return
	}
	
	var totalPaid int32 = 0
	if detail.Payment != nil && detail.Payment.AmountPaid.Valid {
		totalPaid = detail.Payment.AmountPaid.Int32
	}
	
	// Get total refunded so far
	var totalRefunded int32 = 0
	err = db.DB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM transactions
		WHERE lead_id = $1 AND category = 'refund' AND transaction_type = 'OUT'
	`, leadID).Scan(&totalRefunded)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("ERROR: Failed to get total refunded: %v", err)
		http.Error(w, fmt.Sprintf("Failed to validate refund: %v", err), http.StatusInternalServerError)
		return
	}
	
	if int32(amount) > (totalPaid - totalRefunded) {
		http.Error(w, fmt.Sprintf("Refund amount (%d) exceeds available balance (%d - %d = %d)", amount, totalPaid, totalRefunded, totalPaid-totalRefunded), http.StatusBadRequest)
		return
	}

	_, err = models.CreateRefund(leadID, int32(amount), paymentMethod, transactionDate, notes)
	if err != nil {
		log.Printf("ERROR: Failed to create refund: %v", err)
		// Check if it's a validation error (future date)
		if err.Error() == "payment date cannot be in the future" {
			http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?error=future_date", leadID.String()), http.StatusFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to create refund: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?refund_created=1", leadID.String()), http.StatusFound)
}
