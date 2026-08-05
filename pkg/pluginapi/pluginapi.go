// Package pluginapi provides public interfaces for developing custom gateway plugins.
package pluginapi

import (
	"context"
	"net/http"
)

// PluginMetadata describes a gateway extension plugin.
type PluginMetadata struct {
	Name        string
	Version     string
	Description string
	Author      string
}

// Plugin is the base interface that every plugin must implement.
type Plugin interface {
	Init(ctx context.Context, config map[string]interface{}) error
	Metadata() PluginMetadata
	Close() error
}

// MiddlewarePlugin is a plugin that inspects or mutates HTTP requests.
type MiddlewarePlugin interface {
	Plugin
	WrapHandler(next http.Handler) http.Handler
}
