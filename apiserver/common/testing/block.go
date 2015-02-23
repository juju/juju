// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/block"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
)

type BlockSwitch struct {
	ApiState *api.State
	client   *block.Client
}

// NewBlockSwitch creates a block switch used in testing
// to manage desired juju blocks.
func NewBlockSwitch(st *api.State) BlockSwitch {
	return BlockSwitch{
		ApiState: st,
		client:   block.NewClient(st),
	}
}

// on switches on desired block and
// asserts that no errors were encountered.
func (s BlockSwitch) on(c *gc.C, blockType multiwatcher.BlockType, msg string) {
	c.Assert(
		s.client.SwitchBlockOn(
			fmt.Sprintf("%v", blockType),
			msg),
		gc.IsNil)
}

// BlockAllChanges blocks all operations that could change environment.
func (s BlockSwitch) BlockAllChanges(c *gc.C, msg string) {
	s.on(c, multiwatcher.BlockChange, msg)
}

// BlockRemoveObject blocks all operations that remove
// machines, services, units or relations.
func (s BlockSwitch) BlockRemoveObject(c *gc.C, msg string) {
	s.on(c, multiwatcher.BlockRemove, msg)
}

// BlockDestroyEnvironment blocks destroy-environment.
func (s BlockSwitch) BlockDestroyEnvironment(c *gc.C, msg string) {
	s.on(c, multiwatcher.BlockDestroy, msg)
}

// AssertErrorBlocked checks if given error is
// related to switched block.
func (s BlockSwitch) AssertErrorBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, msg)
}
