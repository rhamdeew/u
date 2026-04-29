package integration_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"u/internal/admin"
	"u/internal/auth"
	"u/internal/config"
	"u/internal/db"
	"u/internal/redirect"
	"u/internal/web"
)

func setupServer(t *testing.T) (*httptest.Server, *db.DB) {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	database, err := db.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	pw, _ := auth.HashPassword("testpass")
	cfg := &config.Config{
		SiteURL:   "http://example.com",
		CookieKey: "test-cookie-key-for-integration",
		Admin: config.Admin{
			Username: "admin",
			Password: pw,
		},
	}

	tmpls, err := web.ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	adminHandler := admin.New(database, cfg, tmpls)
	redirectHandler := redirect.New(database, cfg.CookieKey)

	r := chi.NewRouter()
	r.Mount("/admin", adminHandler.Routes())
	r.Handle("/*", redirectHandler)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, database
}

// login performs a login POST and returns the session cookie.
func login(t *testing.T, srv *httptest.Server) *http.Cookie {
	t.Helper()
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.PostForm(srv.URL+"/admin/login", url.Values{
		"username": {"admin"},
		"password": {"testpass"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	for _, c := range resp.Cookies() {
		if c.Name == "session" {
			return c
		}
	}
	t.Fatal("session cookie not set after login")
	return nil
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestRedirectKnownKeyword(t *testing.T) {
	srv, database := setupServer(t)

	_ = database.Insert(&db.Link{Keyword: "go", URL: "https://golang.org"})

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(srv.URL + "/go")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("expected 301, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://golang.org" {
		t.Errorf("expected Location=https://golang.org, got %q", loc)
	}
}

func TestRedirectUnknownKeyword(t *testing.T) {
	srv, _ := setupServer(t)

	resp, err := http.Get(srv.URL + "/doesnotexist")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestLoginSuccess(t *testing.T) {
	srv, _ := setupServer(t)
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.PostForm(srv.URL+"/admin/login", url.Values{
		"username": {"admin"},
		"password": {"testpass"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin" {
		t.Errorf("expected redirect to /admin, got %q", loc)
	}
}

func TestLoginFailure(t *testing.T) {
	srv, _ := setupServer(t)

	resp, err := http.PostForm(srv.URL+"/admin/login", url.Values{
		"username": {"admin"},
		"password": {"wrongpass"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (re-render login), got %d", resp.StatusCode)
	}
}

func TestAdminRequiresAuth(t *testing.T) {
	srv, _ := setupServer(t)
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(srv.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected redirect to login (303), got %d", resp.StatusCode)
	}
}

func TestAdminCreateAndDeleteLink(t *testing.T) {
	srv, database := setupServer(t)
	session := login(t, srv)

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// Create
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/links",
		strings.NewReader(url.Values{
			"url":     {"https://go.dev"},
			"keyword": {"godev"},
			"title":   {"Go Dev"},
		}.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(session)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("create: expected 303, got %d", resp.StatusCode)
	}

	link, _ := database.GetByKeyword("godev")
	if link == nil || link.URL != "https://go.dev" {
		t.Fatal("link was not saved to database")
	}

	// Delete
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/links/godev/delete", nil)
	req2.AddCookie(session)
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	deleted, _ := database.GetByKeyword("godev")
	if deleted != nil {
		t.Error("link should have been deleted")
	}
}

func TestAdminEditLink(t *testing.T) {
	srv, database := setupServer(t)
	session := login(t, srv)

	_ = database.Insert(&db.Link{Keyword: "edit-me", URL: "https://old.com", Title: "Old"})

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/links/edit-me/edit",
		strings.NewReader(url.Values{
			"keyword": {"edit-me"},
			"url":     {"https://new.com"},
			"title":   {"New Title"},
		}.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(session)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	updated, _ := database.GetByKeyword("edit-me")
	if updated == nil || updated.URL != "https://new.com" {
		t.Errorf("link URL was not updated, got: %v", updated)
	}
}
