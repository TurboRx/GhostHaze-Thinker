package showdown

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockTimerSender struct {
	sent []struct {
		room string
		text string
	}
}

func (m *mockTimerSender) SendRoomMessage(room, text string) error {
	m.sent = append(m.sent, struct {
		room string
		text string
	}{room: room, text: text})
	return nil
}

func TestTimerStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "timer_test")
	if err != nil {
		t.Fatalf("temp dir error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "timers.json")
	store := NewTimerStore(filePath)

	timer := ChatroomTimer{
		ID:              "timer_1",
		Name:            "Rules Reminder",
		Room:            "lobby",
		IntervalMinutes: 10,
		Message:         "Welcome to the lobby! Please be respectful.",
		Enabled:         true,
	}

	if err := store.Save(timer); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	list := store.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 timer, got %d", len(list))
	}
	if list[0].Name != "Rules Reminder" {
		t.Errorf("expected 'Rules Reminder', got '%s'", list[0].Name)
	}

	// test toggle
	enabled, err := store.Toggle("timer_1")
	if err != nil {
		t.Fatalf("toggle failed: %v", err)
	}
	if enabled {
		t.Errorf("expected timer to be disabled")
	}

	// toggle back
	enabled, _ = store.Toggle("timer_1")
	if !enabled {
		t.Errorf("expected timer to be re-enabled")
	}

	// test checkandrun
	sender := &mockTimerSender{}
	now := time.Now()
	executed := store.CheckAndRun(sender, now)
	if executed != 1 {
		t.Errorf("expected 1 timer executed, got %d", executed)
	}
	if len(sender.sent) != 1 || sender.sent[0].room != "lobby" {
		t.Errorf("expected message dispatched to lobby, got %+v", sender.sent)
	}

	// test execution before interval passes
	executed2 := store.CheckAndRun(sender, now.Add(5*time.Minute))
	if executed2 != 0 {
		t.Errorf("expected 0 executed before interval, got %d", executed2)
	}

	// test execution after interval passes
	executed3 := store.CheckAndRun(sender, now.Add(11*time.Minute))
	if executed3 != 1 {
		t.Errorf("expected 1 executed after interval, got %d", executed3)
	}

	// test triggernow
	if err := store.TriggerNow("timer_1", sender); err != nil {
		t.Errorf("trigger now failed: %v", err)
	}

	// test delete
	if !store.Delete("timer_1") {
		t.Errorf("expected delete to succeed")
	}
	if len(store.List()) != 0 {
		t.Errorf("expected 0 timers after delete")
	}
}
