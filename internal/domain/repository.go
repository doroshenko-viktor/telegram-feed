package domain

import "context"

// Repository defines the complete storage interface for channels, posts, digests, and settings.
type Repository interface {
	// Channel operations
	UpsertChannels(ctx context.Context, channels []*Channel) error
	ListChannels(ctx context.Context, trackedOnly bool) ([]*Channel, error)
	ListMCPAllowedChannels(ctx context.Context) ([]*Channel, error)
	GetChannel(ctx context.Context, id int64) (*Channel, error)
	SetTracking(ctx context.Context, channelIDs []int64, tracked bool) error
	SetMCPAllowed(ctx context.Context, channelIDs []int64, allowed bool) error
	SetFilterAds(ctx context.Context, channelIDs []int64, filterAds bool) error
	UpdateChannelProfile(ctx context.Context, id int64, summary string, topics []string, contentType string) error
	UpdateWatermark(ctx context.Context, channelID int64, lastMsgID int) error

	// Post operations
	UpsertPosts(ctx context.Context, posts []*Post) error
	GetPosts(ctx context.Context, filter PostFilter) ([]*Post, error)
	SearchPosts(ctx context.Context, query string, mcpOnly bool, limit int) ([]*Post, error)
	MarkPostsRead(ctx context.Context, postIDs []int64) error

	// Digest operations
	SaveDigest(ctx context.Context, digest *Digest) error
	GetLatestDigest(ctx context.Context, topic string, mcpOnly bool) (*Digest, error)
	ListDigests(ctx context.Context, topic string, mcpOnly bool, limit int) ([]*Digest, error)

	// Session operations (stored in DB instead of disk files)
	LoadSession(ctx context.Context, id string) ([]byte, error)
	StoreSession(ctx context.Context, id string, data []byte) error

	// Ad Pattern operations (dynamic DB-driven rules)
	ListAdPatterns(ctx context.Context, enabledOnly bool) ([]*AdPattern, error)
	AddAdPattern(ctx context.Context, pattern *AdPattern) error
	DeleteAdPattern(ctx context.Context, id int64) error
	ToggleAdPattern(ctx context.Context, id int64, enabled bool) error

	// Settings operations
	GetSetting(ctx context.Context, key string, defaultValue string) (string, error)
	SetSetting(ctx context.Context, key string, value string) error
	GetAllSettings(ctx context.Context) (map[string]string, error)

	Close() error
}
