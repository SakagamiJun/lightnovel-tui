package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BookshelfGroup represents a categorized shelf group for organizing cached novels.
type BookshelfGroup struct {
	ID      string   `json:"id"`       // e.g. "all", "reading", "completed", "favorites", "custom_xxx"
	Name    string   `json:"name"`     // Display name e.g. "全部", "在读", "已完结", "精选收藏"
	BookIDs []string `json:"book_ids"` // List of book IDs assigned to this group
}

// DefaultBookshelves defines standard built-in shelf categories.
var DefaultBookshelves = []BookshelfGroup{
	{ID: "all", Name: "全部", BookIDs: []string{}},
	{ID: "reading", Name: "在读", BookIDs: []string{}},
	{ID: "completed", Name: "已完结", BookIDs: []string{}},
	{ID: "favorites", Name: "精选收藏", BookIDs: []string{}},
}

func (s *Storage) shelvesFilePath() string {
	return filepath.Join(s.baseDir, "shelves.json")
}

// LoadShelves loads shelf groups from disk, initializing with DefaultBookshelves if missing.
func (s *Storage) LoadShelves() ([]BookshelfGroup, error) {
	s.mu.RLock()
	filePath := s.shelvesFilePath()
	s.mu.RUnlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			defaults := make([]BookshelfGroup, len(DefaultBookshelves))
			copy(defaults, DefaultBookshelves)
			_ = s.SaveShelves(defaults)
			return defaults, nil
		}
		return nil, err
	}

	var shelves []BookshelfGroup
	if err := json.Unmarshal(data, &shelves); err != nil {
		defaults := make([]BookshelfGroup, len(DefaultBookshelves))
		copy(defaults, DefaultBookshelves)
		return defaults, nil
	}

	// Ensure "all" is always present as the first shelf
	hasAll := false
	for _, sh := range shelves {
		if sh.ID == "all" {
			hasAll = true
			break
		}
	}
	if !hasAll {
		shelves = append([]BookshelfGroup{{ID: "all", Name: "全部", BookIDs: []string{}}}, shelves...)
	}

	return shelves, nil
}

// SaveShelves persists shelf groups to disk.
func (s *Storage) SaveShelves(shelves []BookshelfGroup) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(shelves, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.shelvesFilePath(), data, 0644)
}

// MoveBookToShelf assigns a book to the specified shelf and removes it from other category shelves.
func (s *Storage) MoveBookToShelf(bookID string, targetShelfID string) error {
	if bookID == "" || targetShelfID == "" {
		return fmt.Errorf("bookID and targetShelfID cannot be empty")
	}

	shelves, err := s.LoadShelves()
	if err != nil {
		return err
	}

	targetFound := false
	for i := range shelves {
		if shelves[i].ID == targetShelfID {
			targetFound = true
			// Add to target shelf if not already present
			present := false
			for _, id := range shelves[i].BookIDs {
				if id == bookID {
					present = true
					break
				}
			}
			if !present {
				shelves[i].BookIDs = append(shelves[i].BookIDs, bookID)
			}
		} else if shelves[i].ID != "all" {
			// Remove from other category shelves (excluding "all")
			filtered := make([]string, 0, len(shelves[i].BookIDs))
			for _, id := range shelves[i].BookIDs {
				if id != bookID {
					filtered = append(filtered, id)
				}
			}
			shelves[i].BookIDs = filtered
		}
	}

	if !targetFound {
		return fmt.Errorf("shelf %q not found", targetShelfID)
	}

	return s.SaveShelves(shelves)
}

// AddCustomShelf creates a new custom shelf group.
func (s *Storage) AddCustomShelf(name string) (*BookshelfGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("shelf name cannot be empty")
	}

	shelves, err := s.LoadShelves()
	if err != nil {
		shelves = make([]BookshelfGroup, 0)
	}

	for _, sh := range shelves {
		if sh.Name == name {
			return nil, fmt.Errorf("shelf %q already exists", name)
		}
	}

	id := fmt.Sprintf("custom_%d", len(shelves)+1)
	newShelf := BookshelfGroup{
		ID:      id,
		Name:    name,
		BookIDs: []string{},
	}
	shelves = append(shelves, newShelf)

	if err := s.SaveShelves(shelves); err != nil {
		return nil, err
	}

	return &newShelf, nil
}
