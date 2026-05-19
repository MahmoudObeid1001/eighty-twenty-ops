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
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("warning: failed to close db: %v", closeErr)
		}
	}()

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
	if err := seedStudentSuccessUser(cfg); err != nil {
		log.Printf("Warning: Failed to seed student_success user: %v", err)
	}
	if err := seedHRUser(cfg); err != nil {
		log.Printf("Warning: Failed to seed hr user: %v", err)
	}
	if err := seedManagerUser(cfg); err != nil {
		log.Printf("Warning: Failed to seed manager user: %v", err)
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
	mentorHandler := handlers.NewMentorHandler(cfg)
	studentSuccessHandler := handlers.NewStudentSuccessHandler(cfg)
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

	mux.HandleFunc("/api/mentor/reminders", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"mentor", "admin"}, cfg.SessionSecret)(apiHandler.GetMentorReminders)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor/reminders -> apiHandler.GetMentorReminders [mentor+admin]")

	mux.HandleFunc("/api/mentor/availability", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodPut {
			middleware.RequireAnyRole([]string{"mentor"}, cfg.SessionSecret)(apiHandler.MentorAvailability)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor/availability -> apiHandler.MentorAvailability [mentor]")

	mux.HandleFunc("/api/availability-reminder", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head"}, cfg.SessionSecret)(apiHandler.AvailabilityReminder)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/availability-reminder -> apiHandler.AvailabilityReminder [mentor+mentor_head]")

	mux.HandleFunc("/api/mentor-head/mentors", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.GetMentors)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/mentors -> apiHandler.GetMentors [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/mentors/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/testimonials") {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.CreateMentorTestimonial)(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/mentors/:id/testimonials -> apiHandler.CreateMentorTestimonial [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentors", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/mentors" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager", "hr"}, cfg.SessionSecret)(apiHandler.GetMentorDirectory)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentors -> apiHandler.GetMentorDirectory [mentor_head+admin+manager+hr]")

	mux.HandleFunc("/api/mentors/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if strings.HasSuffix(r.URL.Path, "/availability") {
				middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager", "hr"}, cfg.SessionSecret)(apiHandler.GetMentorAvailability)(w, r)
				return
			}
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager", "hr"}, cfg.SessionSecret)(apiHandler.GetMentorProfile)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentors/:id/profile|availability -> apiHandler [mentor_head+admin+manager+hr]")

	mux.HandleFunc("/api/mentor-head/classes", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.GetMentorHeadClasses)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/classes -> apiHandler.GetMentorHeadClasses [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/dashboard", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.GetMentorHeadDashboard)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/dashboard -> apiHandler.GetMentorHeadDashboard [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/archive", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.GetMentorHeadArchive)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/archive -> apiHandler.GetMentorHeadArchive [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/assign-mentor", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.AssignMentor)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/assign-mentor -> apiHandler.AssignMentor [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/availability-check", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.CheckMentorAvailability)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/availability-check -> apiHandler.CheckMentorAvailability [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/availability-calendar", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager", "hr"}, cfg.SessionSecret)(apiHandler.GetMentorHeadAvailabilityCalendar)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/availability-calendar -> apiHandler.GetMentorHeadAvailabilityCalendar [mentor_head+admin+manager+hr]")

	mux.HandleFunc("/api/mentor-head/return-to-ops", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.ReturnToOps)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/return-to-ops -> apiHandler.ReturnToOps [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/return-class", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.ReturnClass)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/return-class -> apiHandler.ReturnClass [mentor_head+admin+manager] (backward compatibility)")

	mux.HandleFunc("/api/mentor-head/unassign", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.UnassignMentor)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/unassign -> apiHandler.UnassignMentor [mentor_head+manager]")

	mux.HandleFunc("/api/mentor-head/start-round", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.StartRound)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/start-round -> apiHandler.StartRound [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/classes/start-round", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.StartRound)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/classes/start-round -> apiHandler.StartRound [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/shift-start-date", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.ShiftRoundStartDate)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/shift-start-date -> apiHandler.ShiftRoundStartDate [mentor_head+manager]")

	mux.HandleFunc("/api/mentor-head/reschedule-session", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.RescheduleClassSession)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/reschedule-session -> apiHandler.RescheduleClassSession [mentor_head+manager]")

	mux.HandleFunc("/api/mentor-head/close-round", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.CloseRound)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/close-round -> apiHandler.CloseRound [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/absence-promotion-overrides", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.GetAbsencePromotionOverrides)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/absence-promotion-overrides -> apiHandler.GetAbsencePromotionOverrides [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/absence-promotion-overrides/review", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.ReviewAbsencePromotionOverride)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/absence-promotion-overrides/review -> apiHandler.ReviewAbsencePromotionOverride [mentor_head+admin+manager]")

	mux.HandleFunc("/api/mentor-head/reopen-round", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.ReopenRound)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/reopen-round -> apiHandler.ReopenRound [mentor_head+admin+manager]")

	mux.HandleFunc("/api/class-workspace", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Ensure exact path match (no trailing slash)
		if r.URL.Path != "/api/class-workspace" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success", "manager"}, cfg.SessionSecret)(apiHandler.GetClassWorkspace)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/class-workspace -> apiHandler.GetClassWorkspace [mentor+mentor_head+admin+manager]")

	mux.HandleFunc("/api/classes/transfer-options", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.GetClassTransferOptions)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/classes/transfer-options -> apiHandler.GetClassTransferOptions [mentor_head+admin+manager]")

	mux.HandleFunc("/api/classes/transfer", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.TransferClassStudent)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/classes/transfer -> apiHandler.TransferClassStudent [mentor_head+admin+manager]")

	mux.HandleFunc("/api/classes/return-to-admin", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.ReturnClassStudentToAdmin)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/classes/return-to-admin -> apiHandler.ReturnClassStudentToAdmin [mentor_head+admin+manager]")

	mux.HandleFunc("/api/classes/return-early-repeat", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.ReturnClassStudentAsEarlyRepeat)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/classes/return-early-repeat -> apiHandler.ReturnClassStudentAsEarlyRepeat [mentor_head+admin+manager]")

	mux.HandleFunc("/api/classes/request-absence-promotion-override", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "manager"}, cfg.SessionSecret)(apiHandler.RequestAbsencePromotionOverride)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/classes/request-absence-promotion-override -> apiHandler.RequestAbsencePromotionOverride [mentor+mentor_head+admin+manager]")

	mux.HandleFunc("/api/class", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success", "manager"}, cfg.SessionSecret)(apiHandler.GetClass)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/class -> apiHandler.GetClass [mentor+mentor_head+admin+manager] (backward compatibility)")

	mux.HandleFunc("/api/notes", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success", "manager"}, cfg.SessionSecret)(apiHandler.GetNotes)(w, r)
		case http.MethodPost:
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success", "manager"}, cfg.SessionSecret)(apiHandler.CreateNote)(w, r)
		case http.MethodDelete:
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success", "manager"}, cfg.SessionSecret)(apiHandler.DeleteNote)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/notes -> apiHandler.GetNotes/CreateNote/DeleteNote [mentor+mentor_head+admin+manager]")

	mux.HandleFunc("/api/student", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "admin", "student_success", "manager"}, cfg.SessionSecret)(apiHandler.GetStudent)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student -> apiHandler.GetStudent [mentor+mentor_head+admin+manager]")

	// Notification Routes
	mux.HandleFunc("/api/notifications/ops", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "manager", "student_success"}, cfg.SessionSecret)(apiHandler.GetOpsNotifications)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/notifications/ops -> apiHandler.GetOpsNotifications [mentor_head+manager+student_success] (GET+POST)")

	mux.HandleFunc("/api/notifications/complaints/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/read") && r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.MarkComplaintNotificationRead)(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/notifications/complaints/:id/read -> apiHandler.MarkComplaintNotificationRead [mentor_head+manager]")

	mux.HandleFunc("/api/notifications/late-join/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/acknowledge") {
			if r.Method == http.MethodPost {
				middleware.RequireAnyRole([]string{"mentor", "mentor_head", "student_success"}, cfg.SessionSecret)(apiHandler.AcknowledgeLateJoinNotification)(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		http.NotFound(w, r)
	}))
	mux.HandleFunc("/api/notifications/late-join", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "student_success"}, cfg.SessionSecret)(apiHandler.GetLateJoinNotifications)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/notifications/late-join -> apiHandler [mentor+mentor_head+student_success]")

	// Late Joiner Routes (Canonical: /api/pre-enrolment/:leadId/...)
	mux.HandleFunc("/api/pre-enrolment/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		// parts: [api, pre-enrolment, :leadId, ...]
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}

		// Routing logic for sub-paths
		if len(parts) == 4 && parts[3] == "late-join-eligible-classes" {
			if r.Method == http.MethodGet {
				middleware.RequireAnyRole([]string{"admin", "moderator"}, cfg.SessionSecret)(apiHandler.GetEligibleClassesForLateJoin)(w, r)
				return
			}
		} else if len(parts) == 5 && parts[3] == "late-join" && parts[4] == "add" {
			if r.Method == http.MethodPost {
				middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(apiHandler.AddLateJoiner)(w, r)
				return
			}
		} else if len(parts) == 5 && parts[3] == "late-join" && parts[4] == "undo" {
			if r.Method == http.MethodPost {
				middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(apiHandler.UndoLateJoiner)(w, r)
				return
			}
		}

		http.NotFound(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/pre-enrolment/ -> Late Joiner Routes (Eligible/Add/Undo)")

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

	// Grades API routes
	mux.HandleFunc("/api/mentor/grades", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor", "admin"}, cfg.SessionSecret)(apiHandler.CreateGrade)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor/grades -> apiHandler.CreateGrade [mentor+admin]")

	mux.HandleFunc("/api/mentor-head/grades/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			middleware.RequireAnyRole([]string{"mentor_head"}, cfg.SessionSecret)(apiHandler.UpdateGrade)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/grades/:id -> apiHandler.UpdateGrade [mentor_head]")

	mux.HandleFunc("/api/grades", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "student_success", "admin", "manager"}, cfg.SessionSecret)(apiHandler.GetGradesForClass)(w, r)
		case http.MethodPost:
			middleware.RequireAnyRole([]string{"mentor", "mentor_head"}, cfg.SessionSecret)(apiHandler.CreateGrade)(w, r)
		case http.MethodDelete:
			middleware.RequireAnyRole([]string{"mentor", "mentor_head"}, cfg.SessionSecret)(apiHandler.DeleteGrade)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/grades -> apiHandler.GetGradesForClass (GET) or CreateGrade (POST)")

	mux.HandleFunc("/api/grades/preview", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor", "mentor_head", "student_success", "admin", "manager"}, cfg.SessionSecret)(apiHandler.GetGradesPreview)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/grades/preview -> apiHandler.GetGradesPreview (GET)")

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
			middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.GetStudentSuccessClass)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/class -> apiHandler.GetStudentSuccessClass [student_success, mentor_head, admin]")

	mux.HandleFunc("/api/student-success/placement-tests", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/student-success/placement-tests" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"student_success"}, cfg.SessionSecret)(apiHandler.GetStudentSuccessPlacementTests)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/placement-tests -> apiHandler.GetStudentSuccessPlacementTests [student_success only]")

	mux.HandleFunc("/api/student-success/placement-tests/complete", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/student-success/placement-tests/complete" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"student_success"}, cfg.SessionSecret)(apiHandler.CompletePlacementTest)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/placement-tests/complete -> apiHandler.CompletePlacementTest [student_success only]")

	mux.HandleFunc("/api/student-success/class/absence-feed", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.GetAbsenceFeed)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/class/absence-feed -> apiHandler.GetAbsenceFeed")

	mux.HandleFunc("/api/student-success/followups", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.GetFollowUps)(w, r)
		case http.MethodPost:
			middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.CreateFollowUp)(w, r)
		default:
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

	mux.HandleFunc("/api/student-success/feedback-collected", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.GetFeedbackCollected)(w, r)
			return
		}
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"student_success"}, cfg.SessionSecret)(apiHandler.UploadFeedbackCollected)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/feedback-collected -> apiHandler.GetFeedbackCollected/UploadFeedbackCollected")

	mux.HandleFunc("/api/student-success/feedback-collected/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			middleware.RequireAnyRole([]string{"student_success"}, cfg.SessionSecret)(apiHandler.DeleteFeedbackCollected)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/feedback-collected/:id -> apiHandler.DeleteFeedbackCollected")

	// Complaint routes - Student Success
	mux.HandleFunc("/api/student-success/complaints", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success", "admin"}, cfg.SessionSecret)(apiHandler.CreateComplaint)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/student-success/complaints -> apiHandler.CreateComplaint")

	// Complaint routes - Mentor Head
	mux.HandleFunc("/api/mentor-head/complaints", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/mentor-head/complaints" {
			http.NotFound(w, r)
			return
		}
		middleware.RequireAnyRole([]string{"mentor_head", "manager", "admin"}, cfg.SessionSecret)(apiHandler.GetMentorHeadComplaints)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/complaints -> apiHandler.GetMentorHeadComplaints")

	// Complaint actions - Mentor Head (with path params)
	mux.HandleFunc("/api/mentor-head/complaints/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/mentor-head/complaints" || r.URL.Path == "/api/mentor-head/complaints/" {
			http.NotFound(w, r)
			return
		}
		middleware.RequireAnyRole([]string{"mentor_head", "manager", "admin"}, cfg.SessionSecret)(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/update") {
				apiHandler.UpdateComplaintStatusHandler(w, r)
			} else if strings.HasSuffix(r.URL.Path, "/resolve") {
				apiHandler.ResolveComplaintHandler(w, r)
			} else {
				http.NotFound(w, r)
			}
		})(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/mentor-head/complaints/:id/update and /resolve")

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

	// Compliance APIs (Student Success audit)
	mux.HandleFunc("/api/compliance/check", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success"}, cfg.SessionSecret)(apiHandler.UpsertComplianceCheck)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/compliance/check -> apiHandler.UpsertComplianceCheck [student_success]")

	mux.HandleFunc("/api/compliance/class/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success"}, cfg.SessionSecret)(apiHandler.GetComplianceByClass)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/compliance/class/:class_key -> apiHandler.GetComplianceByClass [student_success]")

	mux.HandleFunc("/api/reports/mentors", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success", "mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.GetMentorReports)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/reports/mentors -> apiHandler.GetMentorReports [student_success+mentor_head+manager]")

	mux.HandleFunc("/api/reports/mentors/checklist", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success", "mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.GetMentorReportChecklist)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/reports/mentors/checklist -> apiHandler.GetMentorReportChecklist [student_success+mentor_head+manager]")

	mux.HandleFunc("/api/reports/mentors/classes", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"student_success", "mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.GetMentorClassReports)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/reports/mentors/classes -> apiHandler.GetMentorClassReports [student_success+mentor_head+manager]")

	mux.HandleFunc("/api/reports/mentors/exclude", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.ExcludeMentorReportRow)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/reports/mentors/exclude -> apiHandler.ExcludeMentorReportRow [mentor_head+manager]")

	mux.HandleFunc("/api/reports/daily", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.GetDailyReport)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/reports/daily -> apiHandler.GetDailyReport [mentor_head+manager]")

	mux.HandleFunc("/api/reports/daily/read", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.MarkDailyReportRead)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/reports/daily/read -> apiHandler.MarkDailyReportRead [mentor_head+manager]")

	mux.HandleFunc("/api/reports/manager-ops", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"manager"}, cfg.SessionSecret)(apiHandler.GetManagerOpsReport)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/reports/manager-ops -> apiHandler.GetManagerOpsReport [manager]")

	mux.HandleFunc("/api/reports/manager-overview", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"manager"}, cfg.SessionSecret)(apiHandler.GetManagerOverviewReport)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/reports/manager-overview -> apiHandler.GetManagerOverviewReport [manager]")

	mux.HandleFunc("/api/reports/bi", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAnyRole([]string{"admin", "mentor_head", "manager"}, cfg.SessionSecret)(apiHandler.GetBIReports)(w, r)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/reports/bi -> apiHandler.GetBIReports [admin+mentor_head+manager]")

	mux.HandleFunc("/api/manager/users", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			middleware.RequireAnyRole([]string{"manager"}, cfg.SessionSecret)(apiHandler.GetManagerUsers)(w, r)
		case http.MethodPost:
			middleware.RequireAnyRole([]string{"manager"}, cfg.SessionSecret)(apiHandler.CreateManagerUser)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/manager/users -> manager staff APIs [manager]")

	mux.HandleFunc("/api/manager/users/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/deactivate") {
			middleware.RequireAnyRole([]string{"manager"}, cfg.SessionSecret)(apiHandler.DeactivateManagerUser)(w, r)
			return
		}
		if r.Method == http.MethodDelete {
			middleware.RequireAnyRole([]string{"manager"}, cfg.SessionSecret)(apiHandler.DeleteManagerUser)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/manager/users/:id -> manager remove user API [manager]")
	cfg.Debugf("ROUTE REGISTERED: /api/manager/users/:id/deactivate -> manager deactivate user API [manager]")

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

	// Student Profile API Routes (Milestone 4) - accessible by all roles
	mux.HandleFunc("/api/students/search", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"admin", "moderator", "mentor_head", "mentor", "student_success"}, cfg.SessionSecret)(handlers.SearchStudents)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/students/search -> handlers.SearchStudents [all roles]")

	mux.HandleFunc("/api/students/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Parse student ID from path
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}

		// Route to specific endpoints
		if len(parts) == 3 && parts[2] != "" {
			// /api/students/:id/profile
			if r.Method == http.MethodGet {
				middleware.RequireAnyRole([]string{"admin", "moderator", "mentor_head", "mentor", "student_success"}, cfg.SessionSecret)(handlers.GetStudentProfile)(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		} else if len(parts) == 4 {
			switch parts[3] {
			case "profile":
				if r.Method == http.MethodGet {
					middleware.RequireAnyRole([]string{"admin", "moderator", "mentor_head", "mentor", "student_success"}, cfg.SessionSecret)(handlers.GetStudentProfile)(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case "basic-info":
				if r.Method == http.MethodPut {
					middleware.RequireAnyRole([]string{"admin", "mentor_head"}, cfg.SessionSecret)(handlers.UpdateStudentBasicInfo)(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case "history":
				if r.Method == http.MethodGet {
					middleware.RequireAnyRole([]string{"admin", "moderator", "mentor_head", "mentor", "student_success"}, cfg.SessionSecret)(handlers.GetStudentHistory)(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case "current-status":
				if r.Method == http.MethodGet {
					middleware.RequireAnyRole([]string{"admin", "moderator", "mentor_head", "mentor", "student_success"}, cfg.SessionSecret)(handlers.GetCurrentStatus)(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case "notes":
				if r.Method == http.MethodGet {
					middleware.RequireAnyRole([]string{"admin", "moderator", "mentor_head", "mentor", "student_success"}, cfg.SessionSecret)(handlers.GetStudentNotes)(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			default:
				http.NotFound(w, r)
			}
		} else {
			http.NotFound(w, r)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/students/:id/* -> Student Profile Endpoints [all roles]")

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

	mux.HandleFunc("/api/auth/force-change-password", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			middleware.RequireAuth(authHandler.ForceChangePassword, cfg.SessionSecret)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /api/auth/force-change-password -> authHandler.ForceChangePassword [RequireAuth]")

	// Protected routes - register specific routes BEFORE catch-all
	// /pre-enrolment/new - allow admin + moderator
	mux.HandleFunc("/pre-enrolment/new", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /pre-enrolment/new handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/pre-enrolment/new" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			cfg.Debugf("  → Calling preEnrolmentHandler.NewForm")
			middleware.RequireAnyRole([]string{"admin", "moderator", "manager"}, cfg.SessionSecret)(preEnrolmentHandler.NewForm)(w, r)
		case http.MethodPost:
			cfg.Debugf("  → Calling preEnrolmentHandler.Create")
			middleware.RequireAnyRole([]string{"admin", "moderator", "manager"}, cfg.SessionSecret)(preEnrolmentHandler.Create)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /pre-enrolment/new -> preEnrolmentHandler (NewForm/Create) [admin+moderator]")

	// Routes with path parameters - handle manually (Go stdlib mux doesn't support {id})
	// /pre-enrolment/{id} - GET allows admin+moderator+student_success (read-only for SS), POST/Update/Status admin only
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

		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sleeping-follow-up") {
			cfg.Debugf("  → Calling preEnrolmentHandler.SendSleepingLeadFollowUp")
			middleware.RequireAnyRole([]string{"admin", "moderator", "manager"}, cfg.SessionSecret)(preEnrolmentHandler.SendSleepingLeadFollowUp)(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/offer-follow-up") {
			cfg.Debugf("  → Calling preEnrolmentHandler.SendOfferSentFollowUp")
			middleware.RequireAnyRole([]string{"admin", "moderator", "manager"}, cfg.SessionSecret)(preEnrolmentHandler.SendOfferSentFollowUp)(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			cfg.Debugf("  → Calling preEnrolmentHandler.Detail")
			// GET detail - allow admin + moderator + student_success (read-only)
			middleware.RequireAnyRole([]string{"admin", "moderator", "student_success", "manager"}, cfg.SessionSecret)(preEnrolmentHandler.Detail)(w, r)
		case http.MethodPost:
			// All POST requests to /pre-enrolment/{id} go to Update handler
			// Update handler reads action parameter and routes accordingly
			cfg.Debugf("  → Calling preEnrolmentHandler.Update (action-based routing)")
			// Allow admin + moderator (Update handler enforces restrictions per action)
			middleware.RequireAnyRole([]string{"admin", "moderator", "manager"}, cfg.SessionSecret)(preEnrolmentHandler.Update)(w, r)
		default:
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
			middleware.RequireAnyRole([]string{"admin", "moderator", "manager"}, cfg.SessionSecret)(preEnrolmentHandler.List)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /pre-enrolment -> preEnrolmentHandler.List [admin+moderator]")

	mux.HandleFunc("/private-track/", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /private-track/ (catch-all) handler for %s %s", r.Method, r.URL.Path)
		if !strings.HasPrefix(r.URL.Path, "/private-track/") || r.URL.Path == "/private-track/" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			middleware.RequireAnyRole([]string{"admin", "manager"}, cfg.SessionSecret)(preEnrolmentHandler.PrivateTrackAction)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /private-track/ -> preEnrolmentHandler.PrivateTrackAction [admin+manager]")

	mux.HandleFunc("/private-track", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /private-track handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/private-track" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			middleware.RequireAnyRole([]string{"admin", "manager"}, cfg.SessionSecret)(preEnrolmentHandler.PrivateTrackList)(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))
	cfg.Debugf("ROUTE REGISTERED: /private-track -> preEnrolmentHandler.PrivateTrackList [admin+manager]")

	// Classes routes - admin, manager can manage; mentor_head and student_success are read-only
	mux.HandleFunc("/classes", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling classesHandler.List")
			middleware.RequireAnyRole([]string{"admin", "manager", "mentor_head", "student_success"}, cfg.SessionSecret)(classesHandler.List)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes -> classesHandler.List [GET: admin+manager+mentor_head+student_success; mentor_head/student_success read-only]")

	mux.HandleFunc("/classes/move", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/move handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes/move" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling classesHandler.Move")
			middleware.RequireAnyRole([]string{"admin", "manager"}, cfg.SessionSecret)(classesHandler.Move)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/move -> classesHandler.Move [admin+manager]")

	mux.HandleFunc("/classes/send", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/send handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes/send" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling classesHandler.SendToMentor")
			middleware.RequireAnyRole([]string{"admin", "manager"}, cfg.SessionSecret)(classesHandler.SendToMentor)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/send -> classesHandler.SendToMentor [admin+manager]")

	// POST /classes/return with form field class_key (not path; classKey can contain /)
	mux.HandleFunc("/classes/return", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/return handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes/return" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling classesHandler.ReturnFromMentor")
			middleware.RequireAnyRole([]string{"admin", "manager"}, cfg.SessionSecret)(classesHandler.ReturnFromMentor)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/return -> classesHandler.ReturnFromMentor [admin+manager]")

	// Archived classes (Ops)
	mux.HandleFunc("/classes/archived", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/archived handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes/archived" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling classesHandler.Archived")
			middleware.RequireAnyRole([]string{"admin", "manager", "mentor_head"}, cfg.SessionSecret)(classesHandler.Archived)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/archived -> classesHandler.Archived [admin+manager+mentor_head]")

	// POST /classes/archive
	mux.HandleFunc("/classes/archive", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/archive handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes/archive" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling classesHandler.ArchiveClass")
			middleware.RequireAnyRole([]string{"admin", "manager"}, cfg.SessionSecret)(classesHandler.ArchiveClass)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/archive -> classesHandler.ArchiveClass [admin+manager]")

	// POST /classes/unarchive
	mux.HandleFunc("/classes/unarchive", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /classes/unarchive handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/classes/unarchive" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling classesHandler.UnarchiveClass")
			middleware.RequireAnyRole([]string{"admin", "manager"}, cfg.SessionSecret)(classesHandler.UnarchiveClass)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /classes/unarchive -> classesHandler.UnarchiveClass [admin+manager]")

	// Finance routes - dashboard for admin/manager
	mux.HandleFunc("/finance", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /finance handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/finance" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			cfg.Debugf("  → Calling financeHandler.Dashboard")
			middleware.RequireAnyRole([]string{"admin"}, cfg.SessionSecret)(financeHandler.Dashboard)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /finance -> financeHandler.Dashboard [GET: admin + manager]")

	mux.HandleFunc("/finance/new-expense", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /finance/new-expense handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/finance/new-expense" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			cfg.Debugf("  → Calling financeHandler.NewExpenseForm")
			middleware.RequireAnyRole([]string{"admin", "manager"}, cfg.SessionSecret)(financeHandler.NewExpenseForm)(w, r)
		case http.MethodPost:
			cfg.Debugf("  → Calling financeHandler.CreateExpense")
			middleware.RequireAnyRole([]string{"admin", "manager"}, cfg.SessionSecret)(financeHandler.CreateExpense)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /finance/new-expense -> financeHandler (NewExpenseForm/CreateExpense) [admin + manager]")

	mux.HandleFunc("/finance/new-revenue", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /finance/new-revenue handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/finance/new-revenue" {
			cfg.Debugf("  → Path mismatch, returning 404")
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			cfg.Debugf("  → Calling financeHandler.NewRevenueForm")
			middleware.RequireAnyRole([]string{"manager"}, cfg.SessionSecret)(financeHandler.NewRevenueForm)(w, r)
		case http.MethodPost:
			cfg.Debugf("  → Calling financeHandler.CreateRevenue")
			middleware.RequireAnyRole([]string{"manager"}, cfg.SessionSecret)(financeHandler.CreateRevenue)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /finance/new-revenue -> financeHandler (NewRevenueForm/CreateRevenue) [manager only]")

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

	// Student Success routes - student_success + admin
	mux.HandleFunc("/student-success", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /student-success handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/student-success" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			// Redirect to React app
			http.Redirect(w, r, "/app/student-success", http.StatusFound)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /student-success -> studentSuccessHandler.Dashboard [student_success+admin]")

	mux.HandleFunc("/student-success/feedback", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /student-success/feedback handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling studentSuccessHandler.SubmitFeedback")
			middleware.RequireAnyRole([]string{"student_success", "admin"}, cfg.SessionSecret)(studentSuccessHandler.SubmitFeedback)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /student-success/feedback -> studentSuccessHandler.SubmitFeedback [student_success+admin]")

	mux.HandleFunc("/student-success/follow-up", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /student-success/follow-up handler for %s %s", r.Method, r.URL.Path)
		if r.Method == http.MethodPost {
			cfg.Debugf("  → Calling studentSuccessHandler.LogFollowUp")
			middleware.RequireAnyRole([]string{"student_success", "admin"}, cfg.SessionSecret)(studentSuccessHandler.LogFollowUp)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	cfg.Debugf("ROUTE REGISTERED: /student-success/follow-up -> studentSuccessHandler.LogFollowUp [student_success+admin]")

	// HR routes - hr + admin
	mux.HandleFunc("/hr/mentors", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
		cfg.Debugf("HANDLER: /hr/mentors handler for %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/hr/mentors" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			cfg.Debugf("  → Calling hrHandler.MentorsList")
			middleware.RequireAnyRole([]string{"hr", "admin"}, cfg.SessionSecret)(hrHandler.MentorsList)(w, r)
		case http.MethodPost:
			cfg.Debugf("  → Calling hrHandler.MentorsCreate")
			middleware.RequireAnyRole([]string{"hr", "admin"}, cfg.SessionSecret)(hrHandler.MentorsCreate)(w, r)
		default:
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
	// Serve React app public static files from /app/static/*
	mux.Handle("/app/static/", http.StripPrefix("/app/", reactFS))
	cfg.Debugf("ROUTE REGISTERED: /app/static/ -> React public static files from frontend/dist")

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
	log.Printf("Default student_success login: %s / %s", cfg.StudentSuccessEmail, cfg.StudentSuccessPassword)
	log.Printf("Default hr login: %s / %s", cfg.HREmail, cfg.HRPassword)

	log.Printf("Default manager login: %s / %s", cfg.ManagerEmail, cfg.ManagerPassword)
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
	_, err = models.CreateUser(cfg.AdminEmail, string(hashedPassword), "admin", "Admin", "")
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
	_, err = models.CreateUser(cfg.ModeratorEmail, string(hashedPassword), "moderator", "Moderator", "")
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
	_, err = models.CreateUser(cfg.MentorHeadEmail, string(hashedPassword), "mentor_head", "Mentor Head", "")
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
	_, err = models.CreateUser(cfg.MentorEmail, string(hashedPassword), "mentor", "Default Mentor", "01000000000")
	if err != nil {
		return fmt.Errorf("failed to create mentor user: %w", err)
	}
	log.Printf("Created default mentor user: %s", cfg.MentorEmail)
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
	_, err = models.CreateUser(cfg.HREmail, string(hashedPassword), "hr", "HR", "")
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
	_, err = models.CreateUser(cfg.StudentSuccessEmail, string(hashedPassword), "student_success", "Student Success", "")
	if err != nil {
		return fmt.Errorf("failed to create student_success user: %w", err)
	}
	log.Printf("Created default student_success user: %s", cfg.StudentSuccessEmail)
	return nil
}

func seedManagerUser(cfg *config.Config) error {
	_, err := models.GetUserByEmail(cfg.ManagerEmail)
	if err == nil {
		return nil
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.ManagerPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	_, err = models.CreateUser(cfg.ManagerEmail, string(hashedPassword), "manager", "Manager", "")
	if err != nil {
		return fmt.Errorf("failed to create manager user: %w", err)
	}
	log.Printf("Created default manager user: %s", cfg.ManagerEmail)
	return nil
}
