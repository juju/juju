// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/operation"
)

func AddStoppedFieldToUniterState(tag names.UnitTag, dataDir string) error {
	logger.Tracef("entering upgrade step addStoppedFieldToUniterState")
	defer logger.Tracef("leaving upgrade step addStoppedFieldToUniterState")

	statefile := getUniterStateFile(dataDir, tag)
	state, err := statefile.ReadUnsafe()
	switch err {
	case nil:
		return performUpgrade(statefile, state)
	case operation.ErrNoStateFile:
		logger.Errorf("no operations file found for unit %s, skipping", tag)
		return nil
	default:
		return err
	}

}

func getUniterStateFile(dataDir string, tag names.UnitTag) *operation.StateFile {
	paths := NewPaths(dataDir, tag)
	opsFile := paths.State.OperationsFile
	return operation.NewStateFile(opsFile)
}

func performUpgrade(statefile *operation.StateFile, state *operation.State) error {
	if state.Kind == operation.Continue {
		if state.Hook != nil && state.Hook.Kind == hooks.Stop {
			state.Stopped = true
			state.Hook = nil
			return statefile.Write(state)
		}
	}
	return nil
}
