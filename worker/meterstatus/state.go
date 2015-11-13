// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"os"
	"time"

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
	Code         string        `yaml:"status-code"`
	Info         string        `yaml:"status-info"`
	Disconnected *Disconnected `yaml:"disconnected,omitempty"`
}

// Disconnected stores the information relevant to the inactive meter status worker.
type Disconnected struct {
	Disconnected int64       `yaml:"disconnected-at,omitempty"`
	State        WorkerState `yaml:"disconnected-state,omitempty"`
}

// When returns the time when the unit was disconnected.
func (d Disconnected) When() time.Time {
	return time.Unix(d.Disconnected, 0)
}

// Read reads the current meter status information from disk.
func (f *StateFile) Read() (string, string, *Disconnected, error) {
	var st state
	if err := utils.ReadYaml(f.path, &st); err != nil {
		if os.IsNotExist(err) {
			return "", "", nil, nil
		}
		return "", "", nil, errors.Trace(err)
	}

	return st.Code, st.Info, st.Disconnected, nil
}

// Write stores the supplied status information to disk.
func (f *StateFile) Write(code, info string, disconnected *Disconnected) error {
	st := state{
		Code:         code,
		Info:         info,
		Disconnected: disconnected,
	}

	return errors.Trace(utils.WriteYaml(f.path, st))
}
