package perf

import (
	"log/slog"
	"sync/atomic"
	"time"
)

type Timer struct {
	name     string
	logger   *slog.Logger
	start    time.Time
	threshMs int64
}

type Counter struct {
	value int64
}

type Stats struct {
	Name          string
	Count         int64
	TotalDuration time.Duration
	MinDuration   time.Duration
	MaxDuration   time.Duration
	SlowOps       int64
}

type Recorder struct {
	name      string
	logger    *slog.Logger
	count     int64
	totalDur  int64
	minDur    int64
	maxDur    int64
	slowOps   int64
	threshold time.Duration
}

func NewTimer(name string, logger *slog.Logger, threshMs int64) *Timer {
	return &Timer{
		name:     name,
		logger:   logger,
		start:    time.Now(),
		threshMs: threshMs,
	}
}

func (t *Timer) Stop() {
	elapsed := time.Since(t.start)
	if t.logger != nil {
		t.logger.Debug(t.name, "duration_ms", elapsed.Milliseconds())
		if elapsed.Milliseconds() > t.threshMs {
			t.logger.Warn(t.name+"_slow", "duration_ms", elapsed.Milliseconds(), "threshold_ms", t.threshMs)
		}
	}
}

func NewCounter() *Counter {
	return &Counter{}
}

func (c *Counter) Inc() {
	atomic.AddInt64(&c.value, 1)
}

func (c *Counter) Value() int64 {
	return atomic.LoadInt64(&c.value)
}

func NewRecorder(name string, logger *slog.Logger, threshold time.Duration) *Recorder {
	return &Recorder{
		name:      name,
		logger:    logger,
		threshold: threshold,
		minDur:    1<<63 - 1,
	}
}

func (r *Recorder) Record(elapsed time.Duration) {
	elapsedNs := elapsed.Nanoseconds()
	atomic.AddInt64(&r.count, 1)
	atomic.AddInt64(&r.totalDur, elapsedNs)

	for {
		minDur := atomic.LoadInt64(&r.minDur)
		if elapsedNs >= minDur || minDur == 1<<63-1 {
			break
		}
		if atomic.CompareAndSwapInt64(&r.minDur, minDur, elapsedNs) {
			break
		}
	}

	for {
		maxDur := atomic.LoadInt64(&r.maxDur)
		if elapsedNs <= maxDur {
			break
		}
		if atomic.CompareAndSwapInt64(&r.maxDur, maxDur, elapsedNs) {
			break
		}
	}

	if elapsed >= r.threshold {
		atomic.AddInt64(&r.slowOps, 1)
	}
}

func (r *Recorder) Stats() Stats {
	totalDur := atomic.LoadInt64(&r.totalDur)
	minDur := atomic.LoadInt64(&r.minDur)
	maxDur := atomic.LoadInt64(&r.maxDur)

	if minDur == 1<<63-1 {
		minDur = 0
	}

	return Stats{
		Name:          r.name,
		Count:         atomic.LoadInt64(&r.count),
		TotalDuration: time.Duration(totalDur),
		MinDuration:   time.Duration(minDur),
		MaxDuration:   time.Duration(maxDur),
		SlowOps:       atomic.LoadInt64(&r.slowOps),
	}
}

func (s *Stats) AvgDuration() time.Duration {
	if s.Count == 0 {
		return 0
	}
	return s.TotalDuration / time.Duration(s.Count)
}

func (r *Recorder) LogStats(level slog.Level) {
	stats := r.Stats()
	if stats.Count == 0 {
		return
	}
	attrs := []any{
		"count", stats.Count,
		"total_ms", stats.TotalDuration.Milliseconds(),
		"avg_ms", stats.AvgDuration().Milliseconds(),
		"min_ms", stats.MinDuration.Milliseconds(),
		"max_ms", stats.MaxDuration.Milliseconds(),
		"slow_ops", stats.SlowOps,
	}
	switch level {
	case slog.LevelDebug:
		r.logger.Debug(r.name+"_stats", attrs...)
	case slog.LevelInfo:
		r.logger.Info(r.name+"_stats", attrs...)
	case slog.LevelWarn:
		r.logger.Warn(r.name+"_stats", attrs...)
	}
}

func Measure(name string, logger *slog.Logger, threshMs int64) func() {
	return func() {
		start := time.Now()
		elapsed := time.Since(start)
		if logger != nil {
			logger.Debug(name, "duration_ms", elapsed.Milliseconds())
			if elapsed.Milliseconds() > threshMs {
				logger.Warn(name+"_slow", "duration_ms", elapsed.Milliseconds(), "threshold_ms", threshMs)
			}
		}
	}
}
