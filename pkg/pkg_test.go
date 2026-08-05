package pkg_test

import (
	"context"
	"net/http"

	"github.com/go-php/gateway/pkg/configapi"
	"github.com/go-php/gateway/pkg/pluginapi"
	"github.com/go-php/gateway/pkg/policyapi"
	"testing"
)

func TestConfigAPI(t *testing.T) {
	cfg := configapi.Config{
		Schema: "v1",
		Server: configapi.ServerConfig{Addr: ":8080"},
	}
	if cfg.Schema != "v1" {
		t.Errorf("expected v1, got %s", cfg.Schema)
	}
}

type mockPlugin struct{}

func (m *mockPlugin) Init(ctx context.Context, config map[string]interface{}) error { return nil }
func (m *mockPlugin) Metadata() pluginapi.PluginMetadata {
	return pluginapi.PluginMetadata{Name: "mock", Version: "1.0"}
}
func (m *mockPlugin) Close() error { return nil }
func (m *mockPlugin) WrapHandler(next http.Handler) http.Handler {
	return next
}

func TestPluginAPI(t *testing.T) {
	var p pluginapi.MiddlewarePlugin = &mockPlugin{}
	meta := p.Metadata()
	if meta.Name != "mock" {
		t.Errorf("meta.Name = %q, want mock", meta.Name)
	}
}

type mockPolicyEngine struct{}

func (m *mockPolicyEngine) Evaluate(ctx context.Context, phase policyapi.EvaluationPhase, pctx *policyapi.PolicyContext) policyapi.Decision {
	return policyapi.Decision{Action: policyapi.ActionAllow}
}

func TestPolicyAPI(t *testing.T) {
	var eng policyapi.Engine = &mockPolicyEngine{}
	dec := eng.Evaluate(context.Background(), policyapi.PhaseRequestHeaders, nil)
	if dec.Action != policyapi.ActionAllow {
		t.Errorf("dec.Action = %v, want ActionAllow", dec.Action)
	}
}
