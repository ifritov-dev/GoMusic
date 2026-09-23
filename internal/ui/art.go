package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
)

// Cover art is rendered as truecolor half-blocks: one terminal cell
// shows two vertically stacked pixels ("▀" with fg=top, bg=bottom).
// Only the standard library is used (no image scaling dependency):
// each destination pixel averages its whole source rect (box filter),
// so big covers downscale smoothly instead of nearest-neighbor
// shimmering.

// renderCover scales img to cellW×cellH terminal cells and returns the
// rendered string (cellH lines, no trailing newline). Sources that
// don't match the box aspect (e.g. 16:9 video thumbnails) are
// center-cropped instead of squeezed. A nil img or non-positive size
// yields a placeholder.
func renderCover(img image.Image, cellW, cellH int) string {
	if img == nil || cellW <= 0 || cellH <= 0 {
		return placeholderArt(cellW, cellH)
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return placeholderArt(cellW, cellH)
	}
	dw, dh := cellW, cellH*2
	// Center-crop to the box aspect first: a 16:9 thumbnail squeezed
	// into the square box would look squashed (art-track covers carry
	// blurred filler on the sides, the clean square sits in the middle).
	cw, ch, cx0, cy0 := cropRect(sw, sh, dw, dh)
	// Covers are small enough (a few MP) that direct At() calls are
	// fine, but RGBA() per pixel on NRGBA is cheap — still, preconvert
	// the cropped region once so area averaging doesn't re-decode
	// YCbCr per tap.
	pix := make([][3]uint32, cw*ch)
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			cr, cg, cb, _ := img.At(b.Min.X+cx0+x, b.Min.Y+cy0+y).RGBA()
			pix[y*cw+x] = [3]uint32{cr, cg, cb}
		}
	}
	var sb strings.Builder
	for y := 0; y < cellH; y++ {
		for x := 0; x < cellW; x++ {
			r1, g1, b1 := areaAvg(pix, cw, ch, x, y*2, dw, dh)
			r2, g2, b2 := areaAvg(pix, cw, ch, x, y*2+1, dw, dh)
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

// areaAvg averages all source pixels covered by destination pixel
// (x, y) of a dw×dh grid. Upscaling (rect empty) falls back to the
// nearest source pixel.
func areaAvg(pix [][3]uint32, sw, sh, x, y, dw, dh int) (r, g, b int) {
	x0 := x * sw / dw
	x1 := (x + 1) * sw / dw
	y0 := y * sh / dh
	y1 := (y + 1) * sh / dh
	if x1 <= x0 {
		x1 = x0 + 1
		if x1 > sw {
			x1, x0 = sw, sw-1
		}
	}
	if y1 <= y0 {
		y1 = y0 + 1
		if y1 > sh {
			y1, y0 = sh, sh-1
		}
	}
	var sr, sg, sb uint64
	var n uint64
	for sy := y0; sy < y1; sy++ {
		for sx := x0; sx < x1; sx++ {
			p := pix[sy*sw+sx]
			sr += uint64(p[0])
			sg += uint64(p[1])
			sb += uint64(p[2])
			n++
		}
	}
	if n == 0 {
		return 0, 0, 0
	}
	// pix holds 16-bit RGBA values; >>8 converts to 8-bit.
	return int((sr / n) >> 8), int((sg / n) >> 8), int((sb / n) >> 8)
}

// cropRect returns the largest centered source rect with the target
// aspect (dw×dh). Integer math can only shrink, so the result is
// clamped to [1,sw]×[1,sh]; equal aspects return the full frame.
func cropRect(sw, sh, dw, dh int) (cw, ch, cx0, cy0 int) {
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return sw, sh, 0, 0
	}
	if sw*dh > dw*sh {
		ch = sh
		cw = sh * dw / dh
	} else {
		cw = sw
		ch = sw * dh / dw
	}
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}
	if cw > sw {
		cw = sw
	}
	if ch > sh {
		ch = sh
	}
	return cw, ch, (sw - cw) / 2, (sh - ch) / 2
}

// paletteOf picks up to 3 dominant colors from a cover for the
// visualizer. Pixels are quantized into a coarse RGB histogram and
// near-gray / near-black ones are dropped (they would make gray
// bars); the winners come back ordered dark to light so the bar
// gradient runs left to right. nil means "no usable color".
func paletteOf(img image.Image) []color.RGBA {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil
	}
	step := max(max(sw, sh)/64, 1) // ~64x64 samples, enough for a palette
	type bucket struct{ n, r, g, bl int }
	hist := map[int]*bucket{}
	for y := 0; y < sh; y += step {
		for x := 0; x < sw; x += step {
			r, g, bl := rgb(img.At(b.Min.X+x, b.Min.Y+y))
			mx, mn := max(r, max(g, bl)), min(r, min(g, bl))
			if mx < 48 || mx-mn < 36 {
				continue // black or gray
			}
			k := (r>>4)<<8 | (g>>4)<<4 | (bl >> 4)
			bk := hist[k]
			if bk == nil {
				bk = &bucket{}
				hist[k] = bk
			}
			bk.n++
			bk.r += r
			bk.g += g
			bk.bl += bl
		}
	}
	type cand struct {
		c color.RGBA
		n int
	}
	cands := make([]cand, 0, len(hist))
	for _, bk := range hist {
		cands = append(cands, cand{color.RGBA{uint8(bk.r / bk.n), uint8(bk.g / bk.n), uint8(bk.bl / bk.n), 255}, bk.n})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].n > cands[j].n })
	var out []color.RGBA
	for _, cd := range cands {
		if len(out) == 3 {
			break
		}
		distinct := true
		for _, o := range out {
			if diff(o, cd.c) < 56 {
				distinct = false
				break
			}
		}
		if distinct {
			out = append(out, cd.c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return luma(out[i]) < luma(out[j]) })
	if len(out) == 0 {
		// grayscale fallback: use darkest and lightest sampled pixels
		var minL, maxL int = 255, 0
		var minC, maxC color.RGBA
		for y := 0; y < sh; y += step {
			for x := 0; x < sw; x += step {
				r, g, bl := rgb(img.At(b.Min.X+x, b.Min.Y+y))
				l := (299*r + 587*g + 114*bl) / 1000
				if l < minL {
					minL = l
					minC = color.RGBA{uint8(r), uint8(g), uint8(bl), 255}
				}
				if l > maxL {
					maxL = l
					maxC = color.RGBA{uint8(r), uint8(g), uint8(bl), 255}
				}
			}
		}
		if maxL-minL > 48 {
			out = []color.RGBA{minC, maxC}
		}
	}
	return out
}

// diff is the largest per-channel difference between two colors.
func diff(a, b color.RGBA) int {
	d := max(int(a.R)-int(b.R), int(b.R)-int(a.R))
	d = max(d, max(int(a.G)-int(b.G), int(b.G)-int(a.G)))
	return max(d, max(int(a.B)-int(b.B), int(b.B)-int(a.B)))
}

func luma(c color.RGBA) int { return (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000 }

// defaultPalette keeps the bars readable when a track has no cover.
var defaultPalette = []color.RGBA{{0x7C, 0x3A, 0xED, 255}, {0x22, 0xD3, 0xEE, 255}}

// gradient returns n colors interpolated across the palette stops
// (dark -> light), falling back to the theme pair for an empty
// palette.
func gradient(pal []color.RGBA, n int) []color.RGBA {
	if n <= 0 {
		return nil
	}
	if len(pal) == 0 {
		pal = defaultPalette
	}
	out := make([]color.RGBA, n)
	if len(pal) == 1 {
		for i := range out {
			out[i] = pal[0]
		}
		return out
	}
	for i := range out {
		t := float64(i) / math.Max(float64(n-1), 1) * float64(len(pal)-1)
		j := min(int(t), len(pal)-2)
		out[i] = lerpRGBA(pal[j], pal[j+1], t-float64(j))
	}
	return out
}

func lerpRGBA(a, b color.RGBA, f float64) color.RGBA {
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f + 0.5) }
	return color.RGBA{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), 255}
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
