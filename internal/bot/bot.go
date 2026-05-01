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
	session     *discordgo.Session
	db          *db.DB
	scanner     *scanner.Scanner
	commands    []*discordgo.ApplicationCommand
	devGuildID  string
	devMode     bool
	initialized bool
}

// New creates a new bot: initializes the Discord session, opens the websocket,
// and sets up event handlers. Slash commands are registered when the Ready
// event fires, ensuring the bot's application ID is populated for REST calls.
func New(token string, database *db.DB, commands []*discordgo.ApplicationCommand, devGuildID string, devMode bool) (*Bot, error) {
	session, err := discordgo.New(token)
	if err != nil {
		return nil, err
	}

	// Intents: message content to parse emojis, guild messages to receive them,
	// and guilds to receive GuildCreate events when joining a server.
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent |
		discordgo.IntentsGuilds

	b := &Bot{
		session:    session,
		db:         database,
		scanner:    scanner.New(database),
		commands:   commands,
		devGuildID: devGuildID,
		devMode:    devMode,
	}

	// Register the Ready handler before opening the session so we don't miss it.
	session.AddHandler(b.handleReady)
	session.AddHandler(b.handleMessageCreate)
	session.AddHandler(b.handleGuildCreate)
	session.AddHandler(b.handleInteractionCreate)

	if err := session.Open(); err != nil {
		return nil, err
	}

	return b, nil
}

// handleReady fires once when the bot has connected and is identified.
// At this point session.State.User is populated, so we can safely register
// slash commands via the REST API without 401 errors.
func (b *Bot) handleReady(s *discordgo.Session, event *discordgo.Ready) {
	if b.initialized {
		return
	}
	b.initialized = true

	b.registerCommands()
}

// registerCommands registers slash commands with Discord. If devMode is enabled
// and devGuildID is set, commands are registered guild-specifically for instant
// propagation. Otherwise they are registered globally (up to 1h to propagate).
func (b *Bot) registerCommands() {
	for _, cmd := range b.commands {
		var err error
		if b.devMode && b.devGuildID != "" {
			_, err = b.session.ApplicationCommandCreate(b.session.State.User.ID, b.devGuildID, cmd)
			if err == nil {
				log.Printf("Registered command %s guild-specific (DEV_MODE)", cmd.Name)
			}
		} else {
			_, err = b.session.ApplicationCommandCreate(b.session.State.User.ID, "", cmd)
			if err == nil {
				log.Printf("Registered command %s globally", cmd.Name)
			}
		}
		if err != nil {
			log.Printf("Failed to register command %s: %v", cmd.Name, err)
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

// handleGuildCreate triggers a bulk scan when the bot joins a new guild.
// It only scans if the guild is not already in the database (prevents re-scans
// on reconnects). The scan runs asynchronously.
func (b *Bot) handleGuildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	if b.db.GuildScanned(event.Guild.ID) {
		return
	}

	go b.scanner.ScanGuild(event.Guild.ID, s)
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
