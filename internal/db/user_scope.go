package db

import "strings"

const DefaultUserID = "default"

func normalizeUserID(userID string) string {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return DefaultUserID
	}
	return trimmed
}
