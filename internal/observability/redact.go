package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
)

// SecretRedactor wraps a slog.Handler and redacts sensitive values from log output.
type SecretRedactor struct {
	next      slog.Handler
	patterns  []*regexp.Regexp
	mu        sync.RWMutex
	exactKeys map[string]bool
}

// NewSecretRedactor creates a log handler that redacts secrets.
func NewSecretRedactor(next slog.Handler, patterns []string) *SecretRedactor {
	r := &SecretRedactor{
		next:      next,
		exactKeys: make(map[string]bool),
	}

	// Default secret key patterns.
	secretKeys := []string{
		"token", "secret", "password", "api_key", "apikey",
		"access_token", "refresh_token", "private_key", "csrf_secret",
	}
	for _, k := range secretKeys {
		r.exactKeys[k] = true
	}

	// Compile custom patterns.
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			r.patterns = append(r.patterns, re)
		}
	}

	return r
}

func (r *SecretRedactor) Enabled(ctx context.Context, level slog.Level) bool {
	return r.next.Enabled(ctx, level)
}

func (r *SecretRedactor) Handle(ctx context.Context, record slog.Record) error {
	var newAttrs []slog.Attr

	record.Attrs(func(a slog.Attr) bool {
		a = r.redactAttr(a)
		newAttrs = append(newAttrs, a)
		return true
	})

	// Rebuild record with redacted attrs.
	newRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	for i := range newAttrs {
		newRecord.AddAttrs(newAttrs[i])
	}

	return r.next.Handle(ctx, newRecord)
}

func (r *SecretRedactor) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = r.redactAttr(a)
	}
	return &SecretRedactor{
		next:      r.next.WithAttrs(redacted),
		patterns:  r.patterns,
		exactKeys: r.exactKeys,
	}
}

func (r *SecretRedactor) WithGroup(name string) slog.Handler {
	return &SecretRedactor{
		next:      r.next.WithGroup(name),
		patterns:  r.patterns,
		exactKeys: r.exactKeys,
	}
}

func (r *SecretRedactor) redactAttr(a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)

	// Check exact key match.
	r.mu.RLock()
	isSecret := r.exactKeys[key]
	r.mu.RUnlock()

	if isSecret {
		a.Value = slog.StringValue("[REDACTED]")
		return a
	}

	// Check regex patterns.
	if a.Value.Kind() == slog.KindString {
		val := a.Value.String()
		for _, re := range r.patterns {
			if re.MatchString(val) {
				a.Value = slog.StringValue("[REDACTED]")
				return a
			}
		}
	}

	return a
}

// AddKey adds a key that should be redacted.
func (r *SecretRedactor) AddKey(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exactKeys[strings.ToLower(key)] = true
}

// RemoveKey removes a key from the redaction list.
func (r *SecretRedactor) RemoveKey(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.exactKeys, strings.ToLower(key))
}

// RedactString applies secret redaction to an arbitrary string.
func RedactString(s string, patterns []string) string {
	// Redact common patterns.
	sensitive := []string{
		"password", "secret", "token", "api_key", "apikey",
	}

	for _, key := range sensitive {
		// Redact "key=value" patterns.
		re := regexp.MustCompile(`(?i)` + key + `["\s]*[:=]["\s]*([^\s"&]+)`)
		s = re.ReplaceAllString(s, key+"=[REDACTED]")
	}

	// Redact bearer tokens.
	s = regexp.MustCompile(`Bearer\s+[A-Za-z0-9\-._~+/]+=*`).ReplaceAllString(s, "Bearer [REDACTED]")

	// Apply custom patterns.
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			s = re.ReplaceAllString(s, "[REDACTED]")
		}
	}

	return s
}

// RedactLog returns a new slog.Logger with a redacting handler.
func RedactLog(logger *slog.Logger, patterns []string) *slog.Logger {
	return slog.New(NewSecretRedactor(logger.Handler(), patterns))
}

// StdoutRedactor returns a handler that writes redacted JSON to stdout.
func StdoutRedactor(patterns []string) *SecretRedactor {
	return NewSecretRedactor(
		slog.NewJSONHandler(os.Stdout, nil),
		patterns,
	)
}

// RedactError wraps an error, redacting any sensitive data in its message.
func RedactError(err error, patterns []string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", RedactString(err.Error(), patterns))
}
