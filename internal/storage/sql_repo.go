package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"telegram_feed_summarizer/internal/domain"
)

// SQLRepository implements domain.Repository across both SQLite and PostgreSQL.
type SQLRepository struct {
	db     *sql.DB
	driver string
}

// NewSQLRepository creates a new repository instance wrapping an open *sql.DB handle.
func NewSQLRepository(db *sql.DB, driver string) *SQLRepository {
	return &SQLRepository{
		db:     db,
		driver: driver,
	}
}

// rebind replaces positional '?' parameter markers with '$1, $2, ...' for PostgreSQL.
func (r *SQLRepository) rebind(query string) string {
	if r.driver != "postgres" {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 16)
	paramIdx := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			fmt.Fprintf(&b, "$%d", paramIdx)
			paramIdx++
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

// Close closes the underlying database handle.
func (r *SQLRepository) Close() error {
	return r.db.Close()
}

// --- Channel Operations ---

func (r *SQLRepository) UpsertChannels(ctx context.Context, channels []*ChannelInput) error {
	if len(channels) == 0 {
		return nil
	}

	query := `
		INSERT INTO channels (
			id, access_hash, title, username, about, is_tracked, mcp_allowed, filter_ads,
			summary, topics, content_type, language, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			access_hash = excluded.access_hash,
			title = excluded.title,
			username = excluded.username,
			about = excluded.about,
			updated_at = CURRENT_TIMESTAMP
	`
	rebound := r.rebind(query)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, rebound)
	if err != nil {
		return fmt.Errorf("preparing upsert channels statement: %w", err)
	}
	defer stmt.Close()

	for _, ch := range channels {
		topicsJSON := ch.TopicsJSON()
		_, err := stmt.ExecContext(ctx,
			ch.ID, ch.AccessHash, ch.Title, ch.Username, ch.About,
			ch.IsTracked, ch.MCPAllowed, ch.FilterAds,
			ch.Summary, topicsJSON, ch.ContentType, ch.Language,
		)
		if err != nil {
			return fmt.Errorf("upserting channel %d (%s): %w", ch.ID, ch.Title, err)
		}
	}

	return tx.Commit()
}

type ChannelInput = domain.Channel

func (r *SQLRepository) ListChannels(ctx context.Context, trackedOnly bool) ([]*domain.Channel, error) {
	query := `
		SELECT id, access_hash, title, username, about, is_tracked, mcp_allowed, filter_ads,
		       summary, topics, content_type, language, last_sync_msg_id, profiled_at,
		       created_at, updated_at
		FROM channels
	`
	var args []any
	if trackedOnly {
		query += " WHERE is_tracked = ?"
		args = append(args, true)
	}
	query += " ORDER BY title ASC"

	return r.queryChannels(ctx, r.rebind(query), args...)
}

func (r *SQLRepository) ListMCPAllowedChannels(ctx context.Context) ([]*domain.Channel, error) {
	query := `
		SELECT id, access_hash, title, username, about, is_tracked, mcp_allowed, filter_ads,
		       summary, topics, content_type, language, last_sync_msg_id, profiled_at,
		       created_at, updated_at
		FROM channels
		WHERE mcp_allowed = ?
		ORDER BY title ASC
	`
	return r.queryChannels(ctx, r.rebind(query), true)
}

func (r *SQLRepository) GetChannel(ctx context.Context, id int64) (*domain.Channel, error) {
	query := `
		SELECT id, access_hash, title, username, about, is_tracked, mcp_allowed, filter_ads,
		       summary, topics, content_type, language, last_sync_msg_id, profiled_at,
		       created_at, updated_at
		FROM channels
		WHERE id = ?
	`
	channels, err := r.queryChannels(ctx, r.rebind(query), id)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, sql.ErrNoRows
	}
	return channels[0], nil
}

func (r *SQLRepository) queryChannels(ctx context.Context, query string, args ...any) ([]*domain.Channel, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("executing channel query: %w", err)
	}
	defer rows.Close()

	var channels []*domain.Channel
	for rows.Next() {
		var (
			ch         domain.Channel
			username   sql.NullString
			about      sql.NullString
			summary    sql.NullString
			topics     sql.NullString
			profiledAt sql.NullTime
		)

		err := rows.Scan(
			&ch.ID, &ch.AccessHash, &ch.Title, &username, &about,
			&ch.IsTracked, &ch.MCPAllowed, &ch.FilterAds,
			&summary, &topics, &ch.ContentType, &ch.Language,
			&ch.LastSyncMsgID, &profiledAt, &ch.CreatedAt, &ch.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning channel row: %w", err)
		}

		if username.Valid {
			ch.Username = username.String
		}
		if about.Valid {
			ch.About = about.String
		}
		if summary.Valid {
			ch.Summary = summary.String
		}
		if topics.Valid {
			ch.ParseTopics(topics.String)
		}
		if profiledAt.Valid {
			ch.ProfiledAt = &profiledAt.Time
		}

		channels = append(channels, &ch)
	}

	return channels, rows.Err()
}

func (r *SQLRepository) SetTracking(ctx context.Context, channelIDs []int64, tracked bool) error {
	return r.updateChannelBoolField(ctx, "is_tracked", channelIDs, tracked)
}

func (r *SQLRepository) SetMCPAllowed(ctx context.Context, channelIDs []int64, allowed bool) error {
	return r.updateChannelBoolField(ctx, "mcp_allowed", channelIDs, allowed)
}

func (r *SQLRepository) SetFilterAds(ctx context.Context, channelIDs []int64, filterAds bool) error {
	return r.updateChannelBoolField(ctx, "filter_ads", channelIDs, filterAds)
}

func (r *SQLRepository) updateChannelBoolField(ctx context.Context, col string, channelIDs []int64, val bool) error {
	if len(channelIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(channelIDs))
	args := make([]any, 0, len(channelIDs)+1)
	args = append(args, val)
	for i, id := range channelIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(
		"UPDATE channels SET %s = ?, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)",
		col, strings.Join(placeholders, ", "),
	)
	_, err := r.db.ExecContext(ctx, r.rebind(query), args...)
	return err
}

func (r *SQLRepository) UpdateChannelTopics(ctx context.Context, id int64, topics []string) error {
	ch := &domain.Channel{Topics: topics}
	query := `
		UPDATE channels
		SET topics = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, r.rebind(query), ch.TopicsJSON(), id)
	return err
}

func (r *SQLRepository) UpdateChannelProfile(ctx context.Context, id int64, summary string, topics []string, contentType string) error {
	ch := &domain.Channel{Topics: topics}
	query := `
		UPDATE channels
		SET summary = ?, topics = ?, content_type = ?, profiled_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, r.rebind(query), summary, ch.TopicsJSON(), contentType, id)
	return err
}

func (r *SQLRepository) UpdateWatermark(ctx context.Context, channelID int64, lastMsgID int) error {
	query := `
		UPDATE channels
		SET last_sync_msg_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND last_sync_msg_id < ?
	`
	_, err := r.db.ExecContext(ctx, r.rebind(query), lastMsgID, channelID, lastMsgID)
	return err
}

// --- Post Operations ---

func (r *SQLRepository) UpsertPosts(ctx context.Context, posts []*domain.Post) error {
	if len(posts) == 0 {
		return nil
	}

	query := `
		INSERT INTO posts (
			channel_id, msg_id, date, text, views, forwards, has_media, is_ad, is_read, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (channel_id, msg_id) DO UPDATE SET
			text = excluded.text,
			views = excluded.views,
			forwards = excluded.forwards
	`
	rebound := r.rebind(query)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, rebound)
	if err != nil {
		return fmt.Errorf("preparing upsert posts statement: %w", err)
	}
	defer stmt.Close()

	for _, p := range posts {
		_, err := stmt.ExecContext(ctx,
			p.ChannelID, p.MsgID, p.Date, p.Text, p.Views, p.Forwards,
			p.HasMedia, p.IsAd, p.IsRead,
		)
		if err != nil {
			return fmt.Errorf("upserting post msg_id %d channel %d: %w", p.MsgID, p.ChannelID, err)
		}
	}

	return tx.Commit()
}

func (r *SQLRepository) GetPosts(ctx context.Context, filter domain.PostFilter) ([]*domain.Post, error) {
	var (
		conditions []string
		args       []any
	)

	query := `
		SELECT p.id, p.channel_id, p.msg_id, p.date, p.text, p.views, p.forwards, p.has_media, p.is_ad, p.is_read, p.created_at
		FROM posts p
		JOIN channels c ON p.channel_id = c.id
	`

	if filter.MCPOnly {
		conditions = append(conditions, "c.mcp_allowed = ?")
		args = append(args, true)
	}

	if filter.TrackedOnly {
		conditions = append(conditions, "c.is_tracked = ?")
		args = append(args, true)
	}

	if filter.FilterAds {
		conditions = append(conditions, "p.is_ad = ?")
		args = append(args, false)
	}

	if filter.UnreadOnly {
		conditions = append(conditions, "p.is_read = ?")
		args = append(args, false)
	}

	if len(filter.ChannelIDs) > 0 {
		placeholders := make([]string, len(filter.ChannelIDs))
		for i, id := range filter.ChannelIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		conditions = append(conditions, fmt.Sprintf("p.channel_id IN (%s)", strings.Join(placeholders, ", ")))
	}

	if filter.Topic != "" && filter.Topic != "all" {
		topicStr := filter.Topic
		if strings.Contains(topicStr, "&") {
			// Strict intersection: channel must match all tags
			parts := strings.Split(topicStr, "&")
			for _, p := range parts {
				trimmed := strings.ToLower(strings.TrimSpace(p))
				if trimmed != "" {
					conditions = append(conditions, "LOWER(c.topics) LIKE ?")
					args = append(args, "%"+trimmed+"%")
				}
			}
		} else if strings.Contains(topicStr, "+") || strings.Contains(topicStr, ",") {
			// Union: channel matches any of the requested topics
			var parts []string
			if strings.Contains(topicStr, "+") {
				parts = strings.Split(topicStr, "+")
			} else {
				parts = strings.Split(topicStr, ",")
			}
			var orClauses []string
			for _, p := range parts {
				trimmed := strings.ToLower(strings.TrimSpace(p))
				if trimmed != "" {
					orClauses = append(orClauses, "LOWER(c.topics) LIKE ?")
					args = append(args, "%"+trimmed+"%")
				}
			}
			if len(orClauses) > 0 {
				conditions = append(conditions, "("+strings.Join(orClauses, " OR ")+")")
			}
		} else {
			// Single tag
			trimmed := strings.ToLower(strings.TrimSpace(topicStr))
			conditions = append(conditions, "LOWER(c.topics) LIKE ?")
			args = append(args, "%"+trimmed+"%")
		}
	}

	if !filter.Since.IsZero() {
		conditions = append(conditions, "p.date >= ?")
		args = append(args, filter.Since)
	}

	if !filter.Until.IsZero() {
		conditions = append(conditions, "p.date <= ?")
		args = append(args, filter.Until)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY p.date DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	return r.queryPosts(ctx, r.rebind(query), args...)
}

func (r *SQLRepository) SearchPosts(ctx context.Context, queryText string, mcpOnly bool, limit int) ([]*domain.Post, error) {
	query := `
		SELECT p.id, p.channel_id, p.msg_id, p.date, p.text, p.views, p.forwards, p.has_media, p.is_ad, p.is_read, p.created_at
		FROM posts p
		JOIN channels c ON p.channel_id = c.id
		WHERE p.text LIKE ?
	`
	args := []any{"%" + queryText + "%"}

	if mcpOnly {
		query += " AND c.mcp_allowed = ?"
		args = append(args, true)
	}

	query += " ORDER BY p.date DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	return r.queryPosts(ctx, r.rebind(query), args...)
}

func (r *SQLRepository) MarkPostsRead(ctx context.Context, postIDs []int64) error {
	if len(postIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(postIDs))
	args := make([]any, len(postIDs))
	for i, id := range postIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("UPDATE posts SET is_read = ? WHERE id IN (%s)", strings.Join(placeholders, ", "))
	queryArgs := append([]any{true}, args...)
	_, err := r.db.ExecContext(ctx, r.rebind(query), queryArgs...)
	return err
}

func (r *SQLRepository) CleanPosts(ctx context.Context, olderThan time.Duration, onlyRead bool) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	query := "DELETE FROM posts WHERE date < ?"
	args := []any{cutoff}
	if onlyRead {
		query += " AND is_read = ?"
		args = append(args, true)
	}

	res, err := r.db.ExecContext(ctx, r.rebind(query), args...)
	if err != nil {
		return 0, fmt.Errorf("cleaning posts: %w", err)
	}
	return res.RowsAffected()
}

func (r *SQLRepository) GetPostStats(ctx context.Context) (int, int, error) {
	var total, unread int
	row := r.db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(CASE WHEN NOT is_read THEN 1 ELSE 0 END), 0) FROM posts")
	if err := row.Scan(&total, &unread); err != nil {
		return 0, 0, fmt.Errorf("querying post stats: %w", err)
	}
	return total, unread, nil
}

func (r *SQLRepository) queryPosts(ctx context.Context, query string, args ...any) ([]*domain.Post, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("executing post query: %w", err)
	}
	defer rows.Close()

	var posts []*domain.Post
	for rows.Next() {
		var p domain.Post
		err := rows.Scan(
			&p.ID, &p.ChannelID, &p.MsgID, &p.Date, &p.Text, &p.Views,
			&p.Forwards, &p.HasMedia, &p.IsAd, &p.IsRead, &p.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning post row: %w", err)
		}
		posts = append(posts, &p)
	}

	return posts, rows.Err()
}

// --- Digest Operations ---

func (r *SQLRepository) SaveDigest(ctx context.Context, digest *domain.Digest) error {
	// Zero-Trust Check: Determine if all included channels are mcp_allowed.
	// If any channel is quarantined, mcp_visible must strictly be false!
	mcpVisible := false
	if len(digest.ChannelIDs) > 0 {
		placeholders := make([]string, len(digest.ChannelIDs))
		args := make([]any, len(digest.ChannelIDs))
		for i, id := range digest.ChannelIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		checkQuery := fmt.Sprintf(
			"SELECT COUNT(*) FROM channels WHERE id IN (%s) AND mcp_allowed = ?",
			strings.Join(placeholders, ", "),
		)
		var allowedCount int
		err := r.db.QueryRowContext(ctx, r.rebind(checkQuery), append(args, true)...).Scan(&allowedCount)
		if err == nil && allowedCount == len(digest.ChannelIDs) {
			mcpVisible = true
		}
	}
	digest.MCPVisible = mcpVisible

	query := `
		INSERT INTO digests (
			id, title, topic, period_start, period_end, channel_ids,
			summary_markdown, post_count, mcp_visible, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			title = excluded.title,
			summary_markdown = excluded.summary_markdown,
			post_count = excluded.post_count,
			mcp_visible = excluded.mcp_visible
	`
	_, err := r.db.ExecContext(ctx, r.rebind(query),
		digest.ID, digest.Title, digest.Topic, digest.PeriodStart, digest.PeriodEnd,
		digest.ChannelIDsJSON(), digest.SummaryMarkdown, digest.PostCount, digest.MCPVisible,
	)
	return err
}

func (r *SQLRepository) GetLatestDigest(ctx context.Context, topic string, mcpOnly bool) (*domain.Digest, error) {
	digests, err := r.ListDigests(ctx, topic, mcpOnly, 1)
	if err != nil {
		return nil, err
	}
	if len(digests) == 0 {
		return nil, sql.ErrNoRows
	}
	return digests[0], nil
}

func (r *SQLRepository) ListDigests(ctx context.Context, topic string, mcpOnly bool, limit int) ([]*domain.Digest, error) {
	query := `
		SELECT id, title, topic, period_start, period_end, channel_ids,
		       summary_markdown, post_count, mcp_visible, created_at
		FROM digests
	`
	var (
		conditions []string
		args       []any
	)

	if mcpOnly {
		conditions = append(conditions, "mcp_visible = ?")
		args = append(args, true)
	}

	if topic != "" && topic != "all" {
		conditions = append(conditions, "topic = ?")
		args = append(args, topic)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("querying digests: %w", err)
	}
	defer rows.Close()

	var digests []*domain.Digest
	for rows.Next() {
		var (
			d          domain.Digest
			channelIDs string
		)
		err := rows.Scan(
			&d.ID, &d.Title, &d.Topic, &d.PeriodStart, &d.PeriodEnd,
			&channelIDs, &d.SummaryMarkdown, &d.PostCount, &d.MCPVisible, &d.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning digest row: %w", err)
		}
		d.ParseChannelIDs(channelIDs)
		digests = append(digests, &d)
	}

	return digests, rows.Err()
}

func (r *SQLRepository) DeleteDigest(ctx context.Context, id string) error {
	query := `DELETE FROM digests WHERE id = ?`
	_, err := r.db.ExecContext(ctx, r.rebind(query), id)
	if err != nil {
		return fmt.Errorf("deleting digest %s: %w", id, err)
	}
	return nil
}

// --- Session Operations ---

func (r *SQLRepository) LoadSession(ctx context.Context, id string) ([]byte, error) {
	query := `SELECT data FROM telegram_sessions WHERE id = ?`
	var data []byte
	err := r.db.QueryRowContext(ctx, r.rebind(query), id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil // Return nil bytes if no session exists yet
	}
	if err != nil {
		return nil, fmt.Errorf("loading session %s: %w", id, err)
	}
	return data, nil
}

func (r *SQLRepository) StoreSession(ctx context.Context, id string, data []byte) error {
	query := `
		INSERT INTO telegram_sessions (id, data, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			data = excluded.data,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := r.db.ExecContext(ctx, r.rebind(query), id, data)
	if err != nil {
		return fmt.Errorf("storing session %s: %w", id, err)
	}
	return nil
}

// --- Ad Pattern Operations ---

func (r *SQLRepository) ListAdPatterns(ctx context.Context, enabledOnly bool) ([]*domain.AdPattern, error) {
	query := `SELECT id, pattern, is_regex, description, enabled, created_at FROM ad_patterns`
	var args []any
	if enabledOnly {
		query += " WHERE enabled = ?"
		args = append(args, true)
	}
	query += " ORDER BY id ASC"

	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("listing ad patterns: %w", err)
	}
	defer rows.Close()

	var patterns []*domain.AdPattern
	for rows.Next() {
		var (
			p    domain.AdPattern
			desc sql.NullString
		)
		if err := rows.Scan(&p.ID, &p.Pattern, &p.IsRegex, &desc, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning ad pattern: %w", err)
		}
		if desc.Valid {
			p.Description = desc.String
		}
		patterns = append(patterns, &p)
	}
	return patterns, rows.Err()
}

func (r *SQLRepository) AddAdPattern(ctx context.Context, pattern *domain.AdPattern) error {
	query := `
		INSERT INTO ad_patterns (pattern, is_regex, description, enabled, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (pattern) DO UPDATE SET
			is_regex = excluded.is_regex,
			description = excluded.description,
			enabled = excluded.enabled
	`
	_, err := r.db.ExecContext(ctx, r.rebind(query),
		pattern.Pattern, pattern.IsRegex, pattern.Description, pattern.Enabled,
	)
	return err
}

func (r *SQLRepository) DeleteAdPattern(ctx context.Context, id int64) error {
	query := `DELETE FROM ad_patterns WHERE id = ?`
	_, err := r.db.ExecContext(ctx, r.rebind(query), id)
	return err
}

func (r *SQLRepository) ToggleAdPattern(ctx context.Context, id int64, enabled bool) error {
	query := `UPDATE ad_patterns SET enabled = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, r.rebind(query), enabled, id)
	return err
}

// --- Settings Operations ---

func (r *SQLRepository) GetSetting(ctx context.Context, key string, defaultValue string) (string, error) {
	query := `SELECT value FROM settings WHERE key = ?`
	var val string
	err := r.db.QueryRowContext(ctx, r.rebind(query), key).Scan(&val)
	if err == sql.ErrNoRows {
		return defaultValue, nil
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

func (r *SQLRepository) SetSetting(ctx context.Context, key string, value string) error {
	query := `
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := r.db.ExecContext(ctx, r.rebind(query), key, value)
	return err
}

func (r *SQLRepository) GetAllSettings(ctx context.Context) (map[string]string, error) {
	query := `SELECT key, value FROM settings`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, rows.Err()
}
