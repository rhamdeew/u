package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const cookieName = "session"
const sessionDuration = 24 * time.Hour

// CheckPassword verifies a bcrypt password hash.
func CheckPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// HashPassword generates a bcrypt hash for a plaintext password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// SetSession writes a signed session cookie for the given username.
func SetSession(w http.ResponseWriter, username, key string) {
	expiry := time.Now().Add(sessionDuration).Unix()
	payload := fmt.Sprintf("%s|%d", username, expiry)
	sig := sign(payload, key)
	value := payload + "|" + sig

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearSession removes the session cookie.
func ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// GetSession returns the authenticated username, or "" if the session is invalid/expired.
func GetSession(r *http.Request, key string) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return verifyToken(c.Value, key)
}

// verifyToken validates format, HMAC, and expiry; returns username or "".
func verifyToken(value, key string) string {
	parts := strings.SplitN(value, "|", 3)
	if len(parts) != 3 {
		return ""
	}
	username, expiryStr, sig := parts[0], parts[1], parts[2]

	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return ""
	}

	payload := username + "|" + expiryStr
	if !hmac.Equal([]byte(sig), []byte(sign(payload, key))) {
		return ""
	}
	return username
}

func sign(payload, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
