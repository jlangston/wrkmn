package cloudflare

import (
	"errors"
	"os"
)

const (
	// EnvAPIToken is the environment variable for Cloudflare API token
	EnvAPIToken = "CLOUDFLARE_API_TOKEN"

	// EnvAccountID is the environment variable for default Cloudflare account ID
	EnvAccountID = "CLOUDFLARE_ACCOUNT_ID"
)

var (
	// ErrNoAPIToken is returned when CLOUDFLARE_API_TOKEN is not set
	ErrNoAPIToken = errors.New("CLOUDFLARE_API_TOKEN environment variable not set")

	// ErrNoAccountID is returned when account ID is not available
	ErrNoAccountID = errors.New("no Cloudflare account ID available")
)

// Auth holds Cloudflare authentication credentials
type Auth struct {
	APIToken  string
	AccountID string
}

// GetAuth returns authentication from environment variables
func GetAuth() (*Auth, error) {
	token := os.Getenv(EnvAPIToken)
	if token == "" {
		return nil, ErrNoAPIToken
	}

	return &Auth{
		APIToken:  token,
		AccountID: os.Getenv(EnvAccountID),
	}, nil
}

// IsConfigured returns true if Cloudflare API token is available
func IsConfigured() bool {
	return os.Getenv(EnvAPIToken) != ""
}

// GetAPIToken returns the API token from environment or error if not set
func GetAPIToken() (string, error) {
	token := os.Getenv(EnvAPIToken)
	if token == "" {
		return "", ErrNoAPIToken
	}
	return token, nil
}

// GetDefaultAccountID returns the default account ID from environment
func GetDefaultAccountID() string {
	return os.Getenv(EnvAccountID)
}

// ResolveAccountID returns the account ID to use, preferring the provided value
// over the environment variable default
func ResolveAccountID(accountID string) (string, error) {
	if accountID != "" {
		return accountID, nil
	}

	if envAccountID := GetDefaultAccountID(); envAccountID != "" {
		return envAccountID, nil
	}

	return "", ErrNoAccountID
}
