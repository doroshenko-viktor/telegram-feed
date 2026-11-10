package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/gotd/td/tg"

	"telegram_feed_summarizer/internal/domain"
)

// ListSubscribedChannels retrieves all broadcast channels and supergroups the user is subscribed to.
func (c *Client) ListSubscribedChannels(ctx context.Context) ([]*domain.Channel, error) {
	var result []*domain.Channel

	err := c.client.Run(ctx, func(ctx context.Context) error {
		api := c.client.API()

		// Fetch dialogs
		dialogsResp, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetPeer: &tg.InputPeerEmpty{},
			Limit:      100,
		})
		if err != nil {
			return fmt.Errorf("fetching dialogs: %w", err)
		}

		var chats []tg.ChatClass
		switch d := dialogsResp.(type) {
		case *tg.MessagesDialogs:
			chats = d.Chats
		case *tg.MessagesDialogsSlice:
			chats = d.Chats
		}

		for _, chatClass := range chats {
			ch, ok := chatClass.(*tg.Channel)
			if !ok {
				continue // Skip direct chats, basic groups
			}

			// Exclude channels left by user
			if ch.Left {
				continue
			}

			normID := NormalizeChannelID(ch.ID)
			result = append(result, &domain.Channel{
				ID:          normID,
				AccessHash:  ch.AccessHash,
				Title:       ch.Title,
				Username:    ch.Username,
				IsTracked:   false,
				MCPAllowed:  false, // Zero-trust gateway: private by default
				FilterAds:   true,  // Filter ads by default
				ContentType: "general",
				Language:    "en",
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// FetchChannelPosts fetches messages from a specific channel newer than minMsgID.
// It skips service messages and collapses multi-image media albums sharing a grouped_id.
func (c *Client) FetchChannelPosts(ctx context.Context, channel *domain.Channel, minMsgID int, limit int) ([]*domain.Post, int, error) {
	if limit <= 0 {
		limit = 100
	}

	var (
		posts    []*domain.Post
		maxMsgID = minMsgID
	)

	err := c.client.Run(ctx, func(ctx context.Context) error {
		api := c.client.API()
		peer := &tg.InputPeerChannel{
			ChannelID:  channel.ID,
			AccessHash: channel.AccessHash,
		}

		history, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			MinID:    minMsgID,
			Limit:    limit,
			OffsetID: 0,
		})
		if err != nil {
			return fmt.Errorf("getting history for channel %d: %w", channel.ID, err)
		}

		var messages []tg.MessageClass
		switch h := history.(type) {
		case *tg.MessagesChannelMessages:
			messages = h.Messages
		case *tg.MessagesMessages:
			messages = h.Messages
		case *tg.MessagesMessagesSlice:
			messages = h.Messages
		}

		// Track album grouped IDs to collapse 5 photos into 1 post
		groupedMap := make(map[int64]*domain.Post)

		for _, msgClass := range messages {
			msg, ok := msgClass.(*tg.Message)
			if !ok {
				continue // Skip service messages (e.g. pinned messages, chat info changes)
			}

			if msg.ID > maxMsgID {
				maxMsgID = msg.ID
			}

			postDate := time.Unix(int64(msg.Date), 0)

			// Album handling: if part of an album, consolidate into single post
			if msg.GroupedID != 0 {
				if existing, exists := groupedMap[msg.GroupedID]; exists {
					// If this message has the caption and the existing one was empty, update text
					if existing.Text == "" && msg.Message != "" {
						existing.Text = msg.Message
					}
					existing.HasMedia = true
					continue
				}
			}

			post := &domain.Post{
				ChannelID: channel.ID,
				MsgID:     msg.ID,
				Date:      postDate,
				Text:      msg.Message,
				Views:     msg.Views,
				Forwards:  msg.Forwards,
				HasMedia:  msg.Media != nil,
				IsAd:      false, // Will be classified by pipeline cleaner
				IsRead:    false,
			}

			if msg.GroupedID != 0 {
				groupedMap[msg.GroupedID] = post
			}

			posts = append(posts, post)
		}

		return nil
	})

	if err != nil {
		return nil, minMsgID, err
	}
	return posts, maxMsgID, nil
}
