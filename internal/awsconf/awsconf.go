// Package awsconf reads SSO profiles from the AWS CLI config file.
package awsconf

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/ini.v1"
)

type Profile struct {
	Name      string
	AccountID string
	Role      string
	Region    string
	// SessionKey names the SSO token cache entry: the sso_session name for
	// `[sso-session X]` configs, else the legacy sso_start_url.
	SessionKey string
}

// Path honours AWS_CONFIG_FILE like the CLI does.
func Path() string {
	if p := os.Getenv("AWS_CONFIG_FILE"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "config")
}

// Load returns the SSO-backed profiles in file order; others are skipped.
func Load(path string) ([]Profile, error) {
	f, err := ini.LoadSources(ini.LoadOptions{Insensitive: false, AllowNonUniqueSections: false}, path)
	if err != nil {
		return nil, err
	}
	var out []Profile
	for _, s := range f.Sections() {
		name, ok := profileName(s.Name())
		if !ok {
			continue
		}
		p := Profile{
			Name:      name,
			AccountID: s.Key("sso_account_id").String(),
			Role:      s.Key("sso_role_name").String(),
			Region:    s.Key("region").String(),
		}
		if sess := s.Key("sso_session").String(); sess != "" {
			p.SessionKey = sess
		} else {
			p.SessionKey = s.Key("sso_start_url").String()
		}
		if p.SessionKey == "" || p.AccountID == "" {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func profileName(section string) (string, bool) {
	if section == "default" {
		return section, true
	}
	if n, ok := strings.CutPrefix(section, "profile "); ok {
		return strings.TrimSpace(n), true
	}
	return "", false
}
