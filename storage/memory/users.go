package memory

import (
	"fmt"
	"get-link-tg-bot/models"
	"get-link-tg-bot/storage"
	"sync"
	"time"
)

type Users struct {
	mu    sync.RWMutex
	users map[int64]*models.TelegramUser
}

var _ storage.UsersI = (*Users)(nil)

func NewUsers() *Users {
	return &Users{users: make(map[int64]*models.TelegramUser)}
}

func (u *Users) UserExists(tgID int64) (bool, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	_, ok := u.users[tgID]
	return ok, nil
}

func (u *Users) GetUserByTelegramID(tgID int64) (*models.TelegramUser, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	user, ok := u.users[tgID]
	if !ok {
		return nil, nil
	}
	clone := *user
	return &clone, nil
}

func (u *Users) SaveTelegramUser(req *models.SaveTelegramUserRequest) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, ok := u.users[req.TelegramID]; ok {
		return nil
	}
	now := time.Now()
	u.users[req.TelegramID] = &models.TelegramUser{
		TelegramID:         req.TelegramID,
		Name:               req.Name,
		Username:           req.Username,
		Language:           req.Language,
		DailyLimit:         req.DailyLimit,
		LimitDate:          req.LimitDate,
		DownloadsResetDate: now.Format("2006-01-02"),
		CreateDate:         now,
		UpdateDate:         now,
	}
	return nil
}

func (u *Users) UpdateUserLanguage(tgID int64, language string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	user, ok := u.users[tgID]
	if !ok {
		return fmt.Errorf("user not found")
	}
	user.Language = language
	user.UpdateDate = time.Now()
	return nil
}

func (u *Users) GetCachedAssetByKey(key string) (*models.AssetCacheEntry, error) {
	return nil, fmt.Errorf("memory storage does not implement asset cache")
}

func (u *Users) SaveCachedAsset(key, sourceURL string, channelChatID int64, channelMessageID int, assetType string) error {
	return fmt.Errorf("memory storage does not implement asset cache")
}

func (u *Users) DeleteCachedAssetByKey(key string) error {
	return fmt.Errorf("memory storage does not implement asset cache")
}

func (u *Users) TryIncrementDownload(tgID int64) (*models.DownloadAttempt, error) {
	return nil, fmt.Errorf("memory storage does not implement limit logic")
}

func (u *Users) DecrementDownload(tgID int64) error {
	return fmt.Errorf("memory storage does not implement limit logic")
}

func (u *Users) GetUserStats(tgID int64) (*models.UserStats, error) {
	return nil, fmt.Errorf("memory storage does not implement stats")
}

func (u *Users) UpdateUserLimit(tgID int64, dailyLimit int, limitDate *time.Time) (bool, error) {
	return false, fmt.Errorf("memory storage does not implement admin limits")
}

func (u *Users) ResetDailyLimits() (int64, error) {
	return 0, fmt.Errorf("memory storage does not implement reset")
}
