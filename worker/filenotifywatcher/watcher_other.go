//go:build !linux
// +build !linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

type watcher struct{}

func newWatcher() (INotifyWatcher, error) {
	return &watcher{}, nil
}

func (w *watcher) Watch(path string) error {
	return nil
}

func (w *watcher) Events() <-chan *inotify.Event {
	return nil
}

func (w *watcher) Errors() <-chan error {
	return nil
}

func (w *watcher) Close() error {
	return nil
}
