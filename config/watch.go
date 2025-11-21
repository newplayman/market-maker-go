package config

import (
	"context"
	"os"
	"time"
)

// Watcher is a lightweight placeholder for future fsnotify-based watcher.
// It polls the file mtime periodically and invokes the callback on change.
type Watcher struct {
	Path     string
	Interval time.Duration
}

// Start begins polling; callback receives latest config on change.
func (w Watcher) Start(ctx context.Context, onUpdate func(AppConfig)) error {
	if w.Interval <= 0 {
		w.Interval = 2 * time.Second
	}
	var lastMod time.Time
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			info, err := readFileInfo(w.Path)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastMod) {
				lastMod = info.ModTime()
				if cfg, err := LoadWithEnvOverrides(w.Path); err == nil && onUpdate != nil {
					onUpdate(cfg)
				}
			}
		}
	}
}

// readFileInfo is extracted for testing/mocking.
var readFileInfo = func(path string) (info interface{ ModTime() time.Time }, err error) {
	return os.Stat(path)
}
