// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type storager struct {
	st         StorageAccessor
	unitTag    names.UnitTag
	storageTag names.StorageTag
	state      *stateFile
	source     *storageSource
	sender     hook.Sender
}

// newStorager creates a new storager, watching for changes to the storage
// attachment with the specified tags, and generating hooks on the output
// channel.
func newStorager(
	st StorageAccessor,
	unitTag names.UnitTag,
	storageTag names.StorageTag,
	state *stateFile,
	hooks chan<- hook.Info,
) (*storager, error) {
	source, err := newStorageSource(st, unitTag, storageTag, state.attached)
	if err != nil {
		return nil, errors.Annotate(err, "creating storage event source")
	}
	sender := hook.NewSender(hooks, source)
	return &storager{
		st:         st,
		unitTag:    unitTag,
		storageTag: storageTag,
		state:      state,
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

func (s *storager) Context() (jujuc.ContextStorage, bool) {
	return s.source.Context()
}

func (s *storager) CommitHook(hi hook.Info) error {
	return s.state.CommitHook(hi)
}
