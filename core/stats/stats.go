// Package stats keeps a small daily history: hunks reviewed and the conformity share,
// for the start page (streak, "heute 23 Hunks", the sparkline).
package stats

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	fileName   = "stats.json"
	sparkDays  = 8
	weekDays   = 7
	fileMode   = 0o600
	dayLayout  = time.DateOnly
	keepMonths = 13
)

// Day is one day of history.
type Day struct {
	Reviewed   int     `json:"reviewed"`
	Conformity float64 `json:"conformity"`
	HasConf    bool    `json:"hasConf"`
}

// Summary is what the start page shows.
type Summary struct {
	Conformity    float64   `json:"conformity"`
	WeekAgo       float64   `json:"weekAgo"`
	HasWeekAgo    bool      `json:"hasWeekAgo"`
	ReviewedToday int       `json:"reviewedToday"`
	Streak        int       `json:"streak"`
	Spark         []float64 `json:"spark"`
	SparkDays     []string  `json:"sparkDays"`
	Week          []int     `json:"week"` // changes reviewed per day, the last seven days, oldest first
}

// Stats is the history file.
type Stats struct {
	path string
	mu   sync.Mutex
	Days map[string]*Day `json:"days"`
}

// Open loads the history from dataDir.
func Open(dataDir string) (*Stats, error) {
	s := &Stats{path: filepath.Join(dataDir, fileName), Days: map[string]*Day{}}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, s); err != nil {
		return nil, err
	}
	if s.Days == nil {
		s.Days = map[string]*Day{}
	}
	return s, nil
}

// Reviewed counts decided hunks for today.
func (s *Stats) Reviewed(now time.Time, n int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.day(now).Reviewed += n
	return s.save()
}

// Conformity stores today's share of clean notes.
func (s *Stats) Conformity(now time.Time, share float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.day(now)
	if d.HasConf && d.Conformity == share {
		return nil
	}
	d.Conformity, d.HasConf = share, true
	return s.save()
}

// Summary computes streak, today's count, the value a week ago and the sparkline.
func (s *Stats) Summary(now time.Time, current float64) Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	sum := Summary{Conformity: current}
	if d, ok := s.Days[now.Format(dayLayout)]; ok {
		sum.ReviewedToday = d.Reviewed
	}

	// Streak: consecutive days with reviews, ending today (or yesterday if today has none yet).
	day := now
	if sum.ReviewedToday == 0 {
		day = day.AddDate(0, 0, -1)
	}
	for {
		d, ok := s.Days[day.Format(dayLayout)]
		if !ok || d.Reviewed == 0 {
			break
		}
		sum.Streak++
		day = day.AddDate(0, 0, -1)
	}

	// Week ago: the latest recorded conformity at or before now-7 days.
	limit := now.AddDate(0, 0, -weekDays).Format(dayLayout)
	keys := s.sortedKeys()
	for i := len(keys) - 1; i >= 0; i-- {
		if keys[i] <= limit && s.Days[keys[i]].HasConf {
			sum.WeekAgo, sum.HasWeekAgo = s.Days[keys[i]].Conformity, true
			break
		}
	}

	// Reviewed changes of the last seven days, for the streak tile.
	sum.Week = make([]int, weekDays)
	for i := 0; i < weekDays; i++ {
		if d, ok := s.Days[now.AddDate(0, 0, i-weekDays+1).Format(dayLayout)]; ok {
			sum.Week[i] = d.Reviewed
		}
	}

	// Sparkline: the last recorded conformity values, oldest first.
	for i := len(keys) - 1; i >= 0 && len(sum.Spark) < sparkDays; i-- {
		if s.Days[keys[i]].HasConf {
			sum.Spark = append([]float64{s.Days[keys[i]].Conformity}, sum.Spark...)
			sum.SparkDays = append([]string{keys[i]}, sum.SparkDays...)
		}
	}
	return sum
}

func (s *Stats) sortedKeys() []string {
	keys := make([]string, 0, len(s.Days))
	for k := range s.Days {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *Stats) day(now time.Time) *Day {
	key := now.Format(dayLayout)
	d, ok := s.Days[key]
	if !ok {
		d = &Day{}
		s.Days[key] = d
	}
	return d
}

// save writes the file and drops days older than about a year.
func (s *Stats) save() error {
	cut := time.Now().AddDate(0, -keepMonths, 0).Format(dayLayout)
	for k := range s.Days {
		if k < cut {
			delete(s.Days, k)
		}
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, fileMode); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Seed sets history directly (demo and tests).
func (s *Stats) Seed(days map[string]Day) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, d := range days {
		d := d
		s.Days[k] = &d
	}
	return s.save()
}
