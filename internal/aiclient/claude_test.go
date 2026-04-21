package aiclient

import "testing"

func TestAskFailsWithoutAPIKey(t *testing.T) {
	c := NewClaude("", "claude-sonnet-4-6", 1024)
	if _, err := c.Ask(nil, "system", "hi"); err == nil { //nolint:staticcheck
		t.Fatal("expected error without API key")
	}
}

func TestModelDefaultApplies(t *testing.T) {
	c := NewClaude("fake", "", 0)
	if c.Model() == "" {
		t.Fatal("model should default when empty")
	}
}
