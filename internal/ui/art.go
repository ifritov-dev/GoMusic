package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"
)

// Cover art is rendered as truecolor half-blocks: one terminal cell
// shows two vertically stacked pixels ("▀" with fg=top, bg=bottom).
// Only the standard library is used (no image scaling dependency):
// nearest-neighbor sampling is plenty at these tiny sizes.

// renderCover scales img to cellW×cellH terminal cells and returns the
// rendered string (cellH lines, no trailing newline). A nil img or
// non-positive size yields a placeholder.
func renderCover(img image.Image, cellW, cellH int) string {
	if img == nil || cellW <= 0 || cellH <= 0 {
		return placeholderArt(cellW, cellH)
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return placeholderArt(cellW, cellH)
	}
	var sb strings.Builder
	for y := 0; y < cellH; y++ {
		for x := 0; x < cellW; x++ {
			top := sample(img, b, sw, sh, x, y*2, cellW, cellH*2)
			bot := sample(img, b, sw, sh, x, y*2+1, cellW, cellH*2)
			r1, g1, b1 := rgb(top)
			r2, g2, b2 := rgb(bot)
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
				r1, g1, b1, r2, g2, b2)
		}
		sb.WriteString("\x1b[0m")
		if y < cellH-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func sample(img image.Image, b image.Rectangle, sw, sh, x, y, dw, dh int) color.Color {
	sx := b.Min.X + x*sw/dw
	sy := b.Min.Y + y*sh/dh
	return img.At(sx, sy)
}

func rgb(c color.Color) (r, g, bl int) {
	cr, cg, cb, _ := c.RGBA()
	return int(cr >> 8), int(cg >> 8), int(cb >> 8)
}

// placeholderArt is a dim note glyph shown when a track has no cover.
func placeholderArt(cellW, cellH int) string {
	if cellW <= 0 || cellH <= 0 {
		return ""
	}
	mid := cellH / 2
	var sb strings.Builder
	for y := 0; y < cellH; y++ {
		if y == mid {
			pad := (cellW - 1) / 2
			sb.WriteString(dimStyle.Render(strings.Repeat(" ", pad) + "♪"))
			// Pad the rest so the box keeps its width.
			rest := cellW - pad - 1
			if rest > 0 {
				sb.WriteString(strings.Repeat(" ", rest))
			}
		} else {
			sb.WriteString(strings.Repeat(" ", cellW))
		}
		if y < cellH-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
