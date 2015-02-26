// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v4/hooks"
	"launchpad.net/tomb"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/hook"
)

// storageSource is a hook source that generates storage hooks for
// a single storage attachment.
type storageSource struct {
	tomb tomb.Tomb

	st         StorageAccessor
	unitTag    names.UnitTag
	storageTag names.StorageTag
	watcher    apiwatcher.NotifyWatcher
	changes    chan hook.SourceChange

	// context is the most recently retrieved details of the
	// storage attachment, or nil if none has been retrieved
	// or the storage attachment is no longer of interest.
	context *contextStorage

	// hookInfo is the next hook.Info to return, if non-nil.
	hookInfo *hook.Info
}

// newStorageSource creates a hook source that watches for changes to,
// and generates storage hooks for, a single storage attachment.
func newStorageSource(st StorageAccessor, unitTag names.UnitTag, storageTag names.StorageTag) (*storageSource, error) {
	w, err := st.WatchStorageAttachment(storageTag, unitTag)
	if err != nil {
		return nil, errors.Annotate(err, "watching storage attachment")
	}
	s := &storageSource{
		st:         st,
		watcher:    w,
		changes:    make(chan hook.SourceChange),
		unitTag:    unitTag,
		storageTag: storageTag,
	}
	go func() {
		defer s.tomb.Done()
		defer watcher.Stop(w, &s.tomb)
		s.tomb.Kill(s.loop())
	}()
	return s, nil
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
			logger.Debugf("got storage attachment change")
			if !ok {
				return watcher.EnsureErr(s.watcher)
			}
			inChanges = nil
			outChanges = s.changes
			outChange = func() error {
				defer func() {
					ready <- struct{}{}
				}()
				logger.Debugf("processing storage source change")
				return s.update()
			}
		case outChanges <- outChange:
			logger.Debugf("sent storage source change")
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
	return s.hookInfo == nil
}

// Next is part of the hook.Source interface.
func (s *storageSource) Next() hook.Info {
	if s.Empty() {
		panic("source is empty")
	}
	return *s.hookInfo
}

// Pop is part of the hook.Source interface.
func (s *storageSource) Pop() {
	if s.Empty() {
		panic("source is empty")
	}
	s.hookInfo = nil
}

func (s *storageSource) update() error {
	attachment, err := s.st.StorageAttachment(s.storageTag, s.unitTag)
	if params.IsCodeNotFound(err) {
		// The storage attachment was removed from state, which
		// implies that the storage has been detached already.
		logger.Debugf("storage attachment %q not found", s.storageTag.Id())
		return nil
	} else if params.IsCodeNotProvisioned(err) {
		logger.Debugf("storage attachment %q not provisioned yet", s.storageTag.Id())
		return nil
	} else if err != nil {
		logger.Debugf("error refreshing storage details: %v", err)
		return errors.Annotate(err, "refreshing storage details")
	}

	logger.Debugf("attachment: %+v", attachment)
	switch attachment.Life {
	case params.Alive:
	case params.Dying:
	case params.Dead:
		// The uniter must have handled "detaching" for the
		// attachment to be Dead. The detachment becoming
		// Dead is just the trigger for the storage provisioner
		// to remove the attachment.
		logger.Debugf("storage attachment %q is dead", s.storageTag.Id())
		return nil
	default:
		return errors.Errorf("unknown lifecycle state %q", attachment.Life)
	}

	if s.context == nil {
		// The attachment has not previously been seen.
		if attachment.Life == params.Dying {
			// Nothing to do: attachment is dying, but
			// has not previously been witnessed.
			// TODO(axw) mark attachment as Dead.
			logger.Debugf("storage attachment %q is dying", s.storageTag.Id())
			return nil
		}
		s.context = &contextStorage{
			tag: s.storageTag,
		}
	} else if attachment.Life == params.Dying && !s.Empty() && s.Next().Kind == hooks.StorageAttached {
		// Attachment is dying, but has not previously been witnessed.
		// Dequeue the "storage-attaching" hook; nothing left to do.
		s.Pop()
		// TODO(axw) mark attachment as Dead.
		return nil
	}

	s.context.kind = storage.StorageKind(attachment.Kind)
	s.context.location = attachment.Location
	if s.hookInfo == nil {
		s.hookInfo = &hook.Info{
			StorageId: s.storageTag.Id(),
		}
	}
	if attachment.Life == params.Alive {
		s.hookInfo.Kind = hooks.StorageAttached
	} else {
		// TODO(axw) this should be Detaching, not Detached.
		s.hookInfo.Kind = hooks.StorageDetached
	}
	logger.Debugf("queued hook: %v", s.hookInfo)
	return nil
}
