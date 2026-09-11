package showdown

import "time"

// ChatMessage represents a room chat message received from Pokemon Showdown.
type ChatMessage struct {
	Room      string
	User      string
	Text      string
	Timestamp time.Time
	Raw       string
}

// PrivateMessage represents a direct/private message (PM).
type PrivateMessage struct {
	From string
	To   string
	Text string
	Raw  string
}

// UserUpdate represents a user status update event (|updateuser|).
type UserUpdate struct {
	Username string
	IsGuest  bool
	Avatar   string
}

// RoomEvent represents room lifecycle events (join, leave).
type RoomEvent struct {
	Room     string
	RoomType string
}

// RawMessage represents an unparsed line from the server stream.
type RawMessage struct {
	Room  string
	Type  string
	Parts []string
	Raw   string
}

// CommandHandler represents a function handling a bot command (e.g. .ping).
type CommandHandler func(room, user, args string)

// MessageHandler is a callback function for raw messages.
type MessageHandler func(msg RawMessage)

// ChatHandler is a callback function for chat messages.
type ChatHandler func(msg ChatMessage)

// PMHandler is a callback function for private messages.
type PMHandler func(msg PrivateMessage)
