package server

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateImageThumbnailStdlib(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "source.jpg")
	target := filepath.Join(dir, "thumb.jpg")

	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.RGBA{R: 20, G: 120, B: 90, A: 255})
		}
	}

	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if err := generateImageThumbnailStdlib(source, 128, target); err != nil {
		t.Fatal(err)
	}

	generated, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer generated.Close()

	cfg, _, err := image.DecodeConfig(generated)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 128 || cfg.Height != 64 {
		t.Fatalf("thumbnail size = %dx%d, want 128x64", cfg.Width, cfg.Height)
	}
}
