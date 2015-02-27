// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/block"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

func moveBlocksFromEnvironToState(context Context) error {
	logger.Infof("recording existing blocks")
	st := context.State()
	if st == nil {
		logger.Debugf("no state connection, no block recording required")
		// We're running on a different node than the state server.
		return nil
	}
	blocks, err := getCurrentBlocks(st)

	if err != nil {
		return errors.Trace(err)
	}
	err = upgradeBlocks(context, blocks)
	if err != nil {
		return errors.Trace(err)
	}
	err = removeBlockEnvVar(st)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func upgradeBlocks(context Context, blocks []string) error {
	if len(blocks) == 0 {
		// no existing blocks = nothing to do here :)
		return nil
	}
	blockClient := block.NewClient(context.APIState())
	for _, one := range blocks {
		err := blockClient.SwitchBlockOn(one, "")
		if err != nil {
			return errors.Annotatef(err, "switching on %v", one)
		}
	}
	return nil
}

func getCurrentBlocks(st *state.State) ([]string, error) {
	cfg, err := getEnvironConfig(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return getBlocks(cfg), nil
}

func getEnvironConfig(st *state.State) (*config.Config, error) {
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return nil, errors.Annotatef(err, "reading current config")
	}
	return envConfig, nil
}

func getBlocks(cfg *config.Config) []string {
	var blocks []string
	addBlock := func(aType string) {
		blocks = append(blocks, aType)
	}

	if cfg.PreventAllChanges() {
		addBlock(state.ChangeBlock.String())
	}
	if cfg.PreventRemoveObject() {
		addBlock(state.RemoveBlock.String())
	}
	if cfg.PreventDestroyEnvironment() {
		addBlock(state.DestroyBlock.String())
	}
	return blocks
}

func removeBlockEnvVar(st *state.State) error {
	removeAttrs := []string{
		config.PreventAllChangesKey,
		config.PreventDestroyEnvironmentKey,
		config.PreventRemoveObjectKey,
	}
	return st.UpdateEnvironConfig(map[string]interface{}{}, removeAttrs, nil)
}
