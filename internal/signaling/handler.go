// Package signaling implements the WebSocket signaling endpoint used to
// exchange SDP offers/answers and trickle-ICE candidates between the server
// and WebRTC peers (Req 4.1, 4.2, 7.2).
package signaling

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/pion/webrtc/v3"

	"church-audio-streaming-backend/internal/relay"
)

// msgType enumerates the JSON "type" field values exchanged over the signaling
// WebSocket.
type msgType string

const (
	msgOffer     msgType = "offer"
	msgAnswer    msgType = "answer"
	msgCandidate msgType = "candidate"
	msgPause     msgType = "pause"
	msgResume    msgType = "resume"
	msgEnd       msgType = "end"
)

// SignalMessage is the JSON envelope for all signaling messages.
type SignalMessage struct {
	Type      msgType                  `json:"type"`
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
	Role      string                   `json:"role,omitempty"` // "broadcaster" | "listener"
}

// Handler handles the WebSocket signaling endpoint.
// Route: /api/{tenantSlug}/streams/{streamId}/signal
type Handler struct {
	hub *relay.Hub
}

// NewHandler creates a signaling Handler backed by the given relay Hub.
func NewHandler(hub *relay.Hub) *Handler {
	return &Handler{hub: hub}
}

// ServeHTTP upgrades the connection to a WebSocket and drives the signaling
// exchange for one peer.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	streamID := chi.URLParam(r, "streamId")
	if streamID == "" {
		http.Error(w, "missing streamId", http.StatusBadRequest)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: false})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	if err := h.handleSession(r.Context(), conn, streamID); err != nil {
		slog.Error("signaling session error", "streamId", streamID, "err", err)
	}
}

// handleSession drives the message loop for one signaling session.
func (h *Handler) handleSession(ctx context.Context, conn *websocket.Conn, streamID string) error {
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return nil // normal disconnect
		}

		var msg SignalMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}

		switch msg.Type {
		case msgOffer:
			if err := h.handleOffer(ctx, conn, streamID, msg); err != nil {
				return err
			}

		case msgCandidate:
			// Trickle ICE — forwarded to the appropriate PeerConnection.
			// In a full implementation we'd track pc per conn; for now we log.
			slog.Debug("signaling: trickle ICE candidate received", "streamId", streamID)

		case msgPause:
			if err := h.hub.PauseStream(streamID); err != nil {
				slog.Warn("signaling: PauseStream", "err", err)
			}

		case msgResume:
			if err := h.hub.ResumeStream(streamID); err != nil {
				slog.Warn("signaling: ResumeStream", "err", err)
			}

		case msgEnd:
			if err := h.hub.StopStream(streamID); err != nil {
				slog.Warn("signaling: StopStream", "err", err)
			}
			// Notify listeners that the stream has ended.
			h.broadcastEnd(ctx, conn)
			return nil
		}
	}
}

// handleOffer processes an SDP offer from either a broadcaster or a listener.
func (h *Handler) handleOffer(ctx context.Context, conn *websocket.Conn, streamID string, msg SignalMessage) error {
	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: msg.SDP}

	var (
		answer webrtc.SessionDescription
		err    error
	)

	switch msg.Role {
	case "broadcaster":
		answer, err = h.hub.ConnectBroadcaster(streamID, offer)
	default: // listener
		answer, err = h.hub.ConnectListener(streamID, offer)
	}

	if err != nil {
		return fmt.Errorf("handleOffer(%s): %w", msg.Role, err)
	}

	resp := SignalMessage{Type: msgAnswer, SDP: answer.SDP}
	b, _ := json.Marshal(resp)
	return conn.Write(ctx, websocket.MessageText, b)
}

// broadcastEnd sends {"type":"end"} to the given connection.
func (h *Handler) broadcastEnd(ctx context.Context, conn *websocket.Conn) {
	b, _ := json.Marshal(SignalMessage{Type: msgEnd})
	_ = conn.Write(ctx, websocket.MessageText, b)
}
