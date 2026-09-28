// Package secrets stores API keys outside the config file: in the desktop keyring
// (Secret Service, which is KWallet on Plasma) or, on servers, in environment variables.
package secrets

import (
	"errors"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const service = "hansei"

// ErrNone means no key is stored.
var ErrNone = errors.New("secrets: no key stored")

// envName maps a provider name to its variable, e.g. "claude" → HANSEI_KEY_CLAUDE.
func envName(provider string) string {
	up := strings.ToUpper(provider)
	return "HANSEI_KEY_" + strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, up)
}

// Get returns the key for a provider: environment first, then the keyring.
func Get(provider string) (string, error) {
	if v := os.Getenv(envName(provider)); v != "" {
		return v, nil
	}
	v, err := keyring.Get(service, provider)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNone
	}
	return v, err
}

// Set stores a key in the keyring; an empty key deletes it.
func Set(provider, key string) error {
	if key == "" {
		err := keyring.Delete(service, provider)
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return err
	}
	return keyring.Set(service, provider, key)
}

// Has reports whether a key is available without returning it.
func Has(provider string) bool {
	_, err := Get(provider)
	return err == nil
}
