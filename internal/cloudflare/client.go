package cloudflare

import (
	"context"
	"fmt"

	cf "github.com/cloudflare/cloudflare-go"
)

// Client wraps the Cloudflare API client
type Client struct {
	api       *cf.API
	accountID string
}

// NewClient creates a new Cloudflare API client using the API token from environment
func NewClient() (*Client, error) {
	auth, err := GetAuth()
	if err != nil {
		return nil, err
	}

	api, err := cf.NewWithAPIToken(auth.APIToken)
	if err != nil {
		return nil, fmt.Errorf("creating cloudflare client: %w", err)
	}

	return &Client{
		api:       api,
		accountID: auth.AccountID,
	}, nil
}

// NewClientWithToken creates a new Cloudflare API client with an explicit token
func NewClientWithToken(apiToken string) (*Client, error) {
	api, err := cf.NewWithAPIToken(apiToken)
	if err != nil {
		return nil, fmt.Errorf("creating cloudflare client: %w", err)
	}

	return &Client{
		api: api,
	}, nil
}

// SetAccountID sets the default account ID for API calls
func (c *Client) SetAccountID(accountID string) {
	c.accountID = accountID
}

// GetAccountID returns the current account ID
func (c *Client) GetAccountID() string {
	return c.accountID
}

// VerifyToken verifies that the API token is valid
func (c *Client) VerifyToken(ctx context.Context) error {
	_, err := c.api.VerifyAPIToken(ctx)
	if err != nil {
		return fmt.Errorf("verifying API token: %w", err)
	}
	return nil
}

// GetWorkerScript returns the script content for a Worker
func (c *Client) GetWorkerScript(ctx context.Context, accountID, scriptName string) (string, error) {
	resolvedAccountID, err := ResolveAccountID(accountID)
	if err != nil {
		resolvedAccountID = c.accountID
		if resolvedAccountID == "" {
			return "", ErrNoAccountID
		}
	}

	rc := cf.AccountIdentifier(resolvedAccountID)
	resp, err := c.api.GetWorker(ctx, rc, scriptName)
	if err != nil {
		return "", fmt.Errorf("getting worker script: %w", err)
	}

	return resp.Script, nil
}

// API returns the underlying Cloudflare API client for advanced usage
func (c *Client) API() *cf.API {
	return c.api
}
