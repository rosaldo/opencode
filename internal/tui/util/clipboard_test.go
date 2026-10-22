package util

import (
	"errors"
	"testing"
)

func TestSniffImageMime(t *testing.T) {
	cases := map[string][]byte{
		"image/png":  []byte("\x89PNG\r\n\x1a\n...."),
		"image/jpeg": []byte("\xff\xd8\xff\xe0...."),
		"image/gif":  []byte("GIF89a...."),
		"image/webp": []byte("RIFF\x00\x00\x00\x00WEBPVP8 "),
		"":           []byte("not an image"),
	}
	for want, b := range cases {
		if got := SniffImageMime(b); got != want {
			t.Errorf("SniffImageMime(%q) = %q, want %q", b, got, want)
		}
	}
}

func TestWriteClipboardWithMissingHelper(t *testing.T) {
	err := writeClipboardWith("text", []clipboardCmd{{name: "opencode-no-such-clipboard-tool"}})
	if !errors.Is(err, ErrClipboardUnavailable) {
		t.Errorf("err = %v, want ErrClipboardUnavailable", err)
	}
}

func TestWriteClipboardWithHelper(t *testing.T) {
	// cat stands in for a real clipboard helper: it consumes stdin and exits 0,
	// so the success path is exercised without touching a real clipboard.
	if err := writeClipboardWith("text", []clipboardCmd{{name: "cat"}}); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	err := writeClipboardWith("text", []clipboardCmd{
		{name: "opencode-no-such-clipboard-tool"},
		{name: "cat"},
	})
	if err != nil {
		t.Errorf("a working helper later in the list must win, got %v", err)
	}
}
