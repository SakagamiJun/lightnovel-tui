package storage

import (
	"os"
	"testing"

	"github.com/SakagamiJun/lightnovel-tui/pkg/text"
)

func TestStorageRules(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lnr-test-rules-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial load should return DefaultRules
	rules, err := store.LoadRules()
	if err != nil {
		t.Fatalf("failed to load rules: %v", err)
	}
	if len(rules) != len(text.DefaultRules) {
		t.Errorf("expected %d default rules, got %d", len(text.DefaultRules), len(rules))
	}

	// 2. Add custom rule
	customRule := text.FormattingRule{
		ID:          "test_rule_1",
		Name:        "Test Custom Rule",
		IsRegex:     false,
		Pattern:     "foo",
		Replacement: "bar",
		Enabled:     true,
	}
	if err := store.AddRule(customRule); err != nil {
		t.Fatalf("failed to add rule: %v", err)
	}

	rules, err = store.LoadRules()
	if err != nil {
		t.Fatalf("failed to reload rules: %v", err)
	}
	if len(rules) != len(text.DefaultRules)+1 {
		t.Errorf("expected %d rules after addition, got %d", len(text.DefaultRules)+1, len(rules))
	}

	// 3. Toggle rule
	newState, err := store.ToggleRule("test_rule_1")
	if err != nil {
		t.Fatalf("failed to toggle rule: %v", err)
	}
	if newState != false {
		t.Errorf("expected rule to be disabled after toggle, got %v", newState)
	}

	// 4. Delete rule
	if err := store.DeleteRule("test_rule_1"); err != nil {
		t.Fatalf("failed to delete rule: %v", err)
	}
	rules, err = store.LoadRules()
	if err != nil {
		t.Fatalf("failed to load rules after delete: %v", err)
	}
	if len(rules) != len(text.DefaultRules) {
		t.Errorf("expected %d rules after delete, got %d", len(text.DefaultRules), len(rules))
	}
}
