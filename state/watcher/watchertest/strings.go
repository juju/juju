// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import "gopkg.in/tomb.v1"

// StringsWatcher is an implementation of state.StringsWatcher that can
// be manipulated, for testing.
type StringsWatcher struct {
	T tomb.Tomb
	C chan []string
}

// NewStringsWatcher returns a new StringsWatcher that returns the given
// channel in its "Changes" method. NewStringsWatcher takes ownership of
// the channel, closing it when it is stopped.
func NewStringsWatcher(ch chan []string) *StringsWatcher {
	w := &StringsWatcher{C: ch}
	go func() {
		defer w.T.Done()
		defer w.T.Kill(nil)
		defer close(ch)
		<-w.T.Dying()
	}()
	return w
}

// Changes is part of the state.StringsWatcher interface.
func (w *StringsWatcher) Changes() <-chan []string {
	return w.C
}

// Err is part of the state.StringsWatcher interface.
func (w *StringsWatcher) Err() error {
	return w.T.Err()
}

// Stop is part of the state.StringsWatcher interface.
func (w *StringsWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill is part of the state.StringsWatcher interface.
func (w *StringsWatcher) Kill() {
	w.T.Kill(nil)
}

// Wait is part of the state.StringsWatcher interface.
func (w *StringsWatcher) Wait() error {
	return w.T.Wait()
}
