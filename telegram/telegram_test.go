package telegram

import (
	"net/http"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestCacheFilenamePrefersQueryFilename(t *testing.T) {
	got := cacheFilename(nil, "https://cdn-icons.flaticon.com/svg/785/785116.svg?token=abc&filename=fire_785116.svg&fd=1", "icon")
	if got != "fire_785116.svg" {
		t.Fatalf("expected query filename, got %q", got)
	}
}

func TestCacheFilenameFallsBackToAssetExtension(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	got := cacheFilename(resp, "https://example.com/download/noext", "icon")
	if got != "noext.svg" {
		t.Fatalf("expected icon extension fallback, got %q", got)
	}
}

func TestBuildAssetCacheKeyNormalizesIconURLs(t *testing.T) {
	withFragment := "https://www.freepik.com/icon/fire_785116#fromView=search&page=1"
	plain := "https://www.freepik.com/icon/fire_785116"

	gotA := buildAssetCacheKey(withFragment, "icon")
	gotB := buildAssetCacheKey(plain, "icon")

	if gotA != "icon:785116" {
		t.Fatalf("expected icon resource cache key, got %q", gotA)
	}
	if gotA != gotB {
		t.Fatalf("expected identical keys after normalization, got %q and %q", gotA, gotB)
	}
}

func TestBuildAssetCacheKeyWithVariantSeparates3DFormats(t *testing.T) {
	raw := "https://www.freepik.com/3d-model/tube-box-with-label_23192.htm#fromView=keyword"
	blend := buildAssetCacheKeyWithVariant(raw, "3d", "blend")
	fbx := buildAssetCacheKeyWithVariant(raw, "3d", "fbx")

	if blend != "3d:23192:blend" {
		t.Fatalf("expected blend key, got %q", blend)
	}
	if fbx != "3d:23192:fbx" {
		t.Fatalf("expected fbx key, got %q", fbx)
	}
	if blend == fbx {
		t.Fatal("expected different cache keys for different 3d formats")
	}
}

func TestAssetDownloadAcceptHeaderForIcons(t *testing.T) {
	got := assetDownloadAcceptHeader("icon", "https://cdn-icons.flaticon.com/svg/785/785116.svg")
	if got == "*/*" {
		t.Fatalf("expected specialized icon accept header, got %q", got)
	}
}

func TestIsCacheUploadTooLarge(t *testing.T) {
	if !isCacheUploadTooLarge(100, 50) {
		t.Fatal("expected oversized file to be rejected")
	}
	if isCacheUploadTooLarge(50, 50) {
		t.Fatal("expected equal size to pass")
	}
	if isCacheUploadTooLarge(10, 0) {
		t.Fatal("expected disabled limit to pass")
	}
}

func TestNewCacheDocumentConfigDisablesContentTypeDetectionForVideo(t *testing.T) {
	msg := newCacheDocumentConfig(123, tgbotapi.FilePath("/tmp/test.mp4"), shouldDisableContentTypeDetection("video"))
	if !msg.DisableContentTypeDetection {
		t.Fatal("expected DisableContentTypeDetection to be enabled for video cache uploads")
	}
	if !msg.DisableNotification {
		t.Fatal("expected DisableNotification to stay enabled for cache uploads")
	}
}

func TestShouldDisableContentTypeDetection(t *testing.T) {
	if !shouldDisableContentTypeDetection("video") {
		t.Fatal("expected video uploads to disable content type detection")
	}
	if !shouldDisableContentTypeDetection("icon") {
		t.Fatal("expected icon uploads to disable content type detection")
	}
	if shouldDisableContentTypeDetection("regular") {
		t.Fatal("expected regular uploads to keep default content type detection")
	}
}

func TestShouldRetryCacheCopy(t *testing.T) {
	if !shouldRetryCacheCopy(&tgbotapi.Error{Code: http.StatusTooManyRequests, ResponseParameters: tgbotapi.ResponseParameters{RetryAfter: 3}}) {
		t.Fatal("expected retry_after telegram errors to be retried")
	}
	if !shouldRetryCacheCopy(&tgbotapi.Error{Code: http.StatusBadGateway}) {
		t.Fatal("expected 5xx telegram errors to be retried")
	}
	if shouldRetryCacheCopy(&tgbotapi.Error{Code: http.StatusBadRequest}) {
		t.Fatal("expected 4xx telegram errors without retry_after to fail fast")
	}
	if !shouldRetryCacheCopy(http.ErrHandlerTimeout) {
		t.Fatal("expected timeout-like errors to be retried")
	}
}

func TestCacheCopyRetryDelayPrefersRetryAfter(t *testing.T) {
	delay := cacheCopyRetryDelay(&tgbotapi.Error{Code: http.StatusTooManyRequests, ResponseParameters: tgbotapi.ResponseParameters{RetryAfter: 7}}, 1)
	if delay != 7*time.Second {
		t.Fatalf("expected retry_after delay, got %s", delay)
	}
}
