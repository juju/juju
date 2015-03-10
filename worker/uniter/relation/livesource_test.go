// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/multiwatcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/relation"
)

type LiveSourceSuite struct{}

var _ = gc.Suite(&LiveSourceSuite{})

func (s *LiveSourceSuite) TestLiveHookSource(c *gc.C) {
	for i, t := range aliveHookQueueTests {
		c.Logf("test %d: %s", i, t.summary)
		ruw := &RUW{in: make(chan multiwatcher.RelationUnitsChange), stopped: false}
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

func (s *LiveSourceSuite) TestLiveHookSourceTeardownEvenWhenUnclean(c *gc.C) {
	// If a LiveSource saw a change and generated a change func(), it
	// should still teardown (and close its changes channel) even if the
	// function is never called.
	initialState := &relation.State{21345, nil, ""}
	ruw := &RUW{in: make(chan multiwatcher.RelationUnitsChange, 1), stopped: false}
	source := relation.NewLiveHookSource(initialState, ruw)
	sourceChanges := source.Changes()
	sourceC := coretesting.ContentAsserterC{C: c, Chan: sourceChanges}
	ruw.in <- multiwatcher.RelationUnitsChange{}
	sourceChange := sourceC.AssertOneReceive()
	// assert that it has the right type, but don't actually call it
	_ = sourceChange.(hook.SourceChange)
	// Now we tell the source to stop
	c.Assert(source.Stop(), jc.ErrorIsNil)
	// check that it has cleaned itself up (the source.Changes() channel is closed)
	sourceC.AssertClosed()

}
