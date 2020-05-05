// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils"

	"github.com/juju/juju/worker/uniter/operation"
)

// stateStepsFor276 returns upgrade steps for Juju 2.7.6.
func stepsFor276() []Step {
	return []Step{
		&upgradeStep{
			description: "add remote-application key to hooks in uniter state files",
			targets:     []Target{HostMachine},
			run:         AddRemoteApplicationToRunningHooks(uniterStateGlob),
		},
	}
}

const uniterStateGlob = `/var/lib/juju/agents/unit-*/state/uniter`

// AddRemoteApplicationToRunningHooks finds any uniter state files on
// the machine with running hooks, and makes sure that they contain a
// remote-application key.
func AddRemoteApplicationToRunningHooks(pattern string) func(Context) error {
	return func(_ Context) error {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return errors.Annotate(err, "finding uniter state files")
		}
		for _, path := range matches {
			// First, check whether the file needs rewriting.
			stateFile := newStateFile(path)
			_, err := stateFile.Read()
			if err == nil {
				// This one's fine, leave it alone.
				logger.Debugf("state file valid: %q", path)
				continue
			}

			err = AddRemoteApplicationToHook(path)
			if err != nil {
				return errors.Annotatef(err, "fixing %q", path)
			}
		}
		return nil
	}
}

// AddRemoteApplicationToHook takes a the path to a uniter state file
// that doesn't validate, and sets hook.remote-application to the
// remote application so that it does. (If it doesn't validate for
// some other reason we won't change the file.)
func AddRemoteApplicationToHook(path string) error {
	var uniterState map[string]interface{}
	err := utils.ReadYaml(path, &uniterState)
	if err != nil {
		return errors.Trace(err)
	}

	hookUnconverted, found := uniterState["hook"]
	if !found {
		logger.Warningf("no hook found in %q, unable to fix", path)
		return nil
	}

	hook, ok := hookUnconverted.(map[interface{}]interface{})
	if !ok {
		logger.Warningf("fixing %q: expected hook to be a map[interface{}]interface{}, got %T", path, hookUnconverted)
		return nil
	}

	if val, found := hook["remote-application"]; found {
		logger.Debugf("remote-application in %q set to %v already", path, val)
		return nil
	}

	unitUnconverted, found := hook["remote-unit"]
	if !found {
		logger.Warningf("fixing %q: remote-unit not found")
		return nil
	}

	unit, ok := unitUnconverted.(string)
	if !ok {
		logger.Warningf("fixing %q: expected remote-unit to be string, got %T", path, unitUnconverted)
		return nil
	}

	appName, err := names.UnitApplication(unit)
	if err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("setting remote-application to %q in %q", appName, path)
	hook["remote-application"] = appName
	return errors.Annotatef(utils.WriteYaml(path, uniterState),
		"writing updated state to %q", path)
}

// The uniter state file was removed in 2.8, the data stored in the controller.
// stateFile allows, AddRemoteApplicationToRunningHooks to run.  It's copied
// from the removed operation.StateFile

// stateFile holds the disk state for a uniter.
type stateFile struct {
	path string
}

// newStateFile returns a new StateFile using path.
func newStateFile(path string) *stateFile {
	return &stateFile{path}
}

var errNoStateFile = errors.New("uniter state file does not exist")

// Read reads a State from the file. If the file does not exist it returns
// errNoStateFile.
func (f *stateFile) Read() (*operation.State, error) {
	var st operation.State
	if err := utils.ReadYaml(f.path, &st); err != nil {
		if os.IsNotExist(err) {
			return nil, errNoStateFile
		}
	}
	if err := st.Validate(); err != nil {
		return nil, errors.Errorf("cannot read %q: %v", f.path, err)
	}
	return &st, nil
}
