package app

import (
	"fmt"
	"get-link-tg-bot/pkg/helper"
	"log"
	"os"
	"strings"
	"time"
)

func (a *app) startFreepikAuthMonitor() {
	if a.telegramBot == nil {
		return
	}

	interval := time.Duration(a.cfg.Freepik.AuthCheckIntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		lastState := ""
		a.checkFreepikAuthStatus(&lastState)

		for range ticker.C {
			a.checkFreepikAuthStatus(&lastState)
		}
	}()
}

func (a *app) checkFreepikAuthStatus(lastState *string) {
	status, err := helper.GetFreepikAuthStatus()
	if err != nil {
		state := "error:" + err.Error()
		if state != *lastState {
			log.Printf("freepik auth monitor error: %v", err)
			a.telegramBot.NotifyAdmins("❌ Freepik auth monitor could not read cookie/token status.\n\nError: " + err.Error())
			*lastState = state
		}
		return
	}

	if status == nil {
		return
	}

	if !status.HasToken {
		refreshedStatus, refreshed, refreshErr := a.tryForceFreepikRefresh(status, 0)
		if refreshErr != nil {
			log.Printf("freepik auth monitor refresh error: %v", refreshErr)
		}
		if refreshedStatus != nil {
			status = refreshedStatus
		}
		if refreshed && status.HasToken && status.ExpiresAt.After(time.Now()) {
			state := "ok:" + fmt.Sprintf("%d", status.ExpiresAt.Unix())
			if *lastState != state {
				a.telegramBot.NotifyAdmins(buildFreepikRecoveredMessage(status.Source, status.ExpiresAt, time.Local))
				*lastState = state
			}
			return
		}

		state := "missing:" + status.Source
		if state != *lastState {
			a.telegramBot.NotifyAdmins(buildFreepikMissingTokenMessage(status.Source))
			*lastState = state
		}
		return
	}

	warnBefore := time.Duration(a.cfg.Freepik.WarnMinutes) * time.Minute
	if warnBefore <= 0 {
		warnBefore = 20 * time.Minute
	}

	criticalBefore := time.Duration(a.cfg.Freepik.CriticalMinutes) * time.Minute
	if criticalBefore <= 0 {
		criticalBefore = 5 * time.Minute
	}

	now := time.Now()
	timeLeft := time.Until(status.ExpiresAt)
	expiresKey := fmt.Sprintf("%d", status.ExpiresAt.Unix())
	location := time.Local
	if loc, err := time.LoadLocation(a.cfg.Limits.Timezone); err == nil {
		location = loc
	}

	refreshedStatus, refreshed, refreshErr := a.tryForceFreepikRefresh(status, warnBefore)
	if refreshErr != nil {
		log.Printf("freepik auth monitor refresh error: %v", refreshErr)
	}
	if refreshedStatus != nil {
		status = refreshedStatus
		now = time.Now()
		timeLeft = time.Until(status.ExpiresAt)
		expiresKey = fmt.Sprintf("%d", status.ExpiresAt.Unix())
	}
	if refreshed && status.HasToken && status.ExpiresAt.After(now) {
		state := "ok:" + expiresKey
		if *lastState != state {
			a.telegramBot.NotifyAdmins(buildFreepikRecoveredMessage(status.Source, status.ExpiresAt, location))
		}
		*lastState = state
		return
	}

	switch {
	case !status.ExpiresAt.After(now):
		state := "expired:" + expiresKey
		if state != *lastState {
			a.telegramBot.NotifyAdmins(buildFreepikExpiredMessage(status.Source, status.ExpiresAt, location))
			*lastState = state
		}
	case timeLeft <= criticalBefore:
		state := "critical:" + expiresKey
		if state != *lastState {
			a.telegramBot.NotifyAdmins(buildFreepikExpiringMessage("🚨", "Freepik token is about to expire.", status.Source, status.ExpiresAt, timeLeft, location))
			*lastState = state
		}
	case timeLeft <= warnBefore:
		state := "warn:" + expiresKey
		if state != *lastState {
			a.telegramBot.NotifyAdmins(buildFreepikExpiringMessage("⚠️", "Freepik token will expire soon.", status.Source, status.ExpiresAt, timeLeft, location))
			*lastState = state
		}
	default:
		state := "ok:" + expiresKey
		if *lastState != "" && !strings.HasPrefix(*lastState, "ok:") {
			a.telegramBot.NotifyAdmins(buildFreepikRecoveredMessage(status.Source, status.ExpiresAt, location))
		}
		*lastState = state
	}
}

func (a *app) tryForceFreepikRefresh(status *helper.FreepikAuthStatus, warnBefore time.Duration) (*helper.FreepikAuthStatus, bool, error) {
	if status == nil || !freepikAutoRefreshConfigured() {
		return status, false, nil
	}

	needsRefresh := !status.HasToken
	if status.HasToken {
		timeLeft := time.Until(status.ExpiresAt)
		needsRefresh = !status.ExpiresAt.After(time.Now()) || (warnBefore > 0 && timeLeft <= warnBefore)
	}
	if !needsRefresh {
		return status, false, nil
	}

	refreshedStatus, refreshed, err := helper.ForceRefreshFreepikAuth()
	if refreshedStatus != nil {
		status = refreshedStatus
	}
	return status, refreshed, err
}

func buildFreepikMissingTokenMessage(source string) string {
	var builder strings.Builder
	builder.WriteString("❌ Freepik GR_TOKEN is missing.\n")
	builder.WriteString("Source: ")
	builder.WriteString(nonEmptyValue(source, "unknown"))
	builder.WriteString("\n")
	builder.WriteString(freepikRefreshActionText(source, false))
	return builder.String()
}

func buildFreepikExpiredMessage(source string, expiresAt time.Time, location *time.Location) string {
	var builder strings.Builder
	builder.WriteString("❌ Freepik token has expired.\n")
	builder.WriteString("Source: ")
	builder.WriteString(nonEmptyValue(source, "unknown"))
	builder.WriteString("\nExpired at: ")
	builder.WriteString(expiresAt.In(location).Format("2006-01-02 15:04:05 -0700"))
	builder.WriteString("\nExpired since: ")
	builder.WriteString(formatDuration(time.Since(expiresAt)))
	builder.WriteString(" ago")
	builder.WriteString("\n")
	builder.WriteString(freepikRefreshActionText(source, true))
	return builder.String()
}

func buildFreepikRecoveredMessage(source string, expiresAt time.Time, location *time.Location) string {
	var builder strings.Builder
	builder.WriteString("✅ Freepik token was updated.\n")
	builder.WriteString("Source: ")
	builder.WriteString(nonEmptyValue(source, "unknown"))
	builder.WriteString("\nNew expiry: ")
	builder.WriteString(expiresAt.In(location).Format("2006-01-02 15:04:05 -0700"))
	return builder.String()
}

func buildFreepikExpiringMessage(prefix, title, source string, expiresAt time.Time, timeLeft time.Duration, location *time.Location) string {
	var builder strings.Builder
	builder.WriteString(prefix)
	builder.WriteString(" ")
	builder.WriteString(title)
	builder.WriteString("\nSource: ")
	builder.WriteString(nonEmptyValue(source, "unknown"))
	builder.WriteString("\nExpires at: ")
	builder.WriteString(expiresAt.In(location).Format("2006-01-02 15:04:05 -0700"))
	builder.WriteString("\nTime left: ")
	builder.WriteString(formatDuration(timeLeft))
	builder.WriteString("\n")
	builder.WriteString(freepikRefreshActionText(source, true))
	return builder.String()
}

func formatDuration(value time.Duration) string {
	if value < 0 {
		value = -value
	}

	totalMinutes := int(value.Round(time.Minute).Minutes())
	if totalMinutes <= 0 {
		return "<1m"
	}

	hours := totalMinutes / 60
	minutes := totalMinutes % 60
	switch {
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

func isFileCookieSource(source string) bool {
	return source != "" && !strings.HasPrefix(source, "env:")
}

func nonEmptyValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func freepikRefreshActionText(source string, urgent bool) string {
	var builder strings.Builder
	if urgent {
		builder.WriteString("Action: update the Freepik cookies now.")
	} else {
		builder.WriteString("Action: update the cookie source with a fresh export.")
	}

	if !freepikAutoRefreshConfigured() {
		builder.WriteString("\nAutomatic token refresh is not configured on this server yet.")
	} else if strings.EqualFold(strings.TrimSpace(os.Getenv("FREEPIK_BROWSER_REFRESH_ENABLED")), "true") || strings.TrimSpace(os.Getenv("FREEPIK_BROWSER_REFRESH_ENABLED")) == "1" {
		builder.WriteString("\nBrowser refresh fallback is enabled, but an already-invalid session may still require a fresh cookie export.")
	}

	if isFileCookieSource(source) {
		builder.WriteString("\nReplace the cookie file only. Restart is not required.")
	} else {
		builder.WriteString("\nThis source comes from env, so service restart will be required after update.")
	}
	return builder.String()
}

func freepikAutoRefreshConfigured() bool {
	if strings.TrimSpace(os.Getenv("FREEPIK_FIREBASE_API_KEY")) != "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FREEPIK_BROWSER_REFRESH_ENABLED"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
