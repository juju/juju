//go:build linux
// +build linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import "k8s.io/utils/inotify"

type watcher struct {
	watcher *inotify.Watcher
}

func newWatcher() (INotifyWatcher, error) {
	w, err := inotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &watcher{
		watcher: w,
	}, nil
}

func (w *watcher) Watch(path string) error {
	return w.watcher.Watch(path)
}

func (w *watcher) Events() <-chan *inotify.Event {
	return w.watcher.Event
}

func (w *watcher) Errors() <-chan error {
	return w.watcher.Error
}

func (w *watcher) Close() error {
	return w.watcher.Close()
}
