package config

import (
	"fmt"
	"sort"

	"github.com/go-php/gateway/internal/runtime"
)

// ResolvedExtension is a fully resolved extension with its type.
type ResolvedExtension struct {
	Name string
	Type string // "extension" or "zend_extension"
}

// ResolveExtensions takes a PHPConfig and resolves the final set of enabled extensions.
// It returns the list of extensions that should be enabled, and any custom INI settings.
func ResolveExtensions(phpCfg *PHPConfig) ([]ResolvedExtension, []IniSetting, error) {
	extNames, err := resolveExtensionNames(phpCfg)
	if err != nil {
		return nil, nil, err
	}
	if extNames == nil {
		return nil, phpCfg.PhpIni, nil
	}

	// Build type map from individual config.
	typeMap := make(map[string]string)
	for _, ext := range phpCfg.Extensions {
		if ext.Type != "" {
			typeMap[ext.Name] = ext.Type
		}
	}

	resolved := make([]ResolvedExtension, 0, len(extNames))
	for _, name := range extNames {
		extType := typeMap[name]
		if extType == "" {
			extType = "extension"
		}
		resolved = append(resolved, ResolvedExtension{
			Name: name,
			Type: extType,
		})
	}

	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].Name < resolved[j].Name
	})

	return resolved, phpCfg.PhpIni, nil
}

// resolveExtensionNames returns the sorted list of extension names to enable.
func resolveExtensionNames(phpCfg *PHPConfig) ([]string, error) {
	// Profile takes precedence.
	if phpCfg.ExtensionProfile != "" {
		exts, err := runtime.ProfileExtensions(phpCfg.ExtensionProfile)
		if err != nil {
			return nil, fmt.Errorf("resolve profile: %w", err)
		}
		return exts, nil
	}

	// Individual extensions — filter enabled ones.
	if len(phpCfg.Extensions) > 0 {
		var names []string
		for _, ext := range phpCfg.Extensions {
			if ext.Name == "" {
				continue
			}
			// nil means enabled (default), false means explicitly disabled.
			if ext.Enabled == nil || *ext.Enabled {
				names = append(names, ext.Name)
			}
		}
		sort.Strings(names)
		return names, nil
	}

	// No extension config — return empty.
	return nil, nil
}

// ApplyExtensionOverrides applies per-route extension overrides to a base set.
func ApplyExtensionOverrides(base []ResolvedExtension, override *ExtensionOverride) []ResolvedExtension {
	if override == nil {
		return base
	}

	disableSet := make(map[string]bool, len(override.Disable))
	for _, name := range override.Disable {
		disableSet[name] = true
	}

	enableSet := make(map[string]bool, len(override.Enable))
	for _, name := range override.Enable {
		enableSet[name] = true
	}

	result := make([]ResolvedExtension, 0, len(base))
	for _, ext := range base {
		if disableSet[ext.Name] {
			continue
		}
		result = append(result, ext)
	}

	// Add enabled extensions not already in the set.
	existing := make(map[string]bool, len(base))
	for _, ext := range base {
		existing[ext.Name] = true
	}
	for _, name := range override.Enable {
		if !existing[name] && !disableSet[name] {
			result = append(result, ResolvedExtension{Name: name, Type: "extension"})
		}
	}

	return result
}
