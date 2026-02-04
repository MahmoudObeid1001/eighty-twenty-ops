package handlers

import (
	"net/http"
	"net/url"
)

func flashFromQuery(r *http.Request) (string, string) {
	if r == nil {
		return "", ""
	}
	if msg := r.URL.Query().Get("error"); msg != "" {
		return msg, "error"
	}
	if msg := r.URL.Query().Get("success"); msg != "" {
		return msg, "success"
	}
	return "", ""
}

func redirectWithError(w http.ResponseWriter, r *http.Request, basePath, message string) {
	redirectWithFlash(w, r, basePath, "error", message)
}

func redirectWithSuccess(w http.ResponseWriter, r *http.Request, basePath, message string) {
	redirectWithFlash(w, r, basePath, "success", message)
}

func redirectWithFlash(w http.ResponseWriter, r *http.Request, basePath, kind, message string) {
	u, err := url.Parse(basePath)
	if err != nil {
		http.Redirect(w, r, basePath, http.StatusFound)
		return
	}
	q := u.Query()
	if message != "" {
		q.Set(kind, message)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}
