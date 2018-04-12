// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

type blockAccess interface {
	AllBlocks() ([]state.Block, error)
	SwitchBlockOn(t state.BlockType, msg string) error
	SwitchBlockOff(t state.BlockType) error
	ModelTag() names.ModelTag
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}
