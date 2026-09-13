package showdown

import (
	"strings"
	"time"
)

type ChatMessage struct {
	Room      string
	User      string
	Text      string
	Timestamp time.Time
	IsIntro   bool
	Away      bool
	Raw       string
}

func (m ChatMessage) CleanUser() string {
	return CleanUsername(m.User)
}

func (m ChatMessage) Rank() string {
	return UserRank(m.User)
}

type PrivateMessage struct {
	From     string
	To       string
	Text     string
	IsHidden bool
	Away     bool
	Raw      string
}

func (m PrivateMessage) CleanFrom() string {
	return CleanUsername(m.From)
}

func (m PrivateMessage) Rank() string {
	return UserRank(m.From)
}

type UserUpdate struct {
	Username string
	IsGuest  bool
	Avatar   string
	Away     bool
}

type Format struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Section string `json:"section"`
}

type RawMessage struct {
	Room  string
	Type  string
	Parts []string
	Raw   string
}

type CommandHandler func(room, user, args string)
type MessageHandler func(msg RawMessage)
type ChatHandler func(msg ChatMessage)
type PMHandler func(msg PrivateMessage)

func UserRank(name string) string {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) > 0 && !isAlphanumeric(rune(trimmed[0])) {
		return string(trimmed[0])
	}
	return ""
}

func CleanUsername(name string) string {
	trimmed := strings.TrimSpace(name)
	trimmed = strings.TrimSuffix(trimmed, "@!")
	for len(trimmed) > 0 && !isAlphanumeric(rune(trimmed[0])) {
		trimmed = strings.TrimSpace(trimmed[1:])
	}
	trimmed = strings.TrimSuffix(trimmed, "@!")
	return trimmed
}

func IsAway(name string) bool {
	return strings.HasSuffix(strings.TrimSpace(name), "@!")
}

func EscapeChat(message string) string {
	trimmed := strings.TrimLeft(message, " \t")
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "!") {
		return " " + message
	}
	return message
}

func ToID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ToRoomID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
