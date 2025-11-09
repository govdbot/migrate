package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// represents the type of chat in v2 database
type ChatType string

const (
	ChatTypePrivate ChatType = "private"
	ChatTypeGroup   ChatType = "group"
)

// represents the user structure in v1 database
type User struct {
	gorm.Model
	UserID   int64     `gorm:"primaryKey"`
	LastUsed time.Time `gorm:"autoCreateTime"`
}

func (User) TableName() string {
	return "users"
}

// represents the group settings structure in v1 database
type GroupSettings struct {
	gorm.Model
	ChatID          int64 `gorm:"primaryKey"`
	NSFW            *bool
	Captions        *bool
	MediaGroupLimit int
	Silent          *bool
}

func (GroupSettings) TableName() string {
	return "group_settings"
}

// holds the database connection strings from environment variables
type Config struct {
	V1DSN string // V1 database connection string (MariaDB/MySQL)
	V2DSN string // V2 database connection string (PostgreSQL)
}

func main() {
	log.Println("starting v1 to v2 migration tool...")

	config, err := loadConfigFromEnv()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx := context.Background()

	dbV1, err := connectV1(config.V1DSN)
	if err != nil {
		log.Fatalf("failed to connect to v1 database: %v", err)
	}
	log.Println("✓ connected to v1 database (MariaDB)")

	dbV2, err := connectV2(ctx, config.V2DSN)
	if err != nil {
		log.Fatalf("failed to connect to v2 database: %v", err)
	}
	defer dbV2.Close()
	log.Println("✓ connected to v2 database (PostgreSQL)")

	var users []User
	result := dbV1.Find(&users)
	if result.Error != nil {
		log.Fatalf("failed to fetch users from v1: %v", result.Error)
	}
	log.Printf("✓ found %d users in v1 database", len(users))

	var groupSettings []GroupSettings
	result = dbV1.Find(&groupSettings)
	if result.Error != nil {
		log.Fatalf("failed to fetch group settings from v1: %v", result.Error)
	}
	log.Printf("✓ found %d group settings in v1 database", len(groupSettings))

	usersMigrated := 0
	usersFailed := 0
	settingsMigrated := 0
	settingsFailed := 0

	for _, user := range users {
		err := migrateUser(ctx, dbV2, user)
		if err != nil {
			log.Printf("✗ failed to migrate user %d: %v", user.UserID, err)
			usersFailed++
			continue
		}
		usersMigrated++
		log.Printf("✓ migrated user: %d", user.UserID)
	}

	for _, settings := range groupSettings {
		err := migrateGroup(ctx, dbV2, settings)
		if err != nil {
			log.Printf("✗ failed to migrate chat %d: %v", settings.ChatID, err)
			settingsFailed++
			continue
		}
		settingsMigrated++
		log.Printf("✓ migrated chat: %d", settings.ChatID)
	}

	if usersFailed > 0 || settingsFailed > 0 {
		os.Exit(1)
	}
}

func loadConfigFromEnv() (*Config, error) {
	v1DSN := os.Getenv("V1_DSN")
	v2DSN := os.Getenv("V2_DSN")

	if v1DSN == "" {
		return nil, fmt.Errorf("V1_DSN environment variable is required")
	}
	if v2DSN == "" {
		return nil, fmt.Errorf("V2_DSN environment variable is required")
	}

	return &Config{
		V1DSN: v1DSN,
		V2DSN: v2DSN,
	}, nil
}

func connectV1(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return db, nil
}

func connectV2(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	return pool, nil
}

func getBool(ptr *bool, defaultVal bool) bool {
	if ptr != nil {
		return *ptr
	}
	return defaultVal
}

func migrateUser(ctx context.Context, dbV2 *pgxpool.Pool, user User) error {
	chatQuery := `
		INSERT INTO chat (chat_id, type, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (chat_id) DO NOTHING
	`
	_, err := dbV2.Exec(ctx, chatQuery, user.UserID, ChatTypePrivate, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert chat: %w", err)
	}

	settingsQuery := `
		INSERT INTO settings (chat_id, nsfw, media_album_limit, captions, silent, language, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (chat_id) DO UPDATE SET
			updated_at = EXCLUDED.updated_at
	`
	_, err = dbV2.Exec(ctx, settingsQuery,
		user.UserID,
		false,
		10,
		true,
		false,
		"XX",
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert settings: %w", err)
	}

	return nil
}

func migrateGroup(ctx context.Context, dbV2 *pgxpool.Pool, settings GroupSettings) error {
	chatQuery := `
		INSERT INTO chat (chat_id, type, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (chat_id) DO NOTHING
	`
	_, err := dbV2.Exec(ctx, chatQuery, settings.ChatID, ChatTypeGroup, settings.CreatedAt, settings.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert chat: %w", err)
	}

	settingsQuery := `
		INSERT INTO settings (chat_id, nsfw, media_album_limit, captions, silent, language, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (chat_id) DO UPDATE SET
			nsfw = EXCLUDED.nsfw,
			media_album_limit = EXCLUDED.media_album_limit,
			captions = EXCLUDED.captions,
			silent = EXCLUDED.silent,
			updated_at = EXCLUDED.updated_at
	`
	_, err = dbV2.Exec(ctx, settingsQuery,
		settings.ChatID,
		getBool(settings.NSFW, false),
		settings.MediaGroupLimit,
		getBool(settings.Captions, true),
		getBool(settings.Silent, false),
		"XX",
		settings.CreatedAt,
		settings.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert settings: %w", err)
	}

	return nil
}
