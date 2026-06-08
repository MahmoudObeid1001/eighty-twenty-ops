package handlers

import (
	"net/http"
	"os"
	"path/filepath"
)

// ServePlacementResult serves the static placement result page.
func ServePlacementResult(w http.ResponseWriter, r *http.Request) {
	workDir, _ := os.Getwd()
	filePath := filepath.Join(workDir, "web", "pages", "placement_result.html")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	http.ServeFile(w, r, filePath)
}
