// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
)

type manifestSuite struct {
	testhelpers.IsolationSuite
}

func TestManifestSuite(t *testing.T) {
	tc.Run(t, &manifestSuite{})
}

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

func (s *manifestSuite) TestConvertManifest(c *tc.C) {
	for _, testCase := range manifestTestCases {
		c.Logf("Running test case %q", testCase.name)

		result, err := decodeManifest(testCase.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, testCase.output)

		// Ensure that the conversion is idempotent.
		converted, warnings, err := encodeManifest(&result)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(converted, tc.DeepEquals, testCase.input)
		c.Check(warnings, tc.HasLen, 0)
	}
}

func (s *manifestSuite) TestConvertManifestWarnings(c *tc.C) {
	converted, warnings, err := encodeManifest(&internalcharm.Manifest{
		Bases: []internalcharm.Base{
			{
				Name: "ubuntu",
				Channel: internalcharm.Channel{
					Track:  "latest",
					Risk:   internalcharm.Stable,
					Branch: "foo",
				},
				Architectures: []string{"amd64", "i386", "arm64"},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(converted, tc.DeepEquals, charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track:  "latest",
					Risk:   charm.RiskStable,
					Branch: "foo",
				},
				Architectures: []string{"amd64", "arm64"},
			},
		},
	})
	c.Check(warnings, tc.DeepEquals, []string{`unsupported architectures: i386 for "ubuntu" with channel: "latest/stable/foo"`})
}
