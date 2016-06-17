// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package storage contains the storage subsystem for the uniter, responding
// to changes in storage attachments (lifecycle, volume/filesystem details)
// by queuing hooks and managing the storage attachments' lifecycle.
package storage

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable/hooks"
	"gopkg.in/juju/names.v2"

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

type storageAttachment struct {
	*stateFile
	jujuc.ContextStorageAttachment
}

// Attachments generates storage hooks in response to changes to
// storage attachments, and provides access to information about
// storage attachments to hooks.
type Attachments struct {
	st              StorageAccessor
	unitTag         names.UnitTag
	abort           <-chan struct{}
	storageStateDir string

	// pending is the set of tags for storage attachments
	// for which no hooks have been run.
	pending set.Tags

	// current storage attachments
	storageAttachments map[names.StorageTag]storageAttachment
}

// NewAttachments returns a new Attachments.
func NewAttachments(
	st StorageAccessor,
	tag names.UnitTag,
	storageStateDir string,
	abort <-chan struct{},
) (*Attachments, error) {
	a := &Attachments{
		st:                 st,
		unitTag:            tag,
		abort:              abort,
		storageAttachments: make(map[names.StorageTag]storageAttachment),
		storageStateDir:    storageStateDir,
		pending:            make(set.Tags),
	}
	if err := a.init(); err != nil {
		return nil, err
	}
	return a, nil
}

// init processes the storage state directory and creates storagers
// for the state files found.
func (a *Attachments) init() error {
	if err := os.MkdirAll(a.storageStateDir, 0755); err != nil {
		return errors.Annotate(err, "creating storage state dir")
	}
	// Query all remote, known storage attachments for the unit,
	// so we can cull state files, and store current context.
	attachmentIds, err := a.st.UnitStorageAttachments(a.unitTag)
	if err != nil {
		return errors.Annotate(err, "getting unit attachments")
	}
	attachmentsByTag := make(map[names.StorageTag]struct{})
	for _, attachmentId := range attachmentIds {
		storageTag, err := names.ParseStorageTag(attachmentId.StorageTag)
		if err != nil {
			return errors.Trace(err)
		}
		attachmentsByTag[storageTag] = struct{}{}
	}
	stateFiles, err := readAllStateFiles(a.storageStateDir)
	if err != nil {
		return errors.Annotate(err, "reading storage state dirs")
	}
	for storageTag, stateFile := range stateFiles {
		if _, ok := attachmentsByTag[storageTag]; !ok {
			// We have previously removed the storage from state,
			// but did not remove the state file. Remove the file.
			if err := stateFile.Remove(); err != nil {
				return errors.Trace(err)
			}
			continue
		}
		// Since there's a state file, we must previously have handled
		// at least "storage-attached", so there is no possibility of
		// short-circuiting the storage's removal.
		attachment, err := a.st.StorageAttachment(storageTag, a.unitTag)
		if err != nil {
			return errors.Annotatef(
				err, "querying storage attachment %q",
				storageTag.Id(),
			)
		}
		a.storageAttachments[storageTag] = storageAttachment{
			stateFile,
			&contextStorage{
				tag:      storageTag,
				kind:     storage.StorageKind(attachment.Kind),
				location: attachment.Location,
			},
		}
	}
	for storageTag := range attachmentsByTag {
		if _, ok := stateFiles[storageTag]; !ok {
			// There is no state file for the attachment, so no
			// hooks have been committed for it.
			a.pending.Add(storageTag)
		}
		// Non-locally recorded attachments will be further handled
		// by the resolver.
	}
	return nil
}

// SetDying ensures that any unprovisioned storage attachments are removed
// from state.
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
	tags := set.NewTags()
	for tag := range a.storageAttachments {
		tags.Add(tag)
	}
	storageTags := make([]names.StorageTag, tags.Size())
	for i, tag := range tags.SortedValues() {
		storageTags[i] = tag.(names.StorageTag)
	}
	return storageTags, nil
}

// ValidateHook validates the hook against the current state.
func (a *Attachments) ValidateHook(hi hook.Info) error {
	storageState, err := a.storageStateForHook(hi)
	if err != nil {
		return errors.Trace(err)
	}
	return storageState.ValidateHook(hi)
}

// CommitHook persists the state change encoded in the supplied storage
// hook, or returns an error if the hook is invalid given current state.
func (a *Attachments) CommitHook(hi hook.Info) error {
	storageState, err := a.storageStateForHook(hi)
	if err != nil {
		return errors.Trace(err)
	}
	if err := storageState.CommitHook(hi); err != nil {
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

func (a *Attachments) storageStateForHook(hi hook.Info) (*stateFile, error) {
	if !hi.Kind.IsStorage() {
		return nil, errors.Errorf("not a storage hook: %#v", hi)
	}
	storageAttachment, ok := a.storageAttachments[names.NewStorageTag(hi.StorageId)]
	if !ok {
		return nil, errors.Errorf("unknown storage %q", hi.StorageId)
	}
	return storageAttachment.stateFile, nil
}
