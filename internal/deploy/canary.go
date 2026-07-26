package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// CanarySwitcher manages canary deployments with traffic splitting.
type CanarySwitcher struct {
	releaseMgr *ReleaseManager
	hookRunner *HookRunner
	logger     *slog.Logger
	mu         sync.RWMutex
	canary     *CanaryState
}

// CanaryState holds the current canary deployment state.
type CanaryState struct {
	Active     *Release   `json:"active"`
	Canary     *Release   `json:"canary"`
	Weight     int        `json:"weight"` // 0-100, percentage of traffic to canary
	StartedAt  time.Time  `json:"started_at"`
	Status     string     `json:"status"` // pending, active, promoted, rolled_back
}

// NewCanarySwitcher creates a canary deployment manager.
func NewCanarySwitcher(releaseMgr *ReleaseManager, hookRunner *HookRunner, logger *slog.Logger) *CanarySwitcher {
	return &CanarySwitcher{
		releaseMgr: releaseMgr,
		hookRunner: hookRunner,
		logger:     logger,
	}
}

// StartCanary begins a canary deployment with initial weight.
func (cs *CanarySwitcher) StartCanary(ctx context.Context, activeRelease, canaryRelease *Release, initialWeight int) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.canary != nil && cs.canary.Status == "active" {
		return fmt.Errorf("canary already active")
	}

	cs.canary = &CanaryState{
		Active:    activeRelease,
		Canary:    canaryRelease,
		Weight:    initialWeight,
		StartedAt: time.Now(),
		Status:    "active",
	}

	cs.logger.Info("canary started",
		"active", activeRelease.ID,
		"canary", canaryRelease.ID,
		"weight", initialWeight,
	)

	return nil
}

// IncreaseWeight increases the canary traffic weight.
func (cs *CanarySwitcher) IncreaseWeight(delta int) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.canary == nil || cs.canary.Status != "active" {
		return fmt.Errorf("no active canary")
	}

	cs.canary.Weight += delta
	if cs.canary.Weight > 100 {
		cs.canary.Weight = 100
	}

	cs.logger.Info("canary weight increased", "weight", cs.canary.Weight)
	return nil
}

// Promote promotes the canary to full traffic.
func (cs *CanarySwitcher) Promote(ctx context.Context) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.canary == nil || cs.canary.Status != "active" {
		return fmt.Errorf("no active canary")
	}

	cs.canary.Weight = 100
	cs.canary.Status = "promoted"

	// Activate the canary as the main release.
	if err := cs.releaseMgr.Activate(cs.canary.Canary.ID); err != nil {
		cs.canary.Status = "active"
		return fmt.Errorf("promote: %w", err)
	}

	cs.logger.Info("canary promoted", "release", cs.canary.Canary.ID)
	cs.canary = nil

	return nil
}

// Rollback stops the canary and reverts to the active release.
func (cs *CanarySwitcher) Rollback(ctx context.Context) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.canary == nil {
		return fmt.Errorf("no canary to rollback")
	}

	cs.canary.Status = "rolled_back"
	cs.logger.Info("canary rolled back")
	cs.canary = nil

	return nil
}

// CurrentWeight returns the current canary weight (0-100).
func (cs *CanarySwitcher) CurrentWeight() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.canary == nil {
		return 0
	}
	return cs.canary.Weight
}

// ShouldRouteToCanary determines if a request should go to the canary.
func (cs *CanarySwitcher) ShouldRouteToCanary() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.canary == nil || cs.canary.Status != "active" {
		return false
	}

	// Simple random routing based on weight.
	// In production, use a proper weighted random or consistent hashing.
	return randomInt(100) < cs.canary.Weight
}

// CanaryState returns the current canary state (read-only).
func (cs *CanarySwitcher) CanaryStateView() *CanaryState {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.canary == nil {
		return nil
	}

	copy := *cs.canary
	return &copy
}

func randomInt(max int) int {
	return int(time.Now().UnixNano() % int64(max))
}

// DeployCLI holds configuration for the deploy CLI commands.
type DeployCLI struct {
	Switcher    *Switcher
	Canary      *CanarySwitcher
	Logger      *slog.Logger
}

// Deploy performs a full deploy with optional canary.
func (cli *DeployCLI) Deploy(ctx context.Context, version, runtimeID, srcDir string) (*DeployResult, error) {
	cli.Logger.Info("deploying", "version", version, "runtime", runtimeID)
	return cli.Switcher.Deploy(ctx, version, runtimeID, srcDir, nil)
}

// Rollback reverts to the previous release.
func (cli *DeployCLI) Rollback(ctx context.Context) (*DeployResult, error) {
	cli.Logger.Info("rolling back")
	return cli.Switcher.Rollback(ctx)
}

// Status returns the current deployment status.
func (cli *DeployCLI) Status() *SwitcherStatus {
	return NewSwitcherStatus()
}
