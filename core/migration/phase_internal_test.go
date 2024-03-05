// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/collections/set"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type PhaseInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(new(PhaseInternalSuite))

func (s *PhaseInternalSuite) TestForUnused(c *gc.C) {
	usedPhases := set.NewStrings()
	for source, targets := range validTransitions {
		usedPhases.Add(source.String())
		for _, target := range targets {
			usedPhases.Add(target.String())
		}
	}

	specialPhases := set.NewStrings(
		UNKNOWN.String(),
		NONE.String(),
	)
	allValidPhases := set.NewStrings(phaseNames...).Difference(specialPhases)
	c.Check(allValidPhases.Difference(usedPhases), gc.HasLen, 0)

	// The special phases shouldn't appear in the transition map.
	c.Check(usedPhases.Intersection(specialPhases), gc.HasLen, 0)
}

func (s *PhaseInternalSuite) TestForUnreachable(c *gc.C) {
	const initialPhase = QUIESCE
	allSources := set.NewStrings()
	allTargets := set.NewStrings()
	for source, targets := range validTransitions {
		if source != initialPhase {
			allSources.Add(source.String())
		}
		for _, target := range targets {
			allTargets.Add(target.String())
		}
	}

	// Each source must be referred to at least once.
	c.Check(allSources.Difference(allTargets), gc.HasLen, 0)
}
