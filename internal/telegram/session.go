package telegram

import (
	"context"

	"github.com/gotd/td/session"

	"telegram_feed_summarizer/internal/domain"
)

// DBSessionStorage implements gotd/td's session.Storage interface directly on domain.Repository.
// This eliminates the need for any session files on disk.
type DBSessionStorage struct {
	repo domain.Repository
	id   string
}

// NewDBSessionStorage creates a new database-backed Telegram session storage adapter.
func NewDBSessionStorage(repo domain.Repository, sessionID string) *DBSessionStorage {
	if sessionID == "" {
		sessionID = "default"
	}
	return &DBSessionStorage{
		repo: repo,
		id:   sessionID,
	}
}

// LoadSession loads the serialized MTProto session from the database.
func (s *DBSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	data, err := s.repo.LoadSession(ctx, s.id)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, session.ErrNotFound
	}
	return data, nil
}

// StoreSession stores the serialized MTProto session into the database.
func (s *DBSessionStorage) StoreSession(ctx context.Context, data []byte) error {
	return s.repo.StoreSession(ctx, s.id, data)
}
