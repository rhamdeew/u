package admin

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"u/internal/auth"
	"u/internal/config"
	"u/internal/db"
)

// Handler holds dependencies for all admin HTTP handlers.
type Handler struct {
	db   *db.DB
	cfg  *config.Config
	tmpl *Templates
}

// Templates holds pre-parsed template sets for each page.
type Templates struct {
	Login *template.Template
	Admin *template.Template
	Edit  *template.Template
	Stats *template.Template
}

func New(database *db.DB, cfg *config.Config, tmpl *Templates) *Handler {
	return &Handler{db: database, cfg: cfg, tmpl: tmpl}
}

// Routes returns a chi.Router with all admin sub-routes mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/login", h.getLogin)
	r.Post("/login", h.postLogin)
	r.Post("/logout", h.postLogout)

	// All routes below require authentication
	r.Group(func(r chi.Router) {
		r.Use(h.requireAuth)
		r.Get("/", h.getIndex)
		r.Post("/links", h.postCreateLink)
		r.Get("/links/{keyword}/edit", h.getEditLink)
		r.Post("/links/{keyword}/edit", h.postEditLink)
		r.Post("/links/{keyword}/delete", h.postDeleteLink)
		r.Get("/links/{keyword}/stats", h.getStats)
	})

	return r
}

// ── Auth middleware ────────────────────────────────────────────────────────────

func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.GetSession(r, h.cfg.CookieKey) == "" {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── Login / logout ─────────────────────────────────────────────────────────────

func (h *Handler) getLogin(w http.ResponseWriter, r *http.Request) {
	if auth.GetSession(r, h.cfg.CookieKey) != "" {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	h.renderLogin(w, "")
}

func (h *Handler) postLogin(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	if username != h.cfg.Admin.Username || !auth.CheckPassword(h.cfg.Admin.Password, password) {
		h.renderLogin(w, "Invalid username or password")
		return
	}

	auth.SetSession(w, username, h.cfg.CookieKey)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *Handler) postLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearSession(w)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// ── Index (link list) ──────────────────────────────────────────────────────────

func (h *Handler) getIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := db.ListOpts{
		Search:   strings.TrimSpace(q.Get("search")),
		SortBy:   q.Get("sort"),
		SortDesc: q.Get("dir") == "desc",
		Page:     intParam(q, "page", 1),
		PerPage:  intParam(q, "per_page", 20),
	}

	result, err := h.db.List(opts)
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	totalLinks, totalClicks, _ := h.db.TotalStats()

	type pageData struct {
		SiteURL     string
		Links       []db.Link
		Total       int
		TotalPages  int
		Page        int
		PerPage     int
		Search      string
		SortBy      string
		SortDesc    bool
		TotalLinks  int
		TotalClicks int
		Flash       flash
	}

	h.tmpl.Admin.ExecuteTemplate(w, "base", pageData{
		SiteURL:     h.cfg.SiteURL,
		Links:       result.Links,
		Total:       result.Total,
		TotalPages:  result.TotalPages,
		Page:        opts.Page,
		PerPage:     opts.PerPage,
		Search:      opts.Search,
		SortBy:      opts.SortBy,
		SortDesc:    opts.SortDesc,
		TotalLinks:  totalLinks,
		TotalClicks: totalClicks,
		Flash:       flashFromQuery(q),
	})
}

// ── Create link ────────────────────────────────────────────────────────────────

func (h *Handler) postCreateLink(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.FormValue("url"))
	keyword := strings.TrimSpace(r.FormValue("keyword"))
	title := strings.TrimSpace(r.FormValue("title"))

	if rawURL == "" {
		h.redirectWithFlash(w, r, "/admin", "error", "URL is required")
		return
	}
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		h.redirectWithFlash(w, r, "/admin", "error", "Invalid URL")
		return
	}

	// Check for duplicate keyword
	if keyword != "" {
		existing, _ := h.db.GetByKeyword(keyword)
		if existing != nil {
			h.redirectWithFlash(w, r, "/admin", "error", "Keyword '"+keyword+"' already exists")
			return
		}
	}

	link := &db.Link{
		Keyword: keyword,
		URL:     rawURL,
		Title:   title,
		IP:      clientIP(r),
	}
	if err := h.db.Insert(link); err != nil {
		h.redirectWithFlash(w, r, "/admin", "error", "Could not save link: "+err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/admin", "success", "Link added: "+h.cfg.SiteURL+"/"+link.Keyword)
}

// ── Edit link ──────────────────────────────────────────────────────────────────

func (h *Handler) getEditLink(w http.ResponseWriter, r *http.Request) {
	keyword := chi.URLParam(r, "keyword")
	link, err := h.db.GetByKeyword(keyword)
	if err != nil || link == nil {
		http.NotFound(w, r)
		return
	}

	type pageData struct {
		SiteURL string
		Link    *db.Link
		Error   string
		Flash   flash
	}
	h.tmpl.Edit.ExecuteTemplate(w, "base", pageData{
		SiteURL: h.cfg.SiteURL,
		Link:    link,
		Flash:   flashFromQuery(r.URL.Query()),
	})
}

func (h *Handler) postEditLink(w http.ResponseWriter, r *http.Request) {
	oldKeyword := chi.URLParam(r, "keyword")
	newKeyword := strings.TrimSpace(r.FormValue("keyword"))
	rawURL := strings.TrimSpace(r.FormValue("url"))
	title := strings.TrimSpace(r.FormValue("title"))

	if rawURL == "" {
		h.redirectWithFlash(w, r, "/admin/links/"+oldKeyword+"/edit", "error", "URL is required")
		return
	}

	// If keyword is changing, ensure the new one is free
	if newKeyword != "" && newKeyword != oldKeyword {
		existing, _ := h.db.GetByKeyword(newKeyword)
		if existing != nil {
			h.redirectWithFlash(w, r, "/admin/links/"+oldKeyword+"/edit", "error", "Keyword '"+newKeyword+"' already exists")
			return
		}
	}

	if err := h.db.Update(oldKeyword, newKeyword, rawURL, title); err != nil {
		h.redirectWithFlash(w, r, "/admin/links/"+oldKeyword+"/edit", "error", "Could not update: "+err.Error())
		return
	}

	dest := newKeyword
	if dest == "" {
		dest = oldKeyword
	}
	h.redirectWithFlash(w, r, "/admin", "success", "Link '"+dest+"' updated")
}

// ── Delete link ────────────────────────────────────────────────────────────────

func (h *Handler) postDeleteLink(w http.ResponseWriter, r *http.Request) {
	keyword := chi.URLParam(r, "keyword")
	if err := h.db.Delete(keyword); err != nil {
		h.redirectWithFlash(w, r, "/admin", "error", "Could not delete: "+err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/admin", "success", "Link '"+keyword+"' deleted")
}

// ── Stats ──────────────────────────────────────────────────────────────────────

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	keyword := chi.URLParam(r, "keyword")
	link, err := h.db.GetByKeyword(keyword)
	if err != nil || link == nil {
		http.NotFound(w, r)
		return
	}

	dayStats, err := h.db.DayStats(keyword)
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type pageData struct {
		SiteURL  string
		Link     *db.Link
		DayStats []db.DayStat
	}
	h.tmpl.Stats.ExecuteTemplate(w, "base", pageData{
		SiteURL:  h.cfg.SiteURL,
		Link:     link,
		DayStats: dayStats,
	})
}

// ── Render helpers ─────────────────────────────────────────────────────────────

type flash struct {
	Kind    string
	Message string
}

func (h *Handler) renderLogin(w http.ResponseWriter, errMsg string) {
	type pageData struct {
		Error string
	}
	h.tmpl.Login.ExecuteTemplate(w, "base", pageData{Error: errMsg})
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, dest, kind, msg string) {
	u, _ := url.Parse(dest)
	q := u.Query()
	q.Set("flash_kind", kind)
	q.Set("flash_msg", msg)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

func flashFromQuery(q url.Values) flash {
	return flash{Kind: q.Get("flash_kind"), Message: q.Get("flash_msg")}
}

// ── Utilities ──────────────────────────────────────────────────────────────────

func intParam(q url.Values, key string, def int) int {
	v := q.Get(key)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

// TemplateFuncMap returns the functions available to all templates.
func TemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.Format("2006-01-02 15:04")
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n-1] + "…"
		},
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i + 1
			}
			return s
		},
	}
}
