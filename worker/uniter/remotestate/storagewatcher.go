// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/tomb"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
)

func newStorageAttachmentWatcher(
	st StorageAccessor,
	unitTag names.UnitTag,
	storageTag names.StorageTag,
	changes chan StorageSnapshotEvent,
) (*storageAttachmentWatcher, error) {
	w, err := st.WatchStorageAttachment(storageTag, unitTag)
	if err != nil {
		return nil, errors.Annotate(err, "watching storage attachment")
	}
	s := &storageAttachmentWatcher{
		st:         st,
		watcher:    w,
		changes:    changes,
		storageTag: storageTag,
		unitTag:    unitTag,
	}

	// TODO(wallyworld) - check if we need this
	//	// Consume the initial event and update the storage snapshot.
	//	if _, ok := <-w.Changes(); ok {
	//		if err := s.updateSnapshot(); err != nil {
	//			return nil, errors.Annotatef(err, "processing initial storage attachment change")
	//		}
	//	} else {
	//		err := watcher.EnsureErr(w)
	//		return nil, errors.Annotatef(err, "cannot obtain initial storage attachement status")
	//	}

	go func() {
		defer s.tomb.Done()
		defer watcher.Stop(w, &s.tomb)
		s.tomb.Kill(s.loop())
	}()
	return s, nil
}

type StorageAccessor interface {
	// WatchStorageAttachment starts a watcher for changes to the
	// storage attachment with the specified unit and storage tags.
	WatchStorageAttachment(names.StorageTag, names.UnitTag) (apiwatcher.NotifyWatcher, error)

	// StorageAttachment returns the storage attachment with the specified
	// unit and storage tags.
	StorageAttachment(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
}

type StorageSnapshotEvent struct {
	StorageSnapshot
	remove bool
}

// storageAttachmentWatcher watches for changes to the attachment status of
// the storage with the specified tag and sends the tag to the specified channel
// when a change occurs.
type storageAttachmentWatcher struct {
	tomb tomb.Tomb

	st         StorageAccessor
	watcher    apiwatcher.NotifyWatcher
	storageTag names.StorageTag
	unitTag    names.UnitTag
	changes    chan StorageSnapshotEvent
}

func (s *storageAttachmentWatcher) updateSnapshot() error {
	attachment, err := s.st.StorageAttachment(s.storageTag, s.unitTag)
	remove := false
	if params.IsCodeNotFound(err) {
		// The storage attachment was removed from state, which
		// implies that the storage has been detached already.
		logger.Debugf("storage attachment %q not found", s.storageTag.Id())
		remove = true
	} else if params.IsCodeNotProvisioned(err) {
		logger.Debugf("storage attachment %q not provisioned yet", s.storageTag.Id())
		return nil
	} else if err != nil {
		logger.Debugf("error refreshing storage details: %v", err)
		return errors.Annotate(err, "refreshing storage details")
	}

	attached := attachment.Life == params.Alive
	event := StorageSnapshotEvent{
		StorageSnapshot{
			Tag:      s.storageTag,
			Kind:     attachment.Kind,
			Life:     attachment.Life,
			Attached: attached,
			Location: attachment.Location,
		},
		remove,
	}
	select {
	case s.changes <- event:
	case <-s.tomb.Dying():
		return tomb.ErrDying
	}
	return nil
}

func (s *storageAttachmentWatcher) loop() error {
	for {
		select {
		case <-s.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-s.watcher.Changes():
			if !ok {
				return watcher.EnsureErr(s.watcher)
			}
			if err := s.updateSnapshot(); err != nil {
				return err
			}
		}
	}
}

func (s *storageAttachmentWatcher) Stop() error {
	s.tomb.Kill(nil)
	return s.tomb.Wait()
}
