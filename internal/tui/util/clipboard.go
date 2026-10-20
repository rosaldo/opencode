package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ErrClipboardUnavailable is returned when no clipboard image tool is present.
var ErrClipboardUnavailable = errors.New("no clipboard image available (install wl-clipboard, xclip or pngpaste)")

// clipboardTimeout bounds each clipboard helper invocation.
const clipboardTimeout = 2 * time.Second

// ClipboardImage reads an image from the system clipboard (Spec 09). It returns
// the raw bytes, mime type and a suggested file name. Platform helpers are
// probed in order and the first that yields data wins; if none is available or
// the clipboard holds no image, ErrClipboardUnavailable is returned so callers
// can degrade gracefully.
func ClipboardImage() (content []byte, mimeType, fileName string, err error) {
	type cmd struct {
		name string
		args []string
		mime string
		ext  string
	}
	var candidates []cmd
	switch runtime.GOOS {
	case "darwin":
		candidates = []cmd{
			{name: "pngpaste", mime: "image/png", ext: "png"},
		}
	default: // linux and others
		candidates = []cmd{
			{name: "wl-paste", args: []string{"--no-newline", "--type", "image/png"}, mime: "image/png", ext: "png"},
			{name: "xclip", args: []string{"-selection", "clipboard", "-t", "image/png", "-o"}, mime: "image/png", ext: "png"},
		}
	}

	for _, c := range candidates {
		path, lookErr := exec.LookPath(c.name)
		if lookErr != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
		out, runErr := exec.CommandContext(ctx, path, c.args...).Output()
		cancel()
		if runErr != nil || len(out) == 0 {
			continue
		}
		name := fmt.Sprintf("pasted-%d.%s", time.Now().Unix(), c.ext)
		return out, c.mime, name, nil
	}
	return nil, "", "", ErrClipboardUnavailable
}

// ClipboardText reads plain text from the system clipboard. It returns
// ErrClipboardUnavailable when no helper is present.
func ClipboardText() (string, error) {
	type cmd struct {
		name string
		args []string
	}
	var candidates []cmd
	switch runtime.GOOS {
	case "darwin":
		candidates = []cmd{{name: "pbpaste"}}
	default:
		candidates = []cmd{
			{name: "wl-paste", args: []string{"--no-newline"}},
			{name: "xclip", args: []string{"-selection", "clipboard", "-o"}},
		}
	}
	for _, c := range candidates {
		path, lookErr := exec.LookPath(c.name)
		if lookErr != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
		out, runErr := exec.CommandContext(ctx, path, c.args...).Output()
		cancel()
		if runErr != nil {
			continue
		}
		return string(out), nil
	}
	return "", ErrClipboardUnavailable
}

// SniffImageMime returns the image mime type for the given bytes, or "" when it
// is not a recognised image. Currently PNG and JPEG magic numbers are handled.
func SniffImageMime(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(b, []byte("\xff\xd8\xff")):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return "image/gif"
	case len(b) > 12 && strings.HasPrefix(string(b[:4]), "RIFF") && string(b[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}
