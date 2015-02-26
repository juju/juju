// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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
	WatchStorageAttachment(names.StorageTag, names.UnitTag) (watcher.NotifyWatcher, error)
	StorageAttachment(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
}

// Attachments generates storage hooks in response to changes to
// storage attachments, and provides access to information about
// storage attachments to hooks.
type Attachments struct {
	st        StorageAccessor
	unitTag   names.UnitTag
	abort     <-chan struct{}
	hooks     chan hook.Info
	storagers map[names.StorageTag]*storager
}

// NewAttachments returns a new Attachments.
func NewAttachments(st StorageAccessor, tag names.UnitTag, abort <-chan struct{}) (*Attachments, error) {
	hooks := make(chan hook.Info)
	// TODO(axw) at this point we would read the storage state from disk,
	// so we know which hooks to issue.
	return &Attachments{
		st:        st,
		unitTag:   tag,
		abort:     abort,
		hooks:     hooks,
		storagers: make(map[names.StorageTag]*storager),
	}, nil
}

// Hooks returns the channel on which storage hook execution requests
// are sent.
func (a *Attachments) Hooks() <-chan hook.Info {
	return a.hooks
}

// UpdateStorage responds to changes in the lifecycle states of the
// storage attachments corresponding to the supplied storage tags,
// sending storage hooks on the channel returned by Hooks().
func (a *Attachments) UpdateStorage(tags []names.StorageTag) error {
	// TODO(axw) this method will maintain storage stources based
	// on lifecycle states of storage attachments corresponding to
	// the specified tags. For each source, there will be a sender
	// that sends to a.hooks.
	for _, tag := range tags {
		if _, ok := a.storagers[tag]; ok {
			continue
		}
		s, err := newStorager(a.st, a.unitTag, tag, a.hooks)
		if err != nil {
			return errors.Annotatef(err, "watching storage %q", tag.Id())
		}
		a.storagers[tag] = s
		logger.Debugf("watching storage %q", tag.Id())
	}
	return nil
}

// Storage returns the ContextStorage with the supplied tag if it was
// found, and whether it was found.
func (a *Attachments) Storage(tag names.StorageTag) (jujuc.ContextStorage, bool) {
	if s, ok := a.storagers[tag]; ok {
		return s.Context(), true
	}
	return nil, false
}
