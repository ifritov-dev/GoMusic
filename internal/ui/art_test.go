package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func gradientImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 128, A: 255})
		}
	}
	return img
}

func TestRenderCoverShape(t *testing.T) {
	out := renderCover(gradientImage(64, 64), 20, 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 10 {
		t.Fatalf("got %d lines, want 10", len(lines))
	}
	if got := strings.Count(out, "▀"); got != 200 {
		t.Fatalf("got %d cells, want 200", got)
	}
	if !strings.Contains(out, "\x1b[38;2;") || !strings.Contains(out, "\x1b[48;2;") {
		t.Fatal("missing truecolor escapes")
	}
}

func TestRenderCoverNilGivesPlaceholder(t *testing.T) {
	out := renderCover(nil, 20, 10)
	if !strings.Contains(out, "♪") {
		t.Fatal("placeholder should contain a note glyph")
	}
	if len(strings.Split(out, "\n")) != 10 {
		t.Fatal("placeholder should keep box height")
	}
	if renderCover(gradientImage(8, 8), 0, 0) != "" {
		t.Fatal("zero size should render empty")
	}
}

// Wide art-track-style thumbnails (square art + filler sides) must be
// center-cropped, not squeezed, into the square box.
func TestRenderCoverCropsWideToSquare(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 96, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 96; x++ {
			c := color.NRGBA{B: 255, A: 255}
			if x >= 24 && x < 72 {
				c = color.NRGBA{R: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	out := renderCover(img, 12, 6) // 12×12 dest pixels: square box
	// Every sampled pixel must come from the red center: no blue filler.
	if strings.Contains(out, ";0;0;255m") {
		t.Fatalf("wide art must be center-cropped, got blue filler: %.120s…", out)
	}
	if !strings.Contains(out, ";255;0;0m") {
		t.Fatal("expected red center in output")
	}
}
