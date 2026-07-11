package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"lnr-core/pkg/text"
)

func (s *Storage) rulesFilePath() string {
	return filepath.Join(s.baseDir, "rules.json")
}

// LoadRules loads user formatting rules from disk, initializing with DefaultRules if file doesn't exist.
func (s *Storage) LoadRules() ([]text.FormattingRule, error) {
	s.mu.RLock()
	filePath := s.rulesFilePath()
	s.mu.RUnlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			defaults := make([]text.FormattingRule, len(text.DefaultRules))
			copy(defaults, text.DefaultRules)
			_ = s.SaveRules(defaults)
			return defaults, nil
		}
		return nil, err
	}

	var rules []text.FormattingRule
	if err := json.Unmarshal(data, &rules); err != nil {
		defaults := make([]text.FormattingRule, len(text.DefaultRules))
		copy(defaults, text.DefaultRules)
		return defaults, nil
	}

	return rules, nil
}

// SaveRules persists formatting rules to disk.
func (s *Storage) SaveRules(rules []text.FormattingRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.rulesFilePath(), data, 0644)
}

// AddRule adds a new formatting rule, generating an ID if empty.
func (s *Storage) AddRule(rule text.FormattingRule) error {
	rules, err := s.LoadRules()
	if err != nil {
		rules = make([]text.FormattingRule, 0)
	}

	if rule.ID == "" {
		rule.ID = fmt.Sprintf("custom_%d", len(rules)+1)
	}

	// Update existing if matching ID
	for i, r := range rules {
		if r.ID == rule.ID {
			rules[i] = rule
			return s.SaveRules(rules)
		}
	}

	rules = append(rules, rule)
	return s.SaveRules(rules)
}

// ToggleRule toggles the enabled state of a rule by ID.
func (s *Storage) ToggleRule(ruleID string) (bool, error) {
	rules, err := s.LoadRules()
	if err != nil {
		return false, err
	}

	for i, r := range rules {
		if r.ID == ruleID {
			rules[i].Enabled = !rules[i].Enabled
			if err := s.SaveRules(rules); err != nil {
				return false, err
			}
			return rules[i].Enabled, nil
		}
	}

	return false, fmt.Errorf("rule %q not found", ruleID)
}

// DeleteRule removes a rule by ID.
func (s *Storage) DeleteRule(ruleID string) error {
	rules, err := s.LoadRules()
	if err != nil {
		return err
	}

	filtered := make([]text.FormattingRule, 0, len(rules))
	found := false
	for _, r := range rules {
		if r.ID == ruleID {
			found = true
			continue
		}
		filtered = append(filtered, r)
	}

	if !found {
		return fmt.Errorf("rule %q not found", ruleID)
	}

	return s.SaveRules(filtered)
}
