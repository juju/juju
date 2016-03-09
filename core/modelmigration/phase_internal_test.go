// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type PhaseInternalSuite struct {
	coretesting.BaseSuite
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

	allValidPhases := set.NewStrings(phaseNames...)
	allValidPhases.Remove(UNKNOWN.String())
	unused := allValidPhases.Difference(usedPhases)
	c.Check(unused, gc.HasLen, 0)
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
