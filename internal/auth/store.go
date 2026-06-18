package auth

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// Session represents a stored auth session.
type Session struct {
	CharacterID   int64
	CharacterName string
	AccessToken   string
	RefreshToken  string
	ExpiresAt     time.Time
	Active        bool
}

// SessionStore handles session persistence in SQLite.
type SessionStore struct {
	db    *sql.DB
	vault *TokenVault
}

const defaultUserID = "default"

// NewSessionStore creates a store backed by the given SQL database.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db, vault: NewTokenVault(db)}
}

func (s *SessionStore) Vault() *TokenVault {
	if s == nil {
		return nil
	}
	return s.vault
}

func normalizeUserID(userID string) string {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return defaultUserID
	}
	return trimmed
}

func hostedSessionModeEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EVEFLIPPER_HOSTED"))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// Save stores or updates a character session while preserving active selection.
// If there is no active character yet, this session becomes active.
func (s *SessionStore) Save(sess *Session) error {
	return s.SaveForUser(defaultUserID, sess)
}

// SaveForUser stores or updates a character session for the given user while preserving active selection.
// If there is no active character yet, this session becomes active.
func (s *SessionStore) SaveForUser(userID string, sess *Session) error {
	userID = normalizeUserID(userID)
	if sess == nil {
		return fmt.Errorf("nil session")
	}
	stored, err := s.prepareSessionForStorage(userID, sess)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO auth_session (user_id, character_id, character_name, access_token, refresh_token, expires_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(user_id, character_id) DO UPDATE SET
			character_name = excluded.character_name,
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			expires_at = excluded.expires_at`,
		userID, stored.CharacterID, stored.CharacterName, stored.AccessToken, stored.RefreshToken, stored.ExpiresAt.Unix(),
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE auth_session
		   SET is_active = 1
		 WHERE user_id = ? AND character_id = ?
		   AND NOT EXISTS (SELECT 1 FROM auth_session WHERE user_id = ? AND is_active = 1)`,
		userID, stored.CharacterID, userID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// SaveAndActivate stores or updates a character session and marks it as active.
func (s *SessionStore) SaveAndActivate(sess *Session) error {
	return s.SaveAndActivateForUser(defaultUserID, sess)
}

// SaveAndActivateForUser stores or updates a character session for the given user and marks it as active.
func (s *SessionStore) SaveAndActivateForUser(userID string, sess *Session) error {
	userID = normalizeUserID(userID)
	if sess == nil {
		return fmt.Errorf("nil session")
	}
	stored, err := s.prepareSessionForStorage(userID, sess)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE auth_session SET is_active = 0 WHERE user_id = ?`, userID); err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO auth_session (user_id, character_id, character_name, access_token, refresh_token, expires_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(user_id, character_id) DO UPDATE SET
			character_name = excluded.character_name,
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			expires_at = excluded.expires_at,
			is_active = 1`,
		userID, stored.CharacterID, stored.CharacterName, stored.AccessToken, stored.RefreshToken, stored.ExpiresAt.Unix(),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Get returns the active session, or nil if none.
func (s *SessionStore) Get() *Session {
	return s.GetForUser(defaultUserID)
}

// GetForUser returns the active session for the given user, or nil if none.
func (s *SessionStore) GetForUser(userID string) *Session {
	userID = normalizeUserID(userID)
	if s.vault != nil && s.vault.TableReady() && !s.vault.IsConfiguredForUser(userID) {
		return nil
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM auth_session WHERE user_id = ?`, userID).Scan(&count); err != nil || count == 0 {
		return nil
	}

	if sess := s.querySession(`
		SELECT character_id, character_name, access_token, refresh_token, expires_at, is_active
		FROM auth_session
		WHERE user_id = ? AND is_active = 1
		LIMIT 1`, userID); sess != nil {
		return sess
	}
	// Fallback for legacy/edge states: return first session even if no active flag.
	return s.querySession(`
		SELECT character_id, character_name, access_token, refresh_token, expires_at, is_active
		FROM auth_session
		WHERE user_id = ?
		ORDER BY character_name ASC, character_id ASC
		LIMIT 1`, userID)
}

// GetByCharacterID returns a specific character session.
func (s *SessionStore) GetByCharacterID(characterID int64) *Session {
	return s.GetByCharacterIDForUser(defaultUserID, characterID)
}

// GetByCharacterIDForUser returns a specific character session for the given user.
func (s *SessionStore) GetByCharacterIDForUser(userID string, characterID int64) *Session {
	userID = normalizeUserID(userID)
	if s.vault != nil && s.vault.TableReady() && !s.vault.IsConfiguredForUser(userID) {
		return nil
	}

	return s.querySession(`
		SELECT character_id, character_name, access_token, refresh_token, expires_at, is_active
		FROM auth_session
		WHERE user_id = ? AND character_id = ?
		LIMIT 1`, userID, characterID)
}

// List returns all stored character sessions (active first).
func (s *SessionStore) List() []*Session {
	return s.ListForUser(defaultUserID)
}

// ListForUser returns all stored character sessions for the given user (active first).
func (s *SessionStore) ListForUser(userID string) []*Session {
	userID = normalizeUserID(userID)
	if s.vault != nil && s.vault.TableReady() && !s.vault.IsConfiguredForUser(userID) {
		return nil
	}

	rows, err := s.db.Query(`
		SELECT character_id, character_name, access_token, refresh_token, expires_at, is_active
		FROM auth_session
		WHERE user_id = ?
		ORDER BY is_active DESC, character_name ASC, character_id ASC`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*Session
	for rows.Next() {
		var sess Session
		var expiresUnix int64
		var activeInt int
		if err := rows.Scan(&sess.CharacterID, &sess.CharacterName, &sess.AccessToken, &sess.RefreshToken, &expiresUnix, &activeInt); err != nil {
			continue
		}
		sess.ExpiresAt = time.Unix(expiresUnix, 0)
		sess.Active = activeInt == 1
		out = append(out, &sess)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	rows.Close()
	filtered := out[:0]
	for _, sess := range out {
		if err := s.openSessionFromStorage(userID, sess); err != nil {
			log.Printf("[AUTH] Failed to open stored session for %s: %v", sess.CharacterName, err)
			continue
		}
		filtered = append(filtered, sess)
	}
	return filtered
}

func (s *SessionStore) querySession(query string, args ...interface{}) *Session {
	var sess Session
	var expiresUnix int64
	var activeInt int
	err := s.db.QueryRow(query, args...).
		Scan(&sess.CharacterID, &sess.CharacterName, &sess.AccessToken, &sess.RefreshToken, &expiresUnix, &activeInt)
	if err != nil {
		return nil
	}
	sess.ExpiresAt = time.Unix(expiresUnix, 0)
	sess.Active = activeInt == 1
	userID := defaultUserID
	if len(args) > 0 {
		if v, ok := args[0].(string); ok {
			userID = normalizeUserID(v)
		}
	}
	if err := s.openSessionFromStorage(userID, &sess); err != nil {
		log.Printf("[AUTH] Failed to open stored session for %s: %v", sess.CharacterName, err)
		return nil
	}
	return &sess
}

func (s *SessionStore) prepareSessionForStorage(userID string, sess *Session) (*Session, error) {
	if sess == nil {
		return nil, fmt.Errorf("nil session")
	}
	stored := *sess
	if s.vault == nil || !s.vault.TableReady() {
		return &stored, nil
	}
	if !s.vault.IsConfiguredForUser(userID) {
		if hostedSessionModeEnabled() {
			if err := s.vault.SetupStandardForUser(userID); err != nil {
				return nil, fmt.Errorf("setup hosted security vault: %w", err)
			}
			stored = *sess
		} else {
			return nil, fmt.Errorf("security vault not configured")
		}
	}
	prepared, err := s.prepareSessionForConfiguredVault(userID, &stored)
	if err == nil {
		return prepared, nil
	}
	if !hostedSessionModeEnabled() {
		return nil, err
	}
	log.Printf("[AUTH] Resetting hosted security vault for %s after token encryption failure: %v", userID, err)
	if resetErr := s.vault.ResetForUser(userID, false); resetErr != nil {
		return nil, fmt.Errorf("reset hosted security vault: %w", resetErr)
	}
	if setupErr := s.vault.SetupStandardForUser(userID); setupErr != nil {
		return nil, fmt.Errorf("setup hosted security vault: %w", setupErr)
	}
	stored = *sess
	prepared, err = s.prepareSessionForConfiguredVault(userID, &stored)
	if err != nil {
		return nil, err
	}
	return prepared, nil
}

func (s *SessionStore) prepareSessionForConfiguredVault(userID string, sess *Session) (*Session, error) {
	if sess == nil {
		return nil, fmt.Errorf("nil session")
	}
	if s.vault == nil || !s.vault.TableReady() {
		stored := *sess
		return &stored, nil
	}
	if !s.vault.IsConfiguredForUser(userID) {
		return nil, fmt.Errorf("security vault not configured")
	}
	stored := *sess
	accessToken, err := s.vault.PrepareTokenForStorage(userID, stored.CharacterID, "access", stored.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("encrypt access token: %w", err)
	}
	refreshToken, err := s.vault.PrepareTokenForStorage(userID, stored.CharacterID, "refresh", stored.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("encrypt refresh token: %w", err)
	}
	stored.AccessToken = accessToken
	stored.RefreshToken = refreshToken
	return &stored, nil
}

func (s *SessionStore) openSessionFromStorage(userID string, sess *Session) error {
	if sess == nil || s.vault == nil || !s.vault.TableReady() {
		return nil
	}
	accessToken, err := s.vault.OpenTokenFromStorage(userID, sess.CharacterID, "access", sess.AccessToken)
	if err != nil {
		return fmt.Errorf("decrypt access token: %w", err)
	}
	refreshToken, err := s.vault.OpenTokenFromStorage(userID, sess.CharacterID, "refresh", sess.RefreshToken)
	if err != nil {
		return fmt.Errorf("decrypt refresh token: %w", err)
	}
	sess.AccessToken = accessToken
	sess.RefreshToken = refreshToken
	return nil
}

// SetActive marks a stored character as active.
func (s *SessionStore) SetActive(characterID int64) error {
	return s.SetActiveForUser(defaultUserID, characterID)
}

// SetActiveForUser marks a stored character as active for the given user.
func (s *SessionStore) SetActiveForUser(userID string, characterID int64) error {
	userID = normalizeUserID(userID)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE auth_session SET is_active = 0 WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	_, _ = res.RowsAffected()

	res, err = tx.Exec(`UPDATE auth_session SET is_active = 1 WHERE user_id = ? AND character_id = ?`, userID, characterID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("character not found")
	}

	return tx.Commit()
}

// Delete removes all stored sessions.
func (s *SessionStore) Delete() {
	s.DeleteForUser(defaultUserID)
}

// DeleteForUser removes all stored sessions for the given user.
func (s *SessionStore) DeleteForUser(userID string) {
	userID = normalizeUserID(userID)
	s.db.Exec("DELETE FROM auth_session WHERE user_id = ?", userID)
}

// DeleteByCharacterID removes a specific character session.
func (s *SessionStore) DeleteByCharacterID(characterID int64) error {
	return s.DeleteByCharacterIDForUser(defaultUserID, characterID)
}

// DeleteByCharacterIDForUser removes a specific character session for the given user.
func (s *SessionStore) DeleteByCharacterIDForUser(userID string, characterID int64) error {
	userID = normalizeUserID(userID)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var wasActive int
	err = tx.QueryRow(`SELECT is_active FROM auth_session WHERE user_id = ? AND character_id = ?`, userID, characterID).Scan(&wasActive)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM auth_session WHERE user_id = ? AND character_id = ?`, userID, characterID); err != nil {
		return err
	}

	if wasActive == 1 {
		if _, err := tx.Exec(`
			UPDATE auth_session
			   SET is_active = 1
			 WHERE user_id = ? AND character_id = (
				SELECT character_id
				  FROM auth_session
				 WHERE user_id = ?
				 ORDER BY character_name ASC, character_id ASC
				 LIMIT 1
			 )`, userID, userID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// EnsureValidToken returns a valid access token, refreshing if needed.
func (s *SessionStore) EnsureValidToken(sso *SSOConfig) (string, error) {
	return s.EnsureValidTokenForUser(sso, defaultUserID)
}

// EnsureValidTokenForUser returns a valid access token for the active character of the given user.
func (s *SessionStore) EnsureValidTokenForUser(sso *SSOConfig, userID string) (string, error) {
	userID = normalizeUserID(userID)

	sess := s.GetForUser(userID)
	if sess == nil {
		return "", fmt.Errorf("not logged in")
	}
	return s.ensureValidTokenForSession(userID, sess, sso)
}

// EnsureValidTokenForCharacter returns a valid access token for the given character.
func (s *SessionStore) EnsureValidTokenForCharacter(sso *SSOConfig, characterID int64) (string, error) {
	return s.EnsureValidTokenForUserCharacter(sso, defaultUserID, characterID)
}

// EnsureValidTokenForUserCharacter returns a valid access token for the given user and character.
func (s *SessionStore) EnsureValidTokenForUserCharacter(sso *SSOConfig, userID string, characterID int64) (string, error) {
	userID = normalizeUserID(userID)

	sess := s.GetByCharacterIDForUser(userID, characterID)
	if sess == nil {
		return "", fmt.Errorf("character not logged in")
	}
	return s.ensureValidTokenForSession(userID, sess, sso)
}

func (s *SessionStore) ensureValidTokenForSession(userID string, sess *Session, sso *SSOConfig) (string, error) {
	if sess == nil {
		return "", fmt.Errorf("not logged in")
	}

	// If token is still valid (with 60s buffer), return it
	if time.Now().Before(sess.ExpiresAt.Add(-60 * time.Second)) {
		return sess.AccessToken, nil
	}
	if sso == nil {
		return "", fmt.Errorf("sso not configured")
	}

	// Refresh the token
	log.Printf("[AUTH] Refreshing token for %s", sess.CharacterName)
	tok, err := sso.RefreshToken(sess.RefreshToken)
	if err != nil {
		_ = s.DeleteByCharacterIDForUser(userID, sess.CharacterID)
		return "", fmt.Errorf("refresh failed: %w", err)
	}

	sess.AccessToken = tok.AccessToken
	sess.RefreshToken = tok.RefreshToken
	sess.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	if err := s.SaveForUser(userID, sess); err != nil {
		return "", fmt.Errorf("save session: %w", err)
	}

	return sess.AccessToken, nil
}
