package handlers

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"

	"eighty-twenty-ops/internal/config"
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

		var err2 error
		templates, err2 = template.ParseFS(views.TemplatesFS, "*.html")
		if err2 != nil {
			panic(fmt.Sprintf("Failed to parse templates: %v", err2))
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

func renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if cfg != nil {
		cfg.Debugf("🎨 renderTemplate() called with template name: %s", name)
	}
	initTemplates()

	// Map template filenames to their content template names and layout
	contentTemplateMap := map[string]string{
		"login.html":                "login_content",
		"pre_enrolment_new.html":    "pre_enrolment_new_content",
		"pre_enrolment_list.html":   "pre_enrolment_list_content",
		"pre_enrolment_detail.html": "pre_enrolment_detail_content",
		"classes.html":              "classes_content",
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

	// Verify templates are initialized
	if templates == nil {
		log.Printf("ERROR: Templates not initialized")
		http.Error(w, "Templates not initialized", http.StatusInternalServerError)
		return
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
