// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package storage contains the storage subsystem for the uniter, responding
// to changes in storage attachments (lifecycle, volume/filesystem details)
// by queuing hooks and managing the storage attachments' lifecycle.
package storage

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

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
}

// Attachments generates storage hooks in response to changes to
// storage attachments, and provides access to information about
// storage attachments to hooks.
type Attachments struct {
	hooks chan hook.Info
}

// NewAttachments returns a new Attachments.
func NewAttachments(
	st StorageAccessor,
	tag names.UnitTag,
	storageStateDir string,
	abort <-chan struct{},
) (*Attachments, error) {
	a := &Attachments{
		hooks: make(chan hook.Info),
	}
	if err := a.init(); err != nil {
		return nil, err
	}
	return a, nil
}

// init processes the storage state directory and creates storagers
// for the state files found.
func (a *Attachments) init() error {
	// TODO(axw) implement this in a follow-up.
	return nil
}

// Hooks returns the channel on which storage hook execution requests
// are sent.
func (a *Attachments) Hooks() <-chan hook.Info {
	return a.hooks
}

// Stop stops all of the storagers.
func (a *Attachments) Stop() error {
	// TODO(axw) implement this in a follow-up.
	return nil
}

// UpdateStorage responds to changes in the lifecycle states of the
// storage attachments corresponding to the supplied storage tags,
// sending storage hooks on the channel returned by Hooks().
func (a *Attachments) UpdateStorage(tags []names.StorageTag) error {
	// TODO(axw) implement this in a follow-up.
	return nil
}

// Storage returns the ContextStorage with the supplied tag if it was
// found, and whether it was found.
func (a *Attachments) Storage(tag names.StorageTag) (jujuc.ContextStorage, bool) {
	// TODO(axw) implement this in a follow-up.
	return nil, false
}

// ValidateHook validates the hook against the current state.
func (a *Attachments) ValidateHook(hi hook.Info) error {
	// TODO(axw) implement this in a follow-up.
	return errors.NotImplementedf("ValidateHook")
}

// CommitHook persists the state change encoded in the supplied storage
// hook, or returns an error if the hook is invalid given current state.
func (a *Attachments) CommitHook(hi hook.Info) error {
	// TODO(axw) implement this in a follow-up.
	return errors.NotImplementedf("CommitHook")
}
