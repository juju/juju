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

func StateAttached(s State) bool {
	return s.(*stateFile).attached
}

func ValidateHook(tag names.StorageTag, attached bool, hi hook.Info) error {
	st := &state{tag, attached}
	return st.ValidateHook(hi)
}

func ReadStateFile(dirPath string, tag names.StorageTag) (d State, err error) {
	state, err := readStateFile(dirPath, tag)
	return state, err
}

func ReadAllStateFiles(dirPath string) (map[names.StorageTag]State, error) {
	files, err := readAllStateFiles(dirPath)
	if err != nil {
		return nil, err
	}
	states := make(map[names.StorageTag]State)
	for tag, f := range files {
		states[tag] = f
	}
	return states, nil
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
