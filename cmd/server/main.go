package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/handlers"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	cfg := config.Load()

	// Connect to database
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.RunMigrations(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Seed admin user if it doesn't exist
	if err := seedAdminUser(cfg); err != nil {
		log.Printf("Warning: Failed to seed admin user: %v", err)
	}

	// Seed moderator user if it doesn't exist
	if err := seedModeratorUser(cfg); err != nil {
		log.Printf("Warning: Failed to seed moderator user: %v", err)
	}

	// Initialize handlers
	handlers.SetConfig(cfg) // Set config for template debug logging
	
	// Initialize templates early to catch any errors at startup
	// This will panic if templates can't be loaded, which is better than failing at runtime
	handlers.InitTemplates()
	
	authHandler := handlers.NewAuthHandler(cfg)
	preEnrolmentHandler := handlers.NewPreEnrolmentHandler(cfg)
	classesHandler := handlers.NewClassesHandler(cfg)
	financeHandler := handlers.NewFinanceHandler(cfg)

	// Setup routes
	mux := http.NewServeMux()

	// Request logging middleware - concise request log (optional, can be removed if not needed)
	requestLogMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			cfg.Debugf("REQUEST: %s %s", r.Method, r.URL.Path)
			next(w, r)
		}
	}

	// Static files - use absolute path (must be first)
	workDir, _ := os.Getwd()
	staticDir := filepath.Join(workDir, "web", "static")
	fs := http.FileServer(http.Dir(staticDir))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))
	cfg.Debugf("ROUTE REGISTERED: /static/ -> FileServer")

	// Auth routes (public) - register BEFORE protected routes to ensure exact match
	mux.HandleFunc("/login", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /login handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling authHandler.Login")
			authHandler.Login(w, r)
		} else {
			cfg.Debugf("  → Calling authHandler.LoginForm")
			authHandler.LoginForm(w, r)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /login -> authHandler (LoginForm/Login)")
	mux.HandleFunc("/logout", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /logout handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodGet || r.Method == http.MethodPost {
			authHandler.Logout(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /logout -> authHandler.Logout (GET/POST)")

	// Protected routes - register specific routes BEFORE catch-all
	// /pre-enrolment/new - allow admin + moderator
	mux.HandleFunc("/pre-enrolment/new", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /pre-enrolment/new handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/pre-enrolment/new" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling preEnrolmentHandler.NewForm")
			middleware.RequireAnyRole([]string{"admin", "moderator"}, cfg.SessionSecret)(preEnrolmentHandler.NewForm)(w, r)
		} else if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling preEnrolmentHandler.Create")
			middleware.RequireAnyRole([]string{"admin", "moderator"}, cfg.SessionSecret)(preEnrolmentHandler.Create)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /pre-enrolment/new -> preEnrolmentHandler (NewForm/Create) [admin+moderator]")
	
	// Routes with path parameters - handle manually (Go stdlib mux doesn't support {id})
	// /pre-enrolment/{id} - GET allows admin+moderator, POST/Update/Status admin only
	mux.HandleFunc("/pre-enrolment/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /pre-enrolment/ (catch-all) handler for %s %s", r.Method, r.URL.Path)
		// Explicitly reject /login or any non-pre-enrolment paths
		if !strings.HasPrefix(r.URL.Path, "/pre-enrolment/") {
			cfg.Debugf("  → Path doesn't start with /pre-enrolment/, returning 404")
			http.NotFound(w, r)
			return
		}
		// Skip /pre-enrolment/new (already handled above) and exact /pre-enrolment/
		if r.URL.Path == "/pre-enrolment/new" || r.URL.Path == "/pre-enrolment/" {
			cfg.Debugf("  → Path is /pre-enrolment/new or /pre-enrolment/, returning 404")
			http.NotFound(w, r)
			return
		}
		
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling preEnrolmentHandler.Detail")
			// GET detail - allow admin + moderator (read-only for moderator)
			middleware.RequireAnyRole([]string{"admin", "moderator"}, cfg.SessionSecret)(preEnrolmentHandler.Detail)(w, r)
		} else if r.Method == http.MethodPost {
			// All POST requests to /pre-enrolment/{id} go to Update handler
			// Update handler reads action parameter and routes accordingly
			cfg.Debugf("  → Calling preEnrolmentHandler.Update (action-based routing)")
			// Allow admin + moderator (Update handler enforces restrictions per action)
			middleware.RequireAnyRole([]string{"admin", "moderator"}, cfg.SessionSecret)(preEnrolmentHandler.Update)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /pre-enrolment/ -> Detail [admin+moderator], Update/Status/TestBooked [admin only]")

	// /pre-enrolment (list) - allow admin + moderator
	mux.HandleFunc("/pre-enrolment", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /pre-enrolment (exact) handler for %s %s", r.Method, r.URL.Path)
		// Only handle exact /pre-enrolment, not /pre-enrolment/...
		if r.URL.Path != "/pre-enrolment" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling preEnrolmentHandler.List")
			middleware.RequireAnyRole([]string{"admin", "moderator"}, cfg.SessionSecret)(preEnrolmentHandler.List)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /pre-enrolment -> preEnrolmentHandler.List [admin+moderator]")

	// Classes routes - admin only
	mux.HandleFunc("/classes", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling classesHandler.List")
			middleware.RequireAnyRole([]string{"admin", "moderator"}, cfg.SessionSecret)(classesHandler.List)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes -> classesHandler.List [GET: admin+moderator; moderator gets 403 access-restricted page]")

	mux.HandleFunc("/classes/move", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/move handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes/move" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling classesHandler.Move")
			middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(classesHandler.Move)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/move -> classesHandler.Move [admin only]")

	mux.HandleFunc("/classes/start-round", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/start-round handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes/start-round" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling classesHandler.StartRound")
			middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(classesHandler.StartRound)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/start-round -> classesHandler.StartRound [admin only]")

	mux.HandleFunc("/classes/send", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/send handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes/send" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling classesHandler.SendToMentor")
			middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(classesHandler.SendToMentor)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/send -> classesHandler.SendToMentor [admin only]")

	// /classes/{classKey}/return - dynamic route
	mux.HandleFunc("/classes/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/ (dynamic) handler for %s %s", r.Method, r.URL.Path)
		// Only handle /classes/{classKey}/return
		if !strings.HasPrefix(r.URL.Path, "/classes/") {
			cfg.Debugf("  → Path doesn't start with /classes/, returning 404")
			http.NotFound(w, r)
			return
		}
		// Skip exact /classes and /classes/send, /classes/move, /classes/start-round (already handled)
		if r.URL.Path == "/classes" || r.URL.Path == "/classes/" ||
			r.URL.Path == "/classes/send" || r.URL.Path == "/classes/move" || r.URL.Path == "/classes/start-round" {
			cfg.Debugf("  → Path is exact match for another route, returning 404")
			http.NotFound(w, r)
			return
		}
		// Check if it's a return endpoint: /classes/{classKey}/return
		if strings.HasSuffix(r.URL.Path, "/return") && r.Method == http.MethodPost {
			cfg.Debugf("  → Calling classesHandler.ReturnFromMentor")
			middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(classesHandler.ReturnFromMentor)(w, r)
		} else {
			http.NotFound(w, r)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/{classKey}/return -> classesHandler.ReturnFromMentor [admin only]")

	// Finance routes - admin only
	mux.HandleFunc("/finance", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /finance handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/finance" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling financeHandler.Dashboard")
			middleware.RequireAnyRole([]string{"admin", "moderator"}, cfg.SessionSecret)(financeHandler.Dashboard)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /finance -> financeHandler.Dashboard [GET: admin+moderator; moderator gets 403 access-restricted page]")

	mux.HandleFunc("/finance/new-expense", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /finance/new-expense handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/finance/new-expense" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling financeHandler.NewExpenseForm")
			middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(financeHandler.NewExpenseForm)(w, r)
		} else if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling financeHandler.CreateExpense")
			middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(financeHandler.CreateExpense)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /finance/new-expense -> financeHandler (NewExpenseForm/CreateExpense) [admin only]")

	// /finance/refund/{leadID} - dynamic route
	mux.HandleFunc("/finance/refund/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /finance/refund/ (dynamic) handler for %s %s", r.Method, r.URL.Path)
		if !strings.HasPrefix(r.URL.Path, "/finance/refund/") {
			cfg.Debugf("  → Path doesn't start with /finance/refund/, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling financeHandler.CreateRefund")
			middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(financeHandler.CreateRefund)(w, r)
		} else {
			http.NotFound(w, r)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /finance/refund/{leadID} -> financeHandler.CreateRefund [admin only]")

	// Root redirect - protected route (register last)
	mux.HandleFunc("/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: / (root) handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/" {
			cfg.Debugf("  → Path is not /, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cfg.Debugf("  → Calling RequireAuth -> redirect to /pre-enrolment")
		middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/pre-enrolment", http.StatusFound)
		}, cfg.SessionSecret)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: / -> RequireAuth -> redirect to /pre-enrolment")
	
	cfg.Debugf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	cfg.Debugf("ROUTE REGISTRATION COMPLETE - All routes registered above")
	cfg.Debugf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Start server
	port := cfg.Port
	if port == "" {
		port = "3000"
	}

	log.Printf("Server starting on http://localhost:%s", port)
	log.Printf("Default admin login: %s / %s", cfg.AdminEmail, cfg.AdminPassword)
	log.Printf("Default moderator login: %s / %s", cfg.ModeratorEmail, cfg.ModeratorPassword)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func seedAdminUser(cfg *config.Config) error {
	// Check if admin user exists
	_, err := models.GetUserByEmail(cfg.AdminEmail)
	if err == nil {
		// User already exists
		return nil
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Create admin user
	_, err = models.CreateUser(cfg.AdminEmail, string(hashedPassword), "admin")
	if err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	log.Printf("Created default admin user: %s", cfg.AdminEmail)
	return nil
}

func seedModeratorUser(cfg *config.Config) error {
	// Check if moderator user exists
	_, err := models.GetUserByEmail(cfg.ModeratorEmail)
	if err == nil {
		// User already exists
		return nil
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.ModeratorPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Create moderator user
	_, err = models.CreateUser(cfg.ModeratorEmail, string(hashedPassword), "moderator")
	if err != nil {
		return fmt.Errorf("failed to create moderator user: %w", err)
	}

	log.Printf("Created default moderator user: %s", cfg.ModeratorEmail)
	return nil
}
