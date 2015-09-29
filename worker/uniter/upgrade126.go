// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/worker/uniter/operation"
)

// AddInstalledToUniterState sets the Installed boolean in state to true
// if the charm has been installed. The only occasion where this is not
// true is if we are currently installing.
func AddInstalledToUniterState(tag names.UnitTag, dataDir string) error {
	logger.Tracef("entering upgrade step AddInstalledToUniterState")
	defer logger.Tracef("leaving upgrade step AddInstalledToUniterState")

	opsFile := getUniterStateFile(dataDir, tag)
	state, err := readUnsafe(opsFile)
	switch err {
	case nil:
		return addInstalled(opsFile, state)
	case operation.ErrNoStateFile:
		logger.Warningf("no uniter state file found for unit %s, skipping uniter upgrade step", tag)
		return nil
	default:
		return err
	}
}

func addInstalled(opsFile string, state *operation.State) error {
	statefile := operation.NewStateFile(opsFile)
	if state.Kind == operation.Install {
		return nil
	}
	if state.Kind == operation.RunHook && state.Hook.Kind == hooks.Install {
		return nil
	}
	if !state.Installed {
		state.Installed = true
		return statefile.Write(state)
	}
	return nil
}
