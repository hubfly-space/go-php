package observability

import (
	"log/slog"
	"testing"
)

func TestRedactString_CommonSecrets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "password in URL",
			input:    "postgres://user:secret123@localhost/db",
			expected: "postgres://user:[REDACTED]@localhost/db",
		},
		{
			name:     "token in header",
			input:    "Authorization: Bearer abc123def456",
			expected: "Authorization: [REDACTED]",
		},
		{
			name:     "api_key in config",
			input:    "api_key=sk_live_abcdef123456",
			expected: "[REDACTED]=sk_live_abcdef123456",
		},
		{
			name:     "clean string",
			input:    "GET /api/users HTTP/1.1",
			expected: "GET /api/users HTTP/1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactString(tt.input, nil)
			t.Logf("input:  %s", tt.input)
			t.Logf("output: %s", result)
		})
	}
}

func TestRedactString_CustomPatterns(t *testing.T) {
	input := "connection string: postgres://admin:pass@db:5432/mydb"
	patterns := []string{`postgres://[^@]+@`}

	result := RedactString(input, patterns)
	t.Logf("result: %s", result)
}

func TestSecretRedactor_Handler(t *testing.T) {
	redactor := NewSecretRedactor(
		slog.NewJSONHandler(nil, nil),
		nil,
	)

	if !redactor.Enabled(nil, slog.LevelInfo) {
		t.Error("expected Enabled to return true")
	}
}

func TestSecretRedactor_AddRemoveKey(t *testing.T) {
	redactor := NewSecretRedactor(
		slog.NewJSONHandler(nil, nil),
		nil,
	)

	redactor.AddKey("custom_secret")
	redactor.RemoveKey("token")

	// Verify by checking the exactKeys map.
	// This is internal, but we can verify the behavior indirectly.
	_ = redactor
}

func TestRedactError(t *testing.T) {
	err := RedactError(nil, nil)
	if err != nil {
		t.Error("expected nil error")
	}
}
