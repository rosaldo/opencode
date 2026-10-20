package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSchemaFileMatchesGenerator fails when the committed opencode-schema.json
// drifts from what generateSchema produces. Regenerate with:
//
//	go run ./cmd/schema > opencode-schema.json
func TestSchemaFileMatchesGenerator(t *testing.T) {
	schema := generateSchema()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(schema); err != nil {
		t.Fatalf("encode: %v", err)
	}

	path := filepath.Join("..", "..", "opencode-schema.json")
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if !bytes.Equal(buf.Bytes(), onDisk) {
		t.Fatalf("opencode-schema.json is out of date; run: go run ./cmd/schema > opencode-schema.json")
	}
}
