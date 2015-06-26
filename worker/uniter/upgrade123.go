// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"os"

	"github.com/juju/names"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/worker/uniter/operation"
)

func AddStoppedFieldToUniterState(tag names.UnitTag, dataDir string) error {
	logger.Tracef("entering upgrade step AddStoppedFieldToUniterState")
	defer logger.Tracef("leaving upgrade step AddStoppedFieldToUniterState")

	opsFile := getUniterStateFile(dataDir, tag)
	state, err := readUnsafe(opsFile)
	switch err {
	case nil:
		return performUpgrade(opsFile, state)
	case operation.ErrNoStateFile:
		logger.Warningf("no uniter state file found for unit %s, skipping uniter upgrade step", tag)
		return nil
	default:
		return err
	}

}

func getUniterStateFile(dataDir string, tag names.UnitTag) string {
	paths := NewPaths(dataDir, tag)
	return paths.State.OperationsFile
}

func performUpgrade(opsFile string, state *operation.State) error {
	statefile := operation.NewStateFile(opsFile)
	if state.Kind == operation.Continue && state.Hook != nil && state.Hook.Kind == hooks.Stop {
		state.Stopped = true
		state.Hook = nil
		return statefile.Write(state)
	}
	return nil
}

func readUnsafe(opsfile string) (*operation.State, error) {
	var st operation.State
	if err := utils.ReadYaml(opsfile, &st); err != nil {
		if os.IsNotExist(err) {
			return nil, operation.ErrNoStateFile
		}
	}
	return &st, nil
}
