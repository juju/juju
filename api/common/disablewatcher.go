// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/watcher"
)

// todo(gfouillet): remove this file and watcher once CMR have been implemented.
//   The only purpose of this file is to avoid breaking change while CMR related
//   watcher are not properly implemented on the API Server side. Having this
//   disable implementation here is simpler and save api calls in the meantime.
//   This follow the same technical decision done for relation watcher in uniter:
//   api/agent/uniter/unit.go.

// NewDisabledWatcher creates a watcher that emits a single empty slice and
// no further events, then wait until dying.
func NewDisabledWatcher() watcher.StringsWatcher {
	out := make(chan []string)
	w := &disabledWatcher{out: out}
	emptySlice := []string{}
	w.tomb.Go(func() error {
		return w.loop(emptySlice)
	})
	return w
}

// disabledWatcher returns a watcher which returns an initial empty string array
// on its channel then no further strings. It is used to disable lifecycle
// watcher on remote entities, ie RemoteRelation, RemoteApplication and
// RemoteApplicationRelation.
type disabledWatcher struct {
	tomb tomb.Tomb
	out  chan []string
}

func (d *disabledWatcher) Changes() watcher.StringsChannel {
	return d.out
}

func (d *disabledWatcher) loop(changes []string) error {
	defer close(d.out)
	select {
	// Send the initial event only.
	case d.out <- changes:
	case <-d.tomb.Dying():
		return nil
	}
	<-d.tomb.Dying()
	return nil
}

func (d *disabledWatcher) Kill() {
	d.tomb.Kill(nil)
}

func (d *disabledWatcher) Wait() error {
	return d.tomb.Wait()
}
