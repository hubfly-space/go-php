package diagnostics

import (
	"fmt"
	"regexp"
	"strings"
)

// HtaccessTranslator converts Apache .htaccess rules to gateway.yaml routes.
type HtaccessTranslator struct {
	warnings []string
}

// NewHtaccessTranslator creates a new translator.
func NewHtaccessTranslator() *HtaccessTranslator {
	return &HtaccessTranslator{}
}

// TranslatedRoute is a route converted from .htaccess.
type TranslatedRoute struct {
	PathPrefix string   `json:"path_prefix,omitempty"`
	Path       string   `json:"path,omitempty"`
	Regex      string   `json:"regex,omitempty"`
	Status     int      `json:"status,omitempty"`
	Target     string   `json:"target,omitempty"`
	Methods    []string `json:"methods,omitempty"`
}

// Translate converts .htaccess content to gateway routes.
func (t *HtaccessTranslator) Translate(content string) ([]TranslatedRoute, []string) {
	var routes []TranslatedRoute
	t.warnings = nil

	lines := strings.Split(content, "\n")
	rewriteEngineOn := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for RewriteEngine On.
		if strings.EqualFold(line, "RewriteEngine On") {
			rewriteEngineOn = true
			continue
		}

		// Skip if RewriteEngine not on (but allow Redirect without it).
		if !rewriteEngineOn && !strings.HasPrefix(strings.ToLower(line), "redirect") {
			continue
		}

		// RewriteCond — skip for now (can't represent in gateway routes).
		if strings.HasPrefix(strings.ToLower(line), "rewritecond") {
			t.warnings = append(t.warnings,
				fmt.Sprintf("RewriteCond not supported: %s — converted as unconditional", line))
			continue
		}

		// RewriteRule.
		if strings.HasPrefix(strings.ToLower(line), "rewriterule") {
			route, err := t.parseRewriteRule(line)
			if err != nil {
				t.warnings = append(t.warnings, err.Error())
				continue
			}
			routes = append(routes, *route)
			continue
		}

		// Redirect directive.
		if strings.HasPrefix(strings.ToLower(line), "redirect") {
			route, err := t.parseRedirect(line)
			if err != nil {
				t.warnings = append(t.warnings, err.Error())
				continue
			}
			routes = append(routes, *route)
			continue
		}

		// Unsupported directive.
		t.warnings = append(t.warnings,
			fmt.Sprintf("unsupported directive: %s", line))
	}

	return routes, t.warnings
}

// parseRewriteRule parses a RewriteRule directive.
// Format: RewriteRule pattern target [flags]
func (t *HtaccessTranslator) parseRewriteRule(line string) (*TranslatedRoute, error) {
	// Remove "RewriteRule" prefix.
	line = strings.TrimSpace(line[len("RewriteRule"):])

	// Extract parts: pattern target [flags]
	re := regexp.MustCompile(`^(\S+)\s+(\S+)(?:\s+\[([^\]]+)\])?$`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return nil, fmt.Errorf("invalid RewriteRule: %s", line)
	}

	pattern := matches[1]
	target := matches[2]
	flags := ""
	if len(matches) > 3 {
		flags = matches[3]
	}

	// Parse flags.
	flagSet := parseFlags(flags)

	// Convert Apache regex to Go regex.
	regex := apacheToGoRegex(pattern)

	// Determine route type.
	route := &TranslatedRoute{
		Regex:  regex,
		Target: target,
	}

	// Handle flags.
	if flagSet["R"] || flagSet["r"] {
		// Redirect.
		status := 301
		if flagSet["L"] || flagSet["l"] {
			// L flag with redirect — combined last+redirect.
			status = 301
		}
		route.Status = status
		route.Target = target
	} else if flagSet["L"] || flagSet["l"] {
		// Last rule — use as rewrite.
		route.Target = target
	}

	return route, nil
}

// parseRedirect parses a Redirect directive.
// Format: Redirect [status] URL-path file
func (t *HtaccessTranslator) parseRedirect(line string) (*TranslatedRoute, error) {
	line = strings.TrimSpace(line[len("Redirect"):])

	parts := strings.Fields(line)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid Redirect: %s", line)
	}

	status := 301
	path := parts[0]

	// Check if first arg is a status code.
	if len(parts) >= 3 {
		fmt.Sscanf(parts[0], "%d", &status)
		path = parts[1]
	}

	return &TranslatedRoute{
		Path:   path,
		Status: status,
		Target: parts[len(parts)-1],
	}, nil
}

// apacheToGoRegex converts a simple Apache rewrite pattern to Go regex.
func apacheToGoRegex(pattern string) string {
	// Handle common Apache patterns.
	result := pattern

	// ^ and $ are already valid regex.
	// .* in Apache = .* in Go.
	// (.*) capture groups work the same.

	// Apache %1 = Go $1 (backreferences).
	for i := 1; i <= 9; i++ {
		result = strings.ReplaceAll(result, fmt.Sprintf("%%%d", i), fmt.Sprintf("$%d", i))
	}

	return result
}

func parseFlags(flags string) map[string]bool {
	result := make(map[string]bool)
	for _, f := range strings.Split(flags, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			// Handle R=301 style flags — extract the key before =.
			key := f
			if idx := strings.IndexByte(f, '='); idx >= 0 {
				key = f[:idx]
			}
			result[key] = true
			result[f] = true // also store full flag for reference
		}
	}
	return result
}

// Warnings returns any warnings from the last translation.
func (t *HtaccessTranslator) Warnings() []string {
	return t.warnings
}
