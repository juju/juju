// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/relation"
)

type DyingSourceSuite struct{}

var _ = gc.Suite(&DyingSourceSuite{})

func (s *DyingSourceSuite) TestDyingHookSource(c *gc.C) {
	for i, t := range dyingHookQueueTests {
		c.Logf("test %d: %s", i, t.summary)
		q := relation.NewDyingHookSource(t.initial)
		for i, step := range t.steps {
			c.Logf("  step %d", i)
			step.checkDirect(c, q)
		}
		expect{}.checkDirect(c, q)
		q.Stop()
	}
}
