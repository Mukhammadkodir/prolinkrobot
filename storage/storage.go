package storage

import (
	"get-link-tg-bot/models"
	"time"
)

type UsersI interface {
	UserExists(tgID int64) (bool, error)
	GetUserByTelegramID(tgID int64) (*models.TelegramUser, error)
	SaveTelegramUser(req *models.SaveTelegramUserRequest) error
	UpdateUserLanguage(tgID int64, language string) error
	GetCachedAssetByKey(key string) (*models.AssetCacheEntry, error)
	SaveCachedAsset(key, sourceURL string, channelChatID int64, channelMessageID int, assetType string) error
	DeleteCachedAssetByKey(key string) error
	TryIncrementDownload(tgID int64) (*models.DownloadAttempt, error)
	DecrementDownload(tgID int64) error
	GetUserStats(tgID int64) (*models.UserStats, error)
	UpdateUserLimit(tgID int64, dailyLimit int, limitDate *time.Time) (bool, error)
	ResetDailyLimits() (int64, error)
}
