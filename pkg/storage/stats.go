package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// DailyStat records reading metrics for a calendar day.
type DailyStat struct {
	Date           string `json:"date"`            // "YYYY-MM-DD"
	ReadSeconds    int    `json:"read_seconds"`    // Total reading duration in seconds
	EstimatedWords int    `json:"estimated_words"` // Estimated words read
	ChaptersRead   int    `json:"chapters_read"`   // Count of chapters opened/read
}

// BookStat tracks cumulative reading progress for a single novel.
type BookStat struct {
	BookID          string  `json:"book_id"`
	Title           string  `json:"title"`
	TotalSeconds    int     `json:"total_seconds"`
	TotalWords      int     `json:"total_words"`
	LastReadChapter string  `json:"last_read_chapter"`
	LastReadChapID  string  `json:"last_read_chap_id"`
	LastReadLine    int     `json:"last_read_line"`
	LastReadAt      int64   `json:"last_read_at"`
	ProgressPercent float64 `json:"progress_percent"` // 0.0 - 100.0%
}

// OverallStats aggregates overall reading metrics, daily heatmap data, and book stats.
type OverallStats struct {
	TotalSeconds      int                  `json:"total_seconds"`
	TotalWords        int                  `json:"total_words"`
	CurrentStreakDays int                  `json:"current_streak_days"`
	MaxStreakDays     int                  `json:"max_streak_days"`
	DailyRecords      map[string]DailyStat `json:"daily_records"` // Key: "YYYY-MM-DD"
	BookRecords       map[string]BookStat  `json:"book_records"`  // Key: bookID
	LastActiveBookID  string               `json:"last_active_book_id"`
}

func (s *Storage) statsFilePath() string {
	return filepath.Join(s.baseDir, "stats.json")
}

// LoadStats loads reading statistics from disk, initializing an empty structure if missing.
func (s *Storage) LoadStats() (*OverallStats, error) {
	s.mu.RLock()
	filePath := s.statsFilePath()
	s.mu.RUnlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &OverallStats{
				DailyRecords: make(map[string]DailyStat),
				BookRecords:  make(map[string]BookStat),
			}, nil
		}
		return nil, err
	}

	var stats OverallStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return &OverallStats{
			DailyRecords: make(map[string]DailyStat),
			BookRecords:  make(map[string]BookStat),
		}, nil
	}
	if stats.DailyRecords == nil {
		stats.DailyRecords = make(map[string]DailyStat)
	}
	if stats.BookRecords == nil {
		stats.BookRecords = make(map[string]BookStat)
	}

	return &stats, nil
}

// SaveStats persists reading statistics to disk.
func (s *Storage) SaveStats(stats *OverallStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.statsFilePath(), data, 0644)
}

// RecordReading logs reading time, estimated words, and bookmark position.
func (s *Storage) RecordReading(bookID, title, chapID, chapTitle string, seconds, words, lineIndex, totalLines int) error {
	if seconds <= 0 && words <= 0 && chapID == "" {
		return nil
	}

	stats, err := s.LoadStats()
	if err != nil {
		stats = &OverallStats{
			DailyRecords: make(map[string]DailyStat),
			BookRecords:  make(map[string]BookStat),
		}
	}

	now := time.Now()
	todayStr := now.Format("2006-01-02")

	// 1. Update DailyRecord
	d := stats.DailyRecords[todayStr]
	d.Date = todayStr
	d.ReadSeconds += seconds
	d.EstimatedWords += words
	if chapID != "" {
		d.ChaptersRead++
	}
	stats.DailyRecords[todayStr] = d

	// 2. Update BookRecord
	b := stats.BookRecords[bookID]
	b.BookID = bookID
	if title != "" {
		b.Title = title
	}
	b.TotalSeconds += seconds
	b.TotalWords += words
	b.LastReadAt = now.Unix()
	if chapID != "" {
		b.LastReadChapID = chapID
	}
	if chapTitle != "" {
		b.LastReadChapter = chapTitle
	}
	if lineIndex >= 0 {
		b.LastReadLine = lineIndex
	}
	if totalLines > 0 {
		pct := (float64(lineIndex+1) / float64(totalLines)) * 100.0
		if pct > 100.0 {
			pct = 100.0
		}
		if pct > b.ProgressPercent {
			b.ProgressPercent = pct
		}
	}
	stats.BookRecords[bookID] = b
	stats.LastActiveBookID = bookID

	// 3. Update Aggregate Totals
	stats.TotalSeconds += seconds
	stats.TotalWords += words

	// 4. Calculate Streaks
	stats.CurrentStreakDays = calculateCurrentStreak(stats.DailyRecords, now)
	stats.MaxStreakDays = calculateMaxStreak(stats.DailyRecords)

	return s.SaveStats(stats)
}

// GetRecentBook returns the most recently read book stat, or nil if none.
func (s *Storage) GetRecentBook() *BookStat {
	stats, err := s.LoadStats()
	if err != nil || len(stats.BookRecords) == 0 {
		return nil
	}

	if stats.LastActiveBookID != "" {
		if b, ok := stats.BookRecords[stats.LastActiveBookID]; ok {
			return &b
		}
	}

	var latest *BookStat
	for _, b := range stats.BookRecords {
		bCopy := b
		if latest == nil || bCopy.LastReadAt > latest.LastReadAt {
			latest = &bCopy
		}
	}
	return latest
}

// HeatmapWeek contains daily reading minutes for 7 days (Monday=0 to Sunday=6).
type HeatmapWeek struct {
	Days [7]int // Minutes read per day
}

// GetHeatmapWeeks computes reading activity in minutes for the past `weeksCount` calendar weeks (up to today).
// Returns a slice of HeatmapWeek with length `weeksCount`.
func (s *Storage) GetHeatmapWeeks(weeksCount int) []HeatmapWeek {
	if weeksCount <= 0 {
		weeksCount = 12
	}

	stats, _ := s.LoadStats()
	dailyMap := make(map[string]int)
	if stats != nil {
		for k, v := range stats.DailyRecords {
			dailyMap[k] = v.ReadSeconds / 60
		}
	}

	now := time.Now()
	// Find current week's Sunday
	weekday := int(now.Weekday())
	daysToSunday := (7 - weekday) % 7
	currentSunday := now.AddDate(0, 0, daysToSunday)

	res := make([]HeatmapWeek, weeksCount)

	for w := 0; w < weeksCount; w++ {
		weekIdx := weeksCount - 1 - w
		// Week's Sunday
		weekSunday := currentSunday.AddDate(0, 0, -w*7)
		// Week's Monday is 6 days before Sunday
		weekMonday := weekSunday.AddDate(0, 0, -6)

		for d := 0; d < 7; d++ {
			dayDate := weekMonday.AddDate(0, 0, d)
			dayKey := dayDate.Format("2006-01-02")
			res[weekIdx].Days[d] = dailyMap[dayKey]
		}
	}

	return res
}

func calculateCurrentStreak(records map[string]DailyStat, now time.Time) int {
	streak := 0
	todayStr := now.Format("2006-01-02")
	yesterdayStr := now.AddDate(0, 0, -1).Format("2006-01-02")

	// If read today, start from today; else if read yesterday, start from yesterday
	curDate := now
	if stat, ok := records[todayStr]; ok && stat.ReadSeconds >= 60 {
		// Read at least 1 minute today
	} else if stat, ok := records[yesterdayStr]; ok && stat.ReadSeconds >= 60 {
		curDate = now.AddDate(0, 0, -1)
	} else {
		return 0
	}

	for {
		key := curDate.Format("2006-01-02")
		stat, ok := records[key]
		if !ok || stat.ReadSeconds < 60 {
			break
		}
		streak++
		curDate = curDate.AddDate(0, 0, -1)
	}

	return streak
}

func calculateMaxStreak(records map[string]DailyStat) int {
	if len(records) == 0 {
		return 0
	}

	dates := make([]string, 0, len(records))
	for d, s := range records {
		if s.ReadSeconds >= 60 {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)

	maxStreak := 0
	curStreak := 0
	var prevDate time.Time

	for _, ds := range dates {
		t, err := time.Parse("2006-01-02", ds)
		if err != nil {
			continue
		}
		if curStreak == 0 {
			curStreak = 1
		} else {
			if t.Sub(prevDate).Hours() <= 26 && t.Day() != prevDate.Day() {
				curStreak++
			} else {
				curStreak = 1
			}
		}
		if curStreak > maxStreak {
			maxStreak = curStreak
		}
		prevDate = t
	}

	return maxStreak
}
