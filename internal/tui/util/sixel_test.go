package util

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"testing"
)

func solidImage(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestRenderSixelGoldenSolidRed(t *testing.T) {
	// cols=1, rows=1 scales to a 10x20 pixel canvas (four sixel bands).
	img := solidImage(10, 20, color.RGBA{R: 255, A: 255})
	out := RenderSixel(img, 1, 1)

	// 255 quantizes to level 5 (255) -> cube index 180, palette percent 100.
	want := "\x1bPq\"1;1;10;20" +
		"#180;100;0;0" +
		"#180!10~" + "-#180!10~" + "-#180!10~" + "-#180!10B" +
		"\x1b\\"
	if out != want {
		t.Errorf("RenderSixel() = %q, want %q", out, want)
	}
}

func TestRenderSixelEmptyInputs(t *testing.T) {
	if got := RenderSixel(nil, 4, 4); got != "" {
		t.Errorf("nil image = %q, want empty", got)
	}
	img := solidImage(10, 20, color.RGBA{R: 255, A: 255})
	if got := RenderSixel(img, 0, 4); got != "" {
		t.Errorf("cols=0 = %q, want empty", got)
	}
	transparent := solidImage(10, 20, color.RGBA{})
	if got := RenderSixel(transparent, 1, 1); got != "" {
		t.Errorf("fully transparent = %q, want empty", got)
	}
}

func TestRenderSixelStripesNoRunLength(t *testing.T) {
	// Alternating 1-pixel stripes produce runs of one, never RLE-compressed.
	img := image.NewRGBA(image.Rect(0, 0, 10, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 10; x++ {
			if x%2 == 0 {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				img.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}
	out := RenderSixel(img, 1, 1)
	if !strings.Contains(out, "~?~?~?~?~?") {
		t.Errorf("want red stripe strip in %q", out)
	}
	if !strings.Contains(out, "?~?~?~?~?~") {
		t.Errorf("want blue stripe strip in %q", out)
	}
	if strings.Contains(out, "!") {
		t.Errorf("single-pixel runs must not be RLE-compressed: %q", out)
	}
}

// decodeSixel parses a RenderSixel payload back into per-pixel cube indices
// (-1 = transparent) plus the declared palette percents. It validates that
// every color strip spans exactly the full width and that bands stay inside
// the declared raster height.
func decodeSixel(t *testing.T, out string) (pix []int, palette map[int][3]uint16, pixW, pixH int) {
	t.Helper()
	payload, ok := strings.CutPrefix(out, "\x1bP")
	if !ok {
		t.Fatalf("missing DCS intro: %q", out)
	}
	payload, ok = strings.CutSuffix(payload, "\x1b\\")
	if !ok {
		t.Fatalf("missing ST: %q", out)
	}
	payload, ok = strings.CutPrefix(payload, "q")
	if !ok {
		t.Fatalf("missing sixel command: %q", out)
	}

	palette = make(map[int][3]uint16)
	i := 0
	nextInt := func() int {
		start := i
		for i < len(payload) && payload[i] >= '0' && payload[i] <= '9' {
			i++
		}
		if start == i {
			t.Fatalf("expected digits at %d in %q", i, out)
		}
		n, err := strconv.Atoi(payload[start:i])
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	cur := -2
	x, y := 0, 0
	for i < len(payload) {
		c := payload[i]
		switch {
		case c == '"': // raster attributes "Pan;Pad;Ph;Pv
			i++
			vals := [4]int{}
			for k := 0; k < 4; k++ {
				if k > 0 {
					if payload[i] != ';' {
						t.Fatalf("bad raster attrs at %d: %q", i, out)
					}
					i++
				}
				vals[k] = nextInt()
			}
			pixW, pixH = vals[2], vals[3]
			if pixW <= 0 || pixH <= 0 {
				t.Fatalf("bad raster size %dx%d", pixW, pixH)
			}
			pix = make([]int, pixW*pixH)
			for k := range pix {
				pix[k] = -1
			}
		case c == '#':
			i++
			idx := nextInt()
			if i < len(payload) && payload[i] == ';' {
				rgb := [3]uint16{}
				for k := 0; k < 3; k++ {
					if payload[i] != ';' {
						t.Fatalf("bad palette def at %d: %q", i, out)
					}
					i++
					rgb[k] = uint16(nextInt())
				}
				palette[idx] = rgb
			} else {
				cur = idx
			}
		case c == '!':
			i++
			n := nextInt()
			if i >= len(payload) {
				t.Fatalf("RLE missing char: %q", out)
			}
			decodeSixelRun(t, out, pix, pixW, pixH, &x, y, payload[i], n, cur)
			i++
		case c == '$':
			x = 0
			i++
		case c == '-':
			y += 6
			x = 0
			i++
		case c >= '?' && c <= '~':
			decodeSixelRun(t, out, pix, pixW, pixH, &x, y, c, 1, cur)
			i++
		default:
			t.Fatalf("unexpected byte %q at %d: %q", c, i, out)
		}
		if pixW > 0 && x > pixW {
			t.Fatalf("column %d overflows width %d: %q", x, pixW, out)
		}
		if y >= pixH && c != '-' {
			t.Fatalf("band %d outside height %d: %q", y, pixH, out)
		}
	}
	if pixW == 0 {
		t.Fatalf("no raster attributes: %q", out)
	}
	if x != pixW {
		t.Fatalf("last strip ends at column %d, want %d", x, pixW)
	}
	return pix, palette, pixW, pixH
}

func decodeSixelRun(t *testing.T, out string, pix []int, pixW, pixH int, x *int, y int, ch byte, n, color int) {
	t.Helper()
	if color < 0 {
		t.Fatalf("data before color select: %q", out)
	}
	bits := int(ch - '?')
	for i := 0; i < n; i++ {
		for k := 0; k < 6; k++ {
			if bits&(1<<k) == 0 {
				continue
			}
			yy := y + k
			if yy >= pixH || *x >= pixW {
				t.Fatalf("pixel (%d,%d) outside %dx%d: %q", *x, yy, pixW, pixH, out)
			}
			if prev := pix[yy*pixW+*x]; prev != -1 && prev != color {
				t.Fatalf("pixel (%d,%d) painted by %d and %d", *x, yy, prev, color)
			}
			pix[yy*pixW+*x] = color
		}
		*x++
	}
}

func TestRenderSixelDecodeRoundTrip(t *testing.T) {
	const w, h = 20, 40 // cols=2, rows=2
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x*37 + y*11),
				G: uint8(x*7 + y*53),
				B: uint8(x*91 + y*3),
				A: 255,
			})
		}
	}
	img.Set(0, 0, color.RGBA{A: 0}) // one transparent pixel

	out := RenderSixel(img, 2, 2)
	pix, palette, pixW, pixH := decodeSixel(t, out)
	if pixW != w || pixH != h {
		t.Fatalf("raster = %dx%d, want %dx%d", pixW, pixH, w, h)
	}

	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			want := -1
			r, g, b, a := img.At(xx, yy).RGBA()
			if a != 0 {
				want = int(sixelLevel(uint8(r>>8))*36 +
					sixelLevel(uint8(g>>8))*6 +
					sixelLevel(uint8(b>>8)))
			}
			if got := pix[yy*w+xx]; got != want {
				t.Fatalf("pixel(%d,%d) = %d, want %d", xx, yy, got, want)
			}
		}
	}

	// Every used cube color must be declared with percents that map back to
	// the exact cube level (p*255/100).
	used := map[int]bool{}
	for _, p := range pix {
		if p >= 0 {
			used[p] = true
		}
	}
	for idx := range used {
		rgb, ok := palette[idx]
		if !ok {
			t.Errorf("palette missing #%d", idx)
			continue
		}
		for k, p := range rgb {
			level := uint8(idx/36) * 51
			if k == 1 {
				level = uint8((idx/6)%6) * 51
			}
			if k == 2 {
				level = uint8(idx%6) * 51
			}
			if p != sixelPercent(level) {
				t.Errorf("palette #%d component %d = %d, want %d", idx, k, p, sixelPercent(level))
			}
		}
	}
}

func TestRenderSixelBandCount(t *testing.T) {
	img := solidImage(20, 40, color.RGBA{R: 255, G: 255, A: 255})
	out := RenderSixel(img, 2, 2)
	_, _, _, pixH := decodeSixel(t, out)
	bands := (pixH + 5) / 6
	if got := strings.Count(out, "-"); got != bands-1 {
		t.Errorf("band separators = %d, want %d", got, bands-1)
	}
}
