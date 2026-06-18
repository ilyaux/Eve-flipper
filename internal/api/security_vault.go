package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

type vaultSetupRequest struct {
	Mode       string `json:"mode"`
	Passphrase string `json:"passphrase"`
}

type vaultUnlockRequest struct {
	Passphrase string `json:"passphrase"`
}

type vaultResetRequest struct {
	WipePrivateData bool   `json:"wipe_private_data"`
	Confirm         string `json:"confirm"`
}

func (s *Server) handleSecurityVaultStatus(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	writeJSON(w, s.securityVaultPayload(userID))
}

func (s *Server) handleSecurityVaultSetup(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.sessions == nil || s.sessions.Vault() == nil {
		writeError(w, http.StatusServiceUnavailable, "security vault unavailable")
		return
	}
	var req vaultSetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	switch strings.ToLower(strings.TrimSpace(req.Mode)) {
	case "standard":
		if err := s.sessions.Vault().SetupStandardForUser(userID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	case "private":
		if err := s.sessions.Vault().SetupPrivateForUser(userID, req.Passphrase); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "invalid vault mode")
		return
	}
	s.bumpAuthRevision(userID)
	writeJSON(w, s.securityVaultPayload(userID))
}

func (s *Server) handleSecurityVaultUnlock(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.sessions == nil || s.sessions.Vault() == nil {
		writeError(w, http.StatusServiceUnavailable, "security vault unavailable")
		return
	}
	var req vaultUnlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.sessions.Vault().UnlockPrivateForUser(userID, req.Passphrase); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	s.bumpAuthRevision(userID)
	writeJSON(w, s.securityVaultPayload(userID))
}

func (s *Server) handleSecurityVaultLock(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.sessions != nil && s.sessions.Vault() != nil {
		s.sessions.Vault().LockForUser(userID)
	}
	s.bumpAuthRevision(userID)
	writeJSON(w, s.securityVaultPayload(userID))
}

func (s *Server) handleSecurityVaultReset(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.sessions == nil || s.sessions.Vault() == nil {
		writeError(w, http.StatusServiceUnavailable, "security vault unavailable")
		return
	}
	var req vaultResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.ToUpper(strings.TrimSpace(req.Confirm)) != "RESET" {
		writeError(w, http.StatusBadRequest, "reset confirmation required")
		return
	}
	if err := s.sessions.Vault().ResetForUser(userID, req.WipePrivateData); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bumpAuthRevision(userID)
	writeJSON(w, s.securityVaultPayload(userID))
}

func (s *Server) securityVaultPayload(userID string) map[string]interface{} {
	protectedFields := securityVaultProtectedFields()
	if s != nil && s.isHostedDeployment() {
		return map[string]interface{}{
			"available":                    false,
			"configured":                   true,
			"mode":                         "standard",
			"status":                       "managed",
			"security_migration_required":  false,
			"private_unlock_required":      false,
			"destructive_reset_available":  false,
			"standard_background_sync":     true,
			"zero_knowledge_local_storage": false,
			"field_encryption_active":      true,
			"protected_fields":             protectedFields,
		}
	}
	if s.sessions == nil || s.sessions.Vault() == nil {
		return map[string]interface{}{
			"configured":                  false,
			"security_migration_required": true,
			"available":                   false,
			"field_encryption_active":     false,
			"protected_fields":            []string{},
		}
	}
	status := s.sessions.Vault().StatusForUser(userID)
	return map[string]interface{}{
		"available":                    true,
		"configured":                   status.Configured,
		"mode":                         status.Mode,
		"status":                       status.Status,
		"schema_version":               status.SchemaVersion,
		"checkpoint_version":           status.CheckpointVersion,
		"locked":                       status.Locked,
		"legacy_plaintext_auth":        status.LegacyPlaintextAuth,
		"plaintext_purged_at":          status.PlaintextPurgedAt,
		"created_at":                   status.CreatedAt,
		"updated_at":                   status.UpdatedAt,
		"security_migration_required":  !status.Configured,
		"private_unlock_required":      status.Configured && status.Locked,
		"destructive_reset_available":  status.Configured || status.LegacyPlaintextAuth,
		"standard_background_sync":     status.Configured && status.Mode == "standard",
		"zero_knowledge_local_storage": status.Configured && status.Mode == "private",
		"field_encryption_active":      status.Configured && !status.Locked,
		"protected_fields":             protectedFields,
	}
}

func securityVaultProtectedFields() []string {
	return []string{
		"auth_session.access_token",
		"auth_session.refresh_token",
		"config.alert_telegram_token",
		"config.alert_telegram_chat_id",
		"config.alert_discord_webhook",
		"wallet_archive_sync.wallet_balance",
		"wallet_archive_sync.total_sp",
		"wallet_journal_archive.reason",
		"wallet_journal_archive.description",
		"wallet_journal_archive.context_id_type",
		"paper_trades.notes",
		"paper_trades.source",
		"industry_projects.notes",
		"industry_jobs.notes",
		"cockpit_preferences.payload_json",
		"cockpit_loadouts.payload_json",
	}
}
