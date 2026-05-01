// Doro is a Discord bot that tracks custom emoji usage and message counts
// per user, channel, and server over time.
//
// It persists data to SQLite and provides three slash commands:
//   - /emoji-stats       — top 10 emojis in the server (all time)
//   - /recap-user        — user's personal recap (week/month/year)
//   - /recap-server      — server-wide recap (week/month/year)
//
// When the bot joins a guild for the first time it performs an initial scan of
// the last 30 days of message history to populate the database.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/YoRHa-A5/Doro/internal/bot"
	"github.com/YoRHa-A5/Doro/internal/commands"
	"github.com/YoRHa-A5/Doro/internal/db"
)

func main() {
	godotenv.Load()

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_TOKEN environment variable is required")
	}
	// discordgo expects the "Bot " prefix for REST calls, not just gateway connections.
	if len(token) > 4 && token[:4] != "Bot " {
		token = "Bot " + token
	}

	// GUILD_ID and DEV_MODE are optional. If DEV_MODE=true and GUILD_ID is set,
	// slash commands are registered guild-specifically for faster iteration.
	// Otherwise they are registered globally (up to 1h propagation).
	devGuildID := os.Getenv("GUILD_ID")
	devMode := os.Getenv("DEV_MODE") == "true"

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "data/doro.db"
	}
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	b, err := bot.New(token, database, commands.Commands, devGuildID, devMode)
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}
	defer b.Close()

	log.Println("Doro is running. Press Ctrl+C to exit.")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
}
