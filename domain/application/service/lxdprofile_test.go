// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	internalcharm "github.com/juju/juju/internal/charm"
)

type lxdProfileSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&lxdProfileSuite{})

var lxdProfileTestCases = [...]struct {
	name   string
	input  []byte
	output internalcharm.LXDProfile
}{
	{
		name: "empty",
	},
	{
		name:  "profile config",
		input: []byte(`{"config":{"limits.cpu":"2","limits.memory":"2GB"}}`),
		output: internalcharm.LXDProfile{
			Config: map[string]string{
				"limits.cpu":    "2",
				"limits.memory": "2GB",
			},
		},
	},
	{
		name:  "profile description",
		input: []byte(`{"description":"description"}`),
		output: internalcharm.LXDProfile{
			Description: "description",
		},
	},
	{
		name:  "profile devices",
		input: []byte(`{"devices":{"eth0":{"nictype":"bridged","parent":"lxdbr0"}}}`),
		output: internalcharm.LXDProfile{
			Devices: map[string]map[string]string{
				"eth0": {
					"nictype": "bridged",
					"parent":  "lxdbr0",
				},
			},
		},
	},
}

func (s *metadataSuite) TestConvertLXDProfile(c *tc.C) {
	for _, tc := range lxdProfileTestCases {
		c.Logf("Running test case %q", tc.name)

		result, err := decodeLXDProfile(tc.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, tc.output)

		// Ensure that the conversion is idempotent.
		converted, err := encodeLXDProfile(&result)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(converted, jc.DeepEquals, tc.input)
	}
}

func (s *metadataSuite) TestConvertNilLXDProfile(c *tc.C) {
	converted, err := encodeLXDProfile(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(converted, tc.IsNil)
}
