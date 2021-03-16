// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/testing"
)

type upgradeSeriesGraphSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&upgradeSeriesGraphSuite{})

func (*upgradeSeriesGraphSuite) TestUpgradeSeriesGraphValidate(c *gc.C) {
	graph := model.UpgradeSeriesGraph()
	err := graph.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (*upgradeSeriesGraphSuite) TestValidate(c *gc.C) {
	graph := model.Graph(map[model.UpgradeSeriesStatus][]model.UpgradeSeriesStatus{
		model.UpgradeSeriesNotStarted: {
			model.UpgradeSeriesPrepareStarted,
		},
	})
	err := graph.Validate()
	c.Assert(err, gc.ErrorMatches, `vertex "not started" edge to vertex "prepare started" is not valid`)
}

type upgradeSeriesFSMSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&upgradeSeriesFSMSuite{})

func (*upgradeSeriesFSMSuite) TestTransitionTo(c *gc.C) {
	for _, t := range []struct {
		expected model.UpgradeSeriesStatus
		state    model.UpgradeSeriesStatus
		valid    bool
	}{
		{
			expected: model.UpgradeSeriesPrepareStarted,
			state:    model.UpgradeSeriesPrepareStarted,
			valid:    true,
		},
		{
			expected: model.UpgradeSeriesNotStarted,
			state:    model.UpgradeSeriesStatus("GTFO"),
			valid:    false,
		},
	} {
		fsm, err := model.NewUpgradeSeriesFSM(model.UpgradeSeriesGraph(), model.UpgradeSeriesNotStarted)
		c.Assert(err, jc.ErrorIsNil)

		allowed := fsm.TransitionTo(t.state)
		c.Assert(allowed, gc.Equals, t.valid)
		c.Assert(fsm.State(), gc.Equals, t.expected)
	}
}

func (*upgradeSeriesFSMSuite) TestTransitionGraph(c *gc.C) {
	dag := model.UpgradeSeriesGraph()
	for state, vertices := range dag {
		fsm, err := model.NewUpgradeSeriesFSM(dag, state)
		c.Assert(err, jc.ErrorIsNil)

		for _, vertex := range vertices {
			allowed := fsm.TransitionTo(vertex)
			c.Assert(allowed, jc.IsTrue)
		}
	}
}

func (*upgradeSeriesFSMSuite) TestTransitionGraphChildren(c *gc.C) {
	dag := model.UpgradeSeriesGraph()
	for state, vertices := range dag {
		for _, vertex := range vertices {
			fsm, err := model.NewUpgradeSeriesFSM(dag, state)
			c.Assert(err, jc.ErrorIsNil)

			allowed := fsm.TransitionTo(vertex)
			c.Assert(allowed, jc.IsTrue)

			// Can we transition to the child vertex?
			children := dag[vertex]
			if len(children) == 0 {
				continue
			}
			allowed = fsm.TransitionTo(children[0])
			c.Assert(allowed, jc.IsTrue, gc.Commentf("%v %v", fsm.State(), children[0]))
		}
	}
}
