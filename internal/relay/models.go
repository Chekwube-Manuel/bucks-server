package relay

import (
	"sync"

	"github.com/pion/webrtc/v3"
)

// StreamStatus represents the lifecycle state of a Stream.
type StreamStatus string

const (
	StatusPending StreamStatus = "pending"
	StatusLive    StreamStatus = "live"
	StatusPaused  StreamStatus = "paused"
	StatusEnded   StreamStatus = "ended"
)

// Stream holds the in-memory state for a single live audio session.
type Stream struct {
	ID       string
	TenantID string
	Status   StreamStatus

	// broadcaster is the Pion PeerConnection for the broadcaster side.
	broadcaster *webrtc.PeerConnection

	// broadcastTrack is the incoming audio track from the broadcaster.
	broadcastTrack *webrtc.TrackRemote

	// listenerPeers holds all active listener PeerConnections.
	listenerPeers []*listenerPeer

	// forwardDone signals the RTP-forwarding goroutine to stop.
	forwardDone chan struct{}

	// paused indicates whether forwarding is currently suspended.
	paused bool

	mu sync.RWMutex
}

// listenerPeer pairs a Pion PeerConnection with its local audio track.
type listenerPeer struct {
	pc    *webrtc.PeerConnection
	track *webrtc.TrackLocalStaticRTP
}
