package tls

import (
	"net/http"
	"net/http/httptest"
)

func createTestRequest(method, path string) *http.Request {
	return httptest.NewRequest(method, path, nil)
}

func createTestRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}
