// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"os"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/utils"
)

// LocalState is a cache of the state of the operator
// It is generally compared to the remote state of the
// the application as stored in the controller.
type LocalState struct {
	// CharmModifiedVersion increases any time the charm,
	// or any part of it, is changed in some way.
	CharmModifiedVersion int

	// CharmURL reports the currently installed charm URL. This is set
	// by the committing of deploy (install/upgrade) ops.
	CharmURL *charm.URL
}

// ErrNoStateFile is used to indicate an operator state file does not exist.
var ErrNoStateFile = errors.New("operator state file does not exist")

// StateFile holds the disk state for an operator.
type StateFile struct {
	path string
}

// NewStateFile returns a new StateFile using path.
func NewStateFile(path string) *StateFile {
	return &StateFile{path}
}

// Read reads a State from the file. If the file does not exist it returns
// ErrNoStateFile.
func (f *StateFile) Read() (*LocalState, error) {
	var st LocalState
	if err := utils.ReadYaml(f.path, &st); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoStateFile
		}
	}
	return &st, nil
}

// Write stores the supplied state to the file.
func (f *StateFile) Write(st *LocalState) error {
	return utils.WriteYaml(f.path, st)
}
