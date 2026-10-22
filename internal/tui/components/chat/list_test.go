package chat

import "testing"

func TestToggleThinking(t *testing.T) {
	cmp, ok := NewMessagesCmp(nil).(*messagesCmp)
	if !ok {
		t.Fatal("NewMessagesCmp must return a *messagesCmp")
	}
	if !cmp.showThinking {
		t.Fatal("thinking blocks must be visible by default")
	}

	next, cmd := cmp.Update(ToggleThinkingMsg{})
	got := next.(*messagesCmp)
	if got.showThinking {
		t.Error("first toggle must hide the reasoning blocks")
	}
	if cmd == nil {
		t.Error("toggle must report the new state")
	}

	next, _ = got.Update(ToggleThinkingMsg{})
	if !next.(*messagesCmp).showThinking {
		t.Error("second toggle must show the reasoning blocks again")
	}
}
