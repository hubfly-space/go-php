package deploy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Switcher manages zero-downtime release switching.
type Switcher struct {
	releaseMgr *ReleaseManager
	hookRunner *HookRunner
	prober     *Prober
	logger     *slog.Logger
	extApply   ExtensionApplier
}

// ExtensionApplier applies extensions for a release.
type ExtensionApplier interface {
	ApplyReleaseExtensions(releaseID string, exts []ReleaseExtension) error
}

// NewSwitcher creates a deploy switcher.
func NewSwitcher(releaseMgr *ReleaseManager, hookRunner *HookRunner, extApply ExtensionApplier, logger *slog.Logger) *Switcher {
	return &Switcher{
		releaseMgr: releaseMgr,
		hookRunner: hookRunner,
		prober:     &Prober{},
		logger:     logger,
		extApply:   extApply,
	}
}

// DeployResult holds the result of a deploy operation.
type DeployResult struct {
	Release  *Release
	Success  bool
	Error    string
	Duration time.Duration
	Steps    []DeployStep
}

// DeployStep records a single step in the deploy.
type DeployStep struct {
	Name     string
	Status   string
	Error    string
	Duration time.Duration
}

// Deploy performs a full zero-downtime deploy cycle.
func (s *Switcher) Deploy(ctx context.Context, version, runtimeID, srcDir string, metadata map[string]string, exts []ReleaseExtension) (*DeployResult, error) {
	start := time.Now()
	result := &DeployResult{}

	// Step 1: Create release.
	step := s.stepStart("create_release")
	rel, err := s.releaseMgr.Create(version, runtimeID, srcDir, metadata)
	if err != nil {
		return nil, s.stepFail(result, step, "create release", err)
	}
	rel.Extensions = exts
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

	// Step 2.5: Apply extensions.
	if len(exts) > 0 && s.extApply != nil {
		step = s.stepStart("apply_extensions")
		if err := s.extApply.ApplyReleaseExtensions(rel.ID, exts); err != nil {
			rel.State = StateFailed
			rel.Error = err.Error()
			s.releaseMgr.saveRelease(rel)
			return nil, s.stepFail(result, step, "apply extensions", err)
		}
		s.stepComplete(result, step)
	}

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

// Prober checks if a release is healthy before it is activated.
type Prober struct {
	HTTPClient *http.Client

	// HealthURL, when set, is requested after the structural checks pass. A
	// non-2xx response fails the probe.
	HealthURL string

	// Entrypoints are release-relative files that must exist. Defaults to
	// index.php when empty.
	Entrypoints []string
}

// Probe checks if a release can serve requests.
//
// This used to `return true, nil` unconditionally, with a comment saying it
// verified the release structure — which it also did not do. A health gate that
// cannot fail is worse than no gate, because the deploy pipeline reports a
// passing check that never ran.
//
// Full candidate verification (§21.3: start a candidate pool, check the PHP
// version and extensions, run warm-up URLs) needs the supervisor, which this
// package does not own. What is checked here is checked honestly.
func (p *Prober) Probe(ctx context.Context, rel *Release) (bool, error) {
	if p.HTTPClient == nil {
		p.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}

	if rel == nil || rel.Dir == "" {
		return false, fmt.Errorf("probe: release has no directory")
	}

	info, err := os.Stat(rel.Dir)
	if err != nil {
		return false, fmt.Errorf("probe: release directory: %w", err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("probe: release path %s is not a directory", rel.Dir)
	}

	entrypoints := p.Entrypoints
	if len(entrypoints) == 0 {
		entrypoints = []string{"index.php"}
	}

	// At least one entrypoint must exist, checked in both the release root and
	// a public/ subdirectory so framework layouts pass.
	found := false
	for _, entry := range entrypoints {
		for _, candidate := range []string{
			filepath.Join(rel.Dir, entry),
			filepath.Join(rel.Dir, "public", entry),
		} {
			if st, statErr := os.Stat(candidate); statErr == nil && st.Mode().IsRegular() {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return false, fmt.Errorf("probe: no entrypoint (%s) found in %s",
			strings.Join(entrypoints, ", "), rel.Dir)
	}

	if p.HealthURL == "" {
		return true, nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, p.HealthURL, nil)
	if err != nil {
		return false, fmt.Errorf("probe: build health request: %w", err)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("probe: health request: %w", err)
	}
	defer resp.Body.Close()
	// Drain so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("probe: health check returned %d", resp.StatusCode)
	}

	return true, nil
}

// SwitcherStatus holds the current deploy state.
type SwitcherStatus struct {
	Active *Release
	Recent []*DeployResult
	mu     sync.RWMutex
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
