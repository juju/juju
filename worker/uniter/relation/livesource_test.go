// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker/uniter/relation"
)

type LiveSourceSuite struct{}

var _ = gc.Suite(&LiveSourceSuite{})

func (s *LiveSourceSuite) TestLiveHookSource(c *gc.C) {
	for i, t := range aliveHookQueueTests {
		c.Logf("test %d: %s", i, t.summary)
		ruw := &RUW{make(chan multiwatcher.RelationUnitsChange), false}
		q := relation.NewLiveHookSource(t.initial, ruw)
		for i, step := range t.steps {
			c.Logf("  step %d", i)
			step.checkDirect(c, q)
		}
		expect{}.checkDirect(c, q)
		q.Stop()
		c.Assert(ruw.stopped, jc.IsTrue)
	}
}
