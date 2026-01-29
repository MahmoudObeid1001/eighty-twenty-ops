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

	// Seed Milestone 2 users (mentor_head, mentor, community_officer) if they don't exist
	if err := seedMentorHeadUser(cfg); err != nil {
		log.Printf("Warning: Failed to seed mentor_head user: %v", err)
	}
	if err := seedMentorUser(cfg); err != nil {
		log.Printf("Warning: Failed to seed mentor user: %v", err)
	}
	if err := seedCommunityOfficerUser(cfg); err != nil {
		log.Printf("Warning: Failed to seed community_officer user: %v", err)
	}
	if err := seedHRUser(cfg); err != nil {
		log.Printf("Warning: Failed to seed hr user: %v", err)
	}
	if err := seedStudentSuccessUser(cfg); err != nil {
		log.Printf("Warning: Failed to seed student_success user: %v", err)
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
	mentorHeadHandler := handlers.NewMentorHeadHandler(cfg)
	mentorHandler := handlers.NewMentorHandler(cfg)
	communityOfficerHandler := handlers.NewCommunityOfficerHandler(cfg)
	hrHandler := handlers.NewHRHandler(cfg)
	apiHandler := handlers.NewAPIHandler(cfg)

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

	// API routes (JSON) - register BEFORE React app to avoid shadowing /api/*
	// React app handler will be registered AFTER all API routes
	mux.HandleFunc("/api/me", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAuth(apiHandler.GetMe, cfg.SessionSecret)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/me -> apiHandler.GetMe [RequireAuth]")

	// Attendance routes
	mux.HandleFunc("/api/attendance", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAuth(apiHandler.MarkAttendance, cfg.SessionSecret)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/attendance -> apiHandler.MarkAttendance [RequireAuth]")

	mux.HandleFunc("/api/session/complete", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAuth(apiHandler.CompleteSession, cfg.SessionSecret)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/session/complete -> apiHandler.CompleteSession [RequireAuth]")

	mux.HandleFunc("/api/mentor/classes", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"mentor", "admin", "student_success"}, cfg.SessionSecret)(apiHandler.GetMentorClasses)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor/classes -> apiHandler.GetMentorClasses [mentor+admin]")

	mux.HandleFunc("/api/mentor-head/mentors", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.GetMentors)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/mentors -> apiHandler.GetMentors [mentor_head+admin]")

	mux.HandleFunc("/api/mentor-head/classes", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.GetMentorHeadClasses)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/classes -> apiHandler.GetMentorHeadClasses [mentor_head+admin]")

	mux.HandleFunc("/api/mentor-head/dashboard", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.GetMentorHeadDashboard)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/dashboard -> apiHandler.GetMentorHeadDashboard [mentor_head+admin]")

	mux.HandleFunc("/api/mentor-head/assign-mentor", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.AssignMentor)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/assign-mentor -> apiHandler.AssignMentor [mentor_head+admin]")

	mux.HandleFunc("/api/mentor-head/return-to-ops", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.ReturnToOps)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/return-to-ops -> apiHandler.ReturnToOps [mentor_head+admin]")

	mux.HandleFunc("/api/mentor-head/return-class", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.ReturnClass)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/return-class -> apiHandler.ReturnClass [mentor_head+admin] (backward compatibility)")

	mux.HandleFunc("/api/mentor-head/unassign", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head"}, cfg.SessionSecret)(apiHandler.UnassignMentor)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/unassign -> apiHandler.UnassignMentor [mentor_head only]")

	mux.HandleFunc("/api/mentor-head/start-round", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.StartRound)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/start-round -> apiHandler.StartRound [mentor_head+admin]")

	mux.HandleFunc("/api/mentor-head/classes/start-round", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.StartRound)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/classes/start-round -> apiHandler.StartRound [mentor_head+admin]")

	mux.HandleFunc("/api/mentor-head/close-round", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.CloseRound)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/close-round -> apiHandler.CloseRound [mentor_head+admin]")

	mux.HandleFunc("/api/class-workspace", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Ensure exact path match (no trailing slash)
		if r.URL.Path != "/api/class-workspace" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success"}, cfg.SessionSecret)(apiHandler.GetClassWorkspace)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/class-workspace -> apiHandler.GetClassWorkspace [mentor+mentor_head+admin]")

	mux.HandleFunc("/api/class", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success"}, cfg.SessionSecret)(apiHandler.GetClass)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/class -> apiHandler.GetClass [mentor+mentor_head+admin] (backward compatibility)")

	mux.HandleFunc("/api/notes", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success"}, cfg.SessionSecret)(apiHandler.GetNotes)(w, r)
		} else if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success"}, cfg.SessionSecret)(apiHandler.CreateNote)(w, r)
		} else if r.Method == http.MethodDelete {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success"}, cfg.SessionSecret)(apiHandler.DeleteNote)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/notes -> apiHandler.GetNotes/CreateNote/DeleteNote [mentor+mentor_head+admin]")

	mux.HandleFunc("/api/student", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success"}, cfg.SessionSecret)(apiHandler.GetStudent)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student -> apiHandler.GetStudent [mentor+mentor_head+admin]")

	// Register GET /api/mentor-head/evaluations first (exact match)
	mux.HandleFunc("/api/mentor-head/evaluations", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Only handle exact path match (no trailing slash, no path params)
		if r.URL.Path != "/api/mentor-head/evaluations" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor_head"}, cfg.SessionSecret)(apiHandler.GetMentorEvaluations)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/evaluations -> apiHandler.GetMentorEvaluations [mentor_head only]")

	// Register PUT /api/mentor-head/evaluations/{mentorId} - must come after exact match
	// Go's ServeMux will match any path starting with this prefix
	mux.HandleFunc("/api/mentor-head/evaluations/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Ensure path has more than just the prefix (must have mentor ID)
		if r.URL.Path == "/api/mentor-head/evaluations/" || len(r.URL.Path) <= len("/api/mentor-head/evaluations/") {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPut {
			middleware.RequireAnyRole([]string{"mentor_head"}, cfg.SessionSecret)(apiHandler.UpdateMentorEvaluation)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/evaluations/:mentorId -> apiHandler.UpdateMentorEvaluation [mentor_head only]")

	mux.HandleFunc("/api/student-success/classes", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/student-success/classes" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"student_success"}, cfg.SessionSecret)(apiHandler.GetStudentSuccessClasses)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/classes -> apiHandler.GetStudentSuccessClasses [student_success only]")

	mux.HandleFunc("/api/student-success/class", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/student-success/class" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"student_success"}, cfg.SessionSecret)(apiHandler.GetStudentSuccessClass)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/class -> apiHandler.GetStudentSuccessClass [student_success only]")

	mux.HandleFunc("/api/student-success/class/absence-feed", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.GetAbsenceFeed)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/class/absence-feed -> apiHandler.GetAbsenceFeed")

	mux.HandleFunc("/api/student-success/followups", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.GetFollowUps)(w, r)
		} else if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.CreateFollowUp)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/followups -> apiHandler.CreateFollowUp")

	mux.HandleFunc("/api/student-success/resolve-absence", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.ResolveAbsence)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/resolve-absence -> apiHandler.ResolveAbsence")

	mux.HandleFunc("/api/student-success/feedback", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success", "admin"}, cfg.SessionSecret)(apiHandler.SubmitFeedback)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/feedback -> apiHandler.SubmitFeedback")

	mux.HandleFunc("/api/student-success/feedback/status", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success", "admin"}, cfg.SessionSecret)(apiHandler.UpdateFeedbackStatus)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/feedback/status -> apiHandler.UpdateFeedbackStatus")

	// Specific absence case actions
	mux.HandleFunc("/api/absence-cases/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/follow-up") {
				apiHandler.PostFollowUpUpdate(w, r)
			} else if strings.HasSuffix(r.URL.Path, "/resolve") {
				apiHandler.ResolveFollowUp(w, r)
			} else {
				http.NotFound(w, r)
			}
		})(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/absence-cases/:id/follow-up and /api/absence-cases/:id/resolve")

	// Dynamic classes routes /api/classes/{id}/sessions and /api/classes/{id}/sessions/{n}/complete
	mux.HandleFunc("/api/classes/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Check for completion endpoint first (longer suffix)
		if strings.HasSuffix(path, "/complete") && strings.Contains(path, "/sessions/") {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success"}, cfg.SessionSecret)(apiHandler.CompleteSessionByNumber)(w, r)
			return
		}
		// Check for sessions list endpoint
		if strings.HasSuffix(path, "/sessions") {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success"}, cfg.SessionSecret)(apiHandler.ListClassSessions)(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/classes/* -> sessions list and completion")

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

	// Classes routes - admin, mentor_head (read-only), moderator (403)
	mux.HandleFunc("/classes", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling classesHandler.List")
			middleware.RequireAnyRole([]string{"admin", "moderator", "mentor_head"}, cfg.SessionSecret)(classesHandler.List)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes -> classesHandler.List [GET: admin+mentor_head+moderator; moderator gets 403; mentor_head read-only]")

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

	// POST /classes/return with form field class_key (not path; classKey can contain /)
	mux.HandleFunc("/classes/return", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/return handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes/return" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling classesHandler.ReturnFromMentor")
			middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(classesHandler.ReturnFromMentor)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/return -> classesHandler.ReturnFromMentor [admin only]")

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

	// Mentor Head routes - redirect to React app (backward compatibility)
	mux.HandleFunc("/mentor-head", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor-head redirect for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/mentor-head" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			// Redirect to React app
			http.Redirect(w, r, "/app/mentor-head", http.StatusFound)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor-head -> 302 redirect to /app/mentor-head [backward compatibility]")

	mux.HandleFunc("/mentor-head/assign", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor-head/assign handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling mentorHeadHandler.AssignMentor")
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(mentorHeadHandler.AssignMentor)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor-head/assign -> mentorHeadHandler.AssignMentor [mentor_head+admin]")

	mux.HandleFunc("/mentor-head/session/cancel", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor-head/session/cancel handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling mentorHeadHandler.CancelSession")
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(mentorHeadHandler.CancelSession)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor-head/session/cancel -> mentorHeadHandler.CancelSession [mentor_head+admin]")

	mux.HandleFunc("/mentor-head/close-round", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor-head/close-round handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling mentorHeadHandler.CloseRound")
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(mentorHeadHandler.CloseRound)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor-head/close-round -> mentorHeadHandler.CloseRound [mentor_head+admin]")

	mux.HandleFunc("/mentor-head/start-round", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor-head/start-round handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling mentorHeadHandler.StartRound")
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(mentorHeadHandler.StartRound)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor-head/start-round -> mentorHeadHandler.StartRound [mentor_head+admin]")

	// /mentor-head/class?class_key=... - redirect to React app (backward compatibility)
	mux.HandleFunc("/mentor-head/class", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor-head/class redirect for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/mentor-head/class" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			// Preserve query params (class_key)
			redirectURL := "/app/mentor-head/class"
			if r.URL.RawQuery != "" {
				redirectURL += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, redirectURL, http.StatusFound)
		} else {
			http.NotFound(w, r)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor-head/class -> 302 redirect to /app/mentor-head/class [backward compatibility]")

	// POST /mentor-head/return with form field class_key (not path; classKey can contain /)
	mux.HandleFunc("/mentor-head/return", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor-head/return handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/mentor-head/return" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling mentorHeadHandler.ReturnClass")
			middleware.RequireAnyRole([]string{"mentor_head", "admin"}, cfg.SessionSecret)(mentorHeadHandler.ReturnClass)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor-head/return -> mentorHeadHandler.ReturnClass [mentor_head+admin]")

	// Mentor routes - redirect to React app (backward compatibility)
	mux.HandleFunc("/mentor", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor redirect for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/mentor" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			// Redirect to React app
			http.Redirect(w, r, "/app/mentor", http.StatusFound)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor -> 302 redirect to /app/mentor [backward compatibility]")

	mux.HandleFunc("/mentor/attendance", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor/attendance handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling mentorHandler.MarkAttendance")
			middleware.RequireAnyRole([]string{"mentor", "admin", "student_success"}, cfg.SessionSecret)(mentorHandler.MarkAttendance)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor/attendance -> mentorHandler.MarkAttendance [mentor+admin]")

	mux.HandleFunc("/mentor/grade", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor/grade handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling mentorHandler.EnterGrade")
			middleware.RequireAnyRole([]string{"mentor", "admin", "student_success"}, cfg.SessionSecret)(mentorHandler.EnterGrade)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor/grade -> mentorHandler.EnterGrade [mentor+admin]")

	mux.HandleFunc("/mentor/note", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor/note handler for %s %s", r.Method, r.URL.Path)
		// Check if this is a delete request: DELETE method OR POST with note_id but no note_text
		hasNoteID := r.URL.Query().Get("note_id") != "" || r.FormValue("note_id") != ""
		hasNoteText := r.FormValue("note_text") != ""
		isDelete := r.Method == http.MethodDelete || (r.Method == http.MethodPost && hasNoteID && !hasNoteText)
		if isDelete {
			cfg.Debugf("  → Calling mentorHandler.DeleteNote")
			middleware.RequireAnyRole([]string{"mentor", "admin", "mentor_head"}, cfg.SessionSecret)(mentorHandler.DeleteNote)(w, r)
		} else if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling mentorHandler.AddNote")
			middleware.RequireAnyRole([]string{"mentor", "admin", "mentor_head"}, cfg.SessionSecret)(mentorHandler.AddNote)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor/note -> mentorHandler.AddNote/DeleteNote [mentor+admin+mentor_head]")

	mux.HandleFunc("/mentor/session/complete", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor/session/complete handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling mentorHandler.CompleteSession")
			middleware.RequireAnyRole([]string{"mentor", "admin", "student_success"}, cfg.SessionSecret)(mentorHandler.CompleteSession)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor/session/complete -> mentorHandler.CompleteSession [mentor+admin]")

	// /mentor/class?class_key=... - redirect to React app (backward compatibility)
	mux.HandleFunc("/mentor/class", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /mentor/class redirect for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/mentor/class" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			// Preserve query params (class_key)
			redirectURL := "/app/mentor/class"
			if r.URL.RawQuery != "" {
				redirectURL += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, redirectURL, http.StatusFound)
		} else {
			http.NotFound(w, r)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /mentor/class -> 302 redirect to /app/mentor/class [backward compatibility]")

	// Community Officer routes - community_officer + admin
	mux.HandleFunc("/community-officer", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /community-officer handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/community-officer" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling communityOfficerHandler.Dashboard")
			middleware.RequireAnyRole([]string{"community_officer", "admin"}, cfg.SessionSecret)(communityOfficerHandler.Dashboard)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /community-officer -> communityOfficerHandler.Dashboard [community_officer+admin]")

	mux.HandleFunc("/community-officer/feedback", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /community-officer/feedback handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling communityOfficerHandler.SubmitFeedback")
			middleware.RequireAnyRole([]string{"community_officer", "admin"}, cfg.SessionSecret)(communityOfficerHandler.SubmitFeedback)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /community-officer/feedback -> communityOfficerHandler.SubmitFeedback [community_officer+admin]")

	mux.HandleFunc("/community-officer/follow-up", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /community-officer/follow-up handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling communityOfficerHandler.LogFollowUp")
			middleware.RequireAnyRole([]string{"community_officer", "admin"}, cfg.SessionSecret)(communityOfficerHandler.LogFollowUp)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /community-officer/follow-up -> communityOfficerHandler.LogFollowUp [community_officer+admin]")

	// HR routes - hr + admin
	mux.HandleFunc("/hr/mentors", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /hr/mentors handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/hr/mentors" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling hrHandler.MentorsList")
			middleware.RequireAnyRole([]string{"hr", "admin"}, cfg.SessionSecret)(hrHandler.MentorsList)(w, r)
		} else if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling hrHandler.MentorsCreate")
			middleware.RequireAnyRole([]string{"hr", "admin"}, cfg.SessionSecret)(hrHandler.MentorsCreate)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /hr/mentors -> hrHandler.MentorsList (GET) / MentorsCreate (POST) [hr+admin]")

	// GET /learning - redirect to role home (mentor -> /mentor, mentor_head -> /mentor-head, hr -> /hr/mentors, etc.)
	mux.HandleFunc("/learning", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /learning handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/learning" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling authHandler.LearningRedirect")
			middleware.RequireAuth(authHandler.LearningRedirect, cfg.SessionSecret)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /learning -> authHandler.LearningRedirect [RequireAuth]")

	// React app - serve from frontend/dist (Vite build output)
	// IMPORTANT: Register AFTER all other routes (API, auth, SSR) to avoid shadowing
	reactAppDir := filepath.Join(workDir, "frontend", "dist")
	reactIndexPath := filepath.Join(reactAppDir, "index.html")

	// Check if React app is built
	if _, err := os.Stat(reactIndexPath); os.IsNotExist(err) {
		log.Printf("WARNING: React app not built. Run: cd frontend && npm run build")
		log.Printf("  Expected index.html at: %s", reactIndexPath)
	} else {
		log.Printf("React app found at: %s", reactAppDir)
	}

	// Serve React app static assets (JS, CSS, images, etc.) from /app/assets/*
	reactFS := http.FileServer(http.Dir(reactAppDir))
	mux.Handle("/app/assets/", http.StripPrefix("/app/", reactFS))
	cfg.Debugf("ROUTE REGISTERED: /app/assets/ -> React static assets from frontend/dist")

	// Catch-all handler for /app/* - serves index.html for SPA routing
	// This must be registered AFTER all other routes to avoid shadowing /api/*, /login, etc.
	mux.HandleFunc("/app/", func(w http.ResponseWriter, r *http.Request) {
		// Only handle GET requests
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check if React app is built
		if _, err := os.Stat(reactIndexPath); os.IsNotExist(err) {
			http.Error(w, "React app not built. Run: cd frontend && npm run build", http.StatusServiceUnavailable)
			return
		}

		// Serve index.html for all /app/* routes (SPA routing)
		http.ServeFile(w, r, reactIndexPath)
	})
	cfg.Debugf("ROUTE REGISTERED: /app/* -> React SPA (index.html) from frontend/dist")

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
		cfg.Debugf("  → Calling RequireAuth -> redirect to role home")
		middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			home := handlers.RoleHomePath(middleware.GetUserRole(r))
			http.Redirect(w, r, home, http.StatusFound)
		}, cfg.SessionSecret)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: / -> RequireAuth -> redirect to role home")

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
	log.Printf("Default mentor_head login: %s / %s", cfg.MentorHeadEmail, cfg.MentorHeadPassword)
	log.Printf("Default mentor login: %s / %s", cfg.MentorEmail, cfg.MentorPassword)
	log.Printf("Default community_officer login: %s / %s", cfg.CommunityOfficerEmail, cfg.CommunityOfficerPassword)
	log.Printf("Default hr login: %s / %s", cfg.HREmail, cfg.HRPassword)
	log.Printf("Default student_success login: %s / %s", cfg.StudentSuccessEmail, cfg.StudentSuccessPassword)
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

func seedMentorHeadUser(cfg *config.Config) error {
	_, err := models.GetUserByEmail(cfg.MentorHeadEmail)
	if err == nil {
		return nil
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.MentorHeadPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	_, err = models.CreateUser(cfg.MentorHeadEmail, string(hashedPassword), "mentor_head")
	if err != nil {
		return fmt.Errorf("failed to create mentor_head user: %w", err)
	}
	log.Printf("Created default mentor_head user: %s", cfg.MentorHeadEmail)
	return nil
}

func seedMentorUser(cfg *config.Config) error {
	_, err := models.GetUserByEmail(cfg.MentorEmail)
	if err == nil {
		return nil
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.MentorPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	_, err = models.CreateUser(cfg.MentorEmail, string(hashedPassword), "mentor")
	if err != nil {
		return fmt.Errorf("failed to create mentor user: %w", err)
	}
	log.Printf("Created default mentor user: %s", cfg.MentorEmail)
	return nil
}

func seedCommunityOfficerUser(cfg *config.Config) error {
	_, err := models.GetUserByEmail(cfg.CommunityOfficerEmail)
	if err == nil {
		return nil
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.CommunityOfficerPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	_, err = models.CreateUser(cfg.CommunityOfficerEmail, string(hashedPassword), "community_officer")
	if err != nil {
		return fmt.Errorf("failed to create community_officer user: %w", err)
	}
	log.Printf("Created default community_officer user: %s", cfg.CommunityOfficerEmail)
	return nil
}

func seedHRUser(cfg *config.Config) error {
	_, err := models.GetUserByEmail(cfg.HREmail)
	if err == nil {
		return nil
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.HRPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	_, err = models.CreateUser(cfg.HREmail, string(hashedPassword), "hr")
	if err != nil {
		return fmt.Errorf("failed to create hr user: %w", err)
	}
	log.Printf("Created default hr user: %s", cfg.HREmail)
	return nil
}

func seedStudentSuccessUser(cfg *config.Config) error {
	_, err := models.GetUserByEmail(cfg.StudentSuccessEmail)
	if err == nil {
		return nil
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.StudentSuccessPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	_, err = models.CreateUser(cfg.StudentSuccessEmail, string(hashedPassword), "student_success")
	if err != nil {
		return fmt.Errorf("failed to create student_success user: %w", err)
	}
	log.Printf("Created default student_success user: %s", cfg.StudentSuccessEmail)
	return nil
}
