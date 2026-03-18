package telegram

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"get-link-tg-bot/config"
	"get-link-tg-bot/messages"
	"get-link-tg-bot/models"
	"get-link-tg-bot/pkg/helper"
	"get-link-tg-bot/usecase"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	filepathpkg "path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type setLimitStage int

const (
	setLimitNone setLimitStage = iota
	setLimitDate
	setLimitDaily
	setLimitUser
)

const defaultSupportURL = "https://t.me/ProlinkAdmin"

var resourceIDRegex = regexp.MustCompile(`_(\d+)(?:\.htm|$)`)

type setLimitState struct {
	Stage      setLimitStage
	LimitDate  *time.Time
	DailyLimit int
}

type rawInlineKeyboardMarkup struct {
	InlineKeyboard [][]rawInlineKeyboardButton `json:"inline_keyboard"`
}

type rawInlineKeyboardButton struct {
	Text         string             `json:"text"`
	URL          string             `json:"url,omitempty"`
	CallbackData string             `json:"callback_data,omitempty"`
	CopyText     *rawCopyTextButton `json:"copy_text,omitempty"`
}

type rawCopyTextButton struct {
	Text string `json:"text"`
}

type pendingCacheItem struct {
	URL             string
	CacheKey        string
	AssetType       string
	Duplicate       bool
	DuplicateReason string
}

type autoCacheJob struct {
	CacheKey     string
	SourceURL    string
	AssetType    string
	DownloadLink string
}

type TgBot struct {
	cfg                *config.Config
	bot                *tgbotapi.BotAPI
	cacheBot           *tgbotapi.BotAPI
	usecase            usecase.UsecaseUsersI
	location           *time.Location
	processingMu       sync.Mutex
	processingMessages map[string]struct{}
	setLimitMu         sync.Mutex
	setLimitStates     map[int64]*setLimitState
	cacheModeMu        sync.Mutex
	cacheModeUsers     map[int64]struct{}
	pendingCacheLinks  map[int64][]pendingCacheItem
	copyLinkMu         sync.Mutex
	copyLinks          map[string]string
	updateQueues       []chan tgbotapi.Update
	cacheJobs          chan autoCacheJob
	cacheJobMu         sync.Mutex
	cacheJobKeys       map[string]struct{}
}

func NewTgBot(cfg *config.Config, bot, cacheBot *tgbotapi.BotAPI, usecase usecase.UsecaseUsersI) *TgBot {
	location, err := time.LoadLocation(cfg.Limits.Timezone)
	if err != nil {
		location = time.FixedZone("Asia/Tashkent", 5*60*60)
	}
	if cacheBot == nil {
		cacheBot = bot
	}
	tg := &TgBot{
		cfg:                cfg,
		bot:                bot,
		cacheBot:           cacheBot,
		usecase:            usecase,
		location:           location,
		processingMessages: make(map[string]struct{}),
		setLimitStates:     make(map[int64]*setLimitState),
		cacheModeUsers:     make(map[int64]struct{}),
		pendingCacheLinks:  make(map[int64][]pendingCacheItem),
		copyLinks:          make(map[string]string),
		cacheJobKeys:       make(map[string]struct{}),
	}
	tg.startUpdateWorkers()
	tg.startCacheWorkers()
	return tg
}

func (tg *TgBot) cacheAPI() *tgbotapi.BotAPI {
	if tg.cacheBot != nil {
		return tg.cacheBot
	}
	return tg.bot
}

func (tg *TgBot) Read() {
	tgCfg := tgbotapi.NewUpdate(tg.cfg.Telegram.Offset)
	tgCfg.Timeout = tg.cfg.Telegram.Timeout

	updates := tg.bot.GetUpdatesChan(tgCfg)
	log.Printf("Authorized on account %s", tg.bot.Self.UserName)

	for update := range updates {
		if len(tg.updateQueues) == 0 {
			tg.dispatchUpdate(update)
			continue
		}
		queue := tg.updateQueues[tg.updateQueueIndex(update)]
		queue <- update
	}
}

func (tg *TgBot) startUpdateWorkers() {
	workers := tg.cfg.App.UpdateWorkers
	if workers <= 1 {
		return
	}

	queueSize := tg.cfg.App.UpdateQueueSize
	if queueSize <= 0 {
		queueSize = 128
	}

	tg.updateQueues = make([]chan tgbotapi.Update, workers)
	for i := 0; i < workers; i++ {
		ch := make(chan tgbotapi.Update, queueSize)
		tg.updateQueues[i] = ch
		go func(queue <-chan tgbotapi.Update) {
			for update := range queue {
				tg.dispatchUpdate(update)
			}
		}(ch)
	}
}

func (tg *TgBot) dispatchUpdate(update tgbotapi.Update) {
	switch {
	case update.CallbackQuery != nil:
		tg.handleCallback(update)
	case update.Message != nil:
		tg.handleMessage(update)
	}
}

func (tg *TgBot) updateQueueIndex(update tgbotapi.Update) int {
	if len(tg.updateQueues) == 0 {
		return 0
	}

	var key int64
	switch {
	case update.Message != nil && update.Message.Chat != nil:
		key = update.Message.Chat.ID
	case update.CallbackQuery != nil && update.CallbackQuery.Message != nil:
		key = update.CallbackQuery.Message.Chat.ID
	case update.CallbackQuery != nil && update.CallbackQuery.From != nil:
		key = update.CallbackQuery.From.ID
	default:
		key = 0
	}

	if key < 0 {
		key = -key
	}
	return int(key % int64(len(tg.updateQueues)))
}

func (tg *TgBot) startCacheWorkers() {
	workers := tg.cfg.App.CacheWorkers
	if workers <= 0 {
		workers = 1
	}
	queueSize := tg.cfg.App.CacheQueueSize
	if queueSize <= 0 {
		queueSize = 128
	}

	tg.cacheJobs = make(chan autoCacheJob, queueSize)
	for i := 0; i < workers; i++ {
		go func(queue <-chan autoCacheJob) {
			for job := range queue {
				tg.processAutoCacheDownloadedAsset(job)
				tg.finishAutoCacheJob(job.CacheKey)
			}
		}(tg.cacheJobs)
	}
}

func (tg *TgBot) queueAutoCacheDownloadedAsset(cacheKey, sourceURL, assetType, downloadLink string) {
	if tg.cfg.Cache.ChannelID == 0 || strings.TrimSpace(downloadLink) == "" || tg.cacheJobs == nil {
		return
	}

	if !tg.markAutoCacheJob(cacheKey) {
		return
	}

	job := autoCacheJob{
		CacheKey:     cacheKey,
		SourceURL:    sourceURL,
		AssetType:    assetType,
		DownloadLink: downloadLink,
	}

	select {
	case tg.cacheJobs <- job:
	default:
		tg.finishAutoCacheJob(cacheKey)
		log.Printf("auto cache queue full for key=%s", cacheKey)
	}
}

func (tg *TgBot) markAutoCacheJob(cacheKey string) bool {
	tg.cacheJobMu.Lock()
	defer tg.cacheJobMu.Unlock()
	if _, ok := tg.cacheJobKeys[cacheKey]; ok {
		return false
	}
	tg.cacheJobKeys[cacheKey] = struct{}{}
	return true
}

func (tg *TgBot) finishAutoCacheJob(cacheKey string) {
	tg.cacheJobMu.Lock()
	defer tg.cacheJobMu.Unlock()
	delete(tg.cacheJobKeys, cacheKey)
}

func (tg *TgBot) processAutoCacheDownloadedAsset(job autoCacheJob) {
	if tg.cfg.Cache.ChannelID == 0 || strings.TrimSpace(job.DownloadLink) == "" {
		return
	}

	existing, err := tg.usecase.GetCachedAssetByKey(job.CacheKey)
	if err != nil {
		log.Printf("auto cache lookup failed for key=%s: %v", job.CacheKey, err)
		return
	}
	if existing != nil {
		return
	}

	sent, err := tg.sendCacheDocument(job.SourceURL, job.DownloadLink, job.AssetType)
	if err != nil {
		log.Printf("auto cache send failed for key=%s type=%s link=%s: %v", job.CacheKey, job.AssetType, summarizeURLHost(job.DownloadLink), err)
		return
	}

	if err := tg.usecase.SaveCachedAsset(job.CacheKey, normalizeURLForCache(job.SourceURL), tg.cfg.Cache.ChannelID, sent.MessageID, job.AssetType); err != nil {
		log.Printf("auto cache save failed for key=%s: %v", job.CacheKey, err)
		if _, deleteErr := tg.cacheAPI().Request(tgbotapi.NewDeleteMessage(tg.cfg.Cache.ChannelID, sent.MessageID)); deleteErr != nil {
			log.Printf("auto cache cleanup failed for key=%s message_id=%d: %v", job.CacheKey, sent.MessageID, deleteErr)
		}
	}
}

func (tg *TgBot) handleMessage(update tgbotapi.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	if update.Message.IsCommand() {
		tg.handleCommand(update)
		return
	}

	if tg.hasSetLimitState(update.Message.From.ID) {
		tg.handleSetLimitFlow(update)
		return
	}

	tg.handleRegularMessage(update)
}

func (tg *TgBot) handleCommand(update tgbotapi.Update) {
	message := update.Message
	command := strings.ToLower(message.Command())

	switch command {
	case "start":
		tg.startCommand(update)
	case "language":
		tg.languageCommand(update)
	case "help":
		tg.helpCommand(update)
	case "check_limit":
		tg.checkLimitCommand(update)
	case "set_limit":
		tg.setLimitCommand(update)
	case "cache_mode_on":
		tg.cacheModeOnCommand(update)
	case "cache_mode_off":
		tg.cacheModeOffCommand(update)
	case "cache_mode_status":
		tg.cacheModeStatusCommand(update)
	case "cache_mode_clear":
		tg.cacheModeClearCommand(update)
	case "cancel":
		tg.cancelSetLimit(update)
	default:
		tg.sendText(message.Chat.ID, tg.currentLanguage(message.From.ID, message.From.LanguageCode), messages.GetText(tg.currentLanguage(message.From.ID, message.From.LanguageCode), "help"), 0)
	}
}

func (tg *TgBot) handleCallback(update tgbotapi.Update) {
	query := update.CallbackQuery
	if query == nil || query.From == nil {
		return
	}

	if strings.HasPrefix(query.Data, "copy_") {
		tg.handleCopyCallback(query)
		return
	}

	_, _ = tg.bot.Request(tgbotapi.NewCallback(query.ID, ""))
	if !strings.HasPrefix(query.Data, "lang_") {
		return
	}

	lang := messages.NormalizeLang(strings.TrimPrefix(query.Data, "lang_"))
	user := query.From
	userID := user.ID
	exists, err := tg.usecase.UserExists(userID)
	if err != nil {
		log.Printf("callback user exists failed: %v", err)
		return
	}

	if exists {
		if err := tg.usecase.UpdateUserLanguage(userID, lang); err != nil {
			log.Printf("update language failed: %v", err)
			return
		}
	} else {
		if err := tg.usecase.SaveTelegramUser(&models.SaveTelegramUserRequest{
			TelegramID: userID,
			Name:       fullName(user.FirstName, user.LastName),
			Username:   user.UserName,
			Language:   lang,
			DailyLimit: tg.cfg.Limits.DefaultDailyLimit,
		}); err != nil {
			log.Printf("save user failed: %v", err)
			return
		}
	}

	chatID := userID
	if query.Message != nil {
		chatID = query.Message.Chat.ID
		edit := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, messages.GetText(lang, "language_selected"))
		if _, err := tg.bot.Send(edit); err != nil {
			log.Printf("edit language message failed: %v", err)
		}
	}

	welcome := messages.ReplacePlaceholders(messages.GetText(lang, "welcome"), strconv.FormatInt(userID, 10))
	if _, err := tg.bot.Send(tgbotapi.NewMessage(chatID, welcome)); err != nil {
		log.Printf("send welcome after language failed: %v", err)
	}
}

func (tg *TgBot) startCommand(update tgbotapi.Update) {
	message := update.Message
	userID := message.From.ID
	exists, err := tg.usecase.UserExists(userID)
	if err != nil {
		log.Printf("start user exists failed: %v", err)
		return
	}

	if exists {
		user, err := tg.usecase.GetUserByTelegramID(userID)
		if err != nil {
			log.Printf("get user failed: %v", err)
			return
		}
		lang := tg.languageForUser(user, message.From.LanguageCode)
		welcome := messages.ReplacePlaceholders(messages.GetText(lang, "welcome"), strconv.FormatInt(userID, 10))
		tg.sendText(message.Chat.ID, lang, welcome, 0)
		return
	}

	tg.sendLanguageSelector(message.Chat.ID, messages.GetText(messages.NormalizeLang(message.From.LanguageCode), "choose_language"))
}

func (tg *TgBot) languageCommand(update tgbotapi.Update) {
	message := update.Message
	lang := tg.currentLanguage(message.From.ID, message.From.LanguageCode)
	tg.sendLanguageSelector(message.Chat.ID, messages.GetText(lang, "choose_language"))
}

func (tg *TgBot) helpCommand(update tgbotapi.Update) {
	message := update.Message
	lang := tg.currentLanguage(message.From.ID, message.From.LanguageCode)
	key := "help"
	if tg.usecase.IsAdmin(message.From.ID) {
		key = "help_admin"
	}
	tg.sendText(message.Chat.ID, lang, messages.GetText(lang, key), 0)
}

func (tg *TgBot) checkLimitCommand(update tgbotapi.Update) {
	message := update.Message
	lang := tg.currentLanguage(message.From.ID, message.From.LanguageCode)
	stats, err := tg.usecase.GetUserStats(message.From.ID)
	if err != nil {
		log.Printf("check limit stats failed: %v", err)
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "check_limit_error"), 0)
		return
	}
	if stats == nil {
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "check_limit_not_registered"), 0)
		return
	}

	now := time.Now().In(tg.location)
	var builder strings.Builder
	builder.WriteString(messages.GetText(lang, "check_limit_title"))
	builder.WriteString("\n\n")
	builder.WriteString(messages.GetText(lang, "check_limit_name"))
	builder.WriteString(": ")
	builder.WriteString(nonEmpty(stats.Name, "N/A"))
	builder.WriteString("\n")
	if stats.Username != "" {
		builder.WriteString(messages.GetText(lang, "check_limit_username"))
		builder.WriteString(": @")
		builder.WriteString(stats.Username)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	builder.WriteString(messages.GetText(lang, "check_limit_stats"))
	builder.WriteString(":\n")
	builder.WriteString(messages.GetText(lang, "check_limit_today"))
	builder.WriteString(": ")
	builder.WriteString(strconv.Itoa(stats.DownloadsToday))
	builder.WriteString("/")
	builder.WriteString(strconv.Itoa(stats.DailyLimit))
	builder.WriteString("\n")
	builder.WriteString(messages.GetText(lang, "check_limit_total"))
	builder.WriteString(": ")
	builder.WriteString(strconv.Itoa(stats.TotalDownloads))
	builder.WriteString("\n")

	switch {
	case stats.LimitDate != nil && now.Before(stats.LimitDate.In(tg.location)):
		builder.WriteString("\n")
		builder.WriteString(messages.GetText(lang, "check_limit_status"))
		builder.WriteString(": ")
		builder.WriteString(messages.GetText(lang, "check_limit_premium"))
		builder.WriteString("\n")
		builder.WriteString(messages.GetText(lang, "check_limit_premium_until"))
		builder.WriteString(": ")
		builder.WriteString(stats.LimitDate.In(tg.location).Format("2006-01-02"))
		builder.WriteString("\n")
		builder.WriteString(messages.GetText(lang, "check_limit_daily_limit"))
		builder.WriteString(": ")
		builder.WriteString(strconv.Itoa(stats.DailyLimit))
	case stats.TrialEnd != nil && now.Before(stats.TrialEnd.In(tg.location)):
		daysLeft := int(stats.TrialEnd.In(tg.location).Sub(now).Hours() / 24)
		if daysLeft < 0 {
			daysLeft = 0
		}
		builder.WriteString("\n")
		builder.WriteString(messages.GetText(lang, "check_limit_status"))
		builder.WriteString(": ")
		builder.WriteString(messages.GetText(lang, "check_limit_trial"))
		builder.WriteString("\n")
		builder.WriteString(messages.GetText(lang, "check_limit_trial_ends"))
		builder.WriteString(": ")
		builder.WriteString(stats.TrialEnd.In(tg.location).Format("2006-01-02"))
		builder.WriteString(" (")
		builder.WriteString(strconv.Itoa(daysLeft))
		builder.WriteString(" ")
		builder.WriteString(messages.GetText(lang, "check_limit_days_left"))
		builder.WriteString(")\n")
		builder.WriteString(messages.GetText(lang, "check_limit_daily_limit"))
		builder.WriteString(": ")
		builder.WriteString(strconv.Itoa(stats.DailyLimit))
	default:
		builder.WriteString("\n")
		builder.WriteString(messages.GetText(lang, "check_limit_status"))
		builder.WriteString(": ")
		builder.WriteString(messages.GetText(lang, "check_limit_expired"))
		builder.WriteString("\n")
		builder.WriteString(messages.GetText(lang, "check_limit_contact_admin"))
	}

	if stats.CreateDate != nil {
		builder.WriteString("\n\n")
		builder.WriteString(messages.GetText(lang, "check_limit_registered"))
		builder.WriteString(": ")
		builder.WriteString(stats.CreateDate.In(tg.location).Format("2006-01-02"))
	}
	builder.WriteString("\n\n")
	builder.WriteString(messages.GetText(lang, "check_limit_reset_info"))

	tg.sendText(message.Chat.ID, lang, builder.String(), 0)
}

func (tg *TgBot) setLimitCommand(update tgbotapi.Update) {
	message := update.Message
	lang := tg.currentLanguage(message.From.ID, message.From.LanguageCode)
	if !tg.usecase.IsAdmin(message.From.ID) {
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "unauthorized"), 0)
		return
	}
	state := &setLimitState{Stage: setLimitDate}
	tg.setSetLimitState(message.From.ID, state)
	tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "set_limit_start"), 0)
}

func (tg *TgBot) cancelSetLimit(update tgbotapi.Update) {
	message := update.Message
	lang := tg.currentLanguage(message.From.ID, message.From.LanguageCode)
	tg.clearSetLimitState(message.From.ID)
	tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "cancelled"), 0)
}

func (tg *TgBot) cacheModeOnCommand(update tgbotapi.Update) {
	message := update.Message
	lang := tg.currentLanguage(message.From.ID, message.From.LanguageCode)
	if !tg.usecase.IsAdmin(message.From.ID) {
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "unauthorized"), 0)
		return
	}
	if tg.cfg.Cache.ChannelID == 0 {
		tg.sendText(message.Chat.ID, lang, "❌ CACHE_CHANNEL_ID is not configured.", 0)
		return
	}

	tg.setAdminCacheMode(message.From.ID, true)
	pending := tg.pendingCacheCount(message.From.ID)
	tg.sendText(
		message.Chat.ID,
		lang,
		fmt.Sprintf("✅ Cache import mode enabled.\n\nSend messages in this order:\n1) Freepik link\n2) File (forward/upload)\n\nQueue uses FIFO pairing.\nCurrent pending links: %d", pending),
		0,
	)
}

func (tg *TgBot) cacheModeOffCommand(update tgbotapi.Update) {
	message := update.Message
	lang := tg.currentLanguage(message.From.ID, message.From.LanguageCode)
	if !tg.usecase.IsAdmin(message.From.ID) {
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "unauthorized"), 0)
		return
	}

	pending := tg.pendingCacheCount(message.From.ID)
	tg.setAdminCacheMode(message.From.ID, false)
	tg.sendText(
		message.Chat.ID,
		lang,
		fmt.Sprintf("🛑 Cache import mode disabled.\nPending links kept in queue: %d\nUse /cache_mode_on to continue later or /cache_mode_clear to remove pending links.", pending),
		0,
	)
}

func (tg *TgBot) cacheModeStatusCommand(update tgbotapi.Update) {
	message := update.Message
	lang := tg.currentLanguage(message.From.ID, message.From.LanguageCode)
	if !tg.usecase.IsAdmin(message.From.ID) {
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "unauthorized"), 0)
		return
	}

	mode := "OFF"
	if tg.isAdminCacheMode(message.From.ID) {
		mode = "ON"
	}
	tg.sendText(
		message.Chat.ID,
		lang,
		fmt.Sprintf("📦 Cache import mode: %s\n⏳ Pending links: %d", mode, tg.pendingCacheCount(message.From.ID)),
		0,
	)
}

func (tg *TgBot) cacheModeClearCommand(update tgbotapi.Update) {
	message := update.Message
	lang := tg.currentLanguage(message.From.ID, message.From.LanguageCode)
	if !tg.usecase.IsAdmin(message.From.ID) {
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "unauthorized"), 0)
		return
	}

	cleared := tg.clearPendingCacheLinks(message.From.ID)
	tg.sendText(message.Chat.ID, lang, fmt.Sprintf("🧹 Cleared pending link queue: %d", cleared), 0)
}

func (tg *TgBot) handleSetLimitFlow(update tgbotapi.Update) {
	message := update.Message
	userID := message.From.ID
	lang := tg.currentLanguage(userID, message.From.LanguageCode)
	state := tg.getSetLimitState(userID)
	if state == nil {
		return
	}

	text := strings.TrimSpace(message.Text)
	switch state.Stage {
	case setLimitDate:
		if text == "0" {
			state.LimitDate = nil
		} else {
			limitDate, err := time.ParseInLocation("2006.01.02", text, tg.location)
			if err != nil {
				tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "set_limit_bad_date"), 0)
				return
			}
			if !limitDate.After(time.Now().In(tg.location)) {
				tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "set_limit_past_date"), 0)
				return
			}
			state.LimitDate = &limitDate
		}
		state.Stage = setLimitDaily
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "set_limit_ask_daily"), 0)
	case setLimitDaily:
		dailyLimit, err := strconv.Atoi(text)
		if err != nil || dailyLimit < 0 {
			tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "set_limit_bad_daily"), 0)
			return
		}
		state.DailyLimit = dailyLimit
		state.Stage = setLimitUser
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "set_limit_ask_user"), 0)
	case setLimitUser:
		if !isDigits(text) {
			tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "set_limit_bad_user"), 0)
			return
		}
		targetID, _ := strconv.ParseInt(text, 10, 64)
		targetUser, err := tg.usecase.GetUserByTelegramID(targetID)
		if err != nil {
			log.Printf("set limit get user failed: %v", err)
			tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "set_limit_failed"), 0)
			tg.clearSetLimitState(userID)
			return
		}
		if targetUser == nil {
			tg.sendText(message.Chat.ID, lang, messages.ReplacePlaceholders(messages.GetText(lang, "set_limit_user_missing"), text), 0)
			tg.clearSetLimitState(userID)
			return
		}

		ok, err := tg.usecase.UpdateUserLimit(targetID, state.DailyLimit, state.LimitDate)
		if err != nil {
			log.Printf("set limit update failed: %v", err)
			tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "set_limit_failed"), 0)
			tg.clearSetLimitState(userID)
			return
		}
		if !ok {
			tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "set_limit_failed"), 0)
			tg.clearSetLimitState(userID)
			return
		}

		validUntil := "No expiration"
		if lang == "uz" {
			validUntil = "Muddatsiz"
		} else if lang == "ru" {
			validUntil = "Без срока"
		}
		if state.LimitDate != nil {
			validUntil = state.LimitDate.In(tg.location).Format("2006-01-02")
		}

		username := targetUser.Username
		if username == "" {
			username = "N/A"
		} else {
			username = "@" + username
		}
		text := messages.ReplacePlaceholders(
			messages.GetText(lang, "set_limit_success"),
			nonEmpty(targetUser.Name, "N/A"),
			username,
			strconv.FormatInt(targetID, 10),
			strconv.Itoa(state.DailyLimit),
			validUntil,
		)
		tg.sendText(message.Chat.ID, lang, text, 0)
		tg.clearSetLimitState(userID)
	}
}

func (tg *TgBot) handleRegularMessage(update tgbotapi.Update) {
	message := update.Message
	userID := message.From.ID
	lang := tg.currentLanguage(userID, message.From.LanguageCode)

	exists, err := tg.usecase.UserExists(userID)
	if err != nil {
		log.Printf("regular user exists failed: %v", err)
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "error"), 0)
		return
	}
	if !exists {
		tg.sendLanguageSelector(message.Chat.ID, messages.GetText(lang, "choose_language_first"))
		return
	}

	if tg.usecase.IsAdmin(userID) && tg.isAdminCacheMode(userID) {
		if tg.handleAdminCacheModeMessage(update, lang) {
			return
		}
	}

	messageKey := fmt.Sprintf("%d-%d", message.Chat.ID, message.MessageID)
	if !tg.acquireProcessing(messageKey) {
		return
	}
	defer tg.releaseProcessing(messageKey)

	freepikURL := tg.usecase.ExtractFreepikURL(message.Text)
	if freepikURL == "" {
		tg.sendText(message.Chat.ID, lang, messages.GetText(lang, "no_url"), 0)
		return
	}

	processingMsg, err := tg.sendMarkdown(message.Chat.ID, messages.GetText(lang, "processing"), 0)
	if err != nil {
		log.Printf("send processing failed: %v", err)
		return
	}

	assetType := tg.usecase.DetectAssetType(freepikURL)
	if supported, typeName := tg.usecase.IsSupportedAssetType(assetType); !supported {
		text := messages.ReplacePlaceholders(messages.GetText(lang, "unsupported_type"), typeName)
		if err := tg.editMessage(message.Chat.ID, processingMsg.MessageID, text, true, nil); err != nil {
			log.Printf("edit unsupported message failed: %v", err)
		}
		return
	}

	cacheKey := buildAssetCacheKey(freepikURL, assetType)
	cached, err := tg.usecase.GetCachedAssetByKey(cacheKey)
	if err != nil {
		log.Printf("get cached asset failed: %v", err)
	} else if cached != nil {
		attempt, incrementErr := tg.usecase.TryIncrementDownload(userID)
		if incrementErr != nil {
			log.Printf("increment cached download failed: %v", incrementErr)
			if editErr := tg.editMessage(message.Chat.ID, processingMsg.MessageID, messages.GetText(lang, "error"), true, nil); editErr != nil {
				log.Printf("edit cached increment error failed: %v", editErr)
			}
			return
		}
		if !attempt.Allowed {
			text := messages.BuildLimitMessage(lang, attempt.ErrorMessage, attempt.DownloadsToday, attempt.DailyLimit)
			if err := tg.editMessage(message.Chat.ID, processingMsg.MessageID, text, true, nil); err != nil {
				log.Printf("edit cached limit message failed: %v", err)
			}
			return
		}

		copyCfg := tgbotapi.NewCopyMessage(message.Chat.ID, cached.ChannelChatID, cached.ChannelMessageID)
		limitText := messages.BuildLimitOnlyText(lang, attempt.DownloadsToday, attempt.DailyLimit)
		copyCfg.Caption = messages.BuildCachedDeliveredMessageHTML(lang, limitText, defaultSupportURL, tg.botPublicURL())
		copyCfg.ParseMode = tgbotapi.ModeHTML
		if _, copyErr := tg.bot.CopyMessage(copyCfg); copyErr != nil {
			log.Printf("cached copy failed for key=%s: %v", cacheKey, copyErr)
			if rollbackErr := tg.usecase.DecrementDownload(userID); rollbackErr != nil {
				log.Printf("rollback cached copy failed: %v", rollbackErr)
			}
			if deleteErr := tg.usecase.DeleteCachedAssetByKey(cacheKey); deleteErr != nil {
				log.Printf("delete stale cache failed: %v", deleteErr)
			}
		} else {
			if _, deleteErr := tg.bot.Request(tgbotapi.NewDeleteMessage(message.Chat.ID, processingMsg.MessageID)); deleteErr != nil {
				log.Printf("delete cache served processing message failed: %v", deleteErr)
			}
			return
		}
	}

	downloadLink, err := tg.usecase.GetDownloadLink(freepikURL)
	if err != nil {
		log.Printf("download link extraction failed: %v", err)
		errorText := messages.GetText(lang, "error")
		if shouldHideInternalExtractionError(err) {
			errorText = messages.GetText(lang, "temporarily_unavailable")
		}
		if editErr := tg.editMessage(message.Chat.ID, processingMsg.MessageID, errorText, true, nil); editErr != nil {
			log.Printf("edit extraction error failed: %v", editErr)
		}
		return
	}

	attempt, err := tg.usecase.TryIncrementDownload(userID)
	if err != nil {
		log.Printf("increment download failed: %v", err)
		if editErr := tg.editMessage(message.Chat.ID, processingMsg.MessageID, messages.GetText(lang, "error"), true, nil); editErr != nil {
			log.Printf("edit increment error failed: %v", editErr)
		}
		return
	}
	if !attempt.Allowed {
		text := messages.BuildLimitMessage(lang, attempt.ErrorMessage, attempt.DownloadsToday, attempt.DailyLimit)
		if err := tg.editMessage(message.Chat.ID, processingMsg.MessageID, text, true, nil); err != nil {
			log.Printf("edit limit message failed: %v", err)
		}
		return
	}

	botURL := tg.botPublicURL()
	limitText := messages.BuildLimitOnlyText(lang, attempt.DownloadsToday, attempt.DailyLimit)

	shareText := messages.BuildSuccessMessage(
		lang,
		downloadLink,
		limitText,
		defaultSupportURL,
		botURL,
	)
	visibleText := messages.BuildSuccessMessageHTML(
		lang,
		downloadLink,
		limitText,
		defaultSupportURL,
		botURL,
	)
	if err := tg.editSuccessMessage(message.Chat.ID, processingMsg.MessageID, lang, visibleText, downloadLink, shareText); err != nil {
		log.Printf("edit success message failed, trying fallback without keyboard: %v", err)
		if fallbackErr := tg.editHTMLMessage(message.Chat.ID, processingMsg.MessageID, visibleText); fallbackErr != nil {
			log.Printf("edit fallback success message failed, rolling back: %v", fallbackErr)
			if rollbackErr := tg.usecase.DecrementDownload(userID); rollbackErr != nil {
				log.Printf("rollback download failed: %v", rollbackErr)
			}
			_, _ = tg.sendMarkdown(message.Chat.ID, messages.GetText(lang, "error"), 0)
			return
		}
	}

	tg.queueAutoCacheDownloadedAsset(cacheKey, freepikURL, assetType, downloadLink)

}

func (tg *TgBot) handleAdminCacheModeMessage(update tgbotapi.Update, lang string) bool {
	message := update.Message
	userID := message.From.ID

	if hasCopyableMedia(message) {
		tg.handleAdminCacheFileMessage(update, lang)
		return true
	}

	freepikURL := tg.usecase.ExtractFreepikURL(message.Text)
	if freepikURL == "" {
		tg.sendText(
			message.Chat.ID,
			lang,
			"⚠️ Cache import mode is ON.\nSend Freepik link text, then send/forward a file.",
			0,
		)
		return true
	}

	assetType := tg.usecase.DetectAssetType(freepikURL)
	if supported, typeName := tg.usecase.IsSupportedAssetType(assetType); !supported {
		tg.sendText(message.Chat.ID, lang, fmt.Sprintf("⚠️ Unsupported type for cache import: %s", typeName), 0)
		return true
	}

	cacheKey := buildAssetCacheKey(freepikURL, assetType)
	existing, err := tg.usecase.GetCachedAssetByKey(cacheKey)
	if err != nil {
		log.Printf("cache mode existing lookup failed: %v", err)
		tg.sendText(message.Chat.ID, lang, "❌ Failed to check existing cache mapping.", 0)
		return true
	}

	duplicate := existing != nil || tg.isPendingCacheKey(userID, cacheKey)
	duplicateReason := ""
	if existing != nil {
		duplicateReason = "already cached in database"
	} else if tg.isPendingCacheKey(userID, cacheKey) {
		duplicateReason = "already pending in current import queue"
	}

	pending := tg.queuePendingCacheLink(userID, pendingCacheItem{
		URL:             freepikURL,
		CacheKey:        cacheKey,
		AssetType:       assetType,
		Duplicate:       duplicate,
		DuplicateReason: duplicateReason,
	})

	if duplicate {
		tg.sendText(
			message.Chat.ID,
			lang,
			fmt.Sprintf("⚠️ Duplicate link queued in skip mode.\nNext file will be consumed but NOT uploaded.\n🔑 Key: %s\n📝 Reason: %s\n⏳ Pending links: %d", cacheKey, duplicateReason, pending),
			0,
		)
		return true
	}

	tg.sendText(
		message.Chat.ID,
		lang,
		fmt.Sprintf("✅ Link queued for next file.\n🔑 Key: %s\n⏳ Pending links: %d", cacheKey, pending),
		0,
	)
	return true
}

func (tg *TgBot) handleAdminCacheFileMessage(update tgbotapi.Update, lang string) {
	message := update.Message
	userID := message.From.ID

	if tg.cfg.Cache.ChannelID == 0 {
		tg.sendText(message.Chat.ID, lang, "❌ CACHE_CHANNEL_ID is not configured.", 0)
		return
	}

	item, ok := tg.popPendingCacheLink(userID)
	if !ok {
		tg.sendText(message.Chat.ID, lang, "⚠️ No pending link for this file.\nPlease send a Freepik link first.", 0)
		return
	}

	if item.Duplicate {
		tg.sendText(
			message.Chat.ID,
			lang,
			fmt.Sprintf("♻️ Duplicate link detected. File skipped (not uploaded to cache channel).\n📝 Reason: %s\n⏳ Pending links left: %d", item.DuplicateReason, tg.pendingCacheCount(userID)),
			0,
		)
		return
	}

	copyCfg := tgbotapi.NewCopyMessage(tg.cfg.Cache.ChannelID, message.Chat.ID, message.MessageID)
	copyCfg.DisableNotification = true
	copied, err := tg.cacheAPI().CopyMessage(copyCfg)
	if err != nil {
		tg.prependPendingCacheLink(userID, item)
		tg.sendText(message.Chat.ID, lang, "❌ Failed to copy file to cache channel: "+err.Error(), 0)
		return
	}

	if err := tg.usecase.SaveCachedAsset(item.CacheKey, normalizeURLForCache(item.URL), tg.cfg.Cache.ChannelID, copied.MessageID, item.AssetType); err != nil {
		tg.prependPendingCacheLink(userID, item)
		tg.sendText(message.Chat.ID, lang, "❌ Failed to save cache mapping to database.", 0)
		return
	}

	tg.sendText(
		message.Chat.ID,
		lang,
		fmt.Sprintf("✅ Cached successfully.\n🔑 Key: %s\n🧾 Channel message_id: %d\n⏳ Pending links left: %d", item.CacheKey, copied.MessageID, tg.pendingCacheCount(userID)),
		0,
	)
}

func (tg *TgBot) sendText(chatID int64, lang, text string, replyTo int) {
	msg := tgbotapi.NewMessage(chatID, text)
	if replyTo > 0 {
		msg.ReplyToMessageID = replyTo
	}
	if _, err := tg.bot.Send(msg); err != nil {
		log.Printf("send text failed: %v", err)
	}
}

func (tg *TgBot) NotifyAdmins(text string) {
	for _, adminID := range tg.cfg.Admin.TelegramIDs {
		tg.sendText(adminID, "en", text, 0)
	}
}

func (tg *TgBot) sendMarkdown(chatID int64, text string, replyTo int) (tgbotapi.Message, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	if replyTo > 0 {
		msg.ReplyToMessageID = replyTo
	}
	return tg.bot.Send(msg)
}

func (tg *TgBot) sendLanguageSelector(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "lang_en"),
			tgbotapi.NewInlineKeyboardButtonData("🇺🇿 O'zbekcha", "lang_uz"),
			tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "lang_ru"),
		),
	)
	if _, err := tg.bot.Send(msg); err != nil {
		log.Printf("send language selector failed: %v", err)
	}
}

func (tg *TgBot) editMessage(chatID int64, messageID int, text string, markdown bool, keyboard *tgbotapi.InlineKeyboardMarkup) error {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	if markdown {
		edit.ParseMode = tgbotapi.ModeMarkdown
	}
	if keyboard != nil {
		edit.ReplyMarkup = keyboard
	}
	_, err := tg.bot.Send(edit)
	return err
}

func (tg *TgBot) editSuccessMessage(chatID int64, messageID int, lang, text, downloadLink, shareText string) error {
	replyMarkup, err := tg.successKeyboardJSON(lang, downloadLink, shareText)
	if err != nil {
		return err
	}

	_, err = tg.bot.MakeRequest("editMessageText", tgbotapi.Params{
		"chat_id":                  strconv.FormatInt(chatID, 10),
		"message_id":               strconv.Itoa(messageID),
		"text":                     text,
		"parse_mode":               "HTML",
		"reply_markup":             replyMarkup,
		"disable_web_page_preview": "true",
	})
	return err
}

func (tg *TgBot) editHTMLMessage(chatID int64, messageID int, text string) error {
	_, err := tg.bot.MakeRequest("editMessageText", tgbotapi.Params{
		"chat_id":                  strconv.FormatInt(chatID, 10),
		"message_id":               strconv.Itoa(messageID),
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": "true",
	})
	return err
}

func (tg *TgBot) successKeyboardJSON(lang, downloadLink, shareText string) (string, error) {
	copyToken := tg.storeCopyLink(downloadLink)
	markup := rawInlineKeyboardMarkup{
		InlineKeyboard: [][]rawInlineKeyboardButton{
			{
				{
					Text:         messages.GetText(lang, "copy_button"),
					CallbackData: "copy_" + copyToken,
				},
				{
					Text: messages.GetText(lang, "share_button"),
					URL:  tg.shareURL(shareText),
				},
			},
		},
	}

	data, err := json.Marshal(markup)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (tg *TgBot) handleCopyCallback(query *tgbotapi.CallbackQuery) {
	if query == nil || query.From == nil {
		return
	}

	token := strings.TrimPrefix(query.Data, "copy_")
	link, ok := tg.getCopyLink(token)
	if !ok || strings.TrimSpace(link) == "" {
		_, _ = tg.bot.Request(tgbotapi.NewCallback(query.ID, "Link eskirgan. Yangi link oling."))
		return
	}

	_, _ = tg.bot.Request(tgbotapi.NewCallback(query.ID, "Link yuborildi"))
	targetChatID := query.From.ID
	if query.Message != nil {
		targetChatID = query.Message.Chat.ID
	}

	msg := tgbotapi.NewMessage(targetChatID, link)
	msg.DisableWebPagePreview = true
	if _, err := tg.bot.Send(msg); err != nil {
		log.Printf("send copy link failed: %v", err)
	}
}

func (tg *TgBot) storeCopyLink(link string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", time.Now().UnixNano(), link)))
	token := fmt.Sprintf("%x", sum[:8])

	tg.copyLinkMu.Lock()
	defer tg.copyLinkMu.Unlock()
	tg.copyLinks[token] = link
	return token
}

func (tg *TgBot) getCopyLink(token string) (string, bool) {
	tg.copyLinkMu.Lock()
	defer tg.copyLinkMu.Unlock()
	link, ok := tg.copyLinks[token]
	return link, ok
}

func (tg *TgBot) sendVideoDocument(chatID int64, fileURL string) error {
	msg := newCacheDocumentConfig(chatID, tgbotapi.FileURL(fileURL), true)
	_, err := tg.bot.Send(msg)
	return err
}

func (tg *TgBot) sendCacheDocument(sourceURL, downloadLink, assetType string) (tgbotapi.Message, error) {
	resolvedLink, err := tg.resolveCacheDownloadLink(sourceURL, downloadLink, assetType)
	if err != nil {
		return tgbotapi.Message{}, err
	}

	if assetType == "icon" || assetType == "video" {
		return tg.sendCacheDocumentFromLocalFile(resolvedLink, assetType)
	}

	msg := newCacheDocumentConfig(tg.cfg.Cache.ChannelID, tgbotapi.FileURL(resolvedLink), shouldDisableContentTypeDetection(assetType))
	sent, err := tg.cacheAPI().Send(msg)
	if err == nil {
		return sent, nil
	}

	return tg.sendCacheDocumentFromLocalFile(resolvedLink, assetType)
}

func (tg *TgBot) resolveCacheDownloadLink(sourceURL, downloadLink, assetType string) (string, error) {
	if assetType == "video" && tg.cfg.Cache.MaxUploadBytes > 0 && strings.TrimSpace(sourceURL) != "" {
		videoLink, err := helper.GetCacheableVideoDownloadLinkFreepik(sourceURL, tg.cfg.Cache.MaxUploadBytes)
		if err == nil && strings.TrimSpace(videoLink) != "" {
			log.Printf("auto cache video variant selected for source=%s host=%s", summarizeURLHost(sourceURL), summarizeURLHost(videoLink))
			return videoLink, nil
		}
	}

	if err := tg.ensureCacheUploadSizeAllowed(downloadLink, assetType); err == nil {
		return downloadLink, nil
	} else if assetType != "video" {
		return "", err
	}

	if strings.TrimSpace(sourceURL) == "" {
		return "", fmt.Errorf("cache skipped: video exceeds upload limit and source url is missing")
	}

	videoLink, err := helper.GetCacheableVideoDownloadLinkFreepik(sourceURL, tg.cfg.Cache.MaxUploadBytes)
	if err != nil {
		return "", fmt.Errorf("cache skipped: no cacheable video variant found: %w", err)
	}
	if err := tg.ensureCacheUploadSizeAllowed(videoLink, assetType); err != nil {
		return "", err
	}
	log.Printf("auto cache video fallback selected for source=%s host=%s", summarizeURLHost(sourceURL), summarizeURLHost(videoLink))
	return videoLink, nil
}

func (tg *TgBot) sendCacheDocumentFromLocalFile(downloadLink, assetType string) (tgbotapi.Message, error) {
	tempPath, cleanup, downloadErr := downloadAssetToTempFile(downloadLink, assetType)
	if downloadErr != nil {
		return tgbotapi.Message{}, downloadErr
	}
	defer cleanup()

	fallback := newCacheDocumentConfig(tg.cfg.Cache.ChannelID, tgbotapi.FilePath(tempPath), shouldDisableContentTypeDetection(assetType))
	sent, fallbackErr := tg.cacheAPI().Send(fallback)
	if fallbackErr != nil {
		return tgbotapi.Message{}, fmt.Errorf("local upload failed: %w", fallbackErr)
	}
	return sent, nil
}

func newCacheDocumentConfig(chatID int64, file tgbotapi.RequestFileData, disableContentTypeDetection bool) tgbotapi.DocumentConfig {
	msg := tgbotapi.NewDocument(chatID, file)
	msg.DisableNotification = true
	msg.DisableContentTypeDetection = disableContentTypeDetection
	return msg
}

func shouldDisableContentTypeDetection(assetType string) bool {
	switch strings.ToLower(strings.TrimSpace(assetType)) {
	case "video", "icon":
		return true
	default:
		return false
	}
}

func (tg *TgBot) ensureCacheUploadSizeAllowed(downloadLink, assetType string) error {
	maxBytes := tg.cfg.Cache.MaxUploadBytes
	if maxBytes <= 0 {
		return nil
	}

	size, err := probeRemoteAssetSize(downloadLink, assetType)
	if err != nil {
		return nil
	}
	if size <= 0 {
		return nil
	}
	if isCacheUploadTooLarge(size, maxBytes) {
		return fmt.Errorf("cache skipped: remote file size %d exceeds CACHE_MAX_UPLOAD_BYTES=%d", size, maxBytes)
	}
	return nil
}

func isCacheUploadTooLarge(size, maxBytes int64) bool {
	return maxBytes > 0 && size > maxBytes
}

func downloadAssetToTempFile(downloadLink, assetType string) (string, func(), error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := fetchAssetForCache(client, downloadLink, assetType)
	if err != nil {
		return "", func() {}, err
	}
	defer resp.Body.Close()

	dir, err := os.MkdirTemp("", "prolinkrobot-cache-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	filename := cacheFilename(resp, downloadLink, assetType)
	targetPath := filepathpkg.Join(dir, filename)

	file, err := os.Create(targetPath)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		cleanup()
		return "", func() {}, err
	}

	return targetPath, cleanup, nil
}

func probeRemoteAssetSize(downloadLink, assetType string) (int64, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodHead, downloadLink, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0")
	req.Header.Set("Accept", assetDownloadAcceptHeader(assetType, downloadLink))

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("head status=%d", resp.StatusCode)
	}
	if resp.ContentLength > 0 {
		return resp.ContentLength, nil
	}
	if value := strings.TrimSpace(resp.Header.Get("Content-Length")); value != "" {
		n, convErr := strconv.ParseInt(value, 10, 64)
		if convErr == nil {
			return n, nil
		}
	}
	return 0, nil
}

func fetchAssetForCache(client *http.Client, downloadLink, assetType string) (*http.Response, error) {
	referers := []string{""}
	if assetType == "icon" || isIconDownloadURL(downloadLink) {
		referers = []string{
			"https://www.freepik.com/",
			"https://www.freepik.com/icon/",
			"https://www.flaticon.com/",
		}
	}

	var lastErr error
	for _, referer := range referers {
		resp, err := doAssetDownloadRequest(client, downloadLink, assetType, referer)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("download failed")
	}
	return nil, lastErr
}

func doAssetDownloadRequest(client *http.Client, downloadLink, assetType, referer string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, downloadLink, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept", assetDownloadAcceptHeader(assetType, downloadLink))
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		bodyPreview, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		resp.Body.Close()
		return nil, fmt.Errorf("download status=%d referer=%s body=%s", resp.StatusCode, referer, strings.TrimSpace(string(bodyPreview)))
	}
	return resp, nil
}

func cacheFilename(resp *http.Response, downloadLink, assetType string) string {
	if resp != nil {
		if header := strings.TrimSpace(resp.Header.Get("Content-Disposition")); header != "" {
			if _, params, err := mime.ParseMediaType(header); err == nil {
				if name := sanitizeFilename(params["filename"]); name != "" {
					return name
				}
				if name := sanitizeFilename(params["filename*"]); name != "" {
					return name
				}
			}
		}
	}

	if parsed, err := url.Parse(strings.TrimSpace(downloadLink)); err == nil {
		if name := sanitizeFilename(parsed.Query().Get("filename")); name != "" {
			return name
		}
		if name := sanitizeFilename(pathpkg.Base(parsed.Path)); name != "" && name != "." && name != "/" {
			return ensureAssetExtension(name, assetType)
		}
	}

	return ensureAssetExtension("asset", assetType)
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'")
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, ";", "_")
	return value
}

func assetDownloadAcceptHeader(assetType, downloadLink string) string {
	if assetType == "icon" || isIconDownloadURL(downloadLink) {
		return "image/svg+xml,image/*;q=0.9,*/*;q=0.8"
	}
	return "*/*"
}

func ensureAssetExtension(name, assetType string) string {
	ext := strings.ToLower(filepathpkg.Ext(name))
	if ext != "" {
		return name
	}

	switch assetType {
	case "icon":
		return name + ".svg"
	case "psd":
		return name + ".zip"
	default:
		return name + ".bin"
	}
}

func isIconDownloadURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return strings.Contains(host, "flaticon.com") || strings.Contains(host, "cdn-icons")
}

func isDirectVideoURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.Path)
	if strings.Contains(host, "videocdn.cdnpk.net") && !strings.Contains(path, "/downloads/") {
		return false
	}
	return strings.HasSuffix(path, ".mp4") || strings.HasSuffix(path, ".mov") || strings.HasSuffix(path, ".webm")
}

func shouldHideInternalExtractionError(err error) bool {
	if err == nil {
		return false
	}

	value := strings.ToLower(err.Error())
	internalMarkers := []string{
		"freepik auth",
		"session expired",
		"token expired",
		"ensurefreshcookiemap",
		"permission_denied",
		"access denied",
		"<!doctype html>",
		"securetoken",
		"caller",
	}
	for _, marker := range internalMarkers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func (tg *TgBot) shareURL(shareText string) string {
	lines := strings.Split(shareText, "\n")
	if len(lines) == 0 {
		return "https://t.me/share/url?text="
	}

	firstLine := strings.TrimSpace(lines[0])
	rest := ""
	if len(lines) > 1 {
		rest = strings.Join(lines[1:], "\n")
		rest = strings.TrimLeft(rest, "\n")
	}

	values := url.Values{}
	if firstLine != "" {
		values.Set("url", firstLine)
	}
	if rest != "" {
		values.Set("text", rest)
	}
	return "https://t.me/share/url?" + values.Encode()
}

func (tg *TgBot) botPublicURL() string {
	if strings.TrimSpace(tg.bot.Self.UserName) == "" {
		return ""
	}
	return "https://t.me/" + tg.bot.Self.UserName
}

func (tg *TgBot) currentLanguage(userID int64, fallback string) string {
	user, err := tg.usecase.GetUserByTelegramID(userID)
	if err != nil {
		log.Printf("currentLanguage lookup failed: %v", err)
		return messages.NormalizeLang(fallback)
	}
	return tg.languageForUser(user, fallback)
}

func (tg *TgBot) languageForUser(user *models.TelegramUser, fallback string) string {
	if user != nil && user.Language != "" {
		return messages.NormalizeLang(user.Language)
	}
	return messages.NormalizeLang(fallback)
}

func (tg *TgBot) acquireProcessing(key string) bool {
	tg.processingMu.Lock()
	defer tg.processingMu.Unlock()
	if _, ok := tg.processingMessages[key]; ok {
		return false
	}
	tg.processingMessages[key] = struct{}{}
	return true
}

func (tg *TgBot) releaseProcessing(key string) {
	tg.processingMu.Lock()
	defer tg.processingMu.Unlock()
	delete(tg.processingMessages, key)
}

func (tg *TgBot) hasSetLimitState(userID int64) bool {
	tg.setLimitMu.Lock()
	defer tg.setLimitMu.Unlock()
	_, ok := tg.setLimitStates[userID]
	return ok
}

func (tg *TgBot) getSetLimitState(userID int64) *setLimitState {
	tg.setLimitMu.Lock()
	defer tg.setLimitMu.Unlock()
	return tg.setLimitStates[userID]
}

func (tg *TgBot) setSetLimitState(userID int64, state *setLimitState) {
	tg.setLimitMu.Lock()
	defer tg.setLimitMu.Unlock()
	tg.setLimitStates[userID] = state
}

func (tg *TgBot) clearSetLimitState(userID int64) {
	tg.setLimitMu.Lock()
	defer tg.setLimitMu.Unlock()
	delete(tg.setLimitStates, userID)
}

func fullName(firstName, lastName string) string {
	value := strings.TrimSpace(strings.TrimSpace(firstName) + " " + strings.TrimSpace(lastName))
	if value == "" {
		return "Unknown"
	}
	return value
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func hasCopyableMedia(message *tgbotapi.Message) bool {
	if message == nil {
		return false
	}
	return message.Document != nil ||
		len(message.Photo) > 0 ||
		message.Video != nil ||
		message.Animation != nil ||
		message.Audio != nil ||
		message.Voice != nil
}

func (tg *TgBot) isAdminCacheMode(userID int64) bool {
	tg.cacheModeMu.Lock()
	defer tg.cacheModeMu.Unlock()
	_, ok := tg.cacheModeUsers[userID]
	return ok
}

func (tg *TgBot) setAdminCacheMode(userID int64, enabled bool) {
	tg.cacheModeMu.Lock()
	defer tg.cacheModeMu.Unlock()
	if enabled {
		tg.cacheModeUsers[userID] = struct{}{}
		if _, ok := tg.pendingCacheLinks[userID]; !ok {
			tg.pendingCacheLinks[userID] = nil
		}
		return
	}
	delete(tg.cacheModeUsers, userID)
}

func (tg *TgBot) pendingCacheCount(userID int64) int {
	tg.cacheModeMu.Lock()
	defer tg.cacheModeMu.Unlock()
	return len(tg.pendingCacheLinks[userID])
}

func (tg *TgBot) isPendingCacheKey(userID int64, cacheKey string) bool {
	tg.cacheModeMu.Lock()
	defer tg.cacheModeMu.Unlock()
	for _, item := range tg.pendingCacheLinks[userID] {
		if item.CacheKey == cacheKey {
			return true
		}
	}
	return false
}

func (tg *TgBot) queuePendingCacheLink(userID int64, item pendingCacheItem) int {
	tg.cacheModeMu.Lock()
	defer tg.cacheModeMu.Unlock()
	tg.pendingCacheLinks[userID] = append(tg.pendingCacheLinks[userID], item)
	return len(tg.pendingCacheLinks[userID])
}

func (tg *TgBot) popPendingCacheLink(userID int64) (pendingCacheItem, bool) {
	tg.cacheModeMu.Lock()
	defer tg.cacheModeMu.Unlock()
	queue := tg.pendingCacheLinks[userID]
	if len(queue) == 0 {
		return pendingCacheItem{}, false
	}
	item := queue[0]
	tg.pendingCacheLinks[userID] = queue[1:]
	return item, true
}

func (tg *TgBot) prependPendingCacheLink(userID int64, item pendingCacheItem) {
	tg.cacheModeMu.Lock()
	defer tg.cacheModeMu.Unlock()
	tg.pendingCacheLinks[userID] = append([]pendingCacheItem{item}, tg.pendingCacheLinks[userID]...)
}

func (tg *TgBot) clearPendingCacheLinks(userID int64) int {
	tg.cacheModeMu.Lock()
	defer tg.cacheModeMu.Unlock()
	count := len(tg.pendingCacheLinks[userID])
	tg.pendingCacheLinks[userID] = nil
	return count
}

func buildAssetCacheKey(rawURL, assetType string) string {
	normalized := normalizeURLForCache(rawURL)
	if resourceID := extractResourceID(normalized); resourceID != "" {
		return assetType + ":" + resourceID
	}
	sum := sha256.Sum256([]byte(normalized))
	return "url:" + fmt.Sprintf("%x", sum[:])[:32]
}

func extractResourceID(rawURL string) string {
	matches := resourceIDRegex.FindStringSubmatch(rawURL)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func normalizeURLForCache(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return strings.TrimSpace(rawURL)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String()
}

func debugUpdate(update tgbotapi.Update) string {
	payload, _ := json.Marshal(update)
	return string(payload)
}

func summarizeURLHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
