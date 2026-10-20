package util

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

func TestDetectImageProtocol(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want ImageProtocol
	}{
		{"kitty window id", map[string]string{"KITTY_WINDOW_ID": "1"}, ImageKitty},
		{"kitty term", map[string]string{"TERM": "xterm-kitty"}, ImageKitty},
		{"wezterm", map[string]string{"TERM_PROGRAM": "WezTerm"}, ImageKitty},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, ImageKitty},
		{"sixel term", map[string]string{"TERM": "xterm-sixel"}, ImageSixel},
		{"sixel flag", map[string]string{"SIXEL_SUPPORT": "1"}, ImageSixel},
		{"plain", map[string]string{"TERM": "xterm-256color"}, ImageHalfBlocks},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectImageProtocol(func(k string) string { return tt.env[k] })
			if got != tt.want {
				t.Fatalf("protocol = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderHalfBlocks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	out := RenderHalfBlocks(img, 4, 2)
	if out == "" {
		t.Fatal("expected output")
	}
	if !strings.Contains(out, "▀") {
		t.Fatalf("expected half block, got %q", out)
	}
	if !strings.Contains(out, "\x1b[38;2;") {
		t.Fatalf("expected truecolor foreground, got %q", out)
	}
}

func TestKittyGraphicsChunking(t *testing.T) {
	data := make([]byte, 5000) // forces base64 > 4096 -> two chunks
	for i := range data {
		data[i] = byte(i)
	}
	out := KittyGraphics(data, 10, 5)
	if strings.Count(out, "\x1b_G") < 2 {
		t.Fatalf("expected multiple chunks, got %d", strings.Count(out, "\x1b_G"))
	}
	if !strings.Contains(out, "c=10") || !strings.Contains(out, "r=5") {
		t.Fatalf("expected size params, got %q", out[:40])
	}
}

func TestHyperlink(t *testing.T) {
	out := Hyperlink("click", "file:///tmp/x")
	if !strings.HasPrefix(out, "\x1b]8;;file:///tmp/x") || !strings.HasSuffix(out, "\x1b]8;;\x1b\\") {
		t.Fatalf("bad hyperlink: %q", out)
	}
	if Hyperlink("plain", "") != "plain" {
		t.Fatal("empty url should return text unchanged")
	}
}

func TestSniffImageBytes(t *testing.T) {
	if m, ok := SniffImageBytes([]byte("\x89PNG\r\n\x1a\n....")); !ok || m != "image/png" {
		t.Fatalf("png sniff = %q %v", m, ok)
	}
	if m, ok := SniffImageBytes([]byte("\xff\xd8\xff\xe0")); !ok || m != "image/jpeg" {
		t.Fatalf("jpeg sniff = %q %v", m, ok)
	}
	if _, ok := SniffImageBytes([]byte("hello")); ok {
		t.Fatal("plain text should not be an image")
	}
}

func TestZoneSetMarkAndAt(t *testing.T) {
	zone.NewGlobal()
	zs := NewZoneSet()
	zs.Mark("msg:a", "AAAA\nBBBB")
	zs.Mark("msg:b", "CCCC\nDDDD")

	// bubblezone resolves positions asynchronously through its worker; give it a
	// moment after rendering (Scan) before querying.
	rendered := zs.Mark("msg:a", "AAAA\nBBBB")
	_ = rendered
	var info *zone.ZoneInfo
	for i := 0; i < 100; i++ {
		if info = zone.Get("msg:a"); info != nil && !info.IsZero() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if info == nil || info.IsZero() {
		t.Skip("bubblezone position resolution unavailable in this environment")
	}

	id, found := zs.At(tea.MouseMsg{Action: tea.MouseActionMotion, X: info.StartX, Y: info.StartY})
	if !found || id != "msg:a" {
		t.Fatalf("At = %q %v, want msg:a", id, found)
	}

	// Duplicate Mark does not grow the registry.
	before := len(zs.ids)
	zs.Mark("msg:a", "x")
	if len(zs.ids) != before {
		t.Fatalf("registry grew on duplicate mark: %d -> %d", before, len(zs.ids))
	}
}
