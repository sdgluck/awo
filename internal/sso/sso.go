// Package sso reads token expiry from the AWS CLI's SSO cache, offline.
package sso

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func CacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "sso", "cache")
}

// Expiry returns when the cached token for sessionKey (an sso_session name or
// legacy start URL) expires. The CLI names the file sha1(key).json.
func Expiry(dir, sessionKey string) (time.Time, error) {
	sum := sha1.Sum([]byte(sessionKey))
	b, err := os.ReadFile(filepath.Join(dir, hex.EncodeToString(sum[:])+".json"))
	if err != nil {
		return time.Time{}, err
	}
	var tok struct {
		ExpiresAt string `json:"expiresAt"`
	}
	if err := json.Unmarshal(b, &tok); err != nil {
		return time.Time{}, err
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05UTC"} {
		if t, err := time.Parse(layout, tok.ExpiresAt); err == nil {
			return t, nil
		}
	}
	return time.Time{}, &time.ParseError{Value: tok.ExpiresAt, Message: ": unrecognised expiresAt"}
}
