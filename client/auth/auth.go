package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie   = "session"
	sessionDuration = 30 * 24 * time.Hour
	bcryptCost      = 12
)

type User struct {
	ID        int64
	Username  string
	IsAdmin   bool
	LastLogin *string
}

type contextKey struct{}

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUserNotFound = errors.New("user not found")

// CreateUser hashes the password and inserts a new user. Returns the new user ID.
func CreateUser(db *sql.DB, username, password string, isAdmin bool) (int64, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return 0, err
	}
	admin := 0
	if isAdmin {
		admin = 1
	}
	res, err := db.Exec(
		"INSERT INTO users (username, password, is_admin) VALUES (?, ?, ?)",
		username, string(hash), admin,
	)
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}
	return res.LastInsertId()
}

// Login validates credentials and returns a new session token.
func Login(db *sql.DB, username, password string) (string, error) {
	var id int64
	var hash string
	err := db.QueryRow(
		"SELECT id, password FROM users WHERE username=?", username,
	).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	token, err := newToken()
	if err != nil {
		return "", err
	}

	expires := time.Now().Add(sessionDuration).UTC().Format(time.RFC3339)
	_, err = db.Exec(
		"INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)",
		token, id, expires,
	)
	if err != nil {
		return "", err
	}

	db.Exec("UPDATE users SET last_login=datetime('now') WHERE id=?", id) //nolint:errcheck
	return token, nil
}

// Logout deletes the session.
func Logout(db *sql.DB, token string) error {
	_, err := db.Exec("DELETE FROM sessions WHERE token=?", token)
	return err
}

// Validate looks up a session token and returns the user. Returns nil if invalid/expired.
func Validate(db *sql.DB, token string) (*User, error) {
	var u User
	var isAdmin int
	err := db.QueryRow(`
		SELECT u.id, u.username, u.is_admin, u.last_login
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token=? AND s.expires_at > datetime('now')
	`, token).Scan(&u.ID, &u.Username, &isAdmin, &u.LastLogin)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	return &u, nil
}

// ChangePassword replaces a user's password hash.
func ChangePassword(db *sql.DB, userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return err
	}
	_, err = db.Exec("UPDATE users SET password=? WHERE id=?", string(hash), userID)
	return err
}

// DeleteUser removes a user and all their sessions.
func DeleteUser(db *sql.DB, userID int64) error {
	_, err := db.Exec("DELETE FROM users WHERE id=?", userID)
	return err
}

// ListUsers returns all users.
func ListUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query("SELECT id, username, is_admin, last_login FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var isAdmin int
		if err := rows.Scan(&u.ID, &u.Username, &isAdmin, &u.LastLogin); err != nil {
			return nil, err
		}
		u.IsAdmin = isAdmin == 1
		users = append(users, u)
	}
	return users, rows.Err()
}

// CountUsers returns the total number of users (used for bootstrap check).
func CountUsers(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// --- Middleware ---

type ctxKey = contextKey

// RequireAuth is Chi middleware: validates session cookie, sets user in context.
func RequireAuth(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookie)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			user, err := Validate(db, cookie.Value)
			if err != nil || user == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKey{}, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin is Chi middleware: checks user is admin (chain after RequireAuth).
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil || !user.IsAdmin {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UserFromContext retrieves the authenticated user from request context.
func UserFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(ctxKey{}).(*User)
	return u
}

// SetSessionCookie writes the session cookie to the response.
func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
