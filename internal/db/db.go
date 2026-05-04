// Package db provides database operations backed by SQLite.
//
// SQLite was chosen for zero-configuration persistence. The database uses WAL
// mode for safe concurrent reads. Schema migrations run automatically on startup
// via CREATE TABLE IF NOT EXISTS statements.
//
// Emoji and message counts are stored as independent rows with composite UNIQUE
// constraints. All time comparisons use UTC timestamps.
package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/YoRHa-A5/Doro/internal/models"
)

// DB wraps sql.DB with typed helper methods for the bot's data model.
type DB struct {
	*sql.DB
}

// New opens a SQLite database at dbPath, enables WAL mode, and runs migrations.
// WAL mode allows concurrent reads while writing, which is important since the
// scanner and live message handlers may access the DB simultaneously.
func New(dbPath string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	d := &DB{sqlDB}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return d, nil
}

// migrate creates the database schema if it does not exist.
//
// Schema:
//   - [guilds]     — one row per scanned guild, guards against duplicate scans
//   - [emoji_usage]— per-emoji per-user per-channel per-day usage counters; UNIQUE on
//                    (guild_id, emoji_id, user_id, channel_id, period_date)
//   - [message_counts] — per-channel per-user per-day message counters; UNIQUE on
//                    (guild_id, channel_id, user_id, period_date)
func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS guilds (
		id TEXT PRIMARY KEY,
		scanned_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS emoji_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		guild_id TEXT NOT NULL,
		emoji_name TEXT NOT NULL,
		emoji_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		count INTEGER NOT NULL DEFAULT 0,
		period_date TEXT NOT NULL,
		first_used DATETIME,
		last_used DATETIME,
		UNIQUE(guild_id, emoji_id, user_id, channel_id, period_date)
	);

	CREATE TABLE IF NOT EXISTS message_counts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		guild_id TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		count INTEGER NOT NULL DEFAULT 0,
		period_date TEXT NOT NULL,
		first_seen DATETIME,
		last_seen DATETIME,
		UNIQUE(guild_id, channel_id, user_id, period_date)
	);
	`

	_, err := d.Exec(schema)
	if err != nil {
		return err
	}

	// Migrate from the old schema (without period_date) if needed.
	return d.migratePeriodDate()
}

// migratePeriodDate adds a period_date column to the old tables, copies data
// into new tables with per-day granularity, and replaces the old tables.
func (d *DB) migratePeriodDate() error {
	// Check if migration is needed by trying to query period_date.
	var periodDate string
	err := d.QueryRow("SELECT period_date FROM emoji_usage LIMIT 1").Scan(&periodDate)
	if err == nil {
		// period_date column already exists — already migrated.
		return nil
	}

	// If _old tables already exist (e.g. from a failed prior migration),
	// check whether they have data. If they do, we need to merge their data
	// into the new tables before dropping them.
	//
	// Scenario: a previous migration renamed the original tables to _old,
	// created new empty tables with period_date, then failed before dropping
	// the _old tables. In that case the _old tables have the real data.
	hasOldData := false
	var oldEmojiCount, oldMsgCount int
	err = d.QueryRow("SELECT COUNT(*) FROM emoji_usage_old").Scan(&oldEmojiCount)
	if err == nil && oldEmojiCount > 0 {
		hasOldData = true
	}
	err = d.QueryRow("SELECT COUNT(*) FROM message_counts_old").Scan(&oldMsgCount)
	if err == nil && oldMsgCount > 0 {
		hasOldData = true
	}

	// Only drop _old tables if they have no data.
	if hasOldData {
		// The _old tables have data — keep them. We'll copy from them below.
		// But first we need to rename the current (empty) tables.
		// Rename current tables to _backup.
		d.Exec("DROP TABLE IF EXISTS emoji_usage_backup")
		d.Exec("DROP TABLE IF EXISTS message_counts_backup")
		d.Exec("ALTER TABLE emoji_usage RENAME TO emoji_usage_backup")
		d.Exec("ALTER TABLE message_counts RENAME TO message_counts_backup")
	} else {
		// No old data — safe to drop _old tables.
		d.Exec("DROP TABLE IF EXISTS emoji_usage_old")
		d.Exec("DROP TABLE IF EXISTS message_counts_old")
	}

	// Rename old tables (or backups) to _old for the copy step.
	if hasOldData {
		// emoji_usage_old already exists with data from the previous rename.
		// The current tables are now emoji_usage_backup (empty).
		// We don't need to rename anything — emoji_usage_old already has the data.
	} else {
		if _, err := d.Exec("ALTER TABLE emoji_usage RENAME TO emoji_usage_old"); err != nil {
			return fmt.Errorf("failed to rename emoji_usage: %w", err)
		}
		if _, err := d.Exec("ALTER TABLE message_counts RENAME TO message_counts_old"); err != nil {
			return fmt.Errorf("failed to rename message_counts: %w", err)
		}
	}

	// Create new tables with period_date.
	_, err = d.Exec(`
		CREATE TABLE emoji_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			guild_id TEXT NOT NULL,
			emoji_name TEXT NOT NULL,
			emoji_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			channel_id TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			period_date TEXT NOT NULL,
			first_used DATETIME,
			last_used DATETIME,
			UNIQUE(guild_id, emoji_id, user_id, channel_id, period_date)
		);

		CREATE TABLE message_counts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			guild_id TEXT NOT NULL,
			channel_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			period_date TEXT NOT NULL,
			first_seen DATETIME,
			last_seen DATETIME,
			UNIQUE(guild_id, channel_id, user_id, period_date)
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create new tables: %w", err)
	}

	// Copy old emoji_usage data into new tables, assigning each row to
	// the date of its last_used timestamp.
	_, err = d.Exec(`
		INSERT INTO emoji_usage (guild_id, emoji_name, emoji_id, user_id, channel_id, count, period_date, first_used, last_used)
		SELECT guild_id, emoji_name, emoji_id, user_id, channel_id, count,
			   date(last_used), first_used, last_used
		FROM emoji_usage_old
	`)
	if err != nil {
		return fmt.Errorf("failed to migrate emoji_usage: %w", err)
	}

	// Copy old message_counts data into new tables, assigning each row to
	// the date of its last_seen timestamp.
	_, err = d.Exec(`
		INSERT INTO message_counts (guild_id, channel_id, user_id, count, period_date, first_seen, last_seen)
		SELECT guild_id, channel_id, user_id, count,
			   date(last_seen), first_seen, last_seen
		FROM message_counts_old
	`)
	if err != nil {
		return fmt.Errorf("failed to migrate message_counts: %w", err)
	}

	// Drop old tables.
	d.Exec("DROP TABLE emoji_usage_old")
	d.Exec("DROP TABLE message_counts_old")

	// Drop backup tables if they exist.
	d.Exec("DROP TABLE IF EXISTS emoji_usage_backup")
	d.Exec("DROP TABLE IF EXISTS message_counts_backup")

	return nil
}

// GuildScanned returns true if the given guild ID has already been scanned.
func (d *DB) GuildScanned(guildID string) bool {
	var id string
	err := d.QueryRow("SELECT id FROM guilds WHERE id = ?", guildID).Scan(&id)
	return err == nil
}

// MarkGuildScanned records that a guild has been scanned. Uses INSERT OR REPLACE
// so re-scans are idempotent.
func (d *DB) MarkGuildScanned(guildID string) error {
	_, err := d.Exec(
		"INSERT OR REPLACE INTO guilds (id, scanned_at) VALUES (?, ?)",
		guildID, time.Now().UTC(),
	)
	return err
}

// UpsertEmojiUsage increments the usage count for one emoji used by one user
// in one channel of one guild for today's date. On conflict the count is
// incremented by count and last_used is updated. first_used is set only on insert.
func (d *DB) UpsertEmojiUsage(guildID, emojiName, emojiID, userID, channelID string, count int) error {
	return d.UpsertEmojiUsageAt(guildID, emojiName, emojiID, userID, channelID, count, time.Now().UTC())
}

// UpsertEmojiUsageAt is like UpsertEmojiUsage but accepts an explicit timestamp
// so the emoji usage is recorded under the message's actual date rather than today.
func (d *DB) UpsertEmojiUsageAt(guildID, emojiName, emojiID, userID, channelID string, count int, ts time.Time) error {
	now := ts.UTC()
	day := now.Format("2006-01-02")
	_, err := d.Exec(`
		INSERT INTO emoji_usage (guild_id, emoji_name, emoji_id, user_id, channel_id, count, period_date, first_used, last_used)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(guild_id, emoji_id, user_id, channel_id, period_date) DO UPDATE SET
			count = count + excluded.count,
			last_used = excluded.last_used
	`, guildID, emojiName, emojiID, userID, channelID, count, day, now, now)
	return err
}

// UpsertMessageCount increments the message count for one user in one channel
// of one guild for today's date. On conflict the count is incremented by 1 and
// last_seen is updated.
func (d *DB) UpsertMessageCount(guildID, channelID, userID string) error {
	return d.UpsertMessageCountAt(guildID, channelID, userID, time.Now().UTC())
}

// UpsertMessageCountAt is like UpsertMessageCount but accepts an explicit
// timestamp so the message count is recorded under the message's actual date.
func (d *DB) UpsertMessageCountAt(guildID, channelID, userID string, ts time.Time) error {
	now := ts.UTC()
	day := now.Format("2006-01-02")
	_, err := d.Exec(`
		INSERT INTO message_counts (guild_id, channel_id, user_id, count, period_date, first_seen, last_seen)
		VALUES (?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT(guild_id, channel_id, user_id, period_date) DO UPDATE SET
			count = count + 1,
			last_seen = excluded.last_seen
	`, guildID, channelID, userID, day, now, now)
	return err
}

// GetTopEmojis returns the top [limit] most-used emojis across a guild.
// The result aggregates across all users and channels.
func (d *DB) GetTopEmojis(guildID string, limit int) ([]models.EmojiStat, error) {
	rows, err := d.Query(`
		SELECT emoji_name, emoji_id, SUM(count) as total
		FROM emoji_usage
		WHERE guild_id = ?
		GROUP BY emoji_id
		ORDER BY total DESC
		LIMIT ?
	`, guildID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.EmojiStat
	for rows.Next() {
		var s models.EmojiStat
		if err := rows.Scan(&s.EmojiName, &s.EmojiID, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// GetUserTopEmojis returns the top [limit] most-used emojis by a specific user
// in a guild since [since]. Uses period_date to filter by time.
func (d *DB) GetUserTopEmojis(guildID, userID string, since time.Time, limit int) ([]models.EmojiStat, error) {
	sinceDate := since.Format("2006-01-02")
	rows, err := d.Query(`
		SELECT emoji_name, emoji_id, SUM(count) as total
		FROM emoji_usage
		WHERE guild_id = ? AND user_id = ? AND period_date >= ?
		GROUP BY emoji_id
		ORDER BY total DESC
		LIMIT ?
	`, guildID, userID, sinceDate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.EmojiStat
	for rows.Next() {
		var s models.EmojiStat
		if err := rows.Scan(&s.EmojiName, &s.EmojiID, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// GetServerTopEmojis returns the top [limit] most-used emojis across a guild
// since [since]. Uses period_date to filter by time.
func (d *DB) GetServerTopEmojis(guildID string, since time.Time, limit int) ([]models.EmojiStat, error) {
	sinceDate := since.Format("2006-01-02")
	rows, err := d.Query(`
		SELECT emoji_name, emoji_id, SUM(count) as total
		FROM emoji_usage
		WHERE guild_id = ? AND period_date >= ?
		GROUP BY emoji_id
		ORDER BY total DESC
		LIMIT ?
	`, guildID, sinceDate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.EmojiStat
	for rows.Next() {
		var s models.EmojiStat
		if err := rows.Scan(&s.EmojiName, &s.EmojiID, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// GetUserTopChannels returns the top [limit] channels by message count for a
// specific user in a guild since [since]. Uses period_date to filter by time.
func (d *DB) GetUserTopChannels(guildID, userID string, since time.Time, limit int) ([]models.ChannelStat, error) {
	sinceDate := since.Format("2006-01-02")
	rows, err := d.Query(`
		SELECT mc.channel_id, SUM(mc.count) as total
		FROM message_counts mc
		WHERE mc.guild_id = ? AND mc.user_id = ? AND mc.period_date >= ?
		GROUP BY mc.channel_id
		ORDER BY total DESC
		LIMIT ?
	`, guildID, userID, sinceDate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.ChannelStat
	for rows.Next() {
		var s models.ChannelStat
		if err := rows.Scan(&s.ChannelID, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// GetServerTopChannels returns the top [limit] channels by total message count
// in a guild since [since]. Uses period_date to filter by time.
func (d *DB) GetServerTopChannels(guildID string, since time.Time, limit int) ([]models.ChannelStat, error) {
	sinceDate := since.Format("2006-01-02")
	rows, err := d.Query(`
		SELECT channel_id, SUM(count) as total
		FROM message_counts
		WHERE guild_id = ? AND period_date >= ?
		GROUP BY channel_id
		ORDER BY total DESC
		LIMIT ?
	`, guildID, sinceDate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.ChannelStat
	for rows.Next() {
		var s models.ChannelStat
		if err := rows.Scan(&s.ChannelID, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// GetUserMessageCount returns the total message count for a user in a guild
// since [since]. Uses period_date to filter by time.
func (d *DB) GetUserMessageCount(guildID, userID string, since time.Time) (int, error) {
	sinceDate := since.Format("2006-01-02")
	var count int
	err := d.QueryRow(`
		SELECT COALESCE(SUM(count), 0)
		FROM message_counts
		WHERE guild_id = ? AND user_id = ? AND period_date >= ?
	`, guildID, userID, sinceDate).Scan(&count)
	return count, err
}

// GetServerTopUsers returns the top [limit] users by message count in a guild
// since [since]. Uses period_date to filter by time.
func (d *DB) GetServerTopUsers(guildID string, since time.Time, limit int) ([]models.UserActivityStat, error) {
	sinceDate := since.Format("2006-01-02")
	rows, err := d.Query(`
		SELECT user_id, SUM(count) as total
		FROM message_counts
		WHERE guild_id = ? AND period_date >= ?
		GROUP BY user_id
		ORDER BY total DESC
		LIMIT ?
	`, guildID, sinceDate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.UserActivityStat
	for rows.Next() {
		var s models.UserActivityStat
		if err := rows.Scan(&s.UserID, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}
