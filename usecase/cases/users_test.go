package cases

import "testing"

func TestDetectAssetTypeMotionGraphics(t *testing.T) {
	usecase := &UsecaseUsers{}

	got := usecase.DetectAssetType("https://www.freepik.com/free-motion-graphics/abstract-blue-background_4914295.htm")
	if got != "video" {
		t.Fatalf("expected video, got %q", got)
	}
}

func TestIsSupportedAssetTypeVideo(t *testing.T) {
	usecase := &UsecaseUsers{}

	supported, typeName := usecase.IsSupportedAssetType("video")
	if !supported {
		t.Fatalf("expected video to be supported")
	}
	if typeName != "video" {
		t.Fatalf("expected type name video, got %q", typeName)
	}
}
