// Package bot handles the Discord session, event routing, and live emoji
// tracking as messages arrive in real time.
//
// It registers slash commands on startup and processes MessageCreate events
// to record custom emoji usage and message counts into the database.
package bot

import (
	"log"

	"github.com/bwmarrin/discordgo"

	"github.com/YoRHa-A5/Doro/internal/commands"
	"github.com/YoRHa-A5/Doro/internal/db"
	"github.com/YoRHa-A5/Doro/internal/emoji"
	"github.com/YoRHa-A5/Doro/internal/scanner"
)

// Bot manages the Discord session and its dependencies.
type Bot struct {
	session    *discordgo.Session
	db         *db.DB
	scanner    *scanner.Scanner
	commands   []*discordgo.ApplicationCommand
	initialized bool
}

// New creates a new bot: initializes the Discord session, opens the websocket,
// and sets up event handlers. Commands are registered per-guild when the bot
// joins a server (not globally on startup).
func New(token string, database *db.DB, commands []*discordgo.ApplicationCommand) (*Bot, error) {
	session, err := discordgo.New(token)
	if err != nil {
		return nil, err
	}

	// Intents: message content to parse emojis, guild messages to receive them,
	// reactions to count emoji reactions, and guilds to receive
	// GuildCreate/GuildDelete events.
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent |
		discordgo.IntentsGuildMessageReactions |
		discordgo.IntentsGuilds

	b := &Bot{
		session:    session,
		db:         database,
		scanner:    scanner.New(database),
		commands:   commands,
	}

	// Register the Ready handler before opening the session so we don't miss it.
	session.AddHandler(b.handleReady)
	session.AddHandler(b.handleMessageCreate)
	session.AddHandler(b.handleMessageReactionAdd)
	session.AddHandler(b.handleGuildCreate)
	session.AddHandler(b.handleGuildDelete)
	session.AddHandler(b.handleInteractionCreate)

	if err := session.Open(); err != nil {
		return nil, err
	}

	return b, nil
}

// handleReady fires once when the bot has connected and is identified.
func (b *Bot) handleReady(s *discordgo.Session, event *discordgo.Ready) {
	if b.initialized {
		return
	}
	b.initialized = true
}

// registerCommandsForGuild registers all commands for a specific guild.
// Guild-scoped commands propagate instantly (vs up to 1h for global).
func (b *Bot) registerCommandsForGuild(guildID string) {
	for _, cmd := range b.commands {
		_, err := b.session.ApplicationCommandCreate(
			b.session.State.User.ID, guildID, cmd,
		)
		if err != nil {
			log.Printf("Failed to register command %s in guild %s: %v", cmd.Name, guildID, err)
		} else {
			log.Printf("Registered command %s in guild %s", cmd.Name, guildID)
		}
	}
}

// unregisterCommandsForGuild removes all bot commands from a specific guild.
func (b *Bot) unregisterCommandsForGuild(guildID string) {
	// Fetch all commands for this guild
	cmds, err := b.session.ApplicationCommands(b.session.State.User.ID, guildID)
	if err != nil {
		log.Printf("Failed to fetch commands for guild %s: %v", guildID, err)
		return
	}

	for _, cmd := range cmds {
		err := b.session.ApplicationCommandDelete(
			b.session.State.User.ID, guildID, cmd.ID,
		)
		if err != nil {
			log.Printf("Failed to delete command %s from guild %s: %v", cmd.Name, guildID, err)
		} else {
			log.Printf("Deleted command %s from guild %s", cmd.Name, guildID)
		}
	}
}

// handleMessageCreate processes every message event in real time.
// It ignores messages from bots and DM channels, then parses custom emoji
// mentions and upserts them into the database alongside a message count.
func (b *Bot) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID || m.GuildID == "" {
		return
	}

	if m.Author.Bot {
		return
	}

	emojis := emoji.Parse(m.Content)
	for _, em := range emojis {
		if err := b.db.UpsertEmojiUsage(m.GuildID, em.Name, em.ID, m.Author.ID, m.ChannelID, 1); err != nil {
			log.Printf("Failed to upsert emoji usage: %v", err)
		}
	}

	if err := b.db.UpsertMessageCount(m.GuildID, m.ChannelID, m.Author.ID); err != nil {
		log.Printf("Failed to upsert message count: %v", err)
	}
}

// handleMessageReactionAdd processes emoji reactions on messages.
// Custom emoji reactions are counted toward the emoji's usage statistics.
// Unicode emoji reactions are skipped (no ID, not custom).
func (b *Bot) handleMessageReactionAdd(s *discordgo.Session, m *discordgo.MessageReactionAdd) {
	if m.UserID == s.State.User.ID || m.GuildID == "" {
		return
	}

	// Skip unicode emojis — they have no ID
	if m.Emoji.ID == "" {
		return
	}

	if err := b.db.UpsertEmojiUsage(m.GuildID, m.Emoji.Name, m.Emoji.ID, m.UserID, m.ChannelID, 1); err != nil {
		log.Printf("Failed to upsert emoji reaction: %v", err)
	}
}

// handleGuildCreate registers commands and triggers a bulk scan when the bot
// joins a new guild.
func (b *Bot) handleGuildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	// Register commands for this guild
	b.registerCommandsForGuild(event.Guild.ID)

	// Initial scan if not already done
	if !b.db.GuildScanned(event.Guild.ID) {
		go b.scanner.ScanGuild(event.Guild.ID, s)
	}
}

// handleGuildDelete cleans up commands when the bot leaves a guild.
func (b *Bot) handleGuildDelete(s *discordgo.Session, event *discordgo.GuildDelete) {
	b.unregisterCommandsForGuild(event.Guild.ID)
}

// handleInteractionCreate routes slash command interactions to their respective
// handlers based on command name.
func (b *Bot) handleInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	commands.HandleInteraction(s, i, b.db)
}

// Close terminates the Discord websocket connection.
func (b *Bot) Close() error {
	return b.session.Close()
}
