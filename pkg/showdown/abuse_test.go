package showdown

import (
	"testing"
	"time"
)

func TestAbuseMonitor(t *testing.T) {
	// 2 commands per 200ms, mute for 500ms
	monitor := NewAbuseMonitor(2, 200*time.Millisecond, 500*time.Millisecond)

	// first 2 commands should pass
	if monitor.IsAbusing("player1") {
		t.Errorf("expected command 1 to pass")
	}
	if monitor.IsAbusing("player1") {
		t.Errorf("expected command 2 to pass")
	}

	// third command within window should trigger abuse lock
	if !monitor.IsAbusing("player1") {
		t.Errorf("expected command 3 to be flagged as abuse")
	}

	// should remain locked
	if !monitor.IsAbusing("player1") {
		t.Errorf("expected user to remain locked")
	}

	// manual reset should clear lock
	monitor.Reset("player1")
	if monitor.IsAbusing("player1") {
		t.Errorf("expected command to pass after reset")
	}
}
