// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type PhaseInternalSuite struct {
	testhelpers.IsolationSuite
}

func TestPhaseInternalSuite(t *testing.T) {
	tc.Run(t, new(PhaseInternalSuite))
}

func (s *PhaseInternalSuite) TestForUnused(c *tc.C) {
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
	c.Check(allValidPhases.Difference(usedPhases), tc.HasLen, 0)

	// The special phases shouldn't appear in the transition map.
	c.Check(usedPhases.Intersection(specialPhases), tc.HasLen, 0)
}

func (s *PhaseInternalSuite) TestForUnreachable(c *tc.C) {
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
	c.Check(allSources.Difference(allTargets), tc.HasLen, 0)
}
