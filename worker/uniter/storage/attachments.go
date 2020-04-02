// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package storage contains the storage subsystem for the uniter, responding
// to changes in storage attachments (lifecycle, volume/filesystem details)
// by queuing hooks and managing the storage attachments' lifecycle.
package storage

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var logger = loggo.GetLogger("juju.worker.uniter.storage")

// StorageAccessor is an interface for accessing information about
// storage attachments.
type StorageAccessor interface {
	// StorageAttachment returns details of the storage attachment
	// with the specified unit and storage tags.
	StorageAttachment(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)

	// UnitStorageAttachments returns details of all of the storage
	// attachments for the unit with the specified tag.
	UnitStorageAttachments(names.UnitTag) ([]params.StorageAttachmentId, error)

	// DestroyUnitStorageAttachments ensures that all storage
	// attachments for the specified unit will be removed at
	// some point in the future.
	DestroyUnitStorageAttachments(names.UnitTag) error

	// RemoveStorageAttachment removes that the storage attachment
	// with the specified unit and storage tags. This method is only
	// expected to succeed if the storage attachment is Dying.
	RemoveStorageAttachment(names.StorageTag, names.UnitTag) error
}

// Attachments generates storage hooks in response to changes to
// storage attachments, and provides access to information about
// storage attachments to hooks.
type Attachments struct {
	st      StorageAccessor
	unitTag names.UnitTag
	abort   <-chan struct{}
	stateRW UnitStateReadWriter

	// pending is the set of tags for storage attachments
	// for which no hooks have been run.
	pending names.Set

	stateOps *stateOps

	// TODO: hml
	// Can this be a names.Set?
	storageState *State
	// current storage attachments
	storageAttachments map[names.StorageTag]*contextStorage
}

// NewAttachments returns a new Attachments.
func NewAttachments(
	st StorageAccessor,
	tag names.UnitTag,
	rw UnitStateReadWriter,
	abort <-chan struct{},
) (*Attachments, error) {
	a := &Attachments{
		st:                 st,
		unitTag:            tag,
		abort:              abort,
		stateOps:           NewStateOps(rw),
		storageAttachments: make(map[names.StorageTag]*contextStorage),
		pending:            names.NewSet(),
	}
	if err := a.init(); err != nil {
		return nil, err
	}
	return a, nil
}

// init processes the storage State directory and creates storagers
// for the State files found.
func (a *Attachments) init() error {
	// Query all remote, known storage attachments for the unit,
	// so we can store current context, and find pending storage.
	attachmentIds, err := a.st.UnitStorageAttachments(a.unitTag)
	if err != nil {
		return errors.Annotate(err, "getting unit attachments")
	}
	attachmentsByTag := names.NewSet()
	for _, attachmentId := range attachmentIds {
		storageTag, err := names.ParseStorageTag(attachmentId.StorageTag)
		if err != nil {
			return errors.Trace(err)
		}
		attachmentsByTag.Add(storageTag)
	}
	existingStorageState, err := a.stateOps.Read()
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotate(err, "reading storage State")
	}
	newStateStorage := NewState()
	for _, tag := range attachmentsByTag.Values() {
		storageTag := tag.(names.StorageTag)
		attached, ok := existingStorageState.Attached(storageTag.Id())
		if !ok {
			// There is no State for the attachment, so no
			// hooks have been committed for it.
			a.pending.Add(storageTag)
		}
		if !attached {
			continue
		}
		// Since there's a saved record, we must previously have handled
		// at least "storage-attached", so there is no possibility of
		// short-circuiting the storage's removal.
		attachment, err := a.st.StorageAttachment(storageTag, a.unitTag)
		if err != nil {
			return errors.Annotatef(
				err, "querying storage attachment %q",
				storageTag.Id(),
			)
		}
		a.storageAttachments[storageTag] =
			&contextStorage{
				tag:      storageTag,
				kind:     storage.StorageKind(attachment.Kind),
				location: attachment.Location,
			}
		newStateStorage.Attach(storageTag.Id())
	}
	a.storageState = newStateStorage
	if a.storageState.Empty() {
		return nil
	}
	if err := a.stateOps.Write(newStateStorage); err != nil {
		return err
	}

	return nil
}

// SetDying ensures that any unprovisioned storage attachments are removed
// from State.
func (a *Attachments) SetDying() error {
	if err := a.st.DestroyUnitStorageAttachments(a.unitTag); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Pending reports the number of storage attachments whose hooks have yet
// to be run and committed.
func (a *Attachments) Pending() int {
	return a.pending.Size()
}

// Empty reports whether or not there are any active storage attachments.
func (a *Attachments) Empty() bool {
	return len(a.storageAttachments) == 0
}

// Storage returns the ContextStorage with the supplied tag if it was
// found, and whether it was found.
func (a *Attachments) Storage(tag names.StorageTag) (jujuc.ContextStorageAttachment, error) {
	if attachment, ok := a.storageAttachments[tag]; ok {
		return attachment, nil
	}
	return nil, errors.NotFoundf("storage")
}

// StorageTags returns the names.StorageTags for the active storage attachments.
func (a *Attachments) StorageTags() ([]names.StorageTag, error) {
	tags := names.NewSet()
	for tag := range a.storageAttachments {
		tags.Add(tag)
	}
	storageTags := make([]names.StorageTag, tags.Size())
	for i, tag := range tags.SortedValues() {
		storageTags[i] = tag.(names.StorageTag)
	}
	return storageTags, nil
}

// ValidateHook validates the hook against the current State.
func (a *Attachments) ValidateHook(hi hook.Info) error {
	return a.storageState.ValidateHook(hi)
}

// CommitHook persists the State change encoded in the supplied storage
// hook, or returns an error if the hook is invalid given current State.
func (a *Attachments) CommitHook(hi hook.Info) error {
	if !hi.Kind.IsStorage() {
		return errors.Errorf("not a storage hook: %#v", hi)
	}
	if hi.Kind == hooks.StorageDetaching {
		err := a.storageState.Detach(hi.StorageId)
		if err != nil {
			return errors.Errorf("unknown storage %q", hi.StorageId)
		}
	} else {
		a.storageState.Attach(hi.StorageId)
	}
	if err := a.stateOps.Write(a.storageState); err != nil {
		return err
	}

	storageTag := names.NewStorageTag(hi.StorageId)
	switch hi.Kind {
	case hooks.StorageAttached:
		a.pending.Remove(storageTag)
	case hooks.StorageDetaching:
		if err := a.removeStorageAttachment(storageTag); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (a *Attachments) removeStorageAttachment(tag names.StorageTag) error {
	if err := a.st.RemoveStorageAttachment(tag, a.unitTag); err != nil {
		return errors.Annotate(err, "removing storage attachment")
	}
	a.pending.Remove(tag)
	delete(a.storageAttachments, tag)
	return nil
}
