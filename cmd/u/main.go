package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"text/template"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"u/internal/admin"
	"u/internal/auth"
	"u/internal/config"
	"u/internal/db"
	"u/internal/redirect"
	"u/internal/web"
)

func main() {
	// hashpw subcommand: print bcrypt hash of a password
	if len(os.Args) == 3 && os.Args[1] == "hashpw" {
		hash, err := auth.HashPassword(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(hash)
		return
	}

	// genconfig subcommand: generate a config.yaml
	if len(os.Args) >= 2 && os.Args[1] == "genconfig" {
		runGenconfig(os.Args[2:])
		return
	}

	cfgPath := "data/config.yaml"
	if len(os.Args) >= 2 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	tmpls, err := web.ParseTemplates()
	if err != nil {
		log.Fatalf("templates: %v", err)
	}

	adminHandler := admin.New(database, cfg, tmpls)
	redirectHandler := redirect.New(database, cfg.CookieKey)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Static assets (CSS, etc.)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS()))))

	// Admin panel
	r.Mount("/admin", adminHandler.Routes())

	// Public redirect — catches everything else (GET + HEAD)
	r.Handle("/*", redirectHandler)

	log.Printf("Starting server on %s  site=%s", cfg.Addr, cfg.SiteURL)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		log.Fatal(err)
	}
}

const configTemplate = `site_url:   "{{.SiteURL}}"
db_path:    "{{.DBPath}}"
cookie_key: "{{.CookieKey}}"
addr:       "{{.Addr}}"
debug:      false

admin:
  username: "{{.AdminUser}}"
  # Password is NOT stored here — pass it via U_ADMIN_PASSWORD env var.
  # To pre-hash: make hashpw PASSWORD=yourpassword
`

func runGenconfig(args []string) {
	fs := flag.NewFlagSet("genconfig", flag.ExitOnError)
	siteURL := fs.String("site-url", "http://localhost:8080", "public base URL")
	dbPath := fs.String("db", "data/u.db", "SQLite database path")
	addr := fs.String("addr", ":8080", "listen address")
	adminUser := fs.String("admin-user", "admin", "admin username")
	output := fs.String("output", "data/config.yaml", "output file path")
	_ = fs.Parse(args)

	// Refuse to overwrite an existing config
	if _, err := os.Stat(*output); err == nil {
		log.Fatalf("genconfig: %s already exists; remove it first", *output)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		log.Fatalf("genconfig: generating cookie_key: %v", err)
	}

	data := struct {
		SiteURL   string
		DBPath    string
		CookieKey string
		Addr      string
		AdminUser string
	}{
		SiteURL:   *siteURL,
		DBPath:    *dbPath,
		CookieKey: hex.EncodeToString(key),
		Addr:      *addr,
		AdminUser: *adminUser,
	}

	f, err := os.Create(*output)
	if err != nil {
		log.Fatalf("genconfig: %v", err)
	}
	defer f.Close()

	tmpl := template.Must(template.New("cfg").Parse(configTemplate))
	if err := tmpl.Execute(f, data); err != nil {
		log.Fatalf("genconfig: %v", err)
	}

	fmt.Printf("✓ Config written to %s\n", *output)
	fmt.Println("  Set the admin password:")
	fmt.Println("    U_ADMIN_PASSWORD=yourpassword ./u")
	fmt.Println("  or pre-hash it with: make hashpw PASSWORD=yourpassword")
}
