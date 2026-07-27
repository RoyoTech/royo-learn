package claudecode

import "strconv"

func cursorCheckpoint(cursor map[string]any) (string, string, bool) {
	if cursor == nil {
		return "", "", false
	}
	sessionID, _ := cursor["last_session_id"].(string)
	if sessionID == "" {
		return "", "", false
	}
	var turnUUID string
	switch value := cursor["last_turn_uuid"].(type) {
	case string:
		turnUUID = value
	case int:
		turnUUID = strconv.Itoa(value)
	case int64:
		turnUUID = strconv.FormatInt(value, 10)
	case float64:
		turnUUID = strconv.FormatFloat(value, 'f', -1, 64)
	}
	if turnUUID == "" {
		return "", "", false
	}
	return sessionID, turnUUID, true
}

func cursorAtOrBefore(sessionID, turnUUID, cursorSession, cursorTurn string) bool {
	return sessionID < cursorSession || sessionID == cursorSession && turnUUID <= cursorTurn
}
