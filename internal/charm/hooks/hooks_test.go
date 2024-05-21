// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package hooks

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&HooksSuite{})

type HooksSuite struct{}

func (s *HooksSuite) TestIsRelation(c *gc.C) {
	for _, h := range relationHooks {
		c.Assert(h.IsRelation(), jc.IsTrue)
	}
	for _, h := range unitHooks {
		c.Assert(h.IsRelation(), jc.IsFalse)
	}
}

func (s *HooksSuite) TestIsStorage(c *gc.C) {
	for _, h := range storageHooks {
		c.Assert(h.IsStorage(), jc.IsTrue)
	}
	for _, h := range unitHooks {
		c.Assert(h.IsStorage(), jc.IsFalse)
	}
}

func (s *HooksSuite) TestIsWorkload(c *gc.C) {
	for _, h := range workloadHooks {
		c.Assert(h.IsWorkload(), jc.IsTrue)
	}
	for _, h := range unitHooks {
		c.Assert(h.IsWorkload(), jc.IsFalse)
	}
}

func (s *HooksSuite) TestIsSecret(c *gc.C) {
	for _, h := range secretHooks {
		c.Assert(h.IsSecret(), jc.IsTrue)
	}
	for _, h := range unitHooks {
		c.Assert(h.IsSecret(), jc.IsFalse)
	}
}
