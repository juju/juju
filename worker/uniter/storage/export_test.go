// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/worker/uniter/hook"
)

type State interface {
	hook.Committer
	hook.Validator
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
