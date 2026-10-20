package util

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"strings"

	"golang.org/x/image/draw"
)

// ImageProtocol identifies how images should be drawn in the terminal (Spec 09).
type ImageProtocol int

const (
	// ImageNone means images cannot be drawn; callers show a placeholder.
	ImageNone ImageProtocol = iota
	// ImageHalfBlocks renders images with Unicode block characters in truecolor.
	ImageHalfBlocks
	// ImageKitty uses the Kitty graphics protocol.
	ImageKitty
	// ImageSixel uses the Sixel graphics protocol.
	ImageSixel
)

// DetectImageProtocol picks the best protocol for the terminal described by the
// environment lookup function. Kitty wins (most capable), then Sixel, then the
// universal half-block fallback.
func DetectImageProtocol(getenv func(string) string) ImageProtocol {
	if getenv == nil {
		return ImageHalfBlocks
	}
	term := getenv("TERM")
	termProgram := getenv("TERM_PROGRAM")
	if getenv("KITTY_WINDOW_ID") != "" || strings.Contains(term, "kitty") ||
		strings.EqualFold(termProgram, "WezTerm") || strings.EqualFold(termProgram, "ghostty") {
		return ImageKitty
	}
	if strings.Contains(term, "sixel") || getenv("SIXEL_SUPPORT") == "1" {
		return ImageSixel
	}
	// Truecolor terminals can always render half-blocks.
	return ImageHalfBlocks
}

// RenderHalfBlocks draws img using the Unicode "upper half block" (▀) with the
// top pixel as foreground and the bottom as background, at most cols wide and
// rows tall. Two source rows collapse into one terminal row.
func RenderHalfBlocks(img image.Image, cols, rows int) string {
	if cols <= 0 || rows <= 0 || img == nil {
		return ""
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return ""
	}
	// Keep the aspect ratio roughly square: terminal cells are ~2x taller than
	// wide, and each cell carries two vertical pixels.
	pixelW := cols
	pixelH := rows * 2
	dst := image.NewRGBA(image.Rect(0, 0, pixelW, pixelH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)

	var sb strings.Builder
	for y := 0; y < pixelH; y += 2 {
		for x := 0; x < pixelW; x++ {
			tr, tg, tb, _ := dst.At(x, y).RGBA()
			br, bg, bb, _ := dst.At(x, y+1).RGBA()
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
				tr>>8, tg>>8, tb>>8, br>>8, bg>>8, bb>>8)
		}
		sb.WriteString("\x1b[0m")
		if y+2 < pixelH {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// KittyGraphics encodes image bytes for the Kitty graphics protocol, drawing the
// image in cells wide by rows tall. The payload is base64 and chunked because a
// single escape sequence is size-limited.
func KittyGraphics(data []byte, cols, rows int) string {
	if len(data) == 0 || cols <= 0 || rows <= 0 {
		return ""
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	const chunk = 4096
	var sb strings.Builder
	first := true
	for len(b64) > 0 {
		n := chunk
		if n > len(b64) {
			n = len(b64)
		}
		part := b64[:n]
		b64 = b64[n:]
		more := 1
		if len(b64) == 0 {
			more = 0
		}
		if first {
			fmt.Fprintf(&sb, "\x1b_Ga=T,f=100,c=%d,r=%d,m=%d;%s\x1b\\", cols, rows, more, part)
			first = false
		} else {
			fmt.Fprintf(&sb, "\x1b_Gm=%d;%s\x1b\\", more, part)
		}
	}
	return sb.String()
}

// Hyperlink wraps text in an OSC 8 hyperlink escape sequence pointing at url.
func Hyperlink(text, url string) string {
	if url == "" {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// SniffImageBytes reports the MIME type when data starts with a known image
// magic number, letting callers decide whether to render inline.
func SniffImageBytes(data []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", true
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return "image/jpeg", true
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp", true
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		return "image/gif", true
	}
	return "", false
}
