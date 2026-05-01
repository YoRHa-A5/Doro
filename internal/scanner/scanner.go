// Package scanner handles bulk-population of the database by scanning
// message history when the bot first joins a guild.
//
// Scanning is bounded to the last 30 days to avoid excessive API calls.
// Each message is processed independently: custom emoji mentions are parsed
// and recorded, and a message count is incremented for the author.
//
// If a guild is already recorded in the [guilds] table, it is skipped.
package scanner

import (
	"log"
	"regexp"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/YoRHa-A5/Doro/internal/db"
)

// Matches both animated (<a:name:id>) and static (<:name:id>) custom Discord emojis.
// Capturing groups: [1]=emoji name, [2]=emoji ID.
var customEmojiRegex = regexp.MustCompile(`<a?:([a-zA-Z0-9_]+):(\d+)>`)

// Scanner persists emoji and message data by scanning a guild's message history.
type Scanner struct {
	db  *db.DB
	log func(string, ...any)
}

// New returns a Scanner backed by the given database.
func New(database *db.DB) *Scanner {
	return &Scanner{db: database, log: log.Default().Printf}
}

// ScanGuild fetches all text channels in the given guild and scans each one,
// recording custom emojis and message counts for messages within the last 30 days.
// The guild is marked as scanned in the database upon completion.
// Runs asynchronously so it does not block the bot's event loop.
func (s *Scanner) ScanGuild(guildID string, session *discordgo.Session) {
	s.log("[Scanner] Starting guild scan for guild %s", guildID)

	// session.Guild() does not return channels (REST /guilds/{id}), so use
	// session.GuildChannels() which calls /guilds/{id}/channels instead.
	channels, err := session.GuildChannels(guildID)
	if err != nil {
		s.log("[Scanner] Failed to fetch channels: %v", err)
		return
	}

	var textChannels []*discordgo.Channel
	for _, ch := range channels {
		if ch.Type == discordgo.ChannelTypeGuildText {
			textChannels = append(textChannels, ch)
		}
	}

	s.log("[Scanner] Found %d text channels in guild %s", len(textChannels), guildID)

	since := time.Now().AddDate(0, 0, -30)

	for i, ch := range textChannels {
		s.log("[Scanner] Scanning channel %d/%d: %s", i+1, len(textChannels), ch.ID)
		count := s.scanChannel(guildID, ch.ID, since, session)
		s.log("[Scanner] Scanned %d messages in channel %s", count, ch.ID)

		time.Sleep(100 * time.Millisecond)
	}

	if err := s.db.MarkGuildScanned(guildID); err != nil {
		s.log("[Scanner] Failed to mark guild as scanned: %v", err)
		return
	}

	s.log("[Scanner] Guild %s scan complete", guildID)
}

// scanChannel paginates through message history using beforeID cursors.
// Messages older than [since] stop the scan. Returns the total number of
// messages processed.
func (s *Scanner) scanChannel(guildID, channelID string, since time.Time, session *discordgo.Session) int {
	var totalScanned int
	var beforeID string

	for {
		messages, err := session.ChannelMessages(channelID, 100, beforeID, "", "")
		if err != nil {
			s.log("[Scanner] Error fetching messages from %s: %v", channelID, err)
			break
		}

		if len(messages) == 0 {
			break
		}

		for _, msg := range messages {
			msgTime := msg.Timestamp
			if msgTime.Before(since) {
				return totalScanned
			}

			totalScanned++
			s.processMessage(guildID, channelID, msg)
		}

		// Advance cursor to the oldest message in this batch for the next page.
		last := messages[len(messages)-1]
		beforeID = last.ID

		if len(messages) < 100 {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	return totalScanned
}

// processMessage parses custom emojis from the message content and records
// them in the database. Bots are skipped. Each occurrence of an emoji is
// counted individually (not deduplicated within a message).
func (s *Scanner) processMessage(guildID, channelID string, msg *discordgo.Message) {
	if msg.Author.Bot {
		return
	}

	matches := customEmojiRegex.FindAllStringSubmatch(msg.Content, -1)
	for _, match := range matches {
		name := match[1]
		emojiID := match[2]
		if err := s.db.UpsertEmojiUsage(guildID, name, emojiID, msg.Author.ID, channelID, 1); err != nil {
			s.log("[Scanner] Failed to upsert emoji usage: %v", err)
		}
	}

	if err := s.db.UpsertMessageCount(guildID, channelID, msg.Author.ID); err != nil {
		s.log("[Scanner] Failed to upsert message count: %v", err)
	}
}
