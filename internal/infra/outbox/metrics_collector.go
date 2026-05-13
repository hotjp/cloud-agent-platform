// Package outbox provides outbox metrics collection.
package outbox

import (
	"context"
	"sync"
	"time"

	"github.com/cloud-agent-platform/cap/internal/observability/metrics"
	"go.uber.org/zap"
)

// MetricsCollector collects and records outbox metrics periodically.
type MetricsCollector struct {
	poller  *OutboxPoller
	metrics *metrics.Recorder
	logger  *zap.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewMetricsCollector creates a new outbox metrics collector.
func NewMetricsCollector(poller *OutboxPoller, m *metrics.Recorder, logger *zap.Logger) *MetricsCollector {
	if poller == nil {
		panic("poller is required")
	}
	if m == nil {
		panic("metrics recorder is required")
	}
	if logger == nil {
		panic("logger is required")
	}

	return &MetricsCollector{
		poller:  poller,
		metrics: m,
		logger:  logger,
	}
}

// Start begins collecting metrics in a background goroutine.
func (c *MetricsCollector) Start(ctx context.Context) {
	c.ctx, c.cancel = context.WithCancel(ctx)

	c.wg.Add(1)
	go c.run()

	c.logger.Info("outbox metrics collector started")
}

// Stop stops the metrics collector.
func (c *MetricsCollector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	c.logger.Info("outbox metrics collector stopped")
}

// run is the main collection loop.
func (c *MetricsCollector) run() {
	defer c.wg.Done()

	// Collect immediately on start
	c.collect()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// collect performs a single metrics collection.
func (c *MetricsCollector) collect() {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	count, err := c.poller.QueryPendingCount(ctx)
	if err != nil {
		c.logger.Warn("failed to query pending outbox events count",
			zap.Error(err),
		)
		return
	}

	c.metrics.RecordOutboxPending(int64(count))

	c.logger.Debug("outbox pending events recorded",
		zap.Int64("count", int64(count)),
	)
}