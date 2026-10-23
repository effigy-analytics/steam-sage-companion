package steam

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/protocol"
	"github.com/fsnotify/fsnotify"
)

type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

type Watcher struct {
	mu      sync.RWMutex
	readMu  sync.Mutex
	account Account
	clock   Clock
	current protocol.SnapshotResponse
}

func NewWatcher(account Account, clock Clock) *Watcher {
	if clock == nil {
		clock = realClock{}
	}
	w := &Watcher{account: account, clock: clock}
	w.Refresh()
	return w
}
func (w *Watcher) Refresh() {
	w.readMu.Lock()
	defer w.readMu.Unlock()
	snapshot, err := Read(w.account)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current.Protocol = 1
	w.current.AttemptedAt = w.clock.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		code := err.Error()
		w.current.Error = &code
		w.current.State = "unavailable"
		if w.current.Snapshot != nil {
			w.current.State = "stale"
		}
		return
	}
	snapshot.ReadAt = w.current.AttemptedAt
	w.current.State = "ready"
	w.current.Error = nil
	w.current.Snapshot = &snapshot
}
func (w *Watcher) Current() protocol.SnapshotResponse {
	w.mu.RLock()
	defer w.mu.RUnlock()
	value := w.current
	if value.Error != nil {
		e := *value.Error
		value.Error = &e
	}
	if value.Snapshot != nil {
		copy := *value.Snapshot
		copy.Collections = make([]protocol.Collection, len(value.Snapshot.Collections))
		for i, c := range value.Snapshot.Collections {
			copy.Collections[i] = c
			if c.AppIDs != nil {
				copy.Collections[i].AppIDs = append([]uint32{}, c.AppIDs...)
			}
		}
		value.Snapshot = &copy
	}
	return value
}
func (w *Watcher) Run(ctx context.Context) error {
	file, _ := w.account.collectionPath()
	var events <-chan fsnotify.Event
	var errors <-chan error
	watch, err := fsnotify.NewWatcher()
	if err == nil {
		defer watch.Close()
		if watch.Add(filepath.Dir(file)) == nil {
			events = watch.Events
			errors = watch.Errors
		}
	}
	poll := w.clock.After(60 * time.Second)
	var debounce <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-poll:
			w.Refresh()
			poll = w.clock.After(60 * time.Second)
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if filepath.Clean(event.Name) == file {
				if timer != nil {
					timer.Stop()
				}
				timer = time.NewTimer(300 * time.Millisecond)
				debounce = timer.C
			}
		case _, ok := <-errors:
			if !ok {
				errors = nil
			}
			events = nil // bounded polling recovers a failed watcher
		case <-debounce:
			w.Refresh()
			debounce = nil
		}
	}
}
