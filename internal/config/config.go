package config

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

type Admin struct {
	Username string `yaml:"username"`
	// Password is a bcrypt hash. Leave empty in config.yaml and set
	// U_ADMIN_PASSWORD env var instead (plaintext — hashed at startup).
	Password string `yaml:"password,omitempty"`
}

type Config struct {
	SiteURL   string `yaml:"site_url"`
	DBPath    string `yaml:"db_path"`
	CookieKey string `yaml:"cookie_key"`
	Addr      string `yaml:"addr"`
	Debug     bool   `yaml:"debug"`
	Admin     Admin  `yaml:"admin"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		SiteURL: "http://localhost:8080",
		DBPath:  "./u.db",
		Addr:    ":8080",
	}

	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	}

	if err := applyEnv(cfg); err != nil {
		return nil, err
	}

	cfg.SiteURL = strings.TrimRight(cfg.SiteURL, "/")

	if cfg.CookieKey == "" {
		return nil, fmt.Errorf("cookie_key is required in config or U_COOKIE_KEY env var")
	}
	if cfg.Admin.Username == "" || cfg.Admin.Password == "" {
		return nil, fmt.Errorf("admin.username and admin.password are required")
	}

	return cfg, nil
}

func applyEnv(cfg *Config) error {
	if v := os.Getenv("U_SITE_URL"); v != "" {
		cfg.SiteURL = v
	}
	if v := os.Getenv("U_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("U_COOKIE_KEY"); v != "" {
		cfg.CookieKey = v
	}
	if v := os.Getenv("U_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("U_ADMIN_USERNAME"); v != "" {
		cfg.Admin.Username = v
	}
	if v := os.Getenv("U_ADMIN_PASSWORD"); v != "" {
		// Accept either a pre-hashed bcrypt string or a plaintext password.
		if strings.HasPrefix(v, "$2a$") || strings.HasPrefix(v, "$2b$") {
			cfg.Admin.Password = v
		} else {
			hash, err := bcrypt.GenerateFromPassword([]byte(v), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hashing U_ADMIN_PASSWORD: %w", err)
			}
			cfg.Admin.Password = string(hash)
		}
	}
	return nil
}
