package cases

import "testing"

func TestDetectAssetTypeMotionGraphics(t *testing.T) {
	usecase := &UsecaseUsers{}

	got := usecase.DetectAssetType("https://www.freepik.com/free-motion-graphics/abstract-blue-background_4914295.htm")
	if got != "video" {
		t.Fatalf("expected video, got %q", got)
	}
}

func TestDetectAssetTypeMagnificAIImage(t *testing.T) {
	usecase := &UsecaseUsers{}

	got := usecase.DetectAssetType("https://www.magnific.com/premium-ai-image/example_123.htm")
	if got != "image" {
		t.Fatalf("expected image, got %q", got)
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

func TestIsSupportedAssetTypeThreeD(t *testing.T) {
	usecase := &UsecaseUsers{}

	supported, typeName := usecase.IsSupportedAssetType("3d")
	if !supported {
		t.Fatalf("expected 3d to be supported")
	}
	if typeName != "3d" {
		t.Fatalf("expected type name 3d, got %q", typeName)
	}
}
