// Package guardian implements the Guardian approval mechanism for high-risk operations.
// It manages the confirming state, timeout mechanism, and approval workflow.
package guardian

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TimeoutHandler handles approval timeouts using a ticker-based mechanism.
type TimeoutHandler struct {
	guardian    *Guardian
	logger      *zap.Logger
	checkInterval time.Duration
	stopCh       chan struct{}
	wg          sync.WaitGroup
}

// NewTimeoutHandler creates a new TimeoutHandler.
func NewTimeoutHandler(guardian *Guardian, logger *zap.Logger, checkInterval time.Duration) *TimeoutHandler {
	if checkInterval <= 0 {
		checkInterval = 10 * time.Second
	}
	return &TimeoutHandler{
		guardian:     guardian,
		logger:       logger,
		checkInterval: checkInterval,
		stopCh:       make(chan struct{}),
	}
}

// Start starts the timeout handler loop.
func (h *TimeoutHandler) Start(ctx context.Context) {
	h.wg.Add(1)
	go h.run(ctx)
}

// Stop stops the timeout handler gracefully.
func (h *TimeoutHandler) Stop() {
	close(h.stopCh)
	h.wg.Wait()
	h.logger.Info("timeout handler stopped")
}

// run is the main loop that checks for expired approvals.
func (h *TimeoutHandler) run(ctx context.Context) {
	defer h.wg.Done()

	ticker := time.NewTicker(h.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.checkExpiredApprovals(ctx)
		}
	}
}

// checkExpiredApprovals checks all pending approvals and handles expired ones.
func (h *TimeoutHandler) checkExpiredApprovals(ctx context.Context) {
	h.guardian.mu.RLock()
	now := time.Now().UTC()
	taskIDs := make([]string, 0, len(h.guardian.pendingReq))
	for taskID, req := range h.guardian.pendingReq {
		if now.After(req.ExpiresAt) || now.Equal(req.ExpiresAt) {
			taskIDs = append(taskIDs, taskID)
		}
	}
	h.guardian.mu.RUnlock()

	for _, taskID := range taskIDs {
		h.logger.Info("handling approval timeout",
			zap.String("layer", "L4"),
			zap.String("task_id", taskID),
		)

		if _, err := h.guardian.HandleTimeout(ctx, taskID); err != nil {
			h.logger.Error("failed to handle approval timeout",
				zap.String("layer", "L4"),
				zap.String("task_id", taskID),
				zap.Error(err),
			)
		}
	}
}