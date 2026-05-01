package admin

import (
	"encoding/json"
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
	Login      *template.Template
	Admin      *template.Template
	Edit       *template.Template
	Stats      *template.Template
	Dashboard  *template.Template
	Categories *template.Template
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
		r.Get("/links/{keyword}/stats/{date}/details", h.getClickDetails)
		r.Get("/dashboard", h.getDashboard)
		r.Get("/categories", h.getCategories)
		r.Post("/categories", h.postCreateCategory)
		r.Post("/categories/{id}/delete", h.postDeleteCategory)
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
		Search:     strings.TrimSpace(q.Get("search")),
		SortBy:     q.Get("sort"),
		SortDesc:   q.Get("dir") == "desc",
		Page:       intParam(q, "page", 1),
		PerPage:    intParam(q, "per_page", 20),
		CategoryID: intParam(q, "category", 0),
	}

	result, err := h.db.List(opts)
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	totalLinks, totalClicks, _ := h.db.TotalStats()
	cats, _ := h.db.ListCategories()

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
		Categories  []db.Category
		CategoryID  int
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
		Categories:  cats,
		CategoryID:  opts.CategoryID,
	})
}

// ── Create link ────────────────────────────────────────────────────────────────

func (h *Handler) postCreateLink(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.FormValue("url"))
	keyword := strings.TrimSpace(r.FormValue("keyword"))
	title := strings.TrimSpace(r.FormValue("title"))
	categoryID := intParam(r.Form, "category", 0)

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
		Keyword:    keyword,
		URL:        rawURL,
		Title:      title,
		IP:         clientIP(r),
		CategoryID: categoryID,
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

	cats, _ := h.db.ListCategories()

	type pageData struct {
		SiteURL    string
		Link       *db.Link
		Error      string
		Flash      flash
		Categories []db.Category
	}
	h.tmpl.Edit.ExecuteTemplate(w, "base", pageData{
		SiteURL:    h.cfg.SiteURL,
		Link:       link,
		Flash:      flashFromQuery(r.URL.Query()),
		Categories: cats,
	})
}

func (h *Handler) postEditLink(w http.ResponseWriter, r *http.Request) {
	oldKeyword := chi.URLParam(r, "keyword")
	newKeyword := strings.TrimSpace(r.FormValue("keyword"))
	rawURL := strings.TrimSpace(r.FormValue("url"))
	title := strings.TrimSpace(r.FormValue("title"))
	categoryID := intParam(r.Form, "category", 0)

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

	if err := h.db.Update(oldKeyword, newKeyword, rawURL, title, categoryID); err != nil {
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

// getClickDetails returns individual click records for a keyword+date as JSON.
func (h *Handler) getClickDetails(w http.ResponseWriter, r *http.Request) {
	keyword := chi.URLParam(r, "keyword")
	date := chi.URLParam(r, "date")

	if _, err := time.Parse("2006-01-02", date); err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}

	link, err := h.db.GetByKeyword(keyword)
	if err != nil || link == nil {
		http.NotFound(w, r)
		return
	}

	clicks, err := h.db.DayClickDetails(keyword, date)
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type clickJSON struct {
		ClickedAt string `json:"clicked_at"`
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
		Referrer  string `json:"referrer"`
	}
	result := make([]clickJSON, len(clicks))
	for i, c := range clicks {
		result[i] = clickJSON{
			ClickedAt: c.ClickedAt.Format("2006-01-02 15:04:05"),
			IP:        c.IP,
			UserAgent: c.UserAgent,
			Referrer:  c.Referrer,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ── Dashboard ──────────────────────────────────────────────────────────────────

func (h *Handler) getDashboard(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	type periodDef struct {
		Label string
		From  time.Time
		To    time.Time
	}
	defs := []periodDef{
		{"Today", todayStart, now},
		{"Yesterday", todayStart.AddDate(0, 0, -1), todayStart},
		{"Last 7 days", todayStart.AddDate(0, 0, -7), now},
		{"Last 30 days", todayStart.AddDate(0, 0, -30), now},
	}

	type periodData struct {
		Label     string
		Clicks    int
		LinkCount int
		Links     []db.PeriodStat
	}

	var periods []periodData
	for _, p := range defs {
		clicks, linkCount, err := h.db.PeriodTotals(p.From, p.To)
		if err != nil {
			http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		stats, err := h.db.PeriodStats(p.From, p.To)
		if err != nil {
			http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		periods = append(periods, periodData{
			Label:     p.Label,
			Clicks:    clicks,
			LinkCount: linkCount,
			Links:     stats,
		})
	}

	type pageData struct {
		SiteURL string
		Periods []periodData
	}
	h.tmpl.Dashboard.ExecuteTemplate(w, "base", pageData{
		SiteURL: h.cfg.SiteURL,
		Periods: periods,
	})
}

// ── Categories ─────────────────────────────────────────────────────────────────

func (h *Handler) getCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.db.ListCategories()
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	type pageData struct {
		SiteURL    string
		Categories []db.Category
		Flash      flash
	}
	h.tmpl.Categories.ExecuteTemplate(w, "base", pageData{
		SiteURL:    h.cfg.SiteURL,
		Categories: cats,
		Flash:      flashFromQuery(r.URL.Query()),
	})
}

func (h *Handler) postCreateCategory(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		h.redirectWithFlash(w, r, "/admin/categories", "error", "Name is required")
		return
	}
	if _, err := h.db.CreateCategory(name); err != nil {
		h.redirectWithFlash(w, r, "/admin/categories", "error", "Could not create category: "+err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/admin/categories", "success", "Category '"+name+"' created")
}

func (h *Handler) postDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id := 0
	for _, c := range chi.URLParam(r, "id") {
		if c < '0' || c > '9' {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		id = id*10 + int(c-'0')
	}
	if err := h.db.DeleteCategory(id); err != nil {
		h.redirectWithFlash(w, r, "/admin/categories", "error", "Could not delete: "+err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/admin/categories", "success", "Category deleted")
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
			runes := []rune(s)
			if len(runes) <= n {
				return s
			}
			return string(runes[:n-1]) + "…"
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
