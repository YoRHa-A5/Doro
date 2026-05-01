// Package commands provides Discord slash command definitions and their handlers.
//
// Commands are registered with Discord on bot startup via ApplicationCommandCreate.
// Interaction handlers are dispatched by name in HandleInteraction.
package commands

import (
	"time"

	"github.com/bwmarrin/discordgo"
)

const zeroWidthSpace = "\u200b"

// ParseTimespan extracts and validates the "timespan" option from an interaction.
// Returns the raw timespan string (e.g. "week", "month", "year") and the
// corresponding since time. Panics if the option is missing — callers should
// verify the option exists before invoking this function.
func ParseTimespan(options []*discordgo.ApplicationCommandInteractionDataOption) (string, time.Time) {
	for _, opt := range options {
		if opt.Name == "timespan" {
			ts := opt.StringValue()
			return ts, time.Now().Add(-timespanToDuration(ts))
		}
	}
	return "", time.Time{}
}

// GridHeader defines a column header for the inline 3-column embed layout.
type GridHeader struct {
	Name string
}

// BuildInlineGrid creates an embed field slice in the 3-column inline format:
// row 0 contains the headers, subsequent rows contain values with empty Name
// so Discord merges them into the same column. The number of columns is
// determined by the length of headers; each row must have exactly that many
// entries.
//
// Usage:
//
//	headers := []GridHeader{{Name: "Top Emojis"}, {Name: "Top Channels"}, {Name: "Top Users"}}
//	emojiValues := [][]string{{emoji1, emoji2, emoji3}, {emoji4, emoji5, emoji6}}
//	fields := commands.BuildInlineGrid(headers, emojiValues)
func BuildInlineGrid(headers []GridHeader, rows [][]string) []*discordgo.MessageEmbedField {
	if len(headers) == 0 {
		return nil
	}

	numCols := len(headers)
	var fields []*discordgo.MessageEmbedField

	// Row 0: headers with actual names
	for _, h := range headers {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   h.Name,
			Value:  "",
			Inline: true,
		})
	}

	// Subsequent rows: empty Name (Discord merges into same column)
	for _, row := range rows {
		for col := 0; col < numCols; col++ {
			val := zeroWidthSpace
			if col < len(row) {
				val = row[col]
			}
			fields = append(fields, &discordgo.MessageEmbedField{
				Name:   "",
				Value:  val,
				Inline: true,
			})
		}
	}

	return fields
}
