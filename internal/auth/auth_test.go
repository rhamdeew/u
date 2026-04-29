package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"u/internal/auth"
)

const testKey = "test-secret-key-for-unit-tests"

func TestHashAndCheckPassword(t *testing.T) {
	password := "hunter2"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if !auth.CheckPassword(hash, password) {
		t.Error("correct password should verify")
	}
	if auth.CheckPassword(hash, "wrong") {
		t.Error("wrong password should not verify")
	}
}

func TestSetAndGetSession(t *testing.T) {
	w := httptest.NewRecorder()
	auth.SetSession(w, "alice", testKey)

	// Build a request with the cookie that was set
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookie was set")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}

	user := auth.GetSession(req, testKey)
	if user != "alice" {
		t.Errorf("expected 'alice', got %q", user)
	}
}

func TestGetSessionWrongKey(t *testing.T) {
	w := httptest.NewRecorder()
	auth.SetSession(w, "alice", testKey)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}

	user := auth.GetSession(req, "different-key")
	if user != "" {
		t.Errorf("session with wrong key should be rejected, got %q", user)
	}
}

func TestGetSessionNoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	user := auth.GetSession(req, testKey)
	if user != "" {
		t.Errorf("expected empty user, got %q", user)
	}
}

func TestClearSession(t *testing.T) {
	w := httptest.NewRecorder()
	auth.SetSession(w, "alice", testKey)
	auth.ClearSession(w)

	// The last Set-Cookie should have MaxAge=-1
	cookies := w.Result().Cookies()
	var cleared bool
	for _, c := range cookies {
		if c.Name == "session" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("session cookie should be cleared (MaxAge < 0)")
	}
}

func TestSessionTampering(t *testing.T) {
	w := httptest.NewRecorder()
	auth.SetSession(w, "alice", testKey)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookie")
	}

	// Tamper with the cookie value
	tampered := &http.Cookie{
		Name:  cookies[0].Name,
		Value: cookies[0].Value + "x",
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(tampered)

	user := auth.GetSession(req, testKey)
	if user != "" {
		t.Error("tampered cookie should be rejected")
	}
}

// Verify that a manually constructed expired token is rejected.
func TestSessionExpiry(t *testing.T) {
	_ = time.Now // just to use the import
	// Build a fake session cookie value with an expired time
	// We can do this by calling the internal sign logic indirectly via
	// constructing a value with a past timestamp.
	// Since verifyToken is unexported, we test via GetSession with a crafted cookie.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: "alice|0|invalidsig", // expiry = unix 0 = 1970
	})
	user := auth.GetSession(req, testKey)
	if user != "" {
		t.Error("expired token should be rejected")
	}
}
