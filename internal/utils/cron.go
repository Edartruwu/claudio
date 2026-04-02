package utils

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CronExpr represents a parsed cron expression (5-field: min hour dom month dow).
type CronExpr struct {
	Minute  []int // 0-59
	Hour    []int // 0-23
	Day     []int // 1-31
	Month   []int // 1-12
	Weekday []int // 0-6 (0=Sunday)
}

// ParseCron parses a standard 5-field cron expression.
func ParseCron(expr string) (*CronExpr, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields, got %d", len(fields))
	}

	minute, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}
	hour, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}
	day, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day field: %w", err)
	}
	month, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}
	weekday, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("weekday field: %w", err)
	}

	return &CronExpr{
		Minute:  minute,
		Hour:    hour,
		Day:     day,
		Month:   month,
		Weekday: weekday,
	}, nil
}

// Matches checks if a time matches the cron expression.
func (c *CronExpr) Matches(t time.Time) bool {
	return contains(c.Minute, t.Minute()) &&
		contains(c.Hour, t.Hour()) &&
		contains(c.Day, t.Day()) &&
		contains(c.Month, int(t.Month())) &&
		contains(c.Weekday, int(t.Weekday()))
}

// Next returns the next time after `from` that matches the expression.
func (c *CronExpr) Next(from time.Time) time.Time {
	t := from.Truncate(time.Minute).Add(time.Minute)
	// Search up to 1 year ahead
	limit := t.Add(366 * 24 * time.Hour)
	for t.Before(limit) {
		if c.Matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{} // no match found
}

// CronJob represents a scheduled task.
type CronJob struct {
	ID       string
	Expr     *CronExpr
	Func     func()
	Jitter   time.Duration // random delay added to execution
	OneShot  bool          // run once then remove
	LastRun  time.Time
}

// CronScheduler runs cron jobs on schedule.
type CronScheduler struct {
	mu      sync.Mutex
	jobs    map[string]*CronJob
	stop    chan struct{}
	running bool
}

// NewCronScheduler creates a new scheduler.
func NewCronScheduler() *CronScheduler {
	return &CronScheduler{
		jobs: make(map[string]*CronJob),
		stop: make(chan struct{}),
	}
}

// Add registers a new cron job.
func (s *CronScheduler) Add(job *CronJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

// Remove deletes a cron job.
func (s *CronScheduler) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
}

// Start begins the scheduler loop. Call in a goroutine.
func (s *CronScheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			s.tick(now)
		}
	}
}

// Stop halts the scheduler.
func (s *CronScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stop)
		s.running = false
	}
}

// List returns all registered jobs.
func (s *CronScheduler) List() []*CronJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*CronJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, j)
	}
	return result
}

func (s *CronScheduler) tick(now time.Time) {
	s.mu.Lock()
	var toRun []*CronJob
	var toRemove []string

	for _, job := range s.jobs {
		if job.Expr.Matches(now) && now.Sub(job.LastRun) >= time.Minute {
			toRun = append(toRun, job)
			job.LastRun = now
			if job.OneShot {
				toRemove = append(toRemove, job.ID)
			}
		}
	}

	for _, id := range toRemove {
		delete(s.jobs, id)
	}
	s.mu.Unlock()

	for _, job := range toRun {
		j := job
		go func() {
			if j.Jitter > 0 {
				jitter := time.Duration(rand.Int63n(int64(j.Jitter)))
				time.Sleep(jitter)
			}
			j.Func()
		}()
	}
}

// HumanToCron converts human-readable intervals to cron expressions.
// Supports: "5m", "1h", "2h30m", "daily", "hourly"
func HumanToCron(input string) (string, error) {
	input = strings.ToLower(strings.TrimSpace(input))

	switch input {
	case "hourly":
		return "0 * * * *", nil
	case "daily":
		return "0 9 * * *", nil // 9 AM
	}

	// Parse duration-style intervals
	d, err := time.ParseDuration(input)
	if err != nil {
		return "", fmt.Errorf("unsupported interval: %s", input)
	}

	minutes := int(d.Minutes())
	if minutes < 1 {
		return "", fmt.Errorf("interval must be at least 1 minute")
	}

	if minutes < 60 {
		return fmt.Sprintf("*/%d * * * *", minutes), nil
	}

	hours := minutes / 60
	if hours < 24 && minutes%60 == 0 {
		return fmt.Sprintf("0 */%d * * *", hours), nil
	}

	return fmt.Sprintf("%d */%d * * *", minutes%60, hours), nil
}

func parseCronField(field string, min, max int) ([]int, error) {
	if field == "*" {
		result := make([]int, max-min+1)
		for i := range result {
			result[i] = min + i
		}
		return result, nil
	}

	// Handle */N (step)
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step: %s", field)
		}
		var result []int
		for i := min; i <= max; i += step {
			result = append(result, i)
		}
		return result, nil
	}

	// Handle comma-separated values
	var result []int
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)

		// Handle range (N-M)
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			start, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid range start: %s", part)
			}
			end, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid range end: %s", part)
			}
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
			continue
		}

		// Single value
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid value: %s", part)
		}
		if val < min || val > max {
			return nil, fmt.Errorf("value %d out of range [%d-%d]", val, min, max)
		}
		result = append(result, val)
	}

	return result, nil
}

func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}
