// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import "gopkg.in/tomb.v2"

// StringsNotifyWatcher wraps a corewatcher.StringsWatcher and
// provides a NotifyWatcher interface.
type StringsNotifyWatcher struct {
	tomb    tomb.Tomb
	out     chan struct{}
	watcher Watcher[[]string]
}

// NewStringsNotifyWatcher creates a new StringsNotifyWatcher.
func NewStringsNotifyWatcher(watcher Watcher[[]string]) *StringsNotifyWatcher {
	w := &StringsNotifyWatcher{
		watcher: watcher,
	}
	w.tomb.Go(w.loop)
	return w
}

func (w *StringsNotifyWatcher) loop() error {
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-w.watcher.Changes():
			if !ok {
				return nil
			}

			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case w.out <- struct{}{}:
			}
		}
	}
}

func (w *StringsNotifyWatcher) Kill() {
	w.tomb.Kill(nil)
	w.watcher.Kill()
}

func (w *StringsNotifyWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *StringsNotifyWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *StringsNotifyWatcher) Err() error {
	return w.tomb.Err()
}

func (w *StringsNotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}
