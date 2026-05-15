package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(tokenFile, adminPassword string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.With(requirePassword(adminPassword)).Post("/token", uploadTokenHandler(tokenFile))

	return r
}

func requirePassword(password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if password == "" {
				http.Error(w, "admin_password not configured", http.StatusForbidden)
				return
			}
			auth := r.Header.Get("Authorization")
			got, ok := strings.CutPrefix(auth, "Bearer ")
			if !ok || got != password {
				log.Printf("token upload: unauthorized attempt from %s", r.RemoteAddr)
				w.Header().Set("WWW-Authenticate", `Bearer realm="mattube"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func uploadTokenHandler(tokenFile string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}

		var check json.RawMessage
		if err := json.Unmarshal(body, &check); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := os.WriteFile(tokenFile, body, 0600); err != nil {
			log.Printf("token upload: write %s: %v", tokenFile, err)
			http.Error(w, "write token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("token upload: wrote new token to %s (restart server to apply)", tokenFile)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "note": "restart server to apply"})
	}
}
