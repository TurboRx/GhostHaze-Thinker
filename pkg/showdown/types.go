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
	Raw       string
}

func (m ChatMessage) CleanUser() string {
	return CleanUsername(m.User)
}

func (m ChatMessage) Rank() string {
	return UserRank(m.User)
}

type PrivateMessage struct {
	From string
	To   string
	Text string
	Raw  string
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
	for len(trimmed) > 0 && !isAlphanumeric(rune(trimmed[0])) {
		trimmed = strings.TrimSpace(trimmed[1:])
	}
	return trimmed
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

func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
