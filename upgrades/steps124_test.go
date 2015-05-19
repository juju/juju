// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type steps124Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps124Suite{})

func (s *steps124Suite) TestStateStepsFor124(c *gc.C) {
	expected := []string{
		"add block device documents for existing machines",
		"move service.UnitSeq to sequence collection",
		"add instance id field to IP addresses",
	}
	assertStateSteps(c, version.MustParse("1.24.0"), expected)
}
