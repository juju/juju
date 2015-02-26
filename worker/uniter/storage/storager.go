// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/worker/uniter/hook"
)

type storager struct {
	st         StorageAccessor
	unitTag    names.UnitTag
	storageTag names.StorageTag
	source     *storageSource
	sender     hook.Sender
}

func newStorager(
	st StorageAccessor,
	unitTag names.UnitTag,
	storageTag names.StorageTag,
	hooks chan<- hook.Info,
) (*storager, error) {
	source, err := newStorageSource(st, unitTag, storageTag)
	if err != nil {
		return nil, errors.Annotate(err, "creating storage event source")
	}
	sender := hook.NewSender(hooks, source)
	return &storager{
		st:         st,
		unitTag:    unitTag,
		storageTag: storageTag,
		source:     source,
		sender:     sender,
	}, nil
}

func (s *storager) Stop() error {
	if err := s.sender.Stop(); err != nil {
		return errors.Annotate(err, "stopping storage event sender")
	}
	return s.source.Stop()
}

func (s *storager) Context() *contextStorage {
	// TODO(axw) source may be updating the context while
	// we're reading it; ensure that that doesn't happen
	// until Pop() is called.
	return s.source.context
}
