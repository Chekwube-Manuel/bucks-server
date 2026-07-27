// Package metrics registers a Prometheus registry with per-tenant gauges and
// counters (Req 7.6, 5.6, 10.1â€“10.3, 10.5).
package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"church-audio-streaming-backend/internal/db"
)

// Collector holds all Prometheus instruments and the DB pool for writing
// stream_records and bitrate_events rows.
type Collector struct {
	reg             *prometheus.Registry
	pool            *pgxpool.Pool
	activeStreams    *prometheus.GaugeVec
	activeListeners  *prometheus.GaugeVec
	bytesRelayed    *prometheus.CounterVec
	bitrateChanges  *prometheus.CounterVec
}

// New creates a Collector, registers all instruments on a fresh Prometheus
// registry, and returns it. Pass pool=nil in tests.
func New(pool *pgxpool.Pool) *Collector {
	reg := prometheus.NewRegistry()

	activeStreams := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "church_active_streams",
		Help: "Number of currently live streams per tenant.",
	}, []string{"tenant_id"})

	activeListeners := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "church_active_listeners",
		Help: "Number of currently connected listeners per tenant.",
	}, []string{"tenant_id"})

	bytesRelayed := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "church_bytes_relayed_total",
		Help: "Total bytes relayed per tenant.",
	}, []string{"tenant_id"})

	bitrateChanges := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "church_bitrate_changes_total",
		Help: "Total bitrate change events per tenant.",
	}, []string{"tenant_id"})

	reg.MustRegister(activeStreams, activeListeners, bytesRelayed, bitrateChanges)

	return &Collector{
		reg:             reg,
		pool:            pool,
		activeStreams:   activeStreams,
		activeListeners: activeListeners,
		bytesRelayed:    bytesRelayed,
		bitrateChanges:  bitrateChanges,
	}
}

// Handler returns an http.Handler exposing /metrics for Prometheus scraping.
func (c *Collector) Handler() http.Handler {
	return promhttp.HandlerFor(c.reg, promhttp.HandlerOpts{})
}

// StreamStarted increments active_streams for the tenant.
func (c *Collector) StreamStarted(tenantID string) {
	c.activeStreams.WithLabelValues(tenantID).Inc()
}

// StreamEnded decrements active_streams for the tenant.
func (c *Collector) StreamEnded(tenantID string) {
	c.activeStreams.WithLabelValues(tenantID).Dec()
}

// ListenerJoined increments active_listeners and emits a capacity warning at
// 90 % of maxListeners (Req 10.5).
func (c *Collector) ListenerJoined(tenantID string, currentCount, maxListeners int) {
	c.activeListeners.WithLabelValues(tenantID).Inc()

	threshold := int(float64(maxListeners) * 0.9)
	if currentCount+1 >= threshold {
		slog.Warn("capacity warning: listeners approaching max",
			"tenant_id", tenantID,
			"current", currentCount+1,
			"max", maxListeners,
		)
	}
}

// ListenerLeft decrements active_listeners for the tenant.
func (c *Collector) ListenerLeft(tenantID string) {
	c.activeListeners.WithLabelValues(tenantID).Dec()
}

// AddBytesRelayed increments the bytes_relayed_total counter.
func (c *Collector) AddBytesRelayed(tenantID string, n float64) {
	c.bytesRelayed.WithLabelValues(tenantID).Add(n)
}

// RecordBitrateEvent inserts a bitrate_events row and increments the counter
// (Req 5.6).
func (c *Collector) RecordBitrateEvent(ctx context.Context, streamID, tenantID string, prevKbps, newKbps, detectedBwKbps int) {
	c.bitrateChanges.WithLabelValues(tenantID).Inc()

	if c.pool == nil {
		return
	}
	if err := db.InsertBitrateEvent(ctx, c.pool, db.BitrateEvent{
		StreamID:        streamID,
		TenantID:        tenantID,
		PrevBitrate: prevKbps,
		NewBitrate:  newKbps,
		DetectedBW:  detectedBwKbps,
		OccurredAt:      time.Now().UTC(),
	}); err != nil {
		slog.Error("metrics: insert bitrate event", "err", err)
	}
}

// FinaliseStream updates the stream_records row with end time and stats (Req 7.6).
func (c *Collector) FinaliseStream(ctx context.Context, s db.StreamRecord) error {
	if c.pool == nil {
		return nil
	}
	if err := db.UpdateStreamRecord(ctx, c.pool, s); err != nil {
		return fmt.Errorf("FinaliseStream: %w", err)
	}
	return nil
}
