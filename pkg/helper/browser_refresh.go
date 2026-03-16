package helper

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/sulton0011/errs"
)

func refreshFreepikCookieMapViaBrowser(cookieMap map[string]string) (map[string]string, bool, error) {
	if !freepikBrowserRefreshEnabled() {
		return nil, false, errs.New("browser refresh is disabled")
	}

	failures := make([]string, 0, 2)

	updated, changed, err := attemptBrowserRefresh(cookieMap, false)
	if err == nil {
		return updated, changed, nil
	}
	failures = append(failures, "profile: "+err.Error())

	updated, changed, err = attemptBrowserRefresh(cookieMap, true)
	if err == nil {
		return updated, changed, nil
	}
	failures = append(failures, "seeded: "+err.Error())

	return nil, false, errs.New(strings.Join(failures, " | "))
}

func attemptBrowserRefresh(cookieMap map[string]string, seedCookies bool) (map[string]string, bool, error) {
	timeout := freepikBrowserRefreshTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", freepikBrowserRefreshHeadless()),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	)
	if execPath := strings.TrimSpace(os.Getenv("FREEPIK_BROWSER_PATH")); execPath != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(execPath))
	}
	if userDataDir := strings.TrimSpace(os.Getenv("FREEPIK_BROWSER_USER_DATA_DIR")); userDataDir != "" {
		allocOpts = append(allocOpts, chromedp.UserDataDir(userDataDir))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	browserCookies := make([]*network.Cookie, 0)
	wait := freepikBrowserRefreshWait()
	tasks := chromedp.Tasks{
		network.Enable(),
	}
	if seedCookies {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			params := browserCookieParams(cookieMap)
			if len(params) == 0 {
				return nil
			}
			return network.SetCookies(params).Do(ctx)
		}))
	}
	tasks = append(tasks,
		chromedp.Navigate("https://www.freepik.com/"),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(wait),
		chromedp.Navigate("https://www.freepik.com/user/me"),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(wait),
		chromedp.Navigate("https://www.freepik.com/icon/fire_785116"),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(wait/2),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, exception, err := runtime.Evaluate(browserRefreshWarmupScript).WithAwaitPromise(true).Do(ctx)
			if err != nil {
				return err
			}
			if exception != nil {
				return errs.New(exception.Text)
			}
			return nil
		}),
		chromedp.Sleep(wait/2),
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := network.GetCookies().WithURLs([]string{
				"https://www.freepik.com/",
				"https://ru.freepik.com/",
			}).Do(ctx)
			if err != nil {
				return err
			}
			browserCookies = cookies
			return nil
		}),
	)

	if err := chromedp.Run(browserCtx, tasks); err != nil {
		return nil, false, errs.Wrap(&err, "chromedp.Run")
	}

	return mergeBrowserCookies(cookieMap, browserCookies)
}

func mergeBrowserCookies(cookieMap map[string]string, browserCookies []*network.Cookie) (map[string]string, bool, error) {
	updated := cloneCookieMap(cookieMap)
	changed := false
	for _, item := range browserCookies {
		if item == nil {
			continue
		}
		if !shouldPersistFreepikCookie(item.Name) || strings.TrimSpace(item.Value) == "" {
			continue
		}
		if updated[item.Name] != item.Value {
			changed = true
		}
		updated[item.Name] = item.Value
	}

	token := strings.TrimSpace(updated["GR_TOKEN"])
	if token == "" {
		return nil, false, errs.New("browser refresh did not return GR_TOKEN")
	}
	if isJWTExpired(token) {
		return nil, false, errs.New("browser refresh returned expired GR_TOKEN")
	}
	if !changed {
		return nil, false, errs.New("browser refresh did not update cookies")
	}
	return updated, true, nil
}

func browserCookieParams(cookieMap map[string]string) []*network.CookieParam {
	if len(cookieMap) == 0 {
		return nil
	}

	params := make([]*network.CookieParam, 0, len(cookieMap))
	for key, value := range cookieMap {
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		params = append(params, &network.CookieParam{
			Name:  key,
			Value: value,
			URL:   "https://www.freepik.com/",
		})
	}
	return params
}

const browserRefreshWarmupScript = `(async () => {
	const readCookie = (name) => {
		const match = document.cookie.match(new RegExp('(?:^|; )' + name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '=([^;]+)'));
		return match ? decodeURIComponent(match[1]) : '';
	};
	const csrf = readCookie('csrftoken') || readCookie('csrf_freepik');
	const requests = [
		['/user/me', null],
		['/icon/fire_785116', null],
		['/api/icon/download?optionId=785116&format=svg&type=original', Object.assign({'x-requested-with': 'XMLHttpRequest'}, csrf ? {'x-csrf-token': csrf, 'x-csrftoken': csrf} : {})],
	];
	for (const [path, headers] of requests) {
		try {
			const resp = await fetch(path, {
				credentials: 'include',
				headers: headers || undefined,
			});
			await resp.text();
		} catch (_) {}
	}
	return true;
})()`

func freepikBrowserRefreshEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FREEPIK_BROWSER_REFRESH_ENABLED"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func freepikBrowserRefreshHeadless() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FREEPIK_BROWSER_HEADLESS"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func freepikBrowserRefreshTimeout() time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(os.Getenv("FREEPIK_BROWSER_TIMEOUT_SECONDS")))
	if err != nil || seconds <= 0 {
		return 90 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func freepikBrowserRefreshWait() time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(os.Getenv("FREEPIK_BROWSER_WAIT_SECONDS")))
	if err != nil || seconds <= 0 {
		return 8 * time.Second
	}
	return time.Duration(seconds) * time.Second
}
