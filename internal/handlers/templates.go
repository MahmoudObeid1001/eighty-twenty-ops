package handlers

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/views"
)

var (
	templates     *template.Template
	templatesOnce sync.Once
	cfg           *config.Config
)

// SetConfig sets the config for debug logging
func SetConfig(c *config.Config) {
	cfg = c
}

// InitTemplates initializes templates at startup (can be called explicitly)
// This is the same as initTemplates() but exported for early initialization
func InitTemplates() {
	initTemplates()
}

func initTemplates() {
	templatesOnce.Do(func() {
		if cfg != nil {
			cfg.Debugf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			cfg.Debugf("📦 INITIALIZING TEMPLATES")
			cfg.Debugf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		}

		// List all template files in the embedded FS
		entries, err := fs.ReadDir(views.TemplatesFS, ".")
		if err != nil {
			log.Printf("ERROR: Failed to read template directory: %v", err)
			panic(fmt.Sprintf("Failed to read template directory: %v", err))
		}

		if cfg != nil {
			cfg.Debugf("📁 Template files found in embedded FS:")
		}
		var templateFiles []string
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".html") {
				templateFiles = append(templateFiles, entry.Name())
				if cfg != nil {
					cfg.Debugf("  - %s", entry.Name())
				}
			}
		}

		if len(templateFiles) == 0 {
			log.Printf("ERROR: No template files found in embedded filesystem")
			panic("No template files found in embedded filesystem")
		}

		funcMap := template.FuncMap{
			"urlquery": url.QueryEscape,
			"len": func(slice interface{}) int {
				switch v := slice.(type) {
				case []interface{}:
					return len(v)
				case nil:
					return 0
				default:
					// Use reflection for other slice types
					val := reflect.ValueOf(slice)
					if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
						return val.Len()
					}
					return 0
				}
			},
			"sub": func(a, b int) int {
				return a - b
			},
		}
		tmpl := template.New("").Funcs(funcMap)
		var err2 error
		templates, err2 = tmpl.ParseFS(views.TemplatesFS, "*.html")
		if err2 != nil {
			log.Printf("ERROR: Failed to parse templates: %v", err2)
			panic(fmt.Sprintf("Failed to parse templates: %v", err2))
		}

		// Verify templates were actually parsed
		if templates == nil {
			log.Printf("ERROR: Template parsing returned nil")
			panic("Template parsing returned nil")
		}

		// List all defined templates after parsing
		if cfg != nil {
			cfg.Debugf("📋 Defined templates after parsing:")
			for _, tmpl := range templates.Templates() {
				cfg.Debugf("  - Template name: '%s'", tmpl.Name())
			}
			cfg.Debugf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		}
	})
}

func renderTemplate(w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	if cfg != nil {
		cfg.Debugf("🎨 renderTemplate() called with template name: %s", name)
	}
	
	// Initialize templates (will only run once due to sync.Once)
	initTemplates()

	// Verify templates are initialized - if still nil, there was an initialization error
	if templates == nil {
		log.Printf("ERROR: Templates not initialized after initTemplates() call")
		log.Printf("ERROR: This should not happen - templates should be initialized or panic should have occurred")
		http.Error(w, "Templates not initialized. Please check server logs for template initialization errors.", http.StatusInternalServerError)
		return
	}

	// Map template filenames to their content template names and layout
	contentTemplateMap := map[string]string{
		"login.html":                "login_content",
		"pre_enrolment_new.html":    "pre_enrolment_new_content",
		"pre_enrolment_list.html":   "pre_enrolment_list_content",
		"pre_enrolment_detail.html": "pre_enrolment_detail_content",
		"classes.html":              "classes_content",
		"finance.html":              "finance_content",
		"finance_new_expense.html":  "finance_new_expense_content",
		"access_restricted.html":    "access_restricted_content",
		"mentor_head.html":          "mentor_head_content",
		"mentor.html":               "mentor_content",
		"mentor_class_detail.html":  "mentor_class_detail_content",
		"community_officer.html":   "community_officer_content",
		"hr_mentors.html":          "hr_mentors_content",
	}
	
	// Templates that use auth_layout instead of main layout
	authLayoutTemplates := map[string]bool{
		"login.html": true,
	}

	// Get the content template name for this file
	contentTemplateName, exists := contentTemplateMap[name]
	if !exists {
		if cfg != nil {
			cfg.Debugf("⚠️  No content template mapping for %s, using 'content'", name)
		}
		contentTemplateName = "content"
	}

	// Verify layout template exists
	layoutTmpl := templates.Lookup("layout")
	if layoutTmpl == nil {
		log.Printf("ERROR: Layout template not found")
		http.Error(w, "Layout template not found", http.StatusInternalServerError)
		return
	}

	// Verify content template exists
	contentTmpl := templates.Lookup(contentTemplateName)
	if contentTmpl == nil {
		log.Printf("ERROR: Content template '%s' not found. Available templates:", contentTemplateName)
		for _, tmpl := range templates.Templates() {
			log.Printf("  - %s", tmpl.Name())
		}
		http.Error(w, fmt.Sprintf("Content template '%s' not found", contentTemplateName), http.StatusInternalServerError)
		return
	}
	if cfg != nil {
		cfg.Debugf("  ✅ Verified content template '%s' exists", contentTemplateName)
	}

	// Add ContentTemplate to data so layout knows which content to render
	// Ensure data is a map so we can add ContentTemplate
	var dataMap map[string]interface{}
	if existingMap, ok := data.(map[string]interface{}); ok {
		dataMap = existingMap
	} else {
		// If data is not a map, create a new map (shouldn't happen, but be safe)
		dataMap = make(map[string]interface{})
		if cfg != nil {
			cfg.Debugf("  ⚠️  Data was not a map, created new map")
		}
	}
	dataMap["ContentTemplate"] = contentTemplateName
	if _, ok := dataMap["UserRole"]; !ok && r != nil {
		dataMap["UserRole"] = middleware.GetUserRole(r)
	}
	if cfg != nil {
		cfg.Debugf("  → Set ContentTemplate = '%s'", contentTemplateName)
	}

	// Determine which layout to use
	useAuthLayout := authLayoutTemplates[name]
	layoutName := "layout"
	if useAuthLayout {
		layoutName = "auth_layout"
		if cfg != nil {
			cfg.Debugf("  → Using auth_layout for login page")
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// All templates are already in the same template set, so we can execute directly
	// Use dataMap (which has ContentTemplate set) instead of original data
	if err := templates.ExecuteTemplate(w, layoutName, dataMap); err != nil {
		log.Printf("ERROR: Template execute error: %v", err)
		http.Error(w, fmt.Sprintf("Template execute error: %v", err), http.StatusInternalServerError)
		return
	}
	if cfg != nil {
		cfg.Debugf("  ✅ Template %s rendered successfully", name)
	}
}
