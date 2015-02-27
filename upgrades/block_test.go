// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/block"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
)

type blockSuite struct {
	jujutesting.JujuConnSuite
	ctx         upgrades.Context
	blockClient *block.Client
}

var _ = gc.Suite(&blockSuite{})

func (s *blockSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	conn, err := juju.NewAPIState(s.AdminUserTag(c), s.Environ, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { conn.Close() })

	s.ctx = &mockContext{
		agentConfig: &mockAgentConfig{dataDir: s.DataDir()},
		apiState:    conn,
		state:       s.State,
	}
	s.blockClient = block.NewClient(conn)
}

func (s *blockSuite) TestUpdateBlocksNone(c *gc.C) {
	err := upgrades.MoveBlocksFromEnvironToState(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.ensureBlocksUpdated(c, nil)
	s.ensureBlocksRemovedFromEnvConfig(c)
}

func (s *blockSuite) ensureBlocksRemovedFromEnvConfig(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	attrs := cfg.AllAttrs()
	_, exists := attrs["block-destroy-environment"]
	c.Assert(exists, jc.IsFalse)
	_, exists = attrs["block-remove-object"]
	c.Assert(exists, jc.IsFalse)
	_, exists = attrs["block-all-changes"]
	c.Assert(exists, jc.IsFalse)
}

func (s *blockSuite) ensureBlocksUpdated(c *gc.C, expected []string) {
	blocks, err := s.blockClient.List()
	c.Assert(err, jc.ErrorIsNil)

	var types []string
	for _, ablock := range blocks {
		types = append(types, ablock.Type)
	}
	c.Assert(types, jc.SameContents, expected)
}

func (s *blockSuite) TestUpgradeBlocks(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{
		"block-destroy-environment": true,
		"block-remove-object":       true,
		"block-all-changes":         true,
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = upgrades.MoveBlocksFromEnvironToState(s.ctx)

	c.Assert(err, jc.ErrorIsNil)
	s.ensureBlocksUpdated(c, []string{
		state.ChangeBlock.String(),
		state.DestroyBlock.String(),
		state.RemoveBlock.String(),
	})
	s.ensureBlocksRemovedFromEnvConfig(c)
}
