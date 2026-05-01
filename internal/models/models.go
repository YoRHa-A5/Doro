// Package models defines the data structures used throughout the bot.
//
// The database schema is intentionally flat: emoji and message data are stored
// as independent rows so that queries can GROUP BY and SUM without joins.
package models

import "time"

// Guild records a Discord guild that has been scanned. The primary key is
// the guild ID; the scanned_at timestamp is used for diagnostics and ensures
// the initial history scan runs exactly once per guild.
type Guild struct {
	ID        string
	ScannedAt time.Time
}

// EmojiUsage represents one emoji's usage record scoped to a guild, user, and
// channel. The UNIQUE constraint on (guild_id, emoji_id, user_id, channel_id)
// ensures one row per combination; counts are incremented on subsequent uses.
type EmojiUsage struct {
	ID        int64
	GuildID   string
	EmojiName string
	EmojiID   string
	UserID    string
	ChannelID string
	Count     int
	FirstUsed time.Time
	LastUsed  time.Time
}

// MessageCount tracks how many messages a user has sent in each channel of
// each guild. The UNIQUE constraint on (guild_id, channel_id, user_id) ensures
// one row per combination; count is incremented per message.
type MessageCount struct {
	ID        int64
	GuildID   string
	ChannelID string
	UserID    string
	Count     int
	FirstSeen time.Time
	LastSeen  time.Time
}

// EmojiStat is returned by queries that aggregate emoji usage counts.
// EmojiName and EmojiID are used to render the emoji mention in embeds.
type EmojiStat struct {
	EmojiName string
	EmojiID   string
	Count     int
}

// ChannelStat is returned by queries that aggregate message counts per channel.
type ChannelStat struct {
	ChannelID   string
	ChannelName string
	Count       int
}

// UserActivityStat is returned by queries that aggregate message counts per user.
type UserActivityStat struct {
	UserID      string
	UserName    string
	DisplayName string
	Count       int
}
