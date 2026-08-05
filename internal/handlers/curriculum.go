package handlers

import (
	"net/http"
	"os"
	"path/filepath"
)

// ServeCurriculum serves the static full-curriculum page.
func ServeCurriculum(w http.ResponseWriter, r *http.Request) {
	workDir, _ := os.Getwd()
	filePath := filepath.Join(workDir, "web", "pages", "curriculum.html")
	http.ServeFile(w, r, filePath)
}
