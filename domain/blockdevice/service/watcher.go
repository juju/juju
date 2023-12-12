// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/watcher"
)

// blockDeviceWatcher is a notify watcher which watchers for
// updates to the block devices for a machine and only fires
// when the block device content actually changes.
type blockDeviceWatcher struct {
	tomb tomb.Tomb

	baseWatcher watcher.NotifyWatcher
	st          State

	machine string
	out     chan struct{}
}

// Kill implements Worker.
func (w *blockDeviceWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements Worker.
func (w *blockDeviceWatcher) Wait() error {
	return w.tomb.Wait()
}

func newBlockDeviceWatcher(
	st State,
	baseWatcher watcher.NotifyWatcher,
	machine string,
) (watcher.NotifyWatcher, error) {
	w := &blockDeviceWatcher{
		st:          st,
		baseWatcher: baseWatcher,
		machine:     machine,
		out:         make(chan struct{}),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w, nil
}

// Changes returns the event channel for w.
func (w *blockDeviceWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *blockDeviceWatcher) loop() (err error) {
	defer func() {
		werr := worker.Stop(w.baseWatcher)
		if werr != nil {
			if err == nil {
				err = werr
			} else {
				err = fmt.Errorf("block device watcher error %w; stopping base watcher error %w", err, werr)
			}
		}
	}()

	currentBlockDevices, err := w.st.BlockDevices(w.tomb.Context(nil), w.machine)
	if err != nil {
		return errors.Annotate(err, "getting initial block devices")
	}

	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.baseWatcher.Changes():
			newBlockDevices, err := w.st.BlockDevices(w.tomb.Context(nil), w.machine)
			if errors.Is(err, errors.NotFound) {
				// Machine has been removed so exit gracefully.
				return nil
			} else if err != nil {
				return errors.Annotatef(err, "refreshing block devices")
			}
			if !reflect.DeepEqual(currentBlockDevices, newBlockDevices) {
				currentBlockDevices = newBlockDevices
				out = w.out
			}
		case out <- struct{}{}:
			out = nil
		}
	}
}
