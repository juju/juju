// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var logger = loggo.GetLogger("juju.worker.uniter.storage")

// StorageAccessor is an interface for accessing information about
// storage attachments.
type StorageAccessor interface {
	// TODO(axw) this interface will contain the methods necessary for
	// both Attachments and storageSource.
}

// Attachments generates storage hooks in response to changes to
// storage attachments, and provides access to information about
// storage attachments to hooks.
type Attachments struct {
	st      StorageAccessor
	unitTag names.UnitTag
	abort   <-chan struct{}
	hooks   chan hook.Info
}

// NewAttachments returns a new Attachments.
func NewAttachments(st StorageAccessor, tag names.UnitTag, abort <-chan struct{}) (*Attachments, error) {
	hooks := make(chan hook.Info)
	// TODO(axw) at this point we would read the storage state from disk,
	// so we know which hooks to issue.
	return &Attachments{st, tag, abort, hooks}, nil
}

// Hooks returns the channel on which storage hook execution requests
// are sent.
func (a *Attachments) Hooks() <-chan hook.Info {
	return a.hooks
}

// UpdateStorage responds to changes in the lifecycle states of the
// storage attachments corresponding to the supplied storage tags,
// sending storage hooks on the channel returned by Hook().
func (a *Attachments) UpdateStorage(tags []names.StorageTag) error {
	// TODO(axw) this method will maintain storage stources based
	// on lifecycle states of storage attachments corresponding to
	// the specified tags. For each source, there will be a sender
	// that sends to a.hooks.
	return nil
}

// Storage returns the ContextStorage with the supplied tag if it was
// found, and whether it was found.
func (a *Attachments) Storage(tag names.StorageTag) (jujuc.ContextStorage, bool) {
	// TODO(axw) this method will return a jujuc.ContextStorage for
	// storage attachments that are tracked.
	return nil, false
}
