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
	in apiwatcher.NotifyWatcher,
	unitTag names.UnitTag,
	storageTag names.StorageTag,
	changes chan<- storageAttachmentChange,
) *storageAttachmentWatcher {
	s := &storageAttachmentWatcher{
		st:         st,
		watcher:    in,
		changes:    changes,
		storageTag: storageTag,
		unitTag:    unitTag,
	}
	go func() {
		defer s.tomb.Done()
		defer watcher.Stop(in, &s.tomb)
		s.tomb.Kill(s.loop())
	}()
	return s
}

type StorageAccessor interface {
	// StorageAttachment returns the storage attachment with the specified
	// unit and storage tags.
	StorageAttachment(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
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
	changes    chan<- storageAttachmentChange
}

type storageAttachmentChange struct {
	Tag      names.StorageTag
	Snapshot StorageSnapshot
}

func getStorageSnapshot(
	st StorageAccessor,
	storageTag names.StorageTag,
	unitTag names.UnitTag,
) (StorageSnapshot, error) {
	attachment, err := st.StorageAttachment(storageTag, unitTag)
	if err != nil {
		return StorageSnapshot{}, errors.Annotate(err, "refreshing storage details")
	}
	snapshot := StorageSnapshot{
		Life:     attachment.Life,
		Kind:     attachment.Kind,
		Attached: true,
		Location: attachment.Location,
	}
	return snapshot, nil
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
			snapshot, err := getStorageSnapshot(
				s.st, s.storageTag, s.unitTag,
			)
			if params.IsCodeNotFound(err) {
				// The storage attachment was removed
				// from state, so we can stop watching.
				return nil
			} else if params.IsCodeNotProvisioned(err) {
				// We do not care about unattached
				// storage here.
				continue
			} else if err != nil {
				return err
			}
			change := storageAttachmentChange{
				s.storageTag,
				snapshot,
			}
			select {
			case <-s.tomb.Dying():
				return tomb.ErrDying
			case s.changes <- change:
			}
		}
	}
}

func (s *storageAttachmentWatcher) Stop() error {
	s.tomb.Kill(nil)
	return s.tomb.Wait()
}
