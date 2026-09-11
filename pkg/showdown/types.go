package showdown

import "time"

type ChatMessage struct {
	Room      string
	User      string
	Text      string
	Timestamp time.Time
	Raw       string
}

type PrivateMessage struct {
	From string
	To   string
	Text string
	Raw  string
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
