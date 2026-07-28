package codex

func cursorCheckpoint(cursor map[string]any) (string, string, bool) {
	if cursor == nil {
		return "", "", false
	}
	sessionID, _ := cursor["last_session_id"].(string)
	if sessionID == "" {
		return "", "", false
	}
	turnUUID, _ := cursor["last_turn_uuid"].(string)
	if turnUUID == "" {
		return "", "", false
	}
	return sessionID, turnUUID, true
}

func cursorAtOrBefore(sessionID, turnUUID, cursorSession, cursorTurn string) bool {
	return sessionID < cursorSession ||
		sessionID == cursorSession && turnUUID <= cursorTurn
}
