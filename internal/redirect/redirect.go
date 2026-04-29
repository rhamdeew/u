package redirect

import (
	"net/http"
	"time"

	"u/internal/auth"
	"u/internal/db"
)

type Handler struct {
	db  *db.DB
	key string
}

func New(database *db.DB, cookieKey string) *Handler {
	return &Handler{db: database, key: cookieKey}
}

// ServeHTTP handles GET /{keyword}: redirects to the long URL or returns 404.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Strip leading slash
	keyword := r.URL.Path[1:]
	if keyword == "" {
		// Root — redirect to admin if logged in, otherwise login
		if user := auth.GetSession(r, h.key); user != "" {
			http.Redirect(w, r, "/admin", http.StatusFound)
		} else {
			http.Redirect(w, r, "/admin/login", http.StatusFound)
		}
		return
	}

	link, err := h.db.GetByKeyword(keyword)
	if err != nil || link == nil {
		http.NotFound(w, r)
		return
	}

	// Log the click asynchronously so we don't delay the redirect
	go func() {
		_ = h.db.InsertClick(db.ClickRecord{
			Keyword:   keyword,
			ClickedAt: time.Now(),
			Referrer:  r.Referer(),
			UserAgent: r.UserAgent(),
			IP:        clientIP(r),
		})
		_ = h.db.IncrClicks(keyword)
	}()

	http.Redirect(w, r, link.URL, http.StatusMovedPermanently)
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// Strip port from RemoteAddr
	addr := r.RemoteAddr
	if i := len(addr) - 1; i >= 0 {
		for i >= 0 && addr[i] != ':' {
			i--
		}
		if i > 0 {
			return addr[:i]
		}
	}
	return addr
}
