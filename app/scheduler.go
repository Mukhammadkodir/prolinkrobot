package app

import (
	"log"
	"time"
)

func (a *app) startDailyResetLoop() {
	go func() {
		location, err := time.LoadLocation(a.cfg.Limits.Timezone)
		if err != nil {
			log.Printf("daily reset scheduler disabled: %v", err)
			return
		}

		for {
			now := time.Now().In(location)
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, location)
			timer := time.NewTimer(next.Sub(now))
			<-timer.C

			count, err := a.usecaseUsers.ResetDailyLimits()
			if err != nil {
				log.Printf("daily reset failed: %v", err)
				continue
			}
			log.Printf("daily limits reset for %d users", count)
		}
	}()
}
