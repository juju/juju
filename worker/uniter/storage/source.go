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
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// storageSource is a hook source that generates storage hooks for
// a single storage attachment.
type storageSource struct {
	tomb tomb.Tomb

	*storageHookQueue
	st      StorageAccessor
	watcher apiwatcher.NotifyWatcher
	changes chan hook.SourceChange
}

// storageHookQueue implements a subset of hook.Source, separated from
// storageSource for simpler testing.
type storageHookQueue struct {
	unitTag    names.UnitTag
	storageTag names.StorageTag

	// attached records whether or not the storage-attached
	// hook has been executed.
	attached bool

	// hookInfo is the next hook.Info to return, if non-nil.
	hookInfo *hook.Info

	// context contains the details of the storage attachment.
	context *contextStorage
}

// newStorageSource creates a hook source that watches for changes to,
// and generates storage hooks for, a single storage attachment.
func newStorageSource(
	st StorageAccessor,
	unitTag names.UnitTag,
	storageTag names.StorageTag,
	attached bool,
) (*storageSource, error) {
	w, err := st.WatchStorageAttachment(storageTag, unitTag)
	if err != nil {
		return nil, errors.Annotate(err, "watching storage attachment")
	}
	s := &storageSource{
		storageHookQueue: &storageHookQueue{
			unitTag:    unitTag,
			storageTag: storageTag,
			attached:   attached,
		},
		st:      st,
		watcher: w,
		changes: make(chan hook.SourceChange),
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
				return tomb.ErrDying
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
	s.tomb.Kill(nil)
	watcher.Stop(s.watcher, &s.tomb)
	return s.tomb.Wait()
}

// update is called when hook.SourceChanges are applied.
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
	return s.storageHookQueue.Update(attachment)
}

// Empty is part of the hook.Source interface.
func (s *storageHookQueue) Empty() bool {
	return s.hookInfo == nil
}

// Next is part of the hook.Source interface.
func (s *storageHookQueue) Next() hook.Info {
	if s.Empty() {
		panic("source is empty")
	}
	return *s.hookInfo
}

// Pop is part of the hook.Source interface.
func (s *storageHookQueue) Pop() {
	if s.Empty() {
		panic("source is empty")
	}
	if s.hookInfo.Kind == hooks.StorageAttached {
		s.attached = true
	}
	s.hookInfo = nil
}

// Update updates the hook queue with the freshly acquired information about
// the storage attachment.
func (s *storageHookQueue) Update(attachment params.StorageAttachment) error {
	switch attachment.Life {
	case params.Alive:
		if s.attached {
			// Storage attachments currently do not change
			// (apart from lifecycle) after being provisioned.
			// We don't process unprovisioned storage here,
			// so there's nothing to do.
			return nil
		}
	case params.Dying:
		if !s.attached {
			// Nothing to do: attachment is dying, but
			// the storage-attached hook has not been
			// consumed.
			s.hookInfo = nil
			return nil
		}
	case params.Dead:
		// Storage must been Dying to become Dead;
		// no further action is required.
		return nil
	}

	// Set the storage context when the first hook is generated
	// for this storager. Later, when we need to handle changing
	// storage, we'll need to have a cache in the runner like
	// we have for relations.
	if s.context == nil {
		s.context = &contextStorage{
			tag:      s.storageTag,
			kind:     storage.StorageKind(attachment.Kind),
			location: attachment.Location,
		}
	}

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

func (s *storageHookQueue) Context() (jujuc.ContextStorage, bool) {
	if s.context != nil {
		return s.context, true
	}
	return nil, false
}
