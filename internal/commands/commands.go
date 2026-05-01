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

	"github.com/YoRHa-A5/doromoge/internal/db"
)

// embedColor is the Discord "blurple" colour used in all bot embeds (0x5865F2).
const embedColor = 0x5865F2
const zeroWidthSpace = "\u200b"

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
func HandleEmojiStats(s *discordgo.Session, i *discordgo.InteractionCreate, database *db.DB) {
	guildID := i.GuildID

	stats, err := database.GetTopEmojis(guildID, 10)
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
func HandleRecapUser(s *discordgo.Session, i *discordgo.InteractionCreate, database *db.DB) {
	guildID := i.GuildID
	userID := i.Member.User.ID

	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	tsOption, ok := optionMap["timespan"]
	if !ok {
		return
	}
	timespan := tsOption.StringValue()
	since := time.Now().Add(-timespanToDuration(timespan))

	emojis, err := database.GetUserTopEmojis(guildID, userID, since, 3)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	channels, err := database.GetUserTopChannels(guildID, userID, since, 3)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	msgCount, err := database.GetUserMessageCount(guildID, userID, since)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s Recap for %s", timespanToAdverb(timespan), displayUsername(i.Member.User)),
		Description: fmt.Sprintf("Activity stats for the past %s", timespan),
		Color:       embedColor,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	// Three columns side by side: one header row, then value rows below.
	// Empty Name on subsequent rows tells Discord to merge into the same column.
	maxLen := max(len(emojis), len(channels), 1)

	// Row 0: headers
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "Top Emojis",
		Value:  "",
		Inline: true,
	}, &discordgo.MessageEmbedField{
		Name:   "Top Channels",
		Value:  "",
		Inline: true,
	}, &discordgo.MessageEmbedField{
		Name:   "Total Messages",
		Value:  "",
		Inline: true,
	})

	for rowIdx := 1; rowIdx < maxLen; rowIdx++ {
		var valEmojis string
		if rowIdx-1 < len(emojis) {
			valEmojis = fmt.Sprintf("**%s**\nused %d times", emojiMention(emojis[rowIdx-1].EmojiName, emojis[rowIdx-1].EmojiID), emojis[rowIdx-1].Count)
		} else {
			valEmojis = zeroWidthSpace
		}

		var valChannels string
		if rowIdx-1 < len(channels) {
			valChannels = fmt.Sprintf("**#%s**\n%d messages", resolveChannelName(s, channels[rowIdx-1].ChannelID), channels[rowIdx-1].Count)
		} else {
			valChannels = zeroWidthSpace
		}

		// Third column: single value (user's total message count)
		valMessages := ""
		if rowIdx == 1 {
			valMessages = fmt.Sprintf("%d", msgCount)
		} else {
			valMessages = zeroWidthSpace
		}

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "",
			Value:  valEmojis,
			Inline: true,
		})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "",
			Value:  valChannels,
			Inline: true,
		})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "",
			Value:  valMessages,
			Inline: true,
		})
	}

	respondWithEmbed(s, i, embed)
}

// HandleRecapServer responds with an embed covering the selected timespan at
// server level. It shows: top 5 emojis, top 5 channels by message count, and
// top 5 users by message count.
func HandleRecapServer(s *discordgo.Session, i *discordgo.InteractionCreate, database *db.DB) {
	guildID := i.GuildID

	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	tsOption, ok := optionMap["timespan"]
	if !ok {
		return
	}
	timespan := tsOption.StringValue()
	since := time.Now().Add(-timespanToDuration(timespan))

	emojis, err := database.GetServerTopEmojis(guildID, since, 5)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	channels, err := database.GetServerTopChannels(guildID, since, 5)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	users, err := database.GetServerTopUsers(guildID, since, 5)
	if err != nil {
		databaseGetError(s, i, err)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("Server %s Recap", capitalize(timespan)),
		Description: fmt.Sprintf("Activity stats for the past %s", timespan),
		Color:       embedColor,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	// Three columns side by side: one header row, then value rows below.
	// Empty Name on subsequent rows tells Discord to merge into the same column.
	maxLen := max(len(emojis), len(channels), len(users))

	// Row 0: headers
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "Top Emojis",
		Value:  "",
		Inline: true,
	}, &discordgo.MessageEmbedField{
		Name:   "Top Channels",
		Value:  "",
		Inline: true,
	}, &discordgo.MessageEmbedField{
		Name:   "Top Users",
		Value:  "",
		Inline: true,
	})

	for rowIdx := 1; rowIdx < maxLen; rowIdx++ {
		var valEmojis string
		if rowIdx-1 < len(emojis) {
			valEmojis = fmt.Sprintf("**%s**\nused %d times", emojiMention(emojis[rowIdx-1].EmojiName, emojis[rowIdx-1].EmojiID), emojis[rowIdx-1].Count)
		} else {
			valEmojis = zeroWidthSpace
		}

		var valChannels string
		if rowIdx-1 < len(channels) {
			valChannels = fmt.Sprintf("**#%s**\n%d messages", resolveChannelName(s, channels[rowIdx-1].ChannelID), channels[rowIdx-1].Count)
		} else {
			valChannels = zeroWidthSpace
		}

		var valUsers string
		if rowIdx-1 < len(users) {
			valUsers = fmt.Sprintf("**%s**\n%d messages", resolveUserDisplayName(s, guildID, users[rowIdx-1].UserID), users[rowIdx-1].Count)
		} else {
			valUsers = zeroWidthSpace
		}

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "",
			Value:  valEmojis,
			Inline: true,
		})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "",
			Value:  valChannels,
			Inline: true,
		})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "",
			Value:  valUsers,
			Inline: true,
		})
	}

	respondWithEmbed(s, i, embed)
}

// HandleInteraction dispatches an interaction to the appropriate command handler
// based on the command name.
func HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, database *db.DB) {
	switch i.ApplicationCommandData().Name {
	case "emoji-stats":
		HandleEmojiStats(s, i, database)
	case "recap-user":
		HandleRecapUser(s, i, database)
	case "recap-server":
		HandleRecapServer(s, i, database)
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
