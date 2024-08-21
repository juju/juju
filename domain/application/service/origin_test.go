// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	domaincharm "github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

type originSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&originSuite{})

var originTestCases = [...]struct {
	name   string
	input  corecharm.Origin
	output domaincharm.CharmOrigin
}{
	{

		name: "minimal",
		input: corecharm.Origin{
			Source: corecharm.Local,
		},
		output: domaincharm.CharmOrigin{
			Source:   domaincharm.LocalSource,
			Revision: -1,
		},
	},
	{
		// Empty channels that aren't nil should be normalized to "stable".
		name: "minimal with empty channel",
		input: corecharm.Origin{
			Source:  corecharm.Local,
			Channel: &internalcharm.Channel{},
		},
		output: domaincharm.CharmOrigin{
			Source:   domaincharm.LocalSource,
			Revision: -1,
			Channel: &domaincharm.Channel{
				Risk: "stable",
			},
		},
	},
	{
		name: "full",
		input: corecharm.Origin{
			Source:   corecharm.CharmHub,
			Channel:  &internalcharm.Channel{Track: "track", Risk: "stable", Branch: "branch"},
			Revision: ptr(42),
		},
		output: domaincharm.CharmOrigin{
			Source:   domaincharm.CharmHubSource,
			Revision: 42,
			Channel: &domaincharm.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
		},
	},
}

func (s *originSuite) TestConvertOrigin(c *gc.C) {
	for _, tc := range originTestCases {
		c.Logf("Running test case %q", tc.name)

		// Ensure that the conversion is idempotent.
		resultOrigin, err := encodeCharmOrigin(tc.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(resultOrigin, jc.DeepEquals, tc.output)
	}
}

func (s *originSuite) TestEmptyOrigin(c *gc.C) {
	// It's not possible to have an empty origin, as we don't know what
	// the source should be. We could default to charmhub, but we'd be
	// wrong 50% of the time.

	_, err := encodeCharmOrigin(corecharm.Origin{})
	c.Assert(err, gc.ErrorMatches, "unknown source.*")
}
