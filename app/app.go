package app

import (
	"get-link-tg-bot/config"
	"get-link-tg-bot/storage"
	mongostore "get-link-tg-bot/storage/mongo"
	"get-link-tg-bot/telegram"
	"get-link-tg-bot/usecase"
	"get-link-tg-bot/usecase/cases"

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
	bot, err := tgbotapi.NewBotAPI(a.cfg.Telegram.BotToken)
	if err != nil {
		return errs.Wrap(&err, "tgbotapi.NewBotAPI")
	}

	a.telegramBot = telegram.NewTgBot(a.cfg, bot, a.usecaseUsers)
	return nil
}
