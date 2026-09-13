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
