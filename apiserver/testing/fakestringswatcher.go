// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package testing

// FakeStringsWatcher is a mock implementation for the state.StringsWatcher interface.
type FakeStringsWatcher struct {
	ChangeStrings chan []string
}

// Stop stops the watcher.
func (*FakeStringsWatcher) Stop() error {
	return nil
}

// Kill kills the watcher.
func (*FakeStringsWatcher) Kill() {}

// Wait waits for the watcher.
func (*FakeStringsWatcher) Wait() error {
	return nil
}

// Err returns the last error encountered by the watcher.
func (*FakeStringsWatcher) Err() error {
	return nil
}

// Changes returns the changes detected by the watcher.
func (w *FakeStringsWatcher) Changes() <-chan []string {
	return w.ChangeStrings
}
