// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/juju/state"
	names "gopkg.in/juju/names.v2"
)

type blockAccess interface {
	AllBlocks() ([]state.Block, error)
	SwitchBlockOn(t state.BlockType, msg string) error
	SwitchBlockOff(t state.BlockType) error
	ModelTag() names.ModelTag
}

type stateShim struct {
	*state.State
}
