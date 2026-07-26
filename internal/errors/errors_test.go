package errors

import (
	"errors"
	"testing"
)

func TestGatewayError_Error(t *testing.T) {
	t.Run("no cause", func(t *testing.T) {
		e := New(CodePathRejected, "path rejected")
		if got := e.Error(); got != "E_PATH_REJECTED: path rejected" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("with cause", func(t *testing.T) {
		cause := errors.New("underlying")
		e := Wrap(CodeFastCGIProtocol, "protocol error", cause)
		got := e.Error()
		if got != "E_FASTCGI_PROTOCOL: protocol error: underlying" {
			t.Errorf("got %q", got)
		}
	})
}

func TestGatewayError_Unwrap(t *testing.T) {
	cause := errors.New("inner")
	e := Wrap(CodePHPTimeout, "timeout", cause)
	if !errors.Is(e, cause) {
		t.Error("errors.Is should find the cause")
	}
}

func TestIsCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code Code
		want bool
	}{
		{"direct match", New(CodePathRejected, "no"), CodePathRejected, true},
		{"wrapped gateway error", Wrap(CodePHPTimeout, "t", errors.New("x")), CodePHPTimeout, true},
		{"plain error", errors.New("no code"), CodePathRejected, false},
		{"nil", nil, CodePathRejected, false},
		{"wrong code", New(CodePathRejected, "no"), CodePHPTimeout, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCode(tt.err, tt.code); got != tt.want {
				t.Errorf("IsCode() = %v, want %v", got, tt.want)
			}
		})
	}
}
