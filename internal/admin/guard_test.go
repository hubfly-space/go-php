package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func testGuard(t *testing.T, token string) (*Guard, http.Handler) {
	t.Helper()

	cfg := DefaultGuardConfig()
	cfg.Token = token
	g := NewGuard(cfg, nil)

	return g, g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
}

func TestGuardRejectsUnauthenticated(t *testing.T) {
	_, h := testGuard(t, "secret")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sites", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("401 should advertise the required scheme")
	}
}

func TestGuardAcceptsBearerToken(t *testing.T) {
	_, h := testGuard(t, "secret")

	req := httptest.NewRequest("GET", "/api/sites", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGuardRejectsWrongToken(t *testing.T) {
	_, h := testGuard(t, "secret")

	req := httptest.NewRequest("GET", "/api/sites", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGuardFailsClosedWithoutToken(t *testing.T) {
	// An empty token must deny, never allow.
	_, h := testGuard(t, "")

	req := httptest.NewRequest("POST", "/api/sites", nil)
	req.Header.Set("Authorization", "Bearer anything")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when no token is configured", w.Code)
	}
}

func TestGuardAllowsHealthUnauthenticated(t *testing.T) {
	_, h := testGuard(t, "secret")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/health", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for the liveness probe", w.Code)
	}
}

func TestGuardRejectsCrossOrigin(t *testing.T) {
	_, h := testGuard(t, "secret")

	req := httptest.NewRequest("POST", "/api/sites", nil)
	req.Host = "127.0.0.1:30200"
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a cross-origin request", w.Code)
	}
}

func TestGuardAllowsSameOrigin(t *testing.T) {
	_, h := testGuard(t, "secret")

	req := httptest.NewRequest("GET", "/api/sites", nil)
	req.Host = "127.0.0.1:30200"
	req.Header.Set("Origin", "http://127.0.0.1:30200")
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for a same-origin request", w.Code)
	}
}

func TestGuardCheckOriginRejectsForeignOrigin(t *testing.T) {
	// The WebSocket upgrader previously accepted every origin, so any page in
	// the operator's browser could attach to the management log stream.
	g, _ := testGuard(t, "secret")

	req := httptest.NewRequest("GET", "/api/ws/logs", nil)
	req.Host = "127.0.0.1:30200"
	req.Header.Set("Origin", "https://evil.example")

	if g.CheckOrigin(req) {
		t.Error("CheckOrigin accepted a foreign origin")
	}

	req.Header.Set("Origin", "http://127.0.0.1:30200")
	if !g.CheckOrigin(req) {
		t.Error("CheckOrigin rejected a same-origin request")
	}

	req.Header.Del("Origin")
	if !g.CheckOrigin(req) {
		t.Error("CheckOrigin rejected a non-browser client with no Origin")
	}
}

func TestGuardSetsSecurityHeaders(t *testing.T) {
	_, h := testGuard(t, "secret")

	req := httptest.NewRequest("GET", "/api/sites", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	for _, header := range []string{
		"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy",
	} {
		if w.Header().Get(header) == "" {
			t.Errorf("missing security header %s", header)
		}
	}
}

func TestGuardAllowsWebSocketTokenQuery(t *testing.T) {
	// Browsers cannot set headers on a WebSocket handshake, so the token is
	// accepted as a query parameter for upgrades only.
	_, h := testGuard(t, "secret")

	req := httptest.NewRequest("GET", "/api/ws/logs?token=secret", nil)
	req.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for an upgrade with a token query", w.Code)
	}

	// The same query parameter must not authenticate an ordinary request.
	plain := httptest.NewRequest("POST", "/api/sites?token=secret", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, plain)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401; the token query is upgrade-only", w2.Code)
	}
}

func TestGuardCountsDenials(t *testing.T) {
	g, h := testGuard(t, "secret")

	for i := 0; i < 3; i++ {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/sites", nil))
	}

	if got := g.Denials(); got != 3 {
		t.Errorf("denials = %d, want 3", got)
	}
}

func TestGuardIssuesSessionCookieOnPageLoad(t *testing.T) {
	_, h := testGuard(t, "secret")

	req := httptest.NewRequest("GET", "/?token=secret", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookie {
			session = c
		}
	}
	if session == nil {
		t.Fatal("no session cookie issued")
	}
	if !session.HttpOnly {
		t.Error("session cookie must be HttpOnly so page JS cannot read the token")
	}
	if session.SameSite != http.SameSiteStrictMode {
		t.Error("session cookie must be SameSite=Strict to blunt CSRF")
	}
}

func TestGuardAcceptsSessionCookie(t *testing.T) {
	_, h := testGuard(t, "secret")

	req := httptest.NewRequest("GET", "/api/sites", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "secret"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 with a valid session cookie", w.Code)
	}
}

func TestGuardRejectsWrongSessionCookie(t *testing.T) {
	_, h := testGuard(t, "secret")

	req := httptest.NewRequest("GET", "/api/sites", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "not-the-token"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGuardTokenQueryDoesNotAuthenticateAPICalls(t *testing.T) {
	// The query parameter is for the page load and the WebSocket upgrade only.
	// Honouring it on API calls would make every CSRF defense moot, since a
	// cross-site page can trivially put a token in a URL.
	_, h := testGuard(t, "secret")

	for _, method := range []string{"POST", "PUT", "DELETE"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, "/api/sites?token=secret", nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s status = %d, want 401", method, w.Code)
		}
	}
}

func TestGuardCookieDoesNotBypassOriginCheck(t *testing.T) {
	// A cookie is sent by the browser automatically, so the origin check is
	// what stops a cross-site page from driving the API with it.
	_, h := testGuard(t, "secret")

	req := httptest.NewRequest("POST", "/api/sites", nil)
	req.Host = "127.0.0.1:30200"
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "secret"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}
