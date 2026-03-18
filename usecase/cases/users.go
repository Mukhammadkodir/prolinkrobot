package cases

import (
	"get-link-tg-bot/config"
	"get-link-tg-bot/models"
	"get-link-tg-bot/pkg/helper"
	"get-link-tg-bot/storage"
	"get-link-tg-bot/usecase"
	"regexp"
	"strings"
	"time"

	"github.com/sulton0011/errs"
)

var freepikURLRegex = regexp.MustCompile(`(?i)(https?://[^\s]+|www\.[^\s]+)`)

type UsecaseUsers struct {
	storageUsers storage.UsersI
	adminIDs     map[int64]struct{}
}

var _ usecase.UsecaseUsersI = (*UsecaseUsers)(nil)

func NewUsecaseUsers(cfg *config.Config, storageUsers storage.UsersI) *UsecaseUsers {
	adminIDs := make(map[int64]struct{}, len(cfg.Admin.TelegramIDs))
	for _, id := range cfg.Admin.TelegramIDs {
		adminIDs[id] = struct{}{}
	}
	return &UsecaseUsers{storageUsers: storageUsers, adminIDs: adminIDs}
}

func (u *UsecaseUsers) UserExists(tgID int64) (ok bool, err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "UserExists")
	return u.storageUsers.UserExists(tgID)
}

func (u *UsecaseUsers) GetUserByTelegramID(tgID int64) (user *models.TelegramUser, err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "GetUserByTelegramID")
	return u.storageUsers.GetUserByTelegramID(tgID)
}

func (u *UsecaseUsers) SaveTelegramUser(req *models.SaveTelegramUserRequest) (err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "SaveTelegramUser")
	return u.storageUsers.SaveTelegramUser(req)
}

func (u *UsecaseUsers) UpdateUserLanguage(tgID int64, language string) (err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "UpdateUserLanguage")
	return u.storageUsers.UpdateUserLanguage(tgID, language)
}

func (u *UsecaseUsers) GetCachedAssetByKey(key string) (entry *models.AssetCacheEntry, err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "GetCachedAssetByKey")
	return u.storageUsers.GetCachedAssetByKey(key)
}

func (u *UsecaseUsers) SaveCachedAsset(key, sourceURL string, channelChatID int64, channelMessageID int, assetType string) (err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "SaveCachedAsset")
	return u.storageUsers.SaveCachedAsset(key, sourceURL, channelChatID, channelMessageID, assetType)
}

func (u *UsecaseUsers) DeleteCachedAssetByKey(key string) (err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "DeleteCachedAssetByKey")
	return u.storageUsers.DeleteCachedAssetByKey(key)
}

func (u *UsecaseUsers) IsAdmin(tgID int64) bool {
	_, ok := u.adminIDs[tgID]
	return ok
}

func (u *UsecaseUsers) ExtractFreepikURL(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	matches := freepikURLRegex.FindAllString(text, -1)
	for _, match := range matches {
		candidate := cleanupURL(match)
		if strings.Contains(strings.ToLower(candidate), "freepik.com") {
			return candidate
		}
	}

	candidate := cleanupURL(text)
	if strings.Contains(strings.ToLower(candidate), "freepik.com") {
		return candidate
	}
	return ""
}

func (u *UsecaseUsers) DetectAssetType(url string) string {
	value := strings.ToLower(url)
	switch {
	case strings.Contains(value, "/icon/") || strings.Contains(value, "/stock-icon/"):
		return "icon"
	case strings.Contains(value, "/stock/") && strings.Contains(value, "/icon"):
		return "icon"
	case strings.Contains(value, "/video/") || strings.Contains(value, "premium-video") || strings.Contains(value, "free-video"):
		return "video"
	case strings.Contains(value, "/motion-graphics/") || strings.Contains(value, "premium-motion-graphics") || strings.Contains(value, "free-motion-graphics"):
		return "video"
	case strings.Contains(value, "/stock-video/") || strings.Contains(value, "/footage/"):
		return "video"
	case strings.Contains(value, "/3d-model/") || strings.Contains(value, "3d-models"):
		return "3d"
	case strings.Contains(value, "/audio/") || strings.Contains(value, "premium-audio") || strings.Contains(value, "free-audio") || strings.Contains(value, "/tune/"):
		return "audio"
	case strings.Contains(value, "/font/"):
		return "font"
	case strings.Contains(value, "/psd/") || strings.Contains(value, "premium-psd") || strings.Contains(value, "free-psd"):
		return "psd"
	case strings.Contains(value, "/vector/") || strings.Contains(value, "premium-vector") || strings.Contains(value, "free-vector"):
		return "vector"
	case strings.Contains(value, "/photo/") || strings.Contains(value, "premium-photo") || strings.Contains(value, "free-photo"):
		return "photo"
	case strings.Contains(value, "/template/") || strings.Contains(value, "premium-template") || strings.Contains(value, "free-template"):
		return "template"
	case strings.Contains(value, "/mockup/") || strings.Contains(value, "premium-mockup") || strings.Contains(value, "free-mockup"):
		return "mockup"
	case strings.Contains(value, "/stock/"):
		return "stock"
	default:
		return "unknown"
	}
}

func (u *UsecaseUsers) IsSupportedAssetType(assetType string) (bool, string) {
	switch assetType {
	case "audio":
		return false, "Music and Sound Effects"
	case "font":
		return false, "Fonts"
	default:
		return true, assetType
	}
}

func (u *UsecaseUsers) GetDownloadLink(url string) (downloadLink string, err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "GetDownloadLink")
	return helper.GetDownloadLinkFreepik(url)
}

func (u *UsecaseUsers) Get3DFormatOptions(url string) (options []models.ThreeDFormatOption, err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "Get3DFormatOptions")
	return helper.Get3DFormatOptionsFreepik(url)
}

func (u *UsecaseUsers) Get3DDownloadLink(url, fileType string) (downloadLink string, err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "Get3DDownloadLink")
	return helper.GetDownloadLinkFreepik3D(url, fileType)
}

func (u *UsecaseUsers) TryIncrementDownload(tgID int64) (resp *models.DownloadAttempt, err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "TryIncrementDownload")
	return u.storageUsers.TryIncrementDownload(tgID)
}

func (u *UsecaseUsers) DecrementDownload(tgID int64) (err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "DecrementDownload")
	return u.storageUsers.DecrementDownload(tgID)
}

func (u *UsecaseUsers) GetUserStats(tgID int64) (stats *models.UserStats, err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "GetUserStats")
	return u.storageUsers.GetUserStats(tgID)
}

func (u *UsecaseUsers) UpdateUserLimit(tgID int64, dailyLimit int, limitDate *time.Time) (ok bool, err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "UpdateUserLimit")
	return u.storageUsers.UpdateUserLimit(tgID, dailyLimit, limitDate)
}

func (u *UsecaseUsers) ResetDailyLimits() (count int64, err error) {
	defer errs.WrapLog(&err, "UsecaseUsers", "ResetDailyLimits")
	return u.storageUsers.ResetDailyLimits()
}

func cleanupURL(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "<>[](){}\"'`.,")
	if strings.HasPrefix(strings.ToLower(value), "www.") {
		value = "https://" + value
	}
	return value
}
