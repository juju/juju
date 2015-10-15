// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

// StateFile holds the meter status on disk.
type StateFile struct {
	path string
}

// NewStateFile creates a new file for persistent storage of
// the meter status.
func NewStateFile(path string) *StateFile {
	return &StateFile{path: path}
}

type state struct {
	Code string `yaml:"status-code"`
	Info string `yaml:"status-info"`
}

// Read reads the current meter status information from disk.
func (f *StateFile) Read() (string, string, error) {
	var st state
	if err := utils.ReadYaml(f.path, &st); err != nil {
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return "", "", errors.Trace(err)
	}
	return st.Code, st.Info, nil
}

// Write stores the supplied status information to disk.
func (f *StateFile) Write(code, info string) error {
	st := state{
		Code: code,
		Info: info,
	}
	return errors.Trace(utils.WriteYaml(f.path, st))
}
