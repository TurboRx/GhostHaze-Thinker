package showdown

import (
	"strconv"
	"strings"
	"time"
)

func ParseRawStream(raw string, currentRoom string) ([]RawMessage, string) {
	lines := strings.Split(raw, "\n")
	messages := make([]RawMessage, 0, len(lines))
	room := currentRoom

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if strings.HasPrefix(trimmed, ">") {
			room = strings.TrimSpace(trimmed[1:])
			continue
		}
		if !strings.HasPrefix(trimmed, "|") || len(trimmed) <= 1 {
			continue
		}

		// strip leading '|' and split remaining tokens
		parts := strings.Split(trimmed[1:], "|")
		if len(parts) == 0 {
			continue
		}

		msgType := parts[0]
		paramSlice := parts[1:]

		messages = append(messages, RawMessage{
			Room:  room,
			Type:  msgType,
			Parts: paramSlice,
			Raw:   trimmed,
		})
	}

	return messages, room
}

func ParseChatMessage(msg RawMessage) (ChatMessage, bool) {
	switch msg.Type {
	case "c", "chat":
		if len(msg.Parts) < 2 {
			return ChatMessage{}, false
		}
		user := strings.TrimSpace(msg.Parts[0])
		text := strings.Join(msg.Parts[1:], "|")
		return ChatMessage{
			Room:      msg.Room,
			User:      user,
			Text:      text,
			Timestamp: time.Now().UTC(),
			Away:      IsAway(user),
			Raw:       msg.Raw,
		}, true

	case "c:":
		if len(msg.Parts) < 3 {
			return ChatMessage{}, false
		}
		tsInt, err := strconv.ParseInt(strings.TrimSpace(msg.Parts[0]), 10, 64)
		ts := time.Now().UTC()
		if err == nil && tsInt > 0 {
			// support both second and millisecond timestamps
			if tsInt > 1e11 {
				ts = time.UnixMilli(tsInt).UTC()
			} else {
				ts = time.Unix(tsInt, 0).UTC()
			}
		}
		user := strings.TrimSpace(msg.Parts[1])
		text := strings.Join(msg.Parts[2:], "|")
		return ChatMessage{
			Room:      msg.Room,
			User:      user,
			Text:      text,
			Timestamp: ts,
			Away:      IsAway(user),
			Raw:       msg.Raw,
		}, true

	default:
		return ChatMessage{}, false
	}
}

func ParsePrivateMessage(msg RawMessage) (PrivateMessage, bool) {
	if msg.Type != "pm" || len(msg.Parts) < 3 {
		return PrivateMessage{}, false
	}

	from := strings.TrimSpace(msg.Parts[0])
	to := strings.TrimSpace(msg.Parts[1])
	text := strings.Join(msg.Parts[2:], "|")

	isHidden := false
	if strings.HasPrefix(text, "/botmsg ") {
		text = strings.TrimPrefix(text, "/botmsg ")
		isHidden = true
	}

	return PrivateMessage{
		From:     from,
		To:       to,
		Text:     text,
		IsHidden: isHidden,
		Away:     IsAway(from),
		Raw:      msg.Raw,
	}, true
}

func ParseUserUpdate(msg RawMessage) (UserUpdate, bool) {
	if msg.Type != "updateuser" || len(msg.Parts) < 2 {
		return UserUpdate{}, false
	}

	rawName := strings.TrimSpace(msg.Parts[0])
	away := IsAway(rawName)
	username := CleanUsername(rawName)
	isGuest := msg.Parts[1] == "0"
	avatar := ""
	if len(msg.Parts) > 2 {
		avatar = strings.TrimSpace(msg.Parts[2])
	}

	return UserUpdate{
		Username: username,
		IsGuest:  isGuest,
		Avatar:   avatar,
		Away:     away,
	}, true
}

func ParseChallstr(msg RawMessage) (string, bool) {
	if msg.Type != "challstr" || len(msg.Parts) == 0 {
		return "", false
	}
	return strings.Join(msg.Parts, "|"), true
}

func ParseFormats(msg RawMessage) []Format {
	if msg.Type != "formats" {
		return nil
	}
	var formats []Format
	currentSection := ""
	for _, part := range msg.Parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || trimmed == ",LL" {
			continue
		}
		if strings.HasPrefix(trimmed, ",") {
			currentSection = strings.TrimPrefix(trimmed, ",")
			continue
		}
		name := trimmed
		commaIdx := strings.LastIndex(name, ",")
		if commaIdx >= 0 {
			name = strings.TrimSpace(name[:commaIdx])
		}
		id := ToID(name)
		if id != "" {
			formats = append(formats, Format{
				ID:      id,
				Name:    name,
				Section: currentSection,
			})
		}
	}
	return formats
}
