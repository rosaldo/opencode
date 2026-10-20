package util

import "testing"

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
