package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"

	"telegram_feed_summarizer/internal/config"
	"telegram_feed_summarizer/internal/domain"
)

// Client wraps gotd/td MTProto client with database-backed session storage.
type Client struct {
	client     *telegram.Client
	appID      int
	appHash    string
	sessionDB  session.Storage
	configured bool
}

// NewClient initializes a new Telegram MTProto client backed by database session storage.
func NewClient(cfg config.TelegramConfig, repo domain.Repository) (*Client, error) {
	if cfg.AppID == 0 || cfg.AppHash == "" {
		return &Client{configured: false}, nil
	}

	sessionStorage := NewDBSessionStorage(repo, "default")

	client := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: sessionStorage,
		Middlewares: []telegram.Middleware{
			floodwait.NewSimpleWaiter().WithMaxWait(60 * time.Second),
		},
	})

	return &Client{
		client:     client,
		appID:      cfg.AppID,
		appHash:    cfg.AppHash,
		sessionDB:  sessionStorage,
		configured: true,
	}, nil
}

// IsConfigured returns true if Telegram App ID and Hash are set.
func (c *Client) IsConfigured() bool {
	return c != nil && c.configured
}

// RawClient returns the underlying gotd/td Client pointer.
func (c *Client) RawClient() *telegram.Client {
	return c.client
}

// Run connects to the Telegram MTProto network and invokes the user-provided callback within the connection context.
func (c *Client) Run(ctx context.Context, f func(ctx context.Context) error) error {
	if !c.IsConfigured() {
		return fmt.Errorf("telegram credentials not configured (TELEGRAM_APP_ID and TELEGRAM_APP_HASH required)")
	}
	return c.client.Run(ctx, f)
}
