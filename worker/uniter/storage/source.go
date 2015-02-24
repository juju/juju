// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"launchpad.net/tomb"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker/uniter/hook"
)

// storageSource is a hook source that generates storage hooks for
// a single storage attachment.
type storageSource struct {
	tomb    tomb.Tomb
	watcher apiwatcher.NotifyWatcher
	changes chan hook.SourceChange
}

// newStorageSource creates a hook source that watches for changes to,
// and generates storage hooks for, a single storage attachment.
func newStorageSource(w apiwatcher.NotifyWatcher) *storageSource {
	s := &storageSource{
		watcher: w,
		changes: make(chan hook.SourceChange),
	}
	go func() {
		defer s.tomb.Done()
		defer watcher.Stop(w, &s.tomb)
		s.tomb.Kill(s.loop())
	}()
	return s
}

func (s *storageSource) loop() error {
	defer close(s.changes)

	var inChanges <-chan struct{}
	var outChanges chan<- hook.SourceChange
	var outChange hook.SourceChange
	ready := make(chan struct{}, 1)
	ready <- struct{}{}
	for {
		select {
		case <-s.tomb.Dying():
			return tomb.ErrDying
		case <-ready:
			inChanges = s.watcher.Changes()
		case _, ok := <-inChanges:
			if !ok {
				return watcher.EnsureErr(s.watcher)
			}
			inChanges = nil
			outChanges = s.changes
			outChange = func() error {
				defer func() {
					ready <- struct{}{}
				}()
				return s.update()
			}
		case outChanges <- outChange:
			outChanges = nil
			outChange = nil
		}
	}
}

// Changes is part of the hook.Source interface.
func (s *storageSource) Changes() <-chan hook.SourceChange {
	return s.changes
}

// Stop is part of the hook.Source interface.
func (s *storageSource) Stop() error {
	watcher.Stop(s.watcher, &s.tomb)
	return s.tomb.Wait()
}

// Empty is part of the hook.Source interface.
func (s *storageSource) Empty() bool {
	// TODO(axw) this method will report whether or not the hook execution
	// request queue is empty.
	panic("unimplemented")
}

// Next is part of the hook.Source interface.
func (s *storageSource) Next() hook.Info {
	// TODO(axw) this method will return the first hook execution request
	// in the queue.
	panic("unimplemented")
}

// Pop is part of the hook.Source interface.
func (s *storageSource) Pop() {
	// TODO(axw) this method will remove the first hook execution request
	// from the queue.
	panic("unimplemented")
}

func (s *storageSource) update() error {
	// TODO(axw) this method will query the current state of the storage
	// attachment, and queue or unqueue hook execution requests as
	// necessary.
	return nil
}
