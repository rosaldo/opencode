package message

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/llm/models"
)

func TestMarshallUnmarshallRoundTrip(t *testing.T) {
	parts := []ContentPart{
		ReasoningContent{Thinking: "let me think"},
		TextContent{Text: "hello"},
		ImageURLContent{URL: "https://example.com/a.png", Detail: "high"},
		BinaryContent{MIMEType: "image/png", Data: []byte{1, 2, 3}},
		ToolCall{ID: "call-1", Name: "bash", Input: `{"command":"ls"}`, Type: "function"},
		ToolResult{ToolCallID: "call-1", Name: "bash", Content: "ok", Metadata: "{}"},
		Finish{Reason: FinishReasonEndTurn, Time: 1700000000},
	}

	raw, err := marshallParts(parts)
	if err != nil {
		t.Fatalf("marshallParts: %v", err)
	}
	got, err := unmarshallParts(raw)
	if err != nil {
		t.Fatalf("unmarshallParts: %v", err)
	}
	if len(got) != len(parts) {
		t.Fatalf("round-trip lost parts: got %d, want %d (%#v)", len(got), len(parts), got)
	}

	if got[0].(ReasoningContent).Thinking != "let me think" {
		t.Errorf("reasoning = %#v", got[0])
	}
	if got[1].(TextContent).Text != "hello" {
		t.Errorf("text = %#v", got[1])
	}
	// Regression: the image_url branch used to unmarshal without appending, silently
	// dropping every image part on read.
	if img, ok := got[2].(ImageURLContent); !ok || img.URL != "https://example.com/a.png" || img.Detail != "high" {
		t.Errorf("image part = %#v (ok=%v)", got[2], ok)
	}
	if bc, ok := got[3].(BinaryContent); !ok || bc.MIMEType != "image/png" || len(bc.Data) != 3 {
		t.Errorf("binary part = %#v (ok=%v)", got[3], ok)
	}
	if tc, ok := got[4].(ToolCall); !ok || tc.ID != "call-1" || tc.Name != "bash" {
		t.Errorf("tool call = %#v (ok=%v)", got[4], ok)
	}
	if tr, ok := got[5].(ToolResult); !ok || tr.ToolCallID != "call-1" {
		t.Errorf("tool result = %#v (ok=%v)", got[5], ok)
	}
	if f, ok := got[6].(Finish); !ok || f.Reason != FinishReasonEndTurn || f.Time != 1700000000 {
		t.Errorf("finish = %#v (ok=%v)", got[6], ok)
	}
}

func TestMarshallPartsUnknownType(t *testing.T) {
	type bogus struct{ ContentPart }
	if _, err := marshallParts([]ContentPart{bogus{}}); err == nil {
		t.Fatal("expected error for unknown part type")
	}
}

func TestUnmarshallPartsUnknownType(t *testing.T) {
	if _, err := unmarshallParts([]byte(`[{"type":"nope","data":{}}]`)); err == nil {
		t.Fatal("expected error for unknown part type")
	}
}

func TestUnmarshallPartsInvalidJSON(t *testing.T) {
	if _, err := unmarshallParts([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestAppendContentAndReasoning(t *testing.T) {
	m := &Message{}
	m.AppendContent("a")
	m.AppendContent("b")
	m.AppendReasoningContent("x")
	m.AppendReasoningContent("y")

	if m.Content().Text != "ab" {
		t.Errorf("content = %q, want ab", m.Content().Text)
	}
	if m.ReasoningContent().Thinking != "xy" {
		t.Errorf("reasoning = %q, want xy", m.ReasoningContent().Thinking)
	}
	if n := len(m.Parts); n != 2 {
		t.Errorf("parts = %d, want 2 (append must merge, not accumulate)", n)
	}
}

func TestToolCallLifecycle(t *testing.T) {
	m := &Message{}
	m.AddToolCall(ToolCall{ID: "c1", Name: "bash", Type: "function"})
	// A second AddToolCall with the same id updates in place instead of duplicating.
	m.AddToolCall(ToolCall{ID: "c1", Name: "bash", Type: "function"})
	if n := len(m.ToolCalls()); n != 1 {
		t.Fatalf("tool calls = %d, want 1", n)
	}
	m.AppendToolCallInput("c1", `{"cmd":`)
	m.AppendToolCallInput("c1", `"ls"}`)
	m.FinishToolCall("c1")

	tc := m.ToolCalls()[0]
	if tc.Input != `{"cmd":"ls"}` {
		t.Errorf("input = %q", tc.Input)
	}
	if !tc.Finished {
		t.Error("tool call should be finished")
	}
}

func TestSetToolCallsReplaces(t *testing.T) {
	m := &Message{Parts: []ContentPart{
		TextContent{Text: "keep"},
		ToolCall{ID: "old"},
	}}
	m.SetToolCalls([]ToolCall{{ID: "new1"}, {ID: "new2"}})

	calls := m.ToolCalls()
	if len(calls) != 2 || calls[0].ID != "new1" || calls[1].ID != "new2" {
		t.Fatalf("tool calls = %#v", calls)
	}
	if m.Content().Text != "keep" {
		t.Errorf("non-tool parts must survive SetToolCalls, got %#v", m.Parts)
	}
}

func TestAddFinishReplaces(t *testing.T) {
	m := &Message{}
	m.AddFinish(FinishReasonEndTurn)
	m.AddFinish(FinishReasonCanceled)

	if n := len(m.Parts); n != 1 {
		t.Fatalf("parts = %d, want 1 (finish must be replaced, not appended)", n)
	}
	if m.FinishReason() != FinishReasonCanceled {
		t.Errorf("finish reason = %q", m.FinishReason())
	}
}

func TestIsThinking(t *testing.T) {
	thinking := &Message{Parts: []ContentPart{ReasoningContent{Thinking: "hmm"}}}
	if !thinking.IsThinking() {
		t.Error("reasoning-only unfinished message should be thinking")
	}
	answered := &Message{Parts: []ContentPart{ReasoningContent{Thinking: "hmm"}, TextContent{Text: "hi"}}}
	if answered.IsThinking() {
		t.Error("message with content is not thinking")
	}
	finished := &Message{Parts: []ContentPart{ReasoningContent{Thinking: "hmm"}, Finish{Reason: FinishReasonEndTurn}}}
	if finished.IsThinking() {
		t.Error("finished message is not thinking")
	}
}

func TestBinaryContentString(t *testing.T) {
	bc := BinaryContent{MIMEType: "image/png", Data: []byte("abc")}
	openai := bc.String(models.ProviderOpenAI)
	if want := "data:image/png;base64,YWJj"; openai != want {
		t.Errorf("openai = %q, want %q", openai, want)
	}
	// Non-OpenAI providers get the bare base64 payload.
	if got := bc.String(models.ProviderAnthropic); got != "YWJj" {
		t.Errorf("anthropic = %q, want YWJj", got)
	}
}

func TestImageAndBinaryAccessors(t *testing.T) {
	m := &Message{}
	m.AddImageURL("https://x/y.png", "auto")
	m.AddBinary("image/png", []byte{9})

	if urls := m.ImageURLContent(); len(urls) != 1 || urls[0].URL != "https://x/y.png" {
		t.Errorf("images = %#v", urls)
	}
	if bins := m.BinaryContent(); len(bins) != 1 || bins[0].MIMEType != "image/png" {
		t.Errorf("binaries = %#v", bins)
	}
}

func TestIsFinishedAndFinishPart(t *testing.T) {
	m := &Message{}
	if m.IsFinished() || m.FinishPart() != nil {
		t.Error("empty message must not be finished")
	}
	m.AddFinish(FinishReasonToolUse)
	if !m.IsFinished() {
		t.Error("message with finish part is finished")
	}
	if p := m.FinishPart(); p == nil || p.Reason != FinishReasonToolUse {
		t.Errorf("finish part = %#v", p)
	}
}
