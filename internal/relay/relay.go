package relay

import (
	"fmt"
	"sync"

	"github.com/pion/webrtc/v3"
)

// Hub manages all active streams for the relay SFU.
type Hub struct {
	mu      sync.RWMutex
	streams map[string]*Stream // streamID -> Stream
}

// NewHub creates a ready-to-use Hub.
func NewHub() *Hub {
	return &Hub{streams: make(map[string]*Stream)}
}

// CreateStream initialises an in-memory Stream with status "pending" (Req 4.1,
// 7.1). The stream is keyed by streamID and associated with tenantID.
func (h *Hub) CreateStream(streamID, tenantID string) (*Stream, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.streams[streamID]; exists {
		return nil, fmt.Errorf("CreateStream: stream %q already exists", streamID)
	}
	s := &Stream{
		ID:          streamID,
		TenantID:    tenantID,
		Status:      StatusPending,
		forwardDone: make(chan struct{}),
	}
	h.streams[streamID] = s
	return s, nil
}

// ConnectBroadcaster applies a WebRTC SDP offer from the broadcaster, registers
// the incoming audio TrackRemote, starts the RTP-forwarding goroutine, and
// returns the SDP answer (Req 4.1, 4.2, 7.1).
func (h *Hub) ConnectBroadcaster(streamID string, offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	s, err := h.getStream(streamID)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == StatusEnded {
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectBroadcaster: stream %q has ended", streamID)
	}

	api := webrtc.NewAPI()
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectBroadcaster: new PC: %w", err)
	}

	// Receive one audio track from the broadcaster.
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
	); err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectBroadcaster: add transceiver: %w", err)
	}

	// Register the incoming track once it arrives.
	trackCh := make(chan *webrtc.TrackRemote, 1)
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		select {
		case trackCh <- track:
		default:
		}
	})

	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectBroadcaster: set remote desc: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectBroadcaster: create answer: %w", err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectBroadcaster: set local desc: %w", err)
	}

	s.broadcaster = pc
	s.Status = StatusLive

	// Start the RTP-forwarding goroutine — waits for the track then fans out.
	go s.forwardLoop(trackCh)

	return pc.LocalDescription(), nil
}

// ConnectListener creates a Pion PeerConnection for a new listener, adds a
// TrackLocalStaticRTP subscribed to the broadcaster's codec, and returns the
// SDP answer (Req 3.4, 4.1).
func (h *Hub) ConnectListener(streamID string, offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	s, err := h.getStream(streamID)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == StatusEnded {
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectListener: stream %q has ended", streamID)
	}

	api := webrtc.NewAPI()
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectListener: new PC: %w", err)
	}

	// Create a local static RTP track. Use Opus/48000 stereo by default.
	localTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"audio", streamID,
	)
	if err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectListener: new local track: %w", err)
	}

	if _, err := pc.AddTrack(localTrack); err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectListener: add track: %w", err)
	}

	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectListener: set remote desc: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectListener: create answer: %w", err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("ConnectListener: set local desc: %w", err)
	}

	s.listenerPeers = append(s.listenerPeers, &listenerPeer{pc: pc, track: localTrack})

	return pc.LocalDescription(), nil
}

// PauseStream suspends RTP forwarding and injects silence into all listener
// tracks (Req 7.3). Status transitions: live → paused.
func (h *Hub) PauseStream(streamID string) error {
	s, err := h.getStream(streamID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != StatusLive {
		return fmt.Errorf("PauseStream: stream %q is not live (status=%s)", streamID, s.Status)
	}
	s.paused = true
	s.Status = StatusPaused
	return nil
}

// ResumeStream re-enables RTP forwarding (Req 7.3). Status: paused → live.
func (h *Hub) ResumeStream(streamID string) error {
	s, err := h.getStream(streamID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != StatusPaused {
		return fmt.Errorf("ResumeStream: stream %q is not paused (status=%s)", streamID, s.Status)
	}
	s.paused = false
	s.Status = StatusLive
	return nil
}

// StopStream closes all PeerConnections and marks the stream as ended (Req 7.2).
func (h *Hub) StopStream(streamID string) error {
	s, err := h.getStream(streamID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == StatusEnded {
		return nil // idempotent
	}

	// Signal the forwarding goroutine to stop.
	select {
	case <-s.forwardDone:
	default:
		close(s.forwardDone)
	}

	// Close all listener PeerConnections.
	for _, lp := range s.listenerPeers {
		_ = lp.pc.Close()
	}
	s.listenerPeers = nil

	// Close broadcaster PeerConnection.
	if s.broadcaster != nil {
		_ = s.broadcaster.Close()
		s.broadcaster = nil
	}

	s.Status = StatusEnded
	return nil
}

// GetStream returns the stream by ID, for use by the signaling handler.
func (h *Hub) GetStream(streamID string) (*Stream, error) {
	return h.getStream(streamID)
}

// getStream is the internal helper that reads from the hub map.
func (h *Hub) getStream(streamID string) (*Stream, error) {
	h.mu.RLock()
	s, ok := h.streams[streamID]
	h.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("stream %q not found", streamID)
	}
	return s, nil
}

// forwardLoop reads RTP packets from the broadcaster's TrackRemote and writes
// them to every listener's TrackLocalStaticRTP. It removes disconnected peers
// on write error (Req 4.1, 3.4).
func (s *Stream) forwardLoop(trackCh <-chan *webrtc.TrackRemote) {
	// Wait for the broadcaster track to arrive.
	var track *webrtc.TrackRemote
	select {
	case track = <-trackCh:
	case <-s.forwardDone:
		return
	}

	s.mu.Lock()
	s.broadcastTrack = track
	s.mu.Unlock()

	buf := make([]byte, 1500)
	for {
		select {
		case <-s.forwardDone:
			return
		default:
		}

		n, _, readErr := track.Read(buf)
		if readErr != nil {
			return
		}

		s.mu.RLock()
		paused := s.paused
		peers := make([]*listenerPeer, len(s.listenerPeers))
		copy(peers, s.listenerPeers)
		s.mu.RUnlock()

		if paused {
			// Inject silence (empty Opus packet) — 8 zero bytes is a valid
			// comfort-noise Opus frame recognised by most decoders.
			silence := []byte{0xf8, 0xff, 0xfe, 0x00, 0x00, 0x00, 0x00, 0x00}
			s.writeToListeners(peers, silence)
			continue
		}

		s.writeToListeners(peers, buf[:n])
	}
}

// writeToListeners fans the packet out to all peers, removing those that error.
func (s *Stream) writeToListeners(peers []*listenerPeer, pkt []byte) {
	var dead []*listenerPeer
	for _, lp := range peers {
		if _, err := lp.track.Write(pkt); err != nil {
			dead = append(dead, lp)
		}
	}
	if len(dead) == 0 {
		return
	}
	// Remove dead peers from the stream.
	s.mu.Lock()
	defer s.mu.Unlock()
	remaining := s.listenerPeers[:0]
	for _, lp := range s.listenerPeers {
		isDead := false
		for _, d := range dead {
			if lp == d {
				isDead = true
				break
			}
		}
		if !isDead {
			remaining = append(remaining, lp)
		}
	}
	s.listenerPeers = remaining
}
