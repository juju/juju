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
	"gopkg.in/juju/charm.v5-unstable/hooks"

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

	// UnitStorageAttachments returns details of all of the storage
	// attachments for the unit with the specified tag.
	UnitStorageAttachments(names.UnitTag) ([]params.StorageAttachment, error)

	// EnsureStorageAttachmentDead ensures that the storage attachment
	// with the specified unit and storage tags is Dead.
	EnsureStorageAttachmentDead(names.StorageTag, names.UnitTag) error

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
	attachments, err := a.st.UnitStorageAttachments(a.unitTag)
	if err != nil {
		return errors.Annotate(err, "getting unit attachments")
	}
	attachmentsByTag := make(map[names.StorageTag]*params.StorageAttachment)
	for i, attachment := range attachments {
		storageTag, err := names.ParseStorageTag(attachment.StorageTag)
		if err != nil {
			return errors.Trace(err)
		}
		attachmentsByTag[storageTag] = &attachments[i]
	}
	stateFiles, err := readAllStateFiles(a.storageStateDir)
	if err != nil {
		return errors.Annotate(err, "reading storage state dirs")
	}
	for storageTag, stateFile := range stateFiles {
		_, ok := attachmentsByTag[storageTag]
		if !ok {
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
	// Note: we ignore remote attachments that were not locally recorded;
	// they will be picked up by UpdateStorage. We could handle it now, but
	// we're going to have to refresh the attachment in response to the
	// watcher anyway.
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

// UpdateStorage responds to changes in the lifecycle states of the
// storage attachments corresponding to the supplied storage tags,
// sending storage hooks on the channel returned by Hooks().
func (a *Attachments) UpdateStorage(tags []names.StorageTag) error {
	for _, storageTag := range tags {
		if err := a.updateOneStorage(storageTag); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (a *Attachments) updateOneStorage(storageTag names.StorageTag) error {
	// Fetch the attachment's remote state, so we know when we can
	// stop the storager and possibly short-circuit the attachment's
	// removal.
	att, err := a.st.StorageAttachment(storageTag, a.unitTag)
	if err != nil {
		return errors.Trace(err)
	}
	stateFile, err := readStateFile(a.storageStateDir, storageTag)
	if err != nil {
		return errors.Trace(err)
	}
	storager := a.storagers[storageTag]

	switch att.Life {
	case params.Dying:
		if stateFile.state.attached {
			// Previously ran storage-attached, so we'll
			// need to run a storage-detaching hook.
			if storager == nil {
				panic("missing storager for attached storage")
			}
			// TODO(axw) have storager watch both storage
			// attachment and volume/filesystem attachment,
			// else we need to force updates from here.
			return nil
		}
		// Storage was not previously seen as Dying, so we can
		// short-circuit the storage's death and removal.
		if err := a.st.EnsureStorageAttachmentDead(storageTag, a.unitTag); err != nil {
			return errors.Annotate(err, "ensuring storage is dead")
		}
		fallthrough
	case params.Dead:
		if storager != nil {
			// Stopping the storager guarantees that no hooks will
			// be delivered, which is a crucial requirement for
			// short-circuiting.
			if err := storager.Stop(); err != nil {
				return errors.Trace(err)
			}
			delete(a.storagers, storageTag)
		}
		if err := a.st.RemoveStorageAttachment(storageTag, a.unitTag); err != nil {
			return errors.Annotate(err, "removing storage")
		}
		return nil
	}

	if storager == nil {
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

// Storage returns the ContextStorage with the supplied tag if it was
// found, and whether it was found.
func (a *Attachments) Storage(tag names.StorageTag) (jujuc.ContextStorage, bool) {
	if s, ok := a.storagers[tag]; ok {
		return s.Context()
	}
	return nil, false
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
	if hi.Kind == hooks.StorageDetached {
		// Progress storage attachment to Dead.
		storageTag := names.NewStorageTag(hi.StorageId)
		if err := a.st.EnsureStorageAttachmentDead(storageTag, a.unitTag); err != nil {
			return errors.Annotate(err, "ensuring storage is dead")
		}
	}
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
