package dev

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Event struct {
	Kind string `json:"kind"`
}

type Reloader struct {
	watcher *fsnotify.Watcher
	log     *slog.Logger

	mu   sync.Mutex
	subs []chan Event
}

func NewReloader(dirs []string, log *slog.Logger) (*Reloader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return watcher.Add(path)
			}
			return nil
		})
		if err != nil {
			_ = watcher.Close()
			return nil, err
		}
	}

	return &Reloader{watcher: watcher, log: log}, nil
}

func (r *Reloader) Run() {
	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}

	var pending string

	for {
		select {
		case ev, ok := <-r.watcher.Events:
			if !ok {
				return
			}

			if ev.Has(fsnotify.Create) {
				r.maybeWatchDir(ev.Name)
			}

			kind := classify(ev.Name)
			if kind == "" {
				continue
			}

			pending = merge(pending, kind)
			debounce.Reset(50 * time.Millisecond)

		case <-debounce.C:
			if pending != "" {
				r.log.Info("reload", "kind", pending)
				r.broadcast(Event{Kind: pending})
				pending = ""
			}

		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			r.log.Error("fsnotify", "error", err)
		}
	}
}

func (r *Reloader) maybeWatchDir(path string) {
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return
	}
	if strings.HasPrefix(filepath.Base(path), ".") {
		return
	}
	_ = r.watcher.Add(path)
}

func (r *Reloader) Close() error {
	return r.watcher.Close()
}

func (r *Reloader) subscribe() chan Event {
	ch := make(chan Event, 1)
	r.mu.Lock()
	r.subs = append(r.subs, ch)
	r.mu.Unlock()
	return ch
}

func (r *Reloader) unsubscribe(ch chan Event) {
	r.mu.Lock()
	for i, s := range r.subs {
		if s == ch {
			r.subs = append(r.subs[:i], r.subs[i+1:]...)
			break
		}
	}
	r.mu.Unlock()
}

func (r *Reloader) broadcast(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ch := range r.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (r *Reloader) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Query().Has("is_up") {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Event{Kind: "reload"})
		return
	}

	ch := r.subscribe()
	defer r.unsubscribe(ch)

	timeout := time.NewTimer(25 * time.Second)
	defer timeout.Stop()

	select {
	case ev := <-ch:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ev)
	case <-timeout.C:
		w.WriteHeader(http.StatusNoContent)
	case <-req.Context().Done():
		return
	}
}

func classify(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".css":
		return "css"
	case ".tmpl":
		return "tmpl"
	case ".js":
		return "reload"
	default:
		return ""
	}
}

func merge(current, incoming string) string {
	rank := map[string]int{"css": 1, "tmpl": 2, "reload": 3}
	if rank[incoming] > rank[current] {
		return incoming
	}
	return current
}
