// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package machiner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"
	"github.com/vishvananda/netlink"

	"github.com/juju/juju/core/watcher"
)

type addrSubscribeFunc func(chan<- netlink.AddrUpdate, <-chan struct{}) error

type addressChangeNotifyWatcher struct {
	catacomb catacomb.Catacomb
	changes  chan struct{}
	done     chan struct{}
	updates  chan netlink.AddrUpdate
}

func newAddressChangeNotifyWatcher(_ context.Context) (watcher.NotifyWatcher, error) {
	return newAddressChangeNotifyWatcherForSubscribe(netlink.AddrSubscribe)
}

func newAddressChangeNotifyWatcherForSubscribe(
	subscribe addrSubscribeFunc,
) (watcher.NotifyWatcher, error) {
	w := &addressChangeNotifyWatcher{
		changes: make(chan struct{}, 1),
		done:    make(chan struct{}),
		updates: make(chan netlink.AddrUpdate),
	}

	if err := subscribe(w.updates, w.done); err != nil {
		close(w.done)
		return nil, errors.Trace(err)
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "machiner-address-change-watcher",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		close(w.done)
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *addressChangeNotifyWatcher) loop() error {
	defer close(w.done)

	w.notify()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-w.updates:
			if !ok {
				return nil
			}
			w.notify()
		}
	}
}

func (w *addressChangeNotifyWatcher) notify() {
	select {
	case <-w.catacomb.Dying():
	case w.changes <- struct{}{}:
	default:
	}
}

func (w *addressChangeNotifyWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *addressChangeNotifyWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *addressChangeNotifyWatcher) Changes() <-chan struct{} {
	return w.changes
}
