//go:build linux
// +build linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	"github.com/fsnotify/fsnotify"
)

type watcher struct {
	watcher *fsnotify.Watcher
}

func newWatcher() (INotifyWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &watcher{
		watcher: w,
	}, nil
}

func (w *watcher) Watch(path string) error {
	return w.watcher.Add(path)
}

func (w *watcher) Events() <-chan fsnotify.Event {
	return w.watcher.Events
}

func (w *watcher) Errors() <-chan error {
	return w.watcher.Errors
}

func (w *watcher) Close() error {
	return w.watcher.Close()
}
