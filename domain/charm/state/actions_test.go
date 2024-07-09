// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/charm"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type actionsSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&actionsSuite{})

var actionsTestCases = [...]struct {
	name   string
	input  []charmAction
	output charm.Actions
}{
	{
		name:  "empty",
		input: []charmAction{},
		output: charm.Actions{
			Actions: make(map[string]charm.Action),
		},
	},
	{
		name: "single",
		input: []charmAction{
			{
				Key:            "action",
				Description:    "description",
				Parallel:       true,
				ExecutionGroup: "group",
				Params:         []byte("{}"),
			},
		},
		output: charm.Actions{
			Actions: map[string]charm.Action{
				"action": {
					Description:    "description",
					Parallel:       true,
					ExecutionGroup: "group",
					Params:         []byte("{}"),
				},
			},
		},
	},
}

func (s *actionsSuite) TestConvertActions(c *gc.C) {
	for _, tc := range actionsTestCases {
		c.Logf("Running test case %q", tc.name)

		result := decodeActions(tc.input)
		c.Check(result, gc.DeepEquals, tc.output)
	}
}
