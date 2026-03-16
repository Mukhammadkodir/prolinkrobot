package main

import (
	"get-link-tg-bot/app"
	"get-link-tg-bot/config"

	"github.com/sulton0011/errs"
)

func main() {
	release, err := app.AcquireRunLock()
	if err != nil {
		panic(err)
	}
	defer release()

	cfg, err := config.NewConfig()
	if err != nil {
		panic(err)
	}

	if err = app.RunApp(cfg); err != nil {
		errs.WrapLog(&err, nil, "app.RunApp(cfg)")
		panic(err)
	}
}
