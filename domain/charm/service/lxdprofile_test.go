// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	internalcharm "github.com/juju/juju/internal/charm"
)

type lxdProfileSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&lxdProfileSuite{})

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
		input: []byte(`{"config": {"limits.cpu": "2", "limits.memory": "2GB"}}`),
		output: internalcharm.LXDProfile{
			Config: map[string]string{
				"limits.cpu":    "2",
				"limits.memory": "2GB",
			},
		},
	},
	{
		name:  "profile description",
		input: []byte(`{"description": "description"}`),
		output: internalcharm.LXDProfile{
			Description: "description",
		},
	},
	{
		name:  "profile devices",
		input: []byte(`{"devices": {"eth0": {"nictype": "bridged", "parent": "lxdbr0"}}}`),
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

func (s *metadataSuite) TestConvertLXDProfile(c *gc.C) {
	for _, tc := range lxdProfileTestCases {
		c.Logf("Running test case %q", tc.name)

		result, err := decodeLXDProfile(tc.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, gc.DeepEquals, tc.output)
	}
}
