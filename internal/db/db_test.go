package db

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	db, err := New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()
}

func TestUpsertEmojiUsage(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	// First insert
	if err := db.UpsertEmojiUsage("guild1", "cool", "123", "user1", "ch1", 1); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	// Second insert should increment
	if err := db.UpsertEmojiUsage("guild1", "cool", "123", "user1", "ch1", 1); err != nil {
		t.Fatalf("second insert failed: %v", err)
	}

	// Verify count is 2
	stats, err := db.GetTopEmojis("guild1", 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(stats) != 1 || stats[0].Count != 2 {
		t.Fatalf("expected count 2, got %d", stats[0].Count)
	}
}

func TestUpsertMessageCount(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	// First insert
	if err := db.UpsertMessageCount("guild1", "ch1", "user1"); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	// Second insert should increment
	if err := db.UpsertMessageCount("guild1", "ch1", "user1"); err != nil {
		t.Fatalf("second insert failed: %v", err)
	}

	// Verify count is 2 - use a time that covers all inserted records
	stats, err := db.GetUserTopChannels("guild1", "user1", time.Time{}, 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(stats) != 1 || stats[0].Count != 2 {
		t.Fatalf("expected count 2, got %d", stats[0].Count)
	}
}

func TestGuildScanned(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	// Not scanned yet
	if db.GuildScanned("guild1") {
		t.Fatal("should not be scanned")
	}

	// Mark as scanned
	if err := db.MarkGuildScanned("guild1"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	// Should be scanned now
	if !db.GuildScanned("guild1") {
		t.Fatal("should be scanned")
	}
}

func TestGetTopEmojis(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	// Insert some data
	db.UpsertEmojiUsage("guild1", "a", "1", "user1", "ch1", 5)
	db.UpsertEmojiUsage("guild1", "b", "2", "user1", "ch1", 3)
	db.UpsertEmojiUsage("guild1", "c", "3", "user1", "ch1", 1)

	stats, err := db.GetTopEmojis("guild1", 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(stats) != 3 {
		t.Fatalf("expected 3 emojis, got %d", len(stats))
	}

	// Verify ordering (a=5, b=3, c=1)
	if stats[0].EmojiName != "a" || stats[0].Count != 5 {
		t.Fatalf("first should be 'a' with count 5, got %s %d", stats[0].EmojiName, stats[0].Count)
	}
	if stats[1].EmojiName != "b" || stats[1].Count != 3 {
		t.Fatalf("second should be 'b' with count 3, got %s %d", stats[1].EmojiName, stats[1].Count)
	}
	if stats[2].EmojiName != "c" || stats[2].Count != 1 {
		t.Fatalf("third should be 'c' with count 1, got %s %d", stats[2].EmojiName, stats[2].Count)
	}
}

func TestGetUserTopEmojis(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	since := time.Now().Add(-24 * time.Hour)

	// Insert data for different users
	db.UpsertEmojiUsage("guild1", "a", "1", "user1", "ch1", 5)
	db.UpsertEmojiUsage("guild1", "b", "2", "user2", "ch1", 3)

	stats, err := db.GetUserTopEmojis("guild1", "user1", since, 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(stats) != 1 || stats[0].EmojiName != "a" || stats[0].Count != 5 {
		t.Fatalf("expected user1 to have only emoji 'a' with count 5")
	}
}

func TestGetServerTopEmojis(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	since := time.Now().Add(-24 * time.Hour)

	db.UpsertEmojiUsage("guild1", "a", "1", "user1", "ch1", 5)
	db.UpsertEmojiUsage("guild1", "b", "2", "user2", "ch1", 3)

	stats, err := db.GetServerTopEmojis("guild1", since, 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 emojis, got %d", len(stats))
	}

	// Should be ordered by total count
	if stats[0].EmojiName != "a" || stats[0].Count != 5 {
		t.Fatalf("first should be 'a' with count 5, got %s %d", stats[0].EmojiName, stats[0].Count)
	}
	if stats[1].EmojiName != "b" || stats[1].Count != 3 {
		t.Fatalf("second should be 'b' with count 3, got %s %d", stats[1].EmojiName, stats[1].Count)
	}
}

func TestGetUserTopChannels(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	since := time.Now().Add(-24 * time.Hour)

	db.UpsertMessageCount("guild1", "ch1", "user1")
	db.UpsertMessageCount("guild1", "ch1", "user1")
	db.UpsertMessageCount("guild1", "ch2", "user1")

	stats, err := db.GetUserTopChannels("guild1", "user1", since, 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(stats))
	}

	// ch1 should have 2 messages, ch2 should have 1
	if stats[0].ChannelID != "ch1" || stats[0].Count != 2 {
		t.Fatalf("first should be ch1 with count 2, got %s %d", stats[0].ChannelID, stats[0].Count)
	}
	if stats[1].ChannelID != "ch2" || stats[1].Count != 1 {
		t.Fatalf("second should be ch2 with count 1, got %s %d", stats[1].ChannelID, stats[1].Count)
	}
}

func TestGetServerTopChannels(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	since := time.Now().Add(-24 * time.Hour)

	db.UpsertMessageCount("guild1", "ch1", "user1")
	db.UpsertMessageCount("guild1", "ch1", "user2")
	db.UpsertMessageCount("guild1", "ch2", "user1")

	stats, err := db.GetServerTopChannels("guild1", since, 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(stats))
	}

	// ch1 should have 2 messages total, ch2 should have 1
	if stats[0].ChannelID != "ch1" || stats[0].Count != 2 {
		t.Fatalf("first should be ch1 with count 2, got %s %d", stats[0].ChannelID, stats[0].Count)
	}
	if stats[1].ChannelID != "ch2" || stats[1].Count != 1 {
		t.Fatalf("second should be ch2 with count 1, got %s %d", stats[1].ChannelID, stats[1].Count)
	}
}

func TestGetUserMessageCount(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	since := time.Now().Add(-24 * time.Hour)

	db.UpsertMessageCount("guild1", "ch1", "user1")
	db.UpsertMessageCount("guild1", "ch2", "user1")
	db.UpsertMessageCount("guild1", "ch3", "user1")

	count, err := db.GetUserMessageCount("guild1", "user1", since)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected 3 messages, got %d", count)
	}
}

func TestGetServerTopUsers(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	since := time.Now().Add(-24 * time.Hour)

	db.UpsertMessageCount("guild1", "ch1", "user1")
	db.UpsertMessageCount("guild1", "ch1", "user1")
	db.UpsertMessageCount("guild1", "ch1", "user2")
	db.UpsertMessageCount("guild1", "ch1", "user3")

	stats, err := db.GetServerTopUsers("guild1", since, 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(stats) != 3 {
		t.Fatalf("expected 3 users, got %d", len(stats))
	}

	// user1 should have 2, user2 should have 1, user3 should have 1
	if stats[0].UserID != "user1" || stats[0].Count != 2 {
		t.Fatalf("first should be user1 with count 2, got %s %d", stats[0].UserID, stats[0].Count)
	}
}

func TestTimeFiltering(t *testing.T) {
	db, _ := New(t.TempDir() + "/test.db")
	defer db.Close()

	// Insert with current timestamp
	db.UpsertEmojiUsage("guild1", "a", "1", "user1", "ch1", 5)

	// Query with far future time should return nothing (last_used is now, which is < far future)
	stats, err := db.GetUserTopEmojis("guild1", "user1", time.Now().Add(24*365*time.Hour), 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(stats) != 0 {
		t.Fatalf("expected 0 emojis with far-future since, got %d", len(stats))
	}

	// Query with zero time should return all records
	stats, err = db.GetUserTopEmojis("guild1", "user1", time.Time{}, 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(stats) != 1 {
		t.Fatalf("expected 1 emoji with zero time, got %d", len(stats))
	}
}
