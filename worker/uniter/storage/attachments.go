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
	"github.com/juju/names"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var logger = loggo.GetLogger("juju.worker.uniter.storage")

// StorageAccessor is an interface for accessing information about
// storage attachments.
type StorageAccessor interface {
	// WatchStorageAttachment starts a watcher for changes to the
	// storage attachment with the specified unit and storage tags.
	WatchStorageAttachment(names.StorageTag, names.UnitTag) (watcher.NotifyWatcher, error)

	// StorageAttachment returns details of the storage attachment
	// with the specified unit and storage tags.
	StorageAttachment(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)

	// StorageAttachmentLife returns the lifecycle state of the specified
	// storage attachments.
	StorageAttachmentLife([]params.StorageAttachmentId) ([]params.LifeResult, error)

	// UnitStorageAttachments returns details of all of the storage
	// attachments for the unit with the specified tag.
	UnitStorageAttachments(names.UnitTag) ([]params.StorageAttachmentId, error)

	// DestroyUnitStorageAttachments ensures that all storage
	// attachments for the specified unit will be removed at
	// some point in the future.
	DestroyUnitStorageAttachments(names.UnitTag) error

	// RemoveStorageAttachment removes that the storage attachment
	// with the specified unit and storage tags. This method is only
	// expected to succeed if the storage attachment is Dead.
	RemoveStorageAttachment(names.StorageTag, names.UnitTag) error
}

// Attachments generates storage hooks in response to changes to
// storage attachments, and provides access to information about
// storage attachments to hooks.
type Attachments struct {
	st              StorageAccessor
	unitTag         names.UnitTag
	abort           <-chan struct{}
	hooks           chan hook.Info
	storagers       map[names.StorageTag]*storager
	storageStateDir string

	// pending is the set of tags for storage attachments
	// for which no hooks have been run.
	pending set.Tags
}

// NewAttachments returns a new Attachments.
func NewAttachments(
	st StorageAccessor,
	tag names.UnitTag,
	storageStateDir string,
	abort <-chan struct{},
) (*Attachments, error) {
	a := &Attachments{
		st:              st,
		unitTag:         tag,
		abort:           abort,
		hooks:           make(chan hook.Info),
		storagers:       make(map[names.StorageTag]*storager),
		storageStateDir: storageStateDir,
		pending:         make(set.Tags),
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
			if err := stateFile.Remove(); err != nil {
				return errors.Trace(err)
			}
			continue
		}
		// Since there's a state file, we must previously have handled
		// at least "storage-attached", so there is no possibility of
		// short-circuiting the storage's removal.
		if err := a.add(storageTag, stateFile); err != nil {
			return errors.Trace(err)
		}
	}
	for storageTag := range attachmentsByTag {
		if _, ok := stateFiles[storageTag]; !ok {
			// There is no state file for the attachment, so no
			// hooks have been committed for it.
			a.pending.Add(storageTag)
		}
		// Non-locally recorded attachments will be further handled
		// by UpdateStorage.
	}
	return nil
}

// Hooks returns the channel on which storage hook execution requests
// are sent.
func (a *Attachments) Hooks() <-chan hook.Info {
	return a.hooks
}

// Stop stops all of the storagers.
func (a *Attachments) Stop() error {
	for _, s := range a.storagers {
		if err := s.Stop(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// SetDying ensures that any unprovisioned storage attachments are removed
// from state, and Pending is updated. After SetDying returns successfully,
// and once Pending returns zero and Empty returns true, there will be no
// remaining storage attachments.
func (a *Attachments) SetDying() error {
	if err := a.st.DestroyUnitStorageAttachments(a.unitTag); err != nil {
		return errors.Trace(err)
	}
	if err := a.Refresh(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Refresh fetches all of the unit's storage attachments and processes each
// one as in UpdateStorage.
func (a *Attachments) Refresh() error {
	attachmentIds, err := a.st.UnitStorageAttachments(a.unitTag)
	if err != nil {
		return errors.Annotate(err, "getting unit attachments")
	}
	storageTags := make([]names.StorageTag, len(attachmentIds))
	for i, attachmentId := range attachmentIds {
		storageTag, err := names.ParseStorageTag(attachmentId.StorageTag)
		if err != nil {
			return errors.Trace(err)
		}
		storageTags[i] = storageTag
	}
	// Remove non-existent storage from pending.
	for pending := range a.pending {
		var found bool
		for _, active := range storageTags {
			if pending == active {
				found = true
				break
			}
		}
		if !found {
			a.pending.Remove(pending)
		}
	}
	return a.UpdateStorage(storageTags)
}

// UpdateStorage responds to changes in the lifecycle states of the
// storage attachments corresponding to the supplied storage tags,
// sending storage hooks on the channel returned by Hooks().
func (a *Attachments) UpdateStorage(tags []names.StorageTag) error {
	ids := make([]params.StorageAttachmentId, len(tags))
	for i, storageTag := range tags {
		ids[i] = params.StorageAttachmentId{
			StorageTag: storageTag.String(),
			UnitTag:    a.unitTag.String(),
		}
	}
	results, err := a.st.StorageAttachmentLife(ids)
	if err != nil {
		return errors.Trace(err)
	}
	for i, result := range results {
		if result.Error == nil {
			continue
		} else if params.IsCodeNotFound(result.Error) {
			a.pending.Remove(tags[i])
			continue
		}
		return errors.Annotatef(
			result.Error, "getting life of storage %s attachment", tags[i].Id(),
		)
	}
	for i, result := range results {
		if result.Error != nil {
			continue
		}
		if err := a.updateOneStorage(tags[i], result.Life); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (a *Attachments) updateOneStorage(storageTag names.StorageTag, life params.Life) error {
	// Fetch the attachment's remote state, so we know when we can
	// stop the storager and possibly short-circuit the attachment's
	// removal.
	stateFile, err := readStateFile(a.storageStateDir, storageTag)
	if err != nil {
		return errors.Trace(err)
	}
	storager := a.storagers[storageTag]

	switch life {
	case params.Dying:
		if stateFile.state.attached {
			// Previously ran storage-attached, so we'll
			// leave the storager to handle the lifecycle
			// state change.
			if storager == nil {
				panic("missing storager for attached storage")
			}
			return nil
		}
		// Storage attachment hasn't previously been observed,
		// so we can short-circuit the removal.
		err := a.removeStorageAttachment(storageTag, storager)
		return errors.Trace(err)
	}

	if storager == nil {
		a.pending.Add(storageTag)
		return a.add(storageTag, stateFile)
	}
	return nil
}

// add creates a new storager for the specified storage tag.
func (a *Attachments) add(storageTag names.StorageTag, stateFile *stateFile) error {
	s, err := newStorager(a.st, a.unitTag, storageTag, stateFile, a.hooks)
	if err != nil {
		return errors.Annotatef(err, "watching storage %q", storageTag.Id())
	}
	a.storagers[storageTag] = s
	logger.Debugf("watching storage %q", storageTag.Id())
	return nil
}

// Pending reports the number of storage attachments whose hooks have yet
// to be run and committed.
func (a *Attachments) Pending() int {
	return a.pending.Size()
}

// Empty reports whether or not there are any active storage attachments.
func (a *Attachments) Empty() bool {
	return len(a.storagers) == 0
}

// Storage returns the ContextStorage with the supplied tag if it was
// found, and whether it was found.
func (a *Attachments) Storage(tag names.StorageTag) (jujuc.ContextStorageAttachment, bool) {
	if s, ok := a.storagers[tag]; ok {
		return s.Context()
	}
	return nil, false
}

// StorageTags returns the names.StorageTags for the active storage attachments.
func (a *Attachments) StorageTags() []names.StorageTag {
	tags := set.NewTags()
	for tag := range a.storagers {
		tags.Add(tag)
	}
	storageTags := make([]names.StorageTag, tags.Size())
	for i, tag := range tags.SortedValues() {
		storageTags[i] = tag.(names.StorageTag)
	}
	return storageTags
}

// ValidateHook validates the hook against the current state.
func (a *Attachments) ValidateHook(hi hook.Info) error {
	storager, err := a.storagerForHook(hi)
	if err != nil {
		return errors.Trace(err)
	}
	return storager.state.ValidateHook(hi)
}

// CommitHook persists the state change encoded in the supplied storage
// hook, or returns an error if the hook is invalid given current state.
func (a *Attachments) CommitHook(hi hook.Info) error {
	storager, err := a.storagerForHook(hi)
	if err != nil {
		return errors.Trace(err)
	}
	if err := storager.CommitHook(hi); err != nil {
		return err
	}
	storageTag := names.NewStorageTag(hi.StorageId)
	switch hi.Kind {
	case hooks.StorageAttached:
		a.pending.Remove(storageTag)
	case hooks.StorageDetaching:
		if err := a.removeStorageAttachment(storageTag, storager); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (a *Attachments) removeStorageAttachment(tag names.StorageTag, s *storager) error {
	if err := a.st.RemoveStorageAttachment(tag, a.unitTag); err != nil {
		return errors.Annotate(err, "removing storage attachment")
	}
	a.pending.Remove(tag)
	if s == nil {
		return nil
	}
	if err := s.Stop(); err != nil {
		return errors.Trace(err)
	}
	delete(a.storagers, tag)
	return nil
}

func (a *Attachments) storagerForHook(hi hook.Info) (*storager, error) {
	if !hi.Kind.IsStorage() {
		return nil, errors.Errorf("not a storage hook: %#v", hi)
	}
	storager, ok := a.storagers[names.NewStorageTag(hi.StorageId)]
	if !ok {
		return nil, errors.Errorf("unknown storage %q", hi.StorageId)
	}
	return storager, nil
}
