package handlers

import (
	"net/http"
	"strings"

	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

func (h *APIHandler) GetFinalGradeNoteTranslation(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "pre-enrolment" || parts[3] != "final-grade-note-translation" {
		jsonError(w, http.StatusBadRequest, "Invalid request path")
		return
	}

	leadID, err := uuid.Parse(parts[2])
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid lead ID")
		return
	}

	latestGrade, err := models.GetLatestCompletedGrade(leadID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to load final grade note")
		return
	}
	if latestGrade == nil || !latestGrade.Notes.Valid {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"ok":          true,
			"translation": "",
			"translated":  false,
		})
		return
	}

	note := strings.TrimSpace(latestGrade.Notes.String)
	if note == "" || !looksEnglishGradeNote(note) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"ok":          true,
			"translation": "",
			"translated":  false,
		})
		return
	}

	translation := translateSingleFinalGradeTextToArabic(note)
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"translation": translation,
		"translated":  strings.TrimSpace(translation) != "",
	})
}
