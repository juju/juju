// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/juju/charm/hooks"
	"github.com/juju/names/v5"

	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/hook"
)

// Attachments generates storage hooks in response to changes to
// storage attachments.
type Attachments struct {
	client  api.StorageAccessor
	unitTag names.UnitTag
	abort   <-chan struct{}

	// pending is the set of tags for storage attachments
	// for which no hooks have been run.
	pending names.Set

	stateOps *stateOps

	// TODO: hml
	// Can this be a names.Set?
	storageState *State
}

// NewAttachments returns a new Attachments.
func NewAttachments(
	client api.StorageAccessor,
	tag names.UnitTag,
	rw UnitStateReadWriter,
	abort <-chan struct{},
) (*Attachments, error) {
	a := &Attachments{
		client:   client,
		unitTag:  tag,
		abort:    abort,
		stateOps: NewStateOps(rw),
		pending:  names.NewSet(),
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
	attachmentIds, err := a.client.UnitStorageAttachments(a.unitTag)
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
	if err != nil && !errors.Is(err, errors.NotFound) {
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
	if err := a.client.DestroyUnitStorageAttachments(a.unitTag); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Pending reports the number of storage attachments whose hooks have yet
// to be run and committed.
func (a *Attachments) Pending() int {
	return a.pending.Size()
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
	if err := a.client.RemoveStorageAttachment(tag, a.unitTag); err != nil {
		return errors.Annotate(err, "removing storage attachment")
	}
	a.pending.Remove(tag)
	return nil
}
