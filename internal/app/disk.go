package app

import (
	"context"
	"sync"
	"time"
)

type Sizer interface {
	Size(ctx context.Context, path string) (int64, error)
}

type VolumeStat interface {
	Stat(path string) (free, total uint64, err error)
}

type CleanupHistory interface {
	Recent(n int) []CleanupRecord
}

type sizeEntry struct {
	size       int64
	known      bool
	measuredAt time.Time
	inflight   bool
	forgotten  bool
}

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
