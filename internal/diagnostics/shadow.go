package diagnostics

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ShadowTester compares requests between the active and candidate runtimes.
type ShadowTester struct {
	activeURL   string // base URL of the active runtime
	candidateURL string // base URL of the candidate runtime
	client      *http.Client
	mu          sync.RWMutex
	results     []ShadowResult
	maxResults  int
}

// ShadowResult holds the comparison result for a single shadow request.
type ShadowResult struct {
	RequestURL     string        `json:"request_url"`
	Method         string        `json:"method"`
	ActiveStatus   int           `json:"active_status"`
	CandidateStatus int          `json:"candidate_status"`
	ActiveHash     string        `json:"active_hash"`
	CandidateHash  string        `json:"candidate_hash"`
	ActiveTime     time.Duration `json:"active_time"`
	CandidateTime  time.Duration `json:"candidate_time"`
	StatusMatch    bool          `json:"status_match"`
	BodyMatch      bool          `json:"body_match"`
	Timestamp      time.Time     `json:"timestamp"`
	Error          string        `json:"error,omitempty"`
}

// NewShadowTester creates a shadow testing client.
func NewShadowTester(activeURL, candidateURL string) *ShadowTester {
	return &ShadowTester{
		activeURL:    activeURL,
		candidateURL: candidateURL,
		client:       &http.Client{Timeout: 30 * time.Second},
		maxResults:   1000,
	}
}

// Compare sends a shadow request to both runtimes and compares results.
func (st *ShadowTester) Compare(ctx context.Context, method, path string, headers map[string]string) (*ShadowResult, error) {
	result := &ShadowResult{
		RequestURL: path,
		Method:     method,
		Timestamp:  time.Now(),
	}

	// Send to active.
	activeStart := time.Now()
	activeStatus, activeHash, err := st.sendRequest(ctx, st.activeURL+path, method, headers)
	result.ActiveTime = time.Since(activeStart)
	if err != nil {
		result.Error = fmt.Sprintf("active: %v", err)
	} else {
		result.ActiveStatus = activeStatus
		result.ActiveHash = activeHash
	}

	// Send to candidate.
	candidateStart := time.Now()
	candidateStatus, candidateHash, err := st.sendRequest(ctx, st.candidateURL+path, method, headers)
	result.CandidateTime = time.Since(candidateStart)
	if err != nil {
		result.Error = fmt.Sprintf("candidate: %v", err)
	} else {
		result.CandidateStatus = candidateStatus
		result.CandidateHash = candidateHash
	}

	// Compare.
	result.StatusMatch = result.ActiveStatus == result.CandidateStatus
	result.BodyMatch = result.ActiveHash == result.CandidateHash

	// Store result.
	st.mu.Lock()
	st.results = append(st.results, *result)
	if len(st.results) > st.maxResults {
		st.results = st.results[len(st.results)-st.maxResults:]
	}
	st.mu.Unlock()

	return result, nil
}

func (st *ShadowTester) sendRequest(ctx context.Context, url, method string, headers map[string]string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, "", err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Add a header to mark this as a shadow request (avoid counting in metrics).
	req.Header.Set("X-Shadow-Request", "true")

	resp, err := st.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	// Read body with limit.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
	if err != nil {
		return resp.StatusCode, "", nil
	}

	hash := sha256.Sum256(body)
	return resp.StatusCode, fmt.Sprintf("%x", hash), nil
}

// Results returns all shadow comparison results.
func (st *ShadowTester) Results() []ShadowResult {
	st.mu.RLock()
	defer st.mu.RUnlock()

	result := make([]ShadowResult, len(st.results))
	copy(result, st.results)
	return result
}

// Summary returns an aggregate summary of shadow test results.
func (st *ShadowTester) Summary() ShadowSummary {
	st.mu.RLock()
	defer st.mu.RUnlock()

	summary := ShadowSummary{
		Total: len(st.results),
	}

	for _, r := range st.results {
		if r.Error != "" {
			summary.Errors++
			continue
		}

		if r.StatusMatch {
			summary.StatusMatch++
		} else {
			summary.StatusMismatch++
		}

		if r.BodyMatch {
			summary.BodyMatch++
		} else {
			summary.BodyMismatch++
		}

		summary.AvgActiveTime += r.ActiveTime
		summary.AvgCandidateTime += r.CandidateTime
	}

	if summary.Total > 0 {
		n := float64(summary.Total - summary.Errors)
		if n > 0 {
			summary.AvgActiveTime /= time.Duration(n)
			summary.AvgCandidateTime /= time.Duration(n)
		}
	}

	return summary
}

// ShadowSummary holds aggregate shadow test statistics.
type ShadowSummary struct {
	Total           int           `json:"total"`
	Errors          int           `json:"errors"`
	StatusMatch     int           `json:"status_match"`
	StatusMismatch  int           `json:"status_mismatch"`
	BodyMatch       int           `json:"body_match"`
	BodyMismatch    int           `json:"body_mismatch"`
	AvgActiveTime   time.Duration `json:"avg_active_time"`
	AvgCandidateTime time.Duration `json:"avg_candidate_time"`
}

// String returns a human-readable summary.
func (s ShadowSummary) String() string {
	return fmt.Sprintf(
		"Shadow tests: %d total, %d errors, %d status match, %d body match, active avg: %s, candidate avg: %s",
		s.Total, s.Errors, s.StatusMatch, s.BodyMatch,
		s.AvgActiveTime.Round(time.Microsecond), s.AvgCandidateTime.Round(time.Microsecond),
	)
}

// IsSafe returns true if all shadow tests passed.
func (s ShadowSummary) IsSafe() bool {
	return s.Errors == 0 && s.StatusMismatch == 0
}
