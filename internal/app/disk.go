package app

import (
	"context"
	"sync"
	"time"
)

// Sizer measures the disk a directory uses. Size runs external commands, so
// callers keep it off the event loop and off render paths.
type Sizer interface {
	Size(ctx context.Context, path string) (int64, error)
}

// VolumeStat reports the free and total bytes of the volume holding path.
type VolumeStat interface {
	Stat(path string) (free, total uint64, err error)
}

// CleanupHistory reads back what the audit log recorded.
type CleanupHistory interface {
	// Recent returns at most n records, newest first.
	Recent(n int) []CleanupRecord
}

type sizeEntry struct {
	size       int64
	known      bool
	measuredAt time.Time
	inflight   bool
	// forgotten: Forget came while this was being measured, so the result may predate the change.
	forgotten bool
}

// DiskSizes caches directory sizes. Get never waits for a measurement: it
// returns what is cached and starts a background one for a path that is new
// or older than ttl, at most workers at a time.
type DiskSizes struct {
	sizer Sizer
	ttl   time.Duration
	now   func() time.Time
	sem   chan struct{}
	wg    sync.WaitGroup

	mu      sync.Mutex
	entries map[string]*sizeEntry
}

func NewDiskSizes(sizer Sizer, workers int, ttl time.Duration, now func() time.Time) *DiskSizes {
	return &DiskSizes{sizer: sizer, ttl: ttl, now: now, sem: make(chan struct{}, workers), entries: map[string]*sizeEntry{}}
}

// Get returns the cached size, and whether there is one. A stale size is
// returned while it is measured again.
func (d *DiskSizes) Get(path string) (int64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.entries[path]
	if !ok {
		e = &sizeEntry{}
		d.entries[path] = e
	}
	if !e.inflight && (e.measuredAt.IsZero() || d.now().Sub(e.measuredAt) >= d.ttl) {
		e.inflight = true
		d.wg.Add(1)
		go d.measure(path)
	}
	return e.size, e.known
}

// Forget drops the cached size, for a directory that just changed.
func (d *DiskSizes) Forget(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.entries[path]
	switch {
	case !ok:
	case e.inflight:
		e.forgotten = true
	default:
		delete(d.entries, path)
	}
}

// Wait blocks until every measurement started so far has finished.
func (d *DiskSizes) Wait() { d.wg.Wait() }

func (d *DiskSizes) measure(path string) {
	defer d.wg.Done()
	d.sem <- struct{}{}
	size, err := d.sizer.Size(context.Background(), path)
	<-d.sem
	d.mu.Lock()
	defer d.mu.Unlock()
	e := d.entries[path]
	e.inflight = false
	e.measuredAt = d.now()
	if e.forgotten {
		e.forgotten, e.measuredAt = false, time.Time{}
	}
	if err == nil {
		e.size, e.known = size, true
	}
}
