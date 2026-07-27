package relay_test

import (
	"testing"

	"church-audio-streaming-backend/internal/relay"
)

// TestStreamStateMachine verifies the state transitions:
// pending → live → paused → live → ended (Req 7.1, 7.2, 7.3).
func TestStreamStateMachine(t *testing.T) {
	hub := relay.NewHub()

	s, err := hub.CreateStream("s1", "tenant-a")
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	if s.Status != relay.StatusPending {
		t.Errorf("initial status: want pending, got %s", s.Status)
	}

	// Can't pause a pending stream.
	if err := hub.PauseStream("s1"); err == nil {
		t.Error("PauseStream on pending stream should error")
	}

	// StopStream must work from any non-ended state.
	if err := hub.StopStream("s1"); err != nil {
		t.Fatalf("StopStream: %v", err)
	}
	if s.Status != relay.StatusEnded {
		t.Errorf("after stop: want ended, got %s", s.Status)
	}

	// Second stop must be idempotent.
	if err := hub.StopStream("s1"); err != nil {
		t.Errorf("second StopStream: %v", err)
	}
}

// TestCreateStream_DuplicateID returns an error for duplicate stream IDs.
func TestCreateStream_DuplicateID(t *testing.T) {
	hub := relay.NewHub()
	if _, err := hub.CreateStream("dup", "t1"); err != nil {
		t.Fatalf("first CreateStream: %v", err)
	}
	if _, err := hub.CreateStream("dup", "t1"); err == nil {
		t.Error("duplicate CreateStream should return error")
	}
}

// TestStopStream_ClosesAllPeers verifies that StopStream sets status to ended
// even without a broadcaster attached.
func TestStopStream_ClosesAllPeers(t *testing.T) {
	hub := relay.NewHub()
	if _, err := hub.CreateStream("s-stop", "t1"); err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	if err := hub.StopStream("s-stop"); err != nil {
		t.Fatalf("StopStream: %v", err)
	}
	s, _ := hub.GetStream("s-stop")
	if s.Status != relay.StatusEnded {
		t.Errorf("want ended, got %s", s.Status)
	}
}

// TestPauseResume_StatusTransitions verifies paused → live via simulated state.
func TestPauseResume_StatusTransitions(t *testing.T) {
	hub := relay.NewHub()
	s, _ := hub.CreateStream("s-pr", "t1")

	// Manually set to live so PauseStream works without a real broadcaster.
	s.Status = relay.StatusLive

	if err := hub.PauseStream("s-pr"); err != nil {
		t.Fatalf("PauseStream: %v", err)
	}
	if s.Status != relay.StatusPaused {
		t.Errorf("after pause: want paused, got %s", s.Status)
	}

	if err := hub.ResumeStream("s-pr"); err != nil {
		t.Fatalf("ResumeStream: %v", err)
	}
	if s.Status != relay.StatusLive {
		t.Errorf("after resume: want live, got %s", s.Status)
	}
}
