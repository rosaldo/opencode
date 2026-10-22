package util

import (
	"fmt"
	"image"
	"strings"

	"golang.org/x/image/draw"
)

// Assumed terminal cell size in pixels when converting cells to sixel pixels.
// The ratio (1:2) matches the half-block layout, so the image occupies the
// same cols x rows footprint on terminals with ~2:1 cells (Spec 09t).
const (
	sixelCellWidth  = 10
	sixelCellHeight = 20
)

// sixelLevel quantizes a color channel to one of six cube levels (0, 51, 102,
// 153, 204, 255) using round-to-nearest.
func sixelLevel(c uint8) uint8 {
	return uint8((uint16(c)*5 + 127) / 255)
}

// sixelPercent converts a cube-level channel value (0, 51, ..., 255) to the
// sixel palette percent (0, 20, 40, 60, 80, 100), which terminals map back to
// the exact same value (p*255/100).
func sixelPercent(level uint8) uint16 {
	return (uint16(level)*100 + 127) / 255
}

// RenderSixel encodes img for the sixel graphics protocol, sized to occupy
// cols cells wide by rows tall under the assumed cell size. Colors are
// quantized to a 6x6x6 cube (at most 216 palette entries), the image is split
// into bands of six pixels, and runs of four or more identical characters are
// run-length compressed. Returns "" for empty images so callers can fall back
// to half-blocks (Spec 09t).
func RenderSixel(img image.Image, cols, rows int) string {
	if img == nil || cols <= 0 || rows <= 0 {
		return ""
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return ""
	}
	w := cols * sixelCellWidth
	h := rows * sixelCellHeight
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)

	// Quantize every pixel into the 6x6x6 cube index and note used colors.
	pix := make([]uint8, w*h)
	var used [216]bool
	hasColor := false
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := dst.At(x, y).RGBA()
			if a == 0 {
				pix[y*w+x] = 255 // transparent marker, no palette entry
				continue
			}
			idx := uint8(sixelLevel(uint8(r>>8))*36 +
				sixelLevel(uint8(g>>8))*6 +
				sixelLevel(uint8(bl>>8)))
			pix[y*w+x] = idx
			if !used[idx] {
				used[idx] = true
				hasColor = true
			}
		}
	}
	if !hasColor {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\x1bPq")
	fmt.Fprintf(&sb, "\"1;1;%d;%d", w, h)
	for idx := uint8(0); idx < 216; idx++ {
		if !used[idx] {
			continue
		}
		cr := uint8(idx/36) * 51
		cg := uint8((idx/6)%6) * 51
		cb := uint8(idx%6) * 51
		fmt.Fprintf(&sb, "#%d;%d;%d;%d", idx, sixelPercent(cr), sixelPercent(cg), sixelPercent(cb))
	}

	flush := func(run *int, ch *byte) {
		if *run == 0 {
			return
		}
		if *run >= 4 {
			fmt.Fprintf(&sb, "!%d%c", *run, *ch)
		} else {
			for i := 0; i < *run; i++ {
				sb.WriteByte(*ch)
			}
		}
		*run = 0
	}

	for y := 0; y < h; y += 6 {
		if y > 0 {
			sb.WriteByte('-')
		}
		firstInBand := true
		for idx := uint8(0); idx < 216; idx++ {
			if !used[idx] {
				continue
			}
			if firstInBand {
				firstInBand = false
			} else {
				sb.WriteByte('$')
			}
			fmt.Fprintf(&sb, "#%d", idx)
			var run int
			var runCh byte
			for x := 0; x < w; x++ {
				var bits byte
				for k := 0; k < 6; k++ {
					if y+k < h && pix[(y+k)*w+x] == idx {
						bits |= 1 << k
					}
				}
				ch := '?' + bits
				if run > 0 && ch != runCh {
					flush(&run, &runCh)
				}
				runCh = ch
				run++
			}
			flush(&run, &runCh)
		}
	}
	sb.WriteString("\x1b\\")
	return sb.String()
}
