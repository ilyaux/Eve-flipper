package auth

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Session represents a stored auth session.
type Session struct {
	CharacterID   int64
	CharacterName string
	AccessToken   string
	RefreshToken  string
	ExpiresAt     time.Time
}

// SessionStore handles session persistence in SQLite.
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore creates a store backed by the given SQL database.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

// Save stores or replaces the current session (single-user app).
func (s *SessionStore) Save(sess *Session) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO auth_session (id, character_id, character_name, access_token, refresh_token, expires_at)
		VALUES (1, ?, ?, ?, ?, ?)`,
		sess.CharacterID, sess.CharacterName, sess.AccessToken, sess.RefreshToken, sess.ExpiresAt.Unix(),
	)
	return err
}

// Get returns the current session, or nil if none.
func (s *SessionStore) Get() *Session {
	var sess Session
	var expiresUnix int64
	err := s.db.QueryRow(`
		SELECT character_id, character_name, access_token, refresh_token, expires_at
		FROM auth_session WHERE id = 1`).
		Scan(&sess.CharacterID, &sess.CharacterName, &sess.AccessToken, &sess.RefreshToken, &expiresUnix)
	if err != nil {
		return nil
	}
	sess.ExpiresAt = time.Unix(expiresUnix, 0)
	return &sess
}

// Delete removes the current session.
func (s *SessionStore) Delete() {
	s.db.Exec("DELETE FROM auth_session WHERE id = 1")
}

// EnsureValidToken returns a valid access token, refreshing if needed.
func (s *SessionStore) EnsureValidToken(sso *SSOConfig) (string, error) {
	sess := s.Get()
	if sess == nil {
		return "", fmt.Errorf("not logged in")
	}

	// If token is still valid (with 60s buffer), return it
	if time.Now().Before(sess.ExpiresAt.Add(-60 * time.Second)) {
		return sess.AccessToken, nil
	}

	// Refresh the token
	log.Printf("[AUTH] Refreshing token for %s", sess.CharacterName)
	tok, err := sso.RefreshToken(sess.RefreshToken)
	if err != nil {
		s.Delete()
		return "", fmt.Errorf("refresh failed: %w", err)
	}

	sess.AccessToken = tok.AccessToken
	sess.RefreshToken = tok.RefreshToken
	sess.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	if err := s.Save(sess); err != nil {
		return "", fmt.Errorf("save session: %w", err)
	}

	return sess.AccessToken, nil
}
