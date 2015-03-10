// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type State interface {
	hook.Committer
	hook.Validator
}

type StorageHookQueue interface {
	Empty() bool
	Next() hook.Info
	Pop()
	Update(attachment params.StorageAttachment) error
	Context() (jujuc.ContextStorage, bool)
}

func NewStorageHookQueue(
	unitTag names.UnitTag,
	storageTag names.StorageTag,
	attached bool,
) StorageHookQueue {
	return &storageHookQueue{
		unitTag:    unitTag,
		storageTag: storageTag,
		attached:   attached,
	}
}

func NewStorageSource(
	st StorageAccessor,
	unitTag names.UnitTag,
	storageTag names.StorageTag,
	attached bool,
) (hook.Source, error) {
	source, err := newStorageSource(st, unitTag, storageTag, attached)
	return source, err
}
