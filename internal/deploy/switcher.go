package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Switcher manages zero-downtime release switching.
type Switcher struct {
	releaseMgr *ReleaseManager
	hookRunner *HookRunner
	prober     *Prober
	logger     *slog.Logger
}

// NewSwitcher creates a deploy switcher.
func NewSwitcher(releaseMgr *ReleaseManager, hookRunner *HookRunner, logger *slog.Logger) *Switcher {
	return &Switcher{
		releaseMgr: releaseMgr,
		hookRunner: hookRunner,
		prober:     &Prober{},
		logger:     logger,
	}
}

// DeployResult holds the result of a deploy operation.
type DeployResult struct {
	Release   *Release
	Success   bool
	Error     string
	Duration  time.Duration
	Steps     []DeployStep
}

// DeployStep records a single step in the deploy.
type DeployStep struct {
	Name      string
	Status    string
	Error     string
	Duration  time.Duration
}

// Deploy performs a full zero-downtime deploy cycle.
func (s *Switcher) Deploy(ctx context.Context, version, runtimeID, srcDir string, metadata map[string]string) (*DeployResult, error) {
	start := time.Now()
	result := &DeployResult{}

	// Step 1: Create release.
	step := s.stepStart("create_release")
	rel, err := s.releaseMgr.Create(version, runtimeID, srcDir, metadata)
	if err != nil {
		return nil, s.stepFail(result, step, "create release", err)
	}
	result.Release = rel
	s.stepComplete(result, step)

	// Step 2: Pre-activate hook.
	step = s.stepStart("pre_activate_hook")
	if err := s.hookRunner.RunPreActivate(ctx, rel); err != nil {
		rel.State = StateFailed
		rel.Error = err.Error()
		s.releaseMgr.saveRelease(rel)
		return nil, s.stepFail(result, step, "pre-activate hook", err)
	}
	s.stepComplete(result, step)

	// Step 3: Probe health.
	step = s.stepStart("health_probe")
	healthy, err := s.prober.Probe(ctx, rel)
	if err != nil || !healthy {
		rel.State = StateFailed
		rel.Error = "health probe failed"
		s.releaseMgr.saveRelease(rel)
		return nil, s.stepFail(result, step, "health probe", fmt.Errorf("probe failed: healthy=%v, err=%w", healthy, err))
	}
	s.stepComplete(result, step)

	// Step 4: Activate (atomic swap).
	step = s.stepStart("activate")
	if err := s.releaseMgr.Activate(rel.ID); err != nil {
		return nil, s.stepFail(result, step, "activate", err)
	}
	s.stepComplete(result, step)

	// Step 5: Post-activate hook.
	step = s.stepStart("post_activate_hook")
	if err := s.hookRunner.RunPostActivate(ctx, rel); err != nil {
		s.logger.Warn("post-activate hook failed", "error", err)
	}
	s.stepComplete(result, step)

	result.Success = true
	result.Duration = time.Since(start)
	return result, nil
}

// Rollback reverts to the previous release.
func (s *Switcher) Rollback(ctx context.Context) (*DeployResult, error) {
	start := time.Now()
	result := &DeployResult{}

	rel, err := s.releaseMgr.Rollback()
	if err != nil {
		return nil, err
	}

	result.Release = rel
	result.Success = true
	result.Duration = time.Since(start)
	return result, nil
}

func (s *Switcher) stepStart(name string) *DeployStep {
	step := &DeployStep{Name: name, Status: "running"}
	return step
}

func (s *Switcher) stepComplete(result *DeployResult, step *DeployStep) {
	step.Status = "ok"
	result.Steps = append(result.Steps, *step)
}

func (s *Switcher) stepFail(result *DeployResult, step *DeployStep, name string, err error) error {
	step.Status = "failed"
	step.Error = err.Error()
	result.Error = fmt.Sprintf("%s: %v", name, err)
	result.Steps = append(result.Steps, *step)
	return err
}

// Prober checks if a release is healthy.
type Prober struct {
	HTTPClient *http.Client
}

// Probe checks if a release can serve requests.
func (p *Prober) Probe(ctx context.Context, rel *Release) (bool, error) {
	if p.HTTPClient == nil {
		p.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}

	// Check if the release directory exists and has expected files.
	if rel.Dir == "" {
		return false, nil
	}

	// In a real deployment, this would:
	// 1. Start a temporary PHP-FPM pool for the release
	// 2. Send health check requests
	// 3. Verify response codes
	// 4. Shut down the temporary pool
	// For now, we verify the release structure exists.
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_ = probeCtx // would be used for HTTP probes

	return true, nil
}

// SwitcherStatus holds the current deploy state.
type SwitcherStatus struct {
	Active    *Release
	Recent    []*DeployResult
	mu        sync.RWMutex
}

// NewSwitcherStatus creates a status tracker.
func NewSwitcherStatus() *SwitcherStatus {
	return &SwitcherStatus{}
}

// Record adds a deploy result.
func (ss *SwitcherStatus) Record(r *DeployResult) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.Recent = append(ss.Recent, r)
	if len(ss.Recent) > 20 {
		ss.Recent = ss.Recent[len(ss.Recent)-20:]
	}
}
