package commands

import (
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestBuildInlineGrid(t *testing.T) {
	t.Run("empty headers", func(t *testing.T) {
		result := BuildInlineGrid([]GridHeader{}, nil)
		if len(result) != 0 {
			t.Fatalf("expected 0 fields, got %d", len(result))
		}
	})

	t.Run("single header", func(t *testing.T) {
		headers := []GridHeader{{Name: "Test"}}
		rows := [][]string{{"value1"}, {"value2"}}
		result := BuildInlineGrid(headers, rows)

		if len(result) != 3 {
			t.Fatalf("expected 3 fields, got %d", len(result))
		}

		// Header row
		if result[0].Name != "Test" {
			t.Errorf("expected name 'Test', got %q", result[0].Name)
		}
		if result[0].Value != "" {
			t.Errorf("expected empty value for header, got %q", result[0].Value)
		}

		// Data rows
		if result[1].Name != "" {
			t.Errorf("expected empty name for data row, got %q", result[1].Name)
		}
		if result[1].Value != "value1" {
			t.Errorf("expected value 'value1', got %q", result[1].Value)
		}
		if result[2].Value != "value2" {
			t.Errorf("expected value 'value2', got %q", result[2].Value)
		}
	})

	t.Run("multiple headers with padding", func(t *testing.T) {
		headers := []GridHeader{
			{Name: "Col1"},
			{Name: "Col2"},
			{Name: "Col3"},
		}
		rows := [][]string{
			{"a", "b"}, // row 1: missing third column
			{"c", "d", "e"}, // row 2: complete
		}
		result := BuildInlineGrid(headers, rows)

		// 3 headers + 2 rows * 3 cols = 3 + 6 = 9 fields
		if len(result) != 9 {
			t.Fatalf("expected 9 fields, got %d", len(result))
		}

		// Header row
		if result[0].Name != "Col1" || result[1].Name != "Col2" || result[2].Name != "Col3" {
			t.Fatal("header names mismatch")
		}

		// Row 1: col3 should be zero-width-space
		if result[3].Value != "a" {
			t.Errorf("row1 col1: expected 'a', got %q", result[3].Value)
		}
		if result[4].Value != "b" {
			t.Errorf("row1 col2: expected 'b', got %q", result[4].Value)
		}
		if result[5].Value != "\u200b" {
			t.Errorf("row1 col3: expected zero-width-space, got %q", result[5].Value)
		}

		// Row 2: complete
		if result[6].Value != "c" {
			t.Errorf("row2 col1: expected 'c', got %q", result[6].Value)
		}
	})

	t.Run("empty rows", func(t *testing.T) {
		headers := []GridHeader{{Name: "Test"}}
		rows := [][]string{}
		result := BuildInlineGrid(headers, rows)

		if len(result) != 1 {
			t.Fatalf("expected 1 field, got %d", len(result))
		}
	})

	t.Run("all inline", func(t *testing.T) {
		headers := []GridHeader{
			{Name: "A"},
			{Name: "B"},
		}
		rows := [][]string{{"x", "y"}}
		result := BuildInlineGrid(headers, rows)

		for _, field := range result {
			if !field.Inline {
				t.Error("all fields should be inline")
			}
		}
	})
}

func TestParseTimespan(t *testing.T) {
	tests := []struct {
		name     string
		options  []*discordgo.ApplicationCommandInteractionDataOption
		wantTS   string
		wantTime time.Time
	}{
		{
			name:   "week",
			options: []*discordgo.ApplicationCommandInteractionDataOption{
				{Name: "timespan", Type: discordgo.ApplicationCommandOptionString, Value: "week"},
			},
			wantTS: "week",
		},
		{
			name:   "month",
			options: []*discordgo.ApplicationCommandInteractionDataOption{
				{Name: "timespan", Type: discordgo.ApplicationCommandOptionString, Value: "month"},
			},
			wantTS: "month",
		},
		{
			name:   "year",
			options: []*discordgo.ApplicationCommandInteractionDataOption{
				{Name: "timespan", Type: discordgo.ApplicationCommandOptionString, Value: "year"},
			},
			wantTS: "year",
		},
		{
			name:   "no timespan",
			options: []*discordgo.ApplicationCommandInteractionDataOption{
				{Name: "other", Type: discordgo.ApplicationCommandOptionString, Value: "value"},
			},
			wantTS: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, _ := ParseTimespan(tt.options)
			if ts != tt.wantTS {
				t.Errorf("expected timespan %q, got %q", tt.wantTS, ts)
			}
		})
	}
}

func TestTimespanToDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"week", 7 * 24 * time.Hour},
		{"month", 30 * 24 * time.Hour},
		{"year", 365 * 24 * time.Hour},
		{"unknown", 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			dur := timespanToDuration(tt.input)
			if dur != tt.expected {
				t.Errorf("expected %v for %s, got %v", tt.expected, tt.input, dur)
			}
		})
	}
}

func TestTimespanToAdverb(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"week", "Weekly"},
		{"month", "Monthly"},
		{"year", "Yearly"},
		{"unknown", "Monthly"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := timespanToAdverb(tt.input)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestDisplayUsername(t *testing.T) {
	tests := []struct {
		name     string
		global   string
		username string
		expected string
	}{
		{"with global name", "CustomName", "username", "CustomName"},
		{"without global name", "", "username", "username"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &discordgo.User{
				GlobalName: tt.global,
				Username:   tt.username,
			}
			got := displayUsername(u)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestEmojiMention(t *testing.T) {
	got := emojiMention("cool", "123")
	if got != "<:cool:123>" {
		t.Errorf("expected '<:cool:123>', got %q", got)
	}
}

func TestResolveChannelName(t *testing.T) {
	// Just verify the function exists and has correct signature
	_ = func(s *discordgo.Session, channelID string) string {
		return resolveChannelName(s, channelID)
	}
}

func TestResolveUserDisplayName(t *testing.T) {
	// Just verify the function exists and has correct signature
	_ = func(s *discordgo.Session, guildID, userID string) string {
		return resolveUserDisplayName(s, guildID, userID)
	}
}
