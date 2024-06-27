// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

type manifestSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifestSuite{})

var manifestTestCases = [...]struct {
	name   string
	input  charm.Manifest
	output internalcharm.Manifest
}{
	{
		name:   "empty",
		input:  charm.Manifest{},
		output: internalcharm.Manifest{},
	},
	{
		name: "full bases",
		input: charm.Manifest{
			Bases: []charm.Base{
				{
					Name: "ubuntu",
					Channel: charm.Channel{
						Track:  "latest",
						Risk:   charm.RiskStable,
						Branch: "foo",
					},
					Architectures: []string{"amd64"},
				},
			},
		},
		output: internalcharm.Manifest{
			Bases: []internalcharm.Base{
				{
					Name: "ubuntu",
					Channel: internalcharm.Channel{
						Track:  "latest",
						Risk:   internalcharm.Stable,
						Branch: "foo",
					},
					Architectures: []string{"amd64"},
				},
			},
		},
	},
}

func (s *metadataSuite) TestConvertManifest(c *gc.C) {
	for _, tc := range manifestTestCases {
		c.Logf("Running test case %q", tc.name)

		result, err := decodeManifest(tc.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, gc.DeepEquals, tc.output)
	}
}
