package commands

import (
	"time"

	"github.com/YoRHa-A5/Doro/internal/models"
)

// StatsStore defines the interface for accessing emoji and message statistics.
// It is implemented by *db.DB and can be stubbed for testing.
type StatsStore interface {
	GetTopEmojis(guildID string, limit int) ([]models.EmojiStat, error)
	GetUserTopEmojis(guildID, userID string, since time.Time, limit int) ([]models.EmojiStat, error)
	GetServerTopEmojis(guildID string, since time.Time, limit int) ([]models.EmojiStat, error)
	GetUserTopChannels(guildID, userID string, since time.Time, limit int) ([]models.ChannelStat, error)
	GetServerTopChannels(guildID string, since time.Time, limit int) ([]models.ChannelStat, error)
	GetUserMessageCount(guildID, userID string, since time.Time) (int, error)
	GetServerTopUsers(guildID string, since time.Time, limit int) ([]models.UserActivityStat, error)
}
