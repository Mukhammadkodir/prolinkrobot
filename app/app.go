package app

import (
	"get-link-tg-bot/config"
	"get-link-tg-bot/storage"
	mongostore "get-link-tg-bot/storage/mongo"
	"get-link-tg-bot/telegram"
	"get-link-tg-bot/usecase"
	"get-link-tg-bot/usecase/cases"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sulton0011/errs"
)

type app struct {
	cfg          *config.Config
	telegramBot  *telegram.TgBot
	storageUsers storage.UsersI
	usecaseUsers usecase.UsecaseUsersI
}

func RunApp(cfg *config.Config) (err error) {
	instance := app{cfg: cfg}

	if err = instance.StorageUsers(); err != nil {
		return errs.Wrap(&err, "app.StorageUsers()")
	}
	instance.UsecaseUsers()
	instance.startDailyResetLoop()
	if err = instance.TelegramBot(); err != nil {
		return errs.Wrap(&err, "app.TelegramBot()")
	}
	instance.startFreepikAuthMonitor()

	instance.telegramBot.Read()
	return nil
}

func (a *app) StorageUsers() error {
	store, err := mongostore.NewUsers(a.cfg)
	if err != nil {
		return errs.Wrap(&err, "mongo.NewUsers")
	}
	a.storageUsers = store
	return nil
}

func (a *app) UsecaseUsers() {
	a.usecaseUsers = cases.NewUsecaseUsers(a.cfg, a.storageUsers)
}

func (a *app) TelegramBot() error {
	bot, err := newBotAPI(a.cfg.Telegram.BotToken, a.cfg.Telegram.APIEndpoint)
	if err != nil {
		return errs.Wrap(&err, "tgbotapi.NewBotAPI")
	}

	cacheBot := bot
	if cacheEndpoint := strings.TrimSpace(a.cfg.Telegram.CacheAPIEndpoint); cacheEndpoint != "" && cacheEndpoint != strings.TrimSpace(a.cfg.Telegram.APIEndpoint) {
		cacheBot, err = newBotAPI(a.cfg.Telegram.BotToken, cacheEndpoint)
		if err != nil {
			return errs.Wrap(&err, "tgbotapi.NewBotAPIWithAPIEndpoint(cache)")
		}
	}

	a.telegramBot = telegram.NewTgBot(a.cfg, bot, cacheBot, a.usecaseUsers)
	return nil
}

func newBotAPI(token, endpoint string) (*tgbotapi.BotAPI, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return tgbotapi.NewBotAPI(token)
	}
	if strings.Count(endpoint, "%s") != 2 {
		return nil, errs.New("telegram api endpoint must contain exactly two %s placeholders")
	}
	return tgbotapi.NewBotAPIWithAPIEndpoint(token, endpoint)
}
