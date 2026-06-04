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
	http.ServeFile(w, r, filePath)
}
