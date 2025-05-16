// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/tc"

	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
)

type lxdProfileSuite struct {
	testhelpers.IsolationSuite
}

func TestLxdProfileSuite(t *stdtesting.T) { tc.Run(t, &lxdProfileSuite{}) }

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
	for _, testCase := range lxdProfileTestCases {
		c.Logf("Running test case %q", testCase.name)

		result, err := decodeLXDProfile(testCase.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, testCase.output)

		// Ensure that the conversion is idempotent.
		converted, err := encodeLXDProfile(&result)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(converted, tc.DeepEquals, testCase.input)
	}
}

func (s *metadataSuite) TestConvertNilLXDProfile(c *tc.C) {
	converted, err := encodeLXDProfile(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(converted, tc.IsNil)
}
