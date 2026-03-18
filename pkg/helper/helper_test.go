package helper

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractDownloadURLPrefersOriginalVideoOverIconURL(t *testing.T) {
	body := []byte(`{
		"url": "https://cdn-icons.flaticon.com/svg/945/945775.svg?token=abc",
		"download": {
			"original": "https://videocdn.cdnpk.net/videos/fb6d38ac-c5c2-4715-aed0-a26db05b90b6/horizontal/downloads/original.mov?filename=4914295_100_Bill_1920x1080.mov&user_id=130422127&token=xyz"
		}
	}`)

	got, err := extractDownloadURL(body, "video-option")
	if err != nil {
		t.Fatalf("extractDownloadURL returned error: %v", err)
	}

	want := "https://videocdn.cdnpk.net/videos/fb6d38ac-c5c2-4715-aed0-a26db05b90b6/horizontal/downloads/original.mov?filename=4914295_100_Bill_1920x1080.mov&user_id=130422127&token=xyz"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestExtractDownloadURLRejectsPreviewOnlyVideo(t *testing.T) {
	body := []byte(`{
		"preview": "https://videocdn.cdnpk.net/videos/abc/horizontal/previews/watermarked/large.mp4?token=bad"
	}`)

	_, err := extractDownloadURL(body, "video-option")
	if err == nil {
		t.Fatalf("expected error for preview-only video response")
	}
}

func TestExtractDownloadURLAcceptsGettyVideoURL(t *testing.T) {
	body := []byte(`{
		"filename":"825156360_Horizontal_Abstract_3840x2160.mov",
		"url":"https://delivery.gettyimages.com/downloads/825156360?k=20&e=abc"
	}`)

	got, err := extractDownloadURL(body, "video-detail-option")
	if err != nil {
		t.Fatalf("extractDownloadURL returned error: %v", err)
	}
	if !strings.Contains(got, "delivery.gettyimages.com/downloads/825156360") {
		t.Fatalf("unexpected getty url: %q", got)
	}
}

func TestExtractEmbeddedVideoOptionIDs(t *testing.T) {
	body := []byte(`{
		"optionId":100,
		"nested":{"option_id":"200"},
		"filename":"4914295_100_Bill_1920x1080.mov"
	}`)

	got := extractEmbeddedVideoOptionIDs(body)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 option ids, got %v", got)
	}
	if got[0] != "100" {
		t.Fatalf("expected first option id 100, got %v", got)
	}
}

func TestExtractEmbeddedVideoURLs(t *testing.T) {
	body := []byte(`{"download":"https:\/\/videocdn.cdnpk.net\/videos\/x\/horizontal\/downloads\/original.mov?filename=a.mov&token=1","preview":"https:\/\/videocdn.cdnpk.net\/videos\/x\/horizontal\/previews\/watermarked\/large.mp4?token=2"}`)

	got := extractEmbeddedVideoURLs(body)
	if len(got) != 1 {
		t.Fatalf("expected exactly one embedded video download url, got %v", got)
	}
	if got[0] != "https://videocdn.cdnpk.net/videos/x/horizontal/downloads/original.mov?filename=a.mov&token=1" {
		t.Fatalf("unexpected embedded video url: %v", got)
	}
}

func TestBuildIconDownloadEndpointsReturnsOnlySVG(t *testing.T) {
	got := buildIconDownloadEndpoints("https://www.freepik.com", "785116")
	if len(got) != 2 {
		t.Fatalf("expected 2 svg endpoints, got %d", len(got))
	}
	for _, item := range got {
		if !strings.Contains(item.url, "format=svg") {
			t.Fatalf("expected svg-only endpoints, got %q", item.url)
		}
	}
}

func TestBuild3DFormatOptions(t *testing.T) {
	metadata := &model3DMetadata{
		HasBlendFile: true,
		HasObjFile:   false,
		HasFbxFile:   true,
	}
	metadata.Specifications.IncludeTextures = true

	got := build3DFormatOptions(metadata)
	if len(got) != 4 {
		t.Fatalf("expected 4 format options, got %d", len(got))
	}
	if got[0].Name != "BLEND" || !got[0].Enabled {
		t.Fatalf("expected BLEND to be enabled, got %+v", got[0])
	}
	if got[1].Name != "OBJ" || got[1].Enabled {
		t.Fatalf("expected OBJ to be disabled, got %+v", got[1])
	}
	if got[3].Name != "TEXTURES" || !got[3].Enabled {
		t.Fatalf("expected TEXTURES to be enabled, got %+v", got[3])
	}
}

func TestGetDownloadLinkFreepik3DUsesModel3DEndpoint(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/model3d/23192/download" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			if got := req.URL.Query().Get("fileType"); got != "blend" {
				t.Fatalf("expected fileType=blend, got %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"url":"https://3d.cdnpk.net/models/23192/downloads/tube-box.blend?token=1"
				}`)),
				Request: req,
			}, nil
		}),
	}

	normalized, err := url.Parse("https://www.freepik.com/3d-model/tube-box-with-label_23192.htm")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	got, err := getDownloadLinkFreepik3D(client, normalized, "23192", "blend", &model3DMetadata{}, "GR_TOKEN=test", "", "", "token")
	if err != nil {
		t.Fatalf("getDownloadLinkFreepik3D returned error: %v", err)
	}
	want := "https://3d.cdnpk.net/models/23192/downloads/tube-box.blend?token=1"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildDownloadEndpointsPrioritizesLocaleForRegularAssets(t *testing.T) {
	u, err := url.Parse("https://www.freepik.com/premium-psd/book-cover-mockup-orange-office-chair_423135438.htm")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	got := buildDownloadEndpoints(u, "423135438", nil)
	if len(got) == 0 {
		t.Fatal("expected endpoints")
	}
	if got[0].label != "regular-contentType-locale" {
		t.Fatalf("expected locale-aware contentType candidate first, got %q", got[0].label)
	}
}

func TestGetDownloadLinkFreepikVideoUsesDetailEndpoint(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/video/3898315/download" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			if got := req.URL.Query().Get("optionId"); got != "19716632" {
				t.Fatalf("expected optionId=19716632, got %q", got)
			}
			if got := req.URL.Query().Get("orientation"); got != "horizontal" {
				t.Fatalf("expected orientation=horizontal, got %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"url":"https://videocdn.cdnpk.net/videos/x/horizontal/downloads/original.mov?filename=test.mov&token=1"
				}`)),
				Request: req,
			}, nil
		}),
	}

	normalized, err := url.Parse("https://www.freepik.com/free-video/abstract-geometric-background_3898315")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	pageData := &assetPageData{
		Video: &videoPageData{
			ID:          3898315,
			Orientation: "horizontal",
			Options: []videoOption{
				{ID: 19716632, IsOriginal: true, Container: "mov", Width: 3840, Height: 2160, Active: true},
			},
		},
	}

	got, err := getDownloadLinkFreepikVideo(client, normalized, "3898315", pageData, "GR_TOKEN=test", "", "", "token")
	if err != nil {
		t.Fatalf("getDownloadLinkFreepikVideo returned error: %v", err)
	}

	want := "https://videocdn.cdnpk.net/videos/x/horizontal/downloads/original.mov?filename=test.mov&token=1"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBestVideoCacheOptionIDsPrefersLargestVariantWithinLimit(t *testing.T) {
	pageData := &assetPageData{
		Video: &videoPageData{
			Options: []videoOption{
				{ID: 19716632, IsOriginal: true, Container: "mov", Width: 3840, Height: 2160, Active: true, Size: 212},
				{ID: 19716633, IsOriginal: false, Container: "mp4", Width: 3840, Height: 2160, Active: true, Size: 30},
				{ID: 19716634, IsOriginal: false, Container: "mp4", Width: 1920, Height: 1080, Active: true, Size: 13},
				{ID: 19716635, IsOriginal: false, Container: "mp4", Width: 1280, Height: 720, Active: true, Size: 6},
			},
		},
	}

	got := pageData.bestVideoCacheOptionIDs("3898315", 50*1024*1024)
	if len(got) == 0 {
		t.Fatalf("expected cacheable options")
	}
	if got[0] != "19716633" {
		t.Fatalf("expected 4k mp4 option first, got %v", got)
	}
	for _, id := range got {
		if id == "19716632" {
			t.Fatalf("expected oversized original mov to be excluded, got %v", got)
		}
	}
}

func TestBestVideoCacheOptionIDsExcludesGIFVariants(t *testing.T) {
	pageData := &assetPageData{
		Video: &videoPageData{
			Options: []videoOption{
				{ID: 1, Container: "gif", Width: 640, Height: 360, Active: true, Size: 3},
				{ID: 2, Container: "mp4", Width: 640, Height: 360, Active: true, Size: 4},
			},
		},
	}

	got := pageData.bestVideoCacheOptionIDs("3898315", 50*1024*1024)
	if len(got) == 0 {
		t.Fatalf("expected cacheable options")
	}
	if got[0] != "2" {
		t.Fatalf("expected mp4 option first, got %v", got)
	}
	for _, id := range got {
		if id == "1" {
			t.Fatalf("expected gif option to be excluded, got %v", got)
		}
	}
}

func TestShouldAutoRefreshFreepikAuth(t *testing.T) {
	t.Setenv("FREEPIK_AUTH_REFRESH_BEFORE_MINUTES", "15")

	expiringSoon := map[string]string{
		"GR_REFRESH": "refresh-token",
		"GR_TOKEN":   testJWT(time.Now().Add(10 * time.Minute)),
	}
	if !shouldAutoRefreshFreepikAuth(expiringSoon) {
		t.Fatalf("expected token expiring soon to trigger auto refresh")
	}

	freshEnough := map[string]string{
		"GR_REFRESH": "refresh-token",
		"GR_TOKEN":   testJWT(time.Now().Add(40 * time.Minute)),
	}
	if shouldAutoRefreshFreepikAuth(freshEnough) {
		t.Fatalf("expected token with enough lifetime to skip auto refresh")
	}
}

func TestLoadCookieHeaderWithSourceAutoRefreshesFile(t *testing.T) {
	originalURL := freepikAuthRefreshURL
	originalClient := freepikAuthClient
	originalSecureURL := freepikSecureTokenRefreshURL
	originalSecureClient := freepikSecureTokenClient
	freepikAuthRefreshURL = "https://www.freepik.com/"
	freepikSecureTokenRefreshURL = "https://securetoken.googleapis.com/v1/token"
	freepikAuthClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("homepage refresh should not be called when securetoken succeeds")
			return nil, nil
		}),
	}
	freepikSecureTokenClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST request, got %s", req.Method)
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("io.ReadAll: %v", err)
			}
			if !strings.Contains(string(body), "refresh_token=old-refresh-token") {
				t.Fatalf("expected refresh token in request body, got %q", string(body))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"id_token":"` + testJWT(time.Now().Add(1*time.Hour)) + `",
					"refresh_token":"new-refresh-token",
					"expires_in":"3600"
				}`)),
				Request: req,
			}, nil
		}),
	}
	defer func() { freepikAuthRefreshURL = originalURL }()
	defer func() { freepikAuthClient = originalClient }()
	defer func() { freepikSecureTokenRefreshURL = originalSecureURL }()
	defer func() { freepikSecureTokenClient = originalSecureClient }()

	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "freepik_cookies.json")
	payload := map[string]map[string]string{
		"Request Cookies": {
			"GR_REFRESH": "old-refresh-token",
			"GR_TOKEN":   testJWT(time.Now().Add(5 * time.Minute)),
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(cookieFile, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	t.Setenv("FREEPIK_COOKIES_FILE", cookieFile)
	t.Setenv("FREEPIK_AUTH_REFRESH_BEFORE_MINUTES", "15")

	header, source, err := loadCookieHeaderWithSource()
	if err != nil {
		t.Fatalf("loadCookieHeaderWithSource: %v", err)
	}
	if source != cookieFile {
		t.Fatalf("expected source %q, got %q", cookieFile, source)
	}
	if !strings.Contains(header, "GR_REFRESH=new-refresh-token") {
		t.Fatalf("expected refreshed header, got %q", header)
	}

	updated, err := os.ReadFile(cookieFile)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	if !strings.Contains(string(updated), "new-refresh-token") {
		t.Fatalf("expected cookie file to be updated, got %s", string(updated))
	}
}

func TestLoadCookieHeaderWithSourceFallsBackToHomepageRefresh(t *testing.T) {
	originalURL := freepikAuthRefreshURL
	originalClient := freepikAuthClient
	originalSecureURL := freepikSecureTokenRefreshURL
	originalSecureClient := freepikSecureTokenClient
	freepikAuthRefreshURL = "https://www.freepik.com/"
	freepikSecureTokenRefreshURL = "https://securetoken.googleapis.com/v1/token"
	freepikSecureTokenClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"TOKEN_EXPIRED"}}`)),
				Request:    req,
			}, nil
		}),
	}
	freepikAuthClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := http.Header{}
			rec.Add("Set-Cookie", (&http.Cookie{Name: "GR_TOKEN", Value: testJWT(time.Now().Add(1 * time.Hour)), Path: "/"}).String())
			rec.Add("Set-Cookie", (&http.Cookie{Name: "GR_REFRESH", Value: "homepage-refresh-token", Path: "/"}).String())
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     rec,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		}),
	}
	defer func() { freepikAuthRefreshURL = originalURL }()
	defer func() { freepikAuthClient = originalClient }()
	defer func() { freepikSecureTokenRefreshURL = originalSecureURL }()
	defer func() { freepikSecureTokenClient = originalSecureClient }()

	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "freepik_cookies.json")
	payload := map[string]map[string]string{
		"Request Cookies": {
			"GR_REFRESH": "old-refresh-token",
			"GR_TOKEN":   testJWT(time.Now().Add(5 * time.Minute)),
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(cookieFile, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	t.Setenv("FREEPIK_COOKIES_FILE", cookieFile)
	t.Setenv("FREEPIK_AUTH_REFRESH_BEFORE_MINUTES", "15")

	header, _, err := loadCookieHeaderWithSource()
	if err != nil {
		t.Fatalf("loadCookieHeaderWithSource: %v", err)
	}
	if !strings.Contains(header, "GR_REFRESH=homepage-refresh-token") {
		t.Fatalf("expected homepage refresh token in header, got %q", header)
	}
}

func TestPersistAuthCookiesFromResponseUpdatesFile(t *testing.T) {
	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "freepik_cookies.json")
	payload := map[string]map[string]string{
		"Request Cookies": {
			"GR_TOKEN":   testJWT(time.Now().Add(-1 * time.Hour)),
			"GR_REFRESH": "old-refresh",
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(cookieFile, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	newToken := testJWT(time.Now().Add(1 * time.Hour))
	err = persistAuthCookiesFromResponse(cookieFile, []*http.Cookie{
		{Name: "GR_TOKEN", Value: newToken},
		{Name: "sessionid", Value: "new-session"},
	})
	if err != nil {
		t.Fatalf("persistAuthCookiesFromResponse: %v", err)
	}

	updated, err := os.ReadFile(cookieFile)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	text := string(updated)
	if !strings.Contains(text, newToken) {
		t.Fatalf("expected updated GR_TOKEN in file, got %s", text)
	}
	if !strings.Contains(text, "new-session") {
		t.Fatalf("expected updated sessionid in file, got %s", text)
	}
}

func TestLoadCookieHeaderWithSourceKeepsExistingCookieWhenRefreshFails(t *testing.T) {
	originalURL := freepikAuthRefreshURL
	originalClient := freepikAuthClient
	originalSecureURL := freepikSecureTokenRefreshURL
	originalSecureClient := freepikSecureTokenClient
	freepikAuthRefreshURL = "https://www.freepik.com/"
	freepikSecureTokenRefreshURL = "https://securetoken.googleapis.com/v1/token"
	freepikSecureTokenClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"PERMISSION_DENIED"}}`)),
				Request:    req,
			}, nil
		}),
	}
	freepikAuthClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("<!DOCTYPE html><html><body>challenge</body></html>")),
				Request:    req,
			}, nil
		}),
	}
	defer func() { freepikAuthRefreshURL = originalURL }()
	defer func() { freepikAuthClient = originalClient }()
	defer func() { freepikSecureTokenRefreshURL = originalSecureURL }()
	defer func() { freepikSecureTokenClient = originalSecureClient }()

	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "freepik_cookies.json")
	expiredToken := testJWT(time.Now().Add(-1 * time.Hour))
	payload := map[string]map[string]string{
		"Request Cookies": {
			"GR_REFRESH": "old-refresh-token",
			"GR_TOKEN":   expiredToken,
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(cookieFile, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	t.Setenv("FREEPIK_COOKIES_FILE", cookieFile)
	t.Setenv("FREEPIK_AUTH_REFRESH_BEFORE_MINUTES", "15")

	header, source, err := loadCookieHeaderWithSource()
	if err != nil {
		t.Fatalf("loadCookieHeaderWithSource: %v", err)
	}
	if source != cookieFile {
		t.Fatalf("expected source %q, got %q", cookieFile, source)
	}
	if !strings.Contains(header, "GR_TOKEN="+expiredToken) {
		t.Fatalf("expected original token in header, got %q", header)
	}
}

func TestForceRefreshFreepikAuthRefreshesCookieFile(t *testing.T) {
	originalURL := freepikAuthRefreshURL
	originalClient := freepikAuthClient
	originalSecureURL := freepikSecureTokenRefreshURL
	originalSecureClient := freepikSecureTokenClient
	freepikAuthRefreshURL = "https://www.freepik.com/"
	freepikSecureTokenRefreshURL = "https://securetoken.googleapis.com/v1/token"
	freepikAuthClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("homepage refresh should not be called when securetoken succeeds")
			return nil, nil
		}),
	}
	newToken := testJWT(time.Now().Add(90 * time.Minute))
	freepikSecureTokenClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"id_token":"` + newToken + `",
					"refresh_token":"rotated-refresh-token",
					"expires_in":"5400"
				}`)),
				Request: req,
			}, nil
		}),
	}
	defer func() { freepikAuthRefreshURL = originalURL }()
	defer func() { freepikAuthClient = originalClient }()
	defer func() { freepikSecureTokenRefreshURL = originalSecureURL }()
	defer func() { freepikSecureTokenClient = originalSecureClient }()

	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "freepik_cookies.json")
	payload := map[string]map[string]string{
		"Request Cookies": {
			"GR_REFRESH": "old-refresh-token",
			"GR_TOKEN":   testJWT(time.Now().Add(-10 * time.Minute)),
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(cookieFile, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	t.Setenv("FREEPIK_COOKIES_FILE", cookieFile)

	status, refreshed, err := ForceRefreshFreepikAuth()
	if err != nil {
		t.Fatalf("ForceRefreshFreepikAuth: %v", err)
	}
	if !refreshed {
		t.Fatalf("expected refresh=true")
	}
	if status == nil || !status.HasToken {
		t.Fatalf("expected refreshed status with token, got %#v", status)
	}

	updated, err := os.ReadFile(cookieFile)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	text := string(updated)
	if !strings.Contains(text, newToken) {
		t.Fatalf("expected refreshed GR_TOKEN in file, got %s", text)
	}
	if !strings.Contains(text, "rotated-refresh-token") {
		t.Fatalf("expected refreshed GR_REFRESH in file, got %s", text)
	}
}

func TestFetchAssetPageDataPersistsAuthCookies(t *testing.T) {
	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "freepik_cookies.json")
	payload := map[string]map[string]string{
		"Request Cookies": {
			"GR_TOKEN": testJWT(time.Now().Add(-1 * time.Hour)),
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(cookieFile, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	newToken := testJWT(time.Now().Add(1 * time.Hour))
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := http.Header{}
			header.Add("Set-Cookie", (&http.Cookie{Name: "GR_TOKEN", Value: newToken, Path: "/"}).String())
			header.Add("Set-Cookie", (&http.Cookie{Name: "sessionid", Value: "session-from-page", Path: "/"}).String())
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(`<html><body>ok</body></html>`)),
				Request:    req,
			}, nil
		}),
	}

	pageURL, err := url.Parse("https://www.freepik.com/icon/fire_785116")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	if _, err := fetchAssetPageData(client, pageURL, "GR_TOKEN=expired", cookieFile); err != nil {
		t.Fatalf("fetchAssetPageData: %v", err)
	}

	updated, err := os.ReadFile(cookieFile)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	text := string(updated)
	if !strings.Contains(text, newToken) {
		t.Fatalf("expected GR_TOKEN persisted from page response, got %s", text)
	}
	if !strings.Contains(text, "session-from-page") {
		t.Fatalf("expected sessionid persisted from page response, got %s", text)
	}
}

func TestExecuteDownloadRequestSendsAuthHeaders(t *testing.T) {
	var gotAuth string
	var gotCSRF string
	var gotRequestedWith string
	var gotEncoding string

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotAuth = req.Header.Get("Authorization")
			gotCSRF = req.Header.Get("x-csrf-token")
			gotRequestedWith = req.Header.Get("X-Requested-With")
			gotEncoding = req.Header.Get("Accept-Encoding")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"url":"https://cdn-icons.flaticon.com/svg/785/785116.svg?token=abc"}`)),
				Request:    req,
			}, nil
		}),
	}

	got, status, _, err := executeDownloadRequest(
		client,
		"icon-svg-original",
		"https://www.freepik.com/api/icon/download?optionId=785116&format=svg&type=original",
		"https://www.freepik.com/icon/fire_785116",
		"GR_TOKEN=test-token; csrftoken=test-csrf",
		"",
		"test-csrf",
		"test-token",
	)
	if err != nil {
		t.Fatalf("executeDownloadRequest returned error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
	if got == "" {
		t.Fatal("expected extracted download url")
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("expected Authorization header, got %q", gotAuth)
	}
	if gotCSRF != "test-csrf" {
		t.Fatalf("expected x-csrf-token header, got %q", gotCSRF)
	}
	if gotRequestedWith != "XMLHttpRequest" {
		t.Fatalf("expected X-Requested-With header, got %q", gotRequestedWith)
	}
	if gotEncoding != "gzip" {
		t.Fatalf("expected Accept-Encoding gzip, got %q", gotEncoding)
	}
}

func testJWT(expiresAt time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]int64{
		"exp": expiresAt.Unix(),
	})
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
