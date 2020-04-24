// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

type StorageAccessor interface {
	// StorageAttachment returns the storage attachment with the specified
	// unit and storage tags.
	StorageAttachment(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
}

// newStorageAttachmentsWatcher creates a new worker that wakes on input from
// the supplied watcher's Changes chan, finds out more about them, and delivers
// them on the supplied out chan.
//
// The caller releases responsibility for stopping the supplied watcher and
// waiting for errors, *whether or not this method succeeds*.
func newStorageAttachmentWatcher(
	st StorageAccessor,
	watcher watcher.NotifyWatcher,
	unitTag names.UnitTag,
	storageTag names.StorageTag,
	out chan<- storageAttachmentChange,
) (*storageAttachmentWatcher, error) {
	s := &storageAttachmentWatcher{
		st:         st,
		changes:    watcher.Changes(),
		out:        out,
		storageTag: storageTag,
		unitTag:    unitTag,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &s.catacomb,
		Work: s.loop,
		Init: []worker.Worker{watcher},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s, nil
}

// storageAttachmentWatcher watches for changes to the attachment status of
// the storage with the specified tag and sends the tag to the specified channel
// when a change occurs.
type storageAttachmentWatcher struct {
	catacomb catacomb.Catacomb

	st         StorageAccessor
	changes    watcher.NotifyChannel
	storageTag names.StorageTag
	unitTag    names.UnitTag
	out        chan<- storageAttachmentChange
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
		case <-s.catacomb.Dying():
			return s.catacomb.ErrDying()
		case _, ok := <-s.changes:
			if !ok {
				return errors.New("storage attachment watcher closed")
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
			case <-s.catacomb.Dying():
				return s.catacomb.ErrDying()
			case s.out <- change:
			}
		}
	}
}

// Kill is part of the worker.Worker interface.
func (s *storageAttachmentWatcher) Kill() {
	s.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (s *storageAttachmentWatcher) Wait() error {
	return s.catacomb.Wait()
}
