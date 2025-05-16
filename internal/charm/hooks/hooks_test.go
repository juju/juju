// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package hooks

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

func TestHooksSuite(t *stdtesting.T) { tc.Run(t, &HooksSuite{}) }

type HooksSuite struct{}

func (s *HooksSuite) TestIsRelation(c *tc.C) {
	for _, h := range relationHooks {
		c.Assert(h.IsRelation(), tc.IsTrue)
	}
	for _, h := range unitHooks {
		c.Assert(h.IsRelation(), tc.IsFalse)
	}
}

func (s *HooksSuite) TestIsStorage(c *tc.C) {
	for _, h := range storageHooks {
		c.Assert(h.IsStorage(), tc.IsTrue)
	}
	for _, h := range unitHooks {
		c.Assert(h.IsStorage(), tc.IsFalse)
	}
}

func (s *HooksSuite) TestIsWorkload(c *tc.C) {
	for _, h := range workloadHooks {
		c.Assert(h.IsWorkload(), tc.IsTrue)
	}
	for _, h := range unitHooks {
		c.Assert(h.IsWorkload(), tc.IsFalse)
	}
}

func (s *HooksSuite) TestIsSecret(c *tc.C) {
	for _, h := range secretHooks {
		c.Assert(h.IsSecret(), tc.IsTrue)
	}
	for _, h := range unitHooks {
		c.Assert(h.IsSecret(), tc.IsFalse)
	}
}
