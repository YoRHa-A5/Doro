// Package commands defines Discord slash commands and their handlers.
//
// Command definitions are registered with Discord on bot startup via
// ApplicationCommandCreate. Interaction handlers are dispatched by name
// in HandleInteraction.
package commands

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
)

// embedColor is the Discord "blurple" colour used in all bot embeds (0x5865F2).
const embedColor = 0x5865F2

// cooldown is the package-level rate limiter for slash commands.
var cooldown = NewCooldown()

// Commands is the list of slash commands registered with Discord.
// Each command is defined here and registered in bot.New().
var Commands = []*discordgo.ApplicationCommand{
	{
		Name:        "emoji-stats",
		Description: "Show the top 10 most used emojis in this server",
	},
	{
		Name:        "recap-user",
		Description: "Get your activity recap for a specific time period",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "timespan",
				Description: "Time period for the recap",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Week", Value: "week"},
					{Name: "Month", Value: "month"},
					{Name: "Year", Value: "year"},
				},
			},
		},
	},
	{
		Name:        "recap-server",
		Description: "Get server-wide activity stats for a specific time period",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "timespan",
				Description: "Time period for the recap",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Week", Value: "week"},
					{Name: "Month", Value: "month"},
					{Name: "Year", Value: "year"},
				},
			},
		},
	},
}

// timespanToDuration converts a timespan string to a time.Duration.
// week=7d, month=30d, year=365d. Unknown values default to 30 days.
func timespanToDuration(ts string) time.Duration {
	switch ts {
	case "week":
		return 7 * 24 * time.Hour
	case "month":
		return 30 * 24 * time.Hour
	case "year":
		return 365 * 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}

// timespanToAdverb converts a timespan string to its adverb form (e.g. "week" -> "weekly").
func timespanToAdverb(ts string) string {
	switch ts {
	case "week":
		return "Weekly"
	case "month":
		return "Monthly"
	case "year":
		return "Yearly"
	default:
		return "Monthly"
	}
}

// displayUsername returns the most human-readable display name for a Discord user:
// GlobalName (the name the user set in their profile) if set, otherwise Username.
func displayUsername(u *discordgo.User) string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	return u.Username
}

// emojiMention renders a custom emoji as a Discord mention string: <:name:id>.
func emojiMention(name, id string) string {
	return fmt.Sprintf("<:%s:%s>", name, id)
}

// resolveChannelName fetches a channel by ID and returns its name.
// Falls back to the raw channel ID if the channel cannot be resolved.
func resolveChannelName(s *discordgo.Session, channelID string) string {
	ch, err := s.Channel(channelID)
	if err != nil {
		return channelID
	}
	return ch.Name
}

// resolveUserDisplayName fetches a guild member by ID and returns the most
// human-readable name available: server nickname > global display name > username.
// Falls back to the raw user ID if the member cannot be resolved.
func resolveUserDisplayName(s *discordgo.Session, guildID, userID string) string {
	member, err := s.GuildMember(guildID, userID)
	if err != nil {
		return userID
	}
	if member.Nick != "" {
		return member.Nick
	}
	if member.User.GlobalName != "" {
		return member.User.GlobalName
	}
	return member.User.Username
}

// respondWithEmbed sends an embed response to an interaction.
func respondWithEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

// HandleEmojiStats responds with an embed listing the top 10 most-used emojis
// across the entire server (all time). Emoji counts are aggregated per emoji ID.
func HandleEmojiStats(s *discordgo.Session, i *discordgo.InteractionCreate, store StatsStore) {
	guildID := i.GuildID

	stats, err := store.GetTopEmojis(guildID, 10)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Top 10 Server Emojis",
		Description: zeroWidthSpace,
		Color:       embedColor,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	fields := make([]*discordgo.MessageEmbedField, 0, len(stats))
	for _, stat := range stats {
		// rank := fmt.Sprintf("%d.", i+1)
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%s x%s", emojiMention(stat.EmojiName, stat.EmojiID), strconv.Itoa(stat.Count)),
			Value:  "",
			Inline: true,
		})
	}

	if len(fields) == 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  "No data",
			Value: "No emoji data recorded yet.",
		})
	}

	embed.Fields = fields
	respondWithEmbed(s, i, embed)
}

// HandleRecapUser responds with an embed for the invoking user covering the
// selected timespan. It shows: top 3 emojis used, top 3 channels by message
// count, and total message count.
func HandleRecapUser(s *discordgo.Session, i *discordgo.InteractionCreate, store StatsStore) {
	guildID := i.GuildID
	userID := i.Member.User.ID

	tsOption, since := ParseTimespan(i.ApplicationCommandData().Options)
	if tsOption == "" {
		return
	}

	emojis, err := store.GetUserTopEmojis(guildID, userID, since, 3)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	channels, err := store.GetUserTopChannels(guildID, userID, since, 3)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	msgCount, err := store.GetUserMessageCount(guildID, userID, since)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s Recap for %s", timespanToAdverb(tsOption), displayUsername(i.Member.User)),
		Description: fmt.Sprintf("Activity stats for the past %s", tsOption),
		Color:       embedColor,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	// Build the inline grid: each row has [emoji, channel, messages].
	// The third column (Total Messages) only shows on row 1.
	maxLen := max(len(emojis), len(channels), 1)
	var rows [][]string
	for rowIdx := 0; rowIdx < maxLen; rowIdx++ {
		var valEmojis string
		if rowIdx < len(emojis) {
			valEmojis = fmt.Sprintf("**%s**\nused %d times", emojiMention(emojis[rowIdx].EmojiName, emojis[rowIdx].EmojiID), emojis[rowIdx].Count)
		} else {
			valEmojis = zeroWidthSpace
		}

		var valChannels string
		if rowIdx < len(channels) {
			valChannels = fmt.Sprintf("**#%s**\n%d messages", resolveChannelName(s, channels[rowIdx].ChannelID), channels[rowIdx].Count)
		} else {
			valChannels = zeroWidthSpace
		}

		var valMessages string
		if rowIdx == 0 {
			valMessages = fmt.Sprintf("%d", msgCount)
		} else {
			valMessages = zeroWidthSpace
		}

		rows = append(rows, []string{valEmojis, valChannels, valMessages})
	}

	embed.Fields = BuildInlineGrid([]GridHeader{
		{Name: "Top Emojis"},
		{Name: "Top Channels"},
		{Name: "Total Messages"},
	}, rows)

	respondWithEmbed(s, i, embed)
}

// HandleRecapServer responds with an embed covering the selected timespan at
// server level. It shows: top 5 emojis, top 5 channels by message count, and
// top 5 users by message count.
func HandleRecapServer(s *discordgo.Session, i *discordgo.InteractionCreate, store StatsStore) {
	guildID := i.GuildID

	tsOption, since := ParseTimespan(i.ApplicationCommandData().Options)
	if tsOption == "" {
		return
	}

	emojis, err := store.GetServerTopEmojis(guildID, since, 5)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	channels, err := store.GetServerTopChannels(guildID, since, 5)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	users, err := store.GetServerTopUsers(guildID, since, 5)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("Server %s Recap", capitalize(tsOption)),
		Description: fmt.Sprintf("Activity stats for the past %s", tsOption),
		Color:       embedColor,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	// Build the inline grid: each row has [emoji, channel, user].
	maxLen := max(len(emojis), len(channels), len(users))
	var rows [][]string
	for rowIdx := 0; rowIdx < maxLen; rowIdx++ {
		var valEmojis string
		if rowIdx < len(emojis) {
			valEmojis = fmt.Sprintf("**%s**\nused %d times", emojiMention(emojis[rowIdx].EmojiName, emojis[rowIdx].EmojiID), emojis[rowIdx].Count)
		} else {
			valEmojis = zeroWidthSpace
		}

		var valChannels string
		if rowIdx < len(channels) {
			valChannels = fmt.Sprintf("**#%s**\n%d messages", resolveChannelName(s, channels[rowIdx].ChannelID), channels[rowIdx].Count)
		} else {
			valChannels = zeroWidthSpace
		}

		var valUsers string
		if rowIdx < len(users) {
			valUsers = fmt.Sprintf("**%s**\n%d messages", resolveUserDisplayName(s, guildID, users[rowIdx].UserID), users[rowIdx].Count)
		} else {
			valUsers = zeroWidthSpace
		}

		rows = append(rows, []string{valEmojis, valChannels, valUsers})
	}

	embed.Fields = BuildInlineGrid([]GridHeader{
		{Name: "Top Emojis"},
		{Name: "Top Channels"},
		{Name: "Top Users"},
	}, rows)

	respondWithEmbed(s, i, embed)
}

// HandleInteraction dispatches an interaction to the appropriate command handler
// based on the command name. Each command has an independent per-user cooldown
// (default 60s) to prevent spam.
func HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, store StatsStore) {
	commandName := i.ApplicationCommandData().Name
	userID := i.Member.User.ID

	remaining, ok := cooldown.Check(userID, commandName)
	if !ok {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("⏳ You can use `/%s` again in %.0fs.", commandName, remaining.Seconds()),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	cooldown.Set(userID, commandName)

	switch commandName {
	case "emoji-stats":
		HandleEmojiStats(s, i, store)
	case "recap-user":
		HandleRecapUser(s, i, store)
	case "recap-server":
		HandleRecapServer(s, i, store)
	}
}

// capitalize returns the string with its first character uppercased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0:1][0]-'a'+'A') + s[1:]
}

// databaseGetError sends an error response to the user when a DB query fails.
func databaseGetError(s *discordgo.Session, i *discordgo.InteractionCreate, err error) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Error fetching data: %v", err),
		},
	})
}
