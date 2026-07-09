package storage

import (
	"testing"
	"time"
)

func TestStorageReadingStats(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStorage(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial state
	stats, err := store.LoadStats()
	if err != nil {
		t.Fatalf("LoadStats failed: %v", err)
	}
	if stats.TotalSeconds != 0 || len(stats.DailyRecords) != 0 {
		t.Errorf("expected empty stats, got %+v", stats)
	}

	// 2. Record reading session
	err = store.RecordReading("b1001", "刀剑神域", "c1", "第一章 剑的世界", 1200, 15000, 50, 100)
	if err != nil {
		t.Fatalf("RecordReading failed: %v", err)
	}

	stats, err = store.LoadStats()
	if err != nil {
		t.Fatalf("LoadStats after record failed: %v", err)
	}
	if stats.TotalSeconds != 1200 {
		t.Errorf("expected 1200 total seconds, got %d", stats.TotalSeconds)
	}
	if stats.TotalWords != 15000 {
		t.Errorf("expected 15000 total words, got %d", stats.TotalWords)
	}
	if stats.CurrentStreakDays != 1 {
		t.Errorf("expected 1 current streak day, got %d", stats.CurrentStreakDays)
	}

	// Check book stat
	bStat, ok := stats.BookRecords["b1001"]
	if !ok {
		t.Fatalf("expected book b1001 in records")
	}
	if bStat.Title != "刀剑神域" || bStat.LastReadChapter != "第一章 剑的世界" {
		t.Errorf("unexpected book stat: %+v", bStat)
	}
	if bStat.ProgressPercent != 51.0 { // (50+1)/100 * 100 = 51%
		t.Errorf("expected progress 51.0%%, got %f", bStat.ProgressPercent)
	}

	// 3. Test GetRecentBook
	recent := store.GetRecentBook()
	if recent == nil || recent.BookID != "b1001" {
		t.Errorf("expected recent book b1001, got %+v", recent)
	}

	// 4. Test GetHeatmapWeeks
	weeks := store.GetHeatmapWeeks(12)
	if len(weeks) != 12 {
		t.Fatalf("expected 12 weeks, got %d", len(weeks))
	}
	// Current week should have minutes recorded
	lastWeek := weeks[11]
	hasMinutes := false
	for _, m := range lastWeek.Days {
		if m > 0 {
			hasMinutes = true
			break
		}
	}
	if !hasMinutes {
		t.Errorf("expected some reading minutes in current week, got: %+v", lastWeek)
	}
}

func TestStreaksCalculation(t *testing.T) {
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	dayBefore := now.AddDate(0, 0, -2).Format("2006-01-02")

	records := map[string]DailyStat{
		today:     {ReadSeconds: 120},
		yesterday: {ReadSeconds: 300},
		dayBefore: {ReadSeconds: 600},
	}

	cur := calculateCurrentStreak(records, now)
	if cur != 3 {
		t.Errorf("expected streak 3, got %d", cur)
	}

	maxS := calculateMaxStreak(records)
	if maxS != 3 {
		t.Errorf("expected max streak 3, got %d", maxS)
	}
}
