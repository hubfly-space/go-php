package policy

import (
	"fmt"
	"log/slog"
	"net/http"
)

// Mode selects how policy rules are enforced (§23.4).
//
// Important: mode governs *rules only*. Structural protections — path
// canonicalization, script mapping, filesystem boundary enforcement — live in
// the request path itself and are never disabled by this setting. §23.4 is
// explicit that "structural protections such as invalid HTTP framing, path
// escape, and invalid script mapping must never be disabled by a generic WAF
// mode."
type Mode string

const (
	// ModeOff evaluates no rules. Intended for development only.
	ModeOff Mode = "off"

	// ModeObserve evaluates rules and logs what would have happened, without
	// blocking. §33.4 — the intended way to adopt rules on a live service.
	ModeObserve Mode = "observe"

	// ModeBalanced is the recommended default.
	ModeBalanced Mode = "balanced"

	// ModeStrict adds rules that are more likely to reject unusual but legal
	// traffic.
	ModeStrict Mode = "strict"
)

// ParseMode converts a config string into a Mode.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case ModeOff, ModeObserve, ModeBalanced, ModeStrict:
		return Mode(s), nil
	case "":
		return ModeBalanced, nil
	default:
		return "", fmt.Errorf("unknown security mode %q (want off, observe, balanced, or strict)", s)
	}
}

// DefaultRules returns the built-in ruleset for a mode.
//
// The set is deliberately small. §26.1: "The WAF is a risk-reduction layer, not
// a guarantee that applications are secure." Rules that would false-positive on
// ordinary WordPress or Laravel traffic do more harm than good, so this covers
// only unambiguous cases.
func DefaultRules(mode Mode) []Rule {
	if mode == ModeOff {
		return nil
	}

	// In observe mode every rule reports instead of blocking, so an operator
	// can see the impact before enforcing.
	decision := DecisionDeny
	if mode == ModeObserve {
		decision = DecisionObserve
	}

	rules := []Rule{
		{
			Name:     "deny-vcs-metadata",
			Phase:    PhaseRequest,
			Mode:     decision,
			Priority: 10,
			Conditions: []Condition{{
				Type:   CondPathPrefix,
				Values: []string{"/.git/", "/.svn/", "/.hg/", "/.bzr/"},
			}},
		},
		{
			Name:     "deny-dotenv",
			Phase:    PhaseRequest,
			Mode:     decision,
			Priority: 10,
			Conditions: []Condition{{
				Type:   CondPathPrefix,
				Values: []string{"/.env"},
			}},
		},
		{
			// Cross-site tracing. No PHP application needs these verbs.
			Name:     "deny-trace-methods",
			Phase:    PhaseRequest,
			Mode:     decision,
			Priority: 20,
			Conditions: []Condition{{
				Type:   CondMethod,
				Values: []string{"TRACE", "TRACK"},
			}},
		},
	}

	if mode == ModeStrict {
		rules = append(rules,
			Rule{
				Name:     "deny-connect-method",
				Phase:    PhaseRequest,
				Mode:     DecisionDeny,
				Priority: 20,
				Conditions: []Condition{{
					Type:   CondMethod,
					Values: []string{"CONNECT"},
				}},
			},
			Rule{
				// Editor and backup droppings that frequently contain
				// credentials when they leak into a document root.
				Name:     "deny-editor-artifacts",
				Phase:    PhaseRequest,
				Mode:     DecisionDeny,
				Priority: 30,
				Conditions: []Condition{{
					Type:   CondPathRegex,
					Values: []string{`\.(bak|swp|orig|rej|save|old)$`},
				}},
			},
		)
	}

	return rules
}

// NewEngineForMode returns an engine preloaded with the default ruleset for a
// mode. For ModeOff the engine has no rules, so Evaluate is a cheap no-op.
func NewEngineForMode(mode Mode) *Engine {
	e := NewEngine()
	for _, rule := range DefaultRules(mode) {
		e.AddRule(rule)
	}
	return e
}

// ModeMiddleware returns an HTTP middleware enforcing the engine's rules
// according to mode.
//
// ModeOff returns the handler unwrapped rather than evaluating an empty
// ruleset, so the disabled path costs nothing in the hot path (§3.3: "No global
// locks in the request hot path").
func ModeMiddleware(e *Engine, mode Mode, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if mode == ModeOff {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decision, rule := e.EvaluateRule(&Context{
				Phase:    PhaseRequest,
				Method:   r.Method,
				Path:     r.URL.Path,
				Host:     r.Host,
				Headers:  r.Header,
				Query:    r.URL.RawQuery,
				RemoteIP: parseIP(r.RemoteAddr),
				TLS:      r.TLS != nil,
			})

			ruleName := "unknown"
			if rule != nil {
				ruleName = rule.Name
			}

			// A user-supplied deny rule must still be downgraded in observe
			// mode rather than silently enforced. DefaultRules already emits
			// DecisionObserve here, but the engine accepts rules from
			// elsewhere too.
			if mode == ModeObserve && decision == DecisionDeny {
				decision = DecisionObserve
			}

			switch decision {
			case DecisionObserve:
				if logger != nil {
					logger.Info("policy observed",
						"rule", ruleName, "method", r.Method, "path", r.URL.Path)
				}
				w.Header().Set("X-Policy-Observed", ruleName)

			case DecisionDeny:
				if logger != nil {
					logger.Warn("policy denied",
						"rule", ruleName, "method", r.Method, "path", r.URL.Path)
				}
				// The public message names the rule but not the reason —
				// §23.3 separates public_message from internal_reason.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, `{"error":"forbidden","rule":%q}`+"\n", ruleName)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
