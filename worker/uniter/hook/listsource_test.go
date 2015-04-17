// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hook_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/hook/hooktesting"
)

type ListSourceSuite struct{}

var _ = gc.Suite(&ListSourceSuite{})

func (s *ListSourceSuite) TestNoUpdates(c *gc.C) {
	source := hook.NewListSource(hooktesting.HookList(hooks.Start, hooks.Stop))
	c.Check(source.Changes(), gc.IsNil)

	ch := source.Changes()
	c.Check(ch, gc.IsNil)

	err := source.Stop()
	c.Check(err, jc.ErrorIsNil)
}

func (s *ListSourceSuite) TestQueue(c *gc.C) {
	for i, test := range [][]hook.Info{
		hooktesting.HookList(),
		hooktesting.HookList(hooks.Install, hooks.Install),
		hooktesting.HookList(hooks.Stop, hooks.Start, hooks.Stop),
	} {
		c.Logf("test %d: %v", i, test)
		source := hook.NewListSource(test)
		for _, expect := range test {
			c.Check(source.Empty(), jc.IsFalse)
			c.Check(source.Next(), gc.DeepEquals, expect)
			source.Pop()
		}
		c.Check(source.Empty(), jc.IsTrue)
		c.Check(source.Next, gc.PanicMatches, "Source is empty")
		c.Check(source.Pop, gc.PanicMatches, "Source is empty")
	}
}
