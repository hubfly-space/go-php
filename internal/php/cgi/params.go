// Package cgi provides CGI/FastCGI environment variable construction for PHP.
package cgi

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// BuildParams constructs the CGI/FastCGI environment variables for a PHP request.
// It maps HTTP headers and request metadata into the CGI parameter format.
func BuildParams(r *http.Request, scriptFilename, scriptName, documentRoot string) map[string]string {
	params := make(map[string]string)

	// CGI standard variables.
	params["GATEWAY_INTERFACE"] = "CGI/1.1"
	params["SERVER_SOFTWARE"] = "go-php-gateway/1.0"
	params["SERVER_PROTOCOL"] = r.Proto

	params["REQUEST_METHOD"] = r.Method
	params["REQUEST_URI"] = requestURI(r)
	params["DOCUMENT_URI"] = r.URL.Path
	params["QUERY_STRING"] = r.URL.RawQuery

	params["SCRIPT_FILENAME"] = scriptFilename
	params["SCRIPT_NAME"] = scriptName
	params["DOCUMENT_ROOT"] = documentRoot

	// PATH_INFO: extra path after the script name.
	if scriptName != "" && strings.HasPrefix(r.URL.Path, scriptName) {
		params["PATH_INFO"] = r.URL.Path[len(scriptName):]
		params["PATH_TRANSLATED"] = documentRoot + params["PATH_INFO"]
	}

	// Remote address.
	host, port, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
		port = "0"
	}
	params["REMOTE_ADDR"] = host
	params["REMOTE_PORT"] = port

	// Server address — use the Host header.
	serverName, serverPort, err := net.SplitHostPort(r.Host)
	if err != nil {
		serverName = r.Host
		serverPort = "80"
		if r.TLS != nil {
			serverPort = "443"
		}
	}
	params["SERVER_NAME"] = serverName
	params["SERVER_PORT"] = serverPort

	// HTTPS flag.
	if r.TLS != nil {
		params["HTTPS"] = "on"
	}

	// Content-Type and Content-Length.
	if ct := r.Header.Get("Content-Type"); ct != "" {
		params["CONTENT_TYPE"] = ct
	}
	if r.ContentLength >= 0 {
		params["CONTENT_LENGTH"] = fmt.Sprintf("%d", r.ContentLength)
	}

	// HTTP_* variables from request headers.
	for name, values := range r.Header {
		cgiName := "HTTP_" + httpHeaderToCGI(name)
		params[cgiName] = strings.Join(values, ", ")
	}

	// REDIRECT_STATUS — set for front-controller patterns.
	params["REDIRECT_STATUS"] = "200"

	return params
}

// requestURI reconstructs the original request URI.
func requestURI(r *http.Request) string {
	uri := r.URL.RequestURI()
	if uri == "" {
		uri = "/"
	}
	return uri
}

// httpHeaderToCGI converts an HTTP header name to CGI format.
// Dashes become underscores, and the name is uppercased.
func httpHeaderToCGI(name string) string {
	name = strings.ReplaceAll(name, "-", "_")
	return strings.ToUpper(name)
}
