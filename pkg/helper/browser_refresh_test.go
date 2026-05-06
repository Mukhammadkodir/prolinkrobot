package helper

import "testing"

func TestBrowserCookieParamsSeedsAllNonEmptyCookies(t *testing.T) {
	params := browserCookieParams(map[string]string{
		"GR_TOKEN":                         "token",
		"GR_REFRESH":                       "refresh",
		"sessionid":                        "session",
		"_ga":                              "analytics",
		"ph_phc_Rc6y1yvZwwwR09Pl9NtKBo5gz": "{\"json\":true}",
		"UID":                              "130422127",
		"":                                 "skip",
	})

	if len(params) != 6 {
		t.Fatalf("expected 6 seeded cookies, got %d", len(params))
	}

	names := map[string]bool{}
	for _, param := range params {
		if param == nil {
			t.Fatal("expected non-nil cookie param")
		}
		names[param.Name] = true
		if param.URL != primaryAssetBaseURL() {
			t.Fatalf("expected cookie URL to be set, got %q", param.URL)
		}
	}

	for _, expected := range []string{"GR_TOKEN", "GR_REFRESH", "sessionid", "UID", "_ga", "ph_phc_Rc6y1yvZwwwR09Pl9NtKBo5gz"} {
		if !names[expected] {
			t.Fatalf("expected cookie %s to be included", expected)
		}
	}
}
