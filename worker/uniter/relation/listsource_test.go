// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/relation"
)

type ListSourceSuite struct{}

var _ = gc.Suite(&ListSourceSuite{})

func (s *ListSourceSuite) TestNoUpdates(c *gc.C) {
	source := relation.NewListSource(hookList(hooks.Start, hooks.Stop))
	c.Check(source.Changes(), gc.IsNil)

	err := source.Update(params.RelationUnitsChange{})
	c.Check(err, gc.ErrorMatches, "HookSource does not accept updates")

	err = source.Stop()
	c.Check(err, gc.IsNil)
}

func (s *ListSourceSuite) TestQueue(c *gc.C) {
	for i, test := range [][]hook.Info{
		hookList(),
		hookList(hooks.Install, hooks.Install),
		hookList(hooks.Stop, hooks.Start, hooks.Stop),
	} {
		c.Logf("test %d: %v", i, test)
		source := relation.NewListSource(test)
		for _, expect := range test {
			c.Check(source.Empty(), jc.IsFalse)
			c.Check(source.Next(), gc.DeepEquals, expect)
			source.Pop()
		}
		c.Check(source.Empty(), jc.IsTrue)
		c.Check(source.Next, gc.PanicMatches, "HookSource is empty")
		c.Check(source.Pop, gc.PanicMatches, "HookSource is empty")
	}
}
