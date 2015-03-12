// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/operation"
)

func addStoppedFieldToUniterState(context Context) error {
	logger.Tracef("entering upgrade step addStoppedFieldToUniterState")
	defer logger.Tracef("leaving upgrade step addStoppedFieldToUniterState")

	config := context.AgentConfig()
	agentTag := config.Tag()
	tag, ok := agentTag.(names.UnitTag)
	if !ok {
		logger.Debugf("agent %s is not a Unit, skipping", agentTag)
		return nil
	}

	statefile := getUniterStateFile(config, tag)
	state, err := statefile.Read()
	switch err {
	case nil:
		return performUpgrade(statefile, state)
	case operation.ErrNoStateFile:
		logger.Debugf("no operations file found for unit %s, skipping", tag)
		return nil
	default:
		return err
	}

}

func getUniterStateFile(config agent.ConfigSetter, tag names.UnitTag) *operation.StateFile {
	dataDir := config.DataDir()
	paths := uniter.NewPaths(dataDir, tag)
	opsFile := paths.State.OperationsFile
	return operation.NewStateFile(opsFile)
}

func performUpgrade(statefile *operation.StateFile, state *operation.State) error {
	switch state.Kind {
	case operation.Continue:
		if state.Hook != nil && state.Hook.Kind == hooks.Stop {
			state.Stopped = true
			state.Hook = nil
			return statefile.Write(state)
		}
	}
	return nil
}
