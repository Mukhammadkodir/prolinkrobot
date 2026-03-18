package models

import "time"

type SaveTelegramUserRequest struct {
	TelegramID int64
	Name       string
	Username   string
	Language   string
	DailyLimit int
	LimitDate  *time.Time
}

type TelegramUser struct {
	TelegramID         int64      `bson:"tg_id"`
	Name               string     `bson:"name"`
	Username           string     `bson:"username,omitempty"`
	Language           string     `bson:"lang"`
	DailyLimit         int        `bson:"daily_limit"`
	LimitDate          *time.Time `bson:"limit_date,omitempty"`
	DownloadsToday     int        `bson:"downloads_today"`
	DownloadsResetDate string     `bson:"downloads_reset_date"`
	TotalDownloads     int        `bson:"total_downloads"`
	CreateDate         time.Time  `bson:"create_date"`
	UpdateDate         time.Time  `bson:"update_date"`
}

type DownloadAttempt struct {
	Allowed        bool
	DownloadsToday int
	DailyLimit     int
	ErrorMessage   string
}

type UserStats struct {
	Name           string
	Username       string
	DownloadsToday int
	TotalDownloads int
	DailyLimit     int
	LimitDate      *time.Time
	CreateDate     *time.Time
	TrialEnd       *time.Time
}

type AssetCacheEntry struct {
	Key              string    `bson:"key"`
	SourceURL        string    `bson:"source_url"`
	ChannelChatID    int64     `bson:"channel_chat_id"`
	ChannelMessageID int       `bson:"channel_message_id"`
	AssetType        string    `bson:"asset_type,omitempty"`
	CreatedAt        time.Time `bson:"created_at"`
}

type ThreeDFormatOption struct {
	ID       int
	Name     string
	FileType string
	Enabled  bool
}
