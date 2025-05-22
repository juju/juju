// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/application/charm"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type actionsSuite struct {
	schematesting.ModelSuite
}

func TestActionsSuite(t *stdtesting.T) {
	tc.Run(t, &actionsSuite{})
}

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

func (s *actionsSuite) TestDecodeActions(c *tc.C) {
	for _, testCase := range actionsTestCases {
		c.Logf("Running test case %q", testCase.name)

		result := decodeActions(testCase.input)
		c.Check(result, tc.DeepEquals, testCase.output)
	}
}

func (s *actionsSuite) TestEncodeActions(c *tc.C) {
	for _, testCase := range actionsTestCases {
		c.Logf("Running test case %q", testCase.name)

		decoded := decodeActions(testCase.input)
		c.Check(decoded, tc.DeepEquals, testCase.output)

		encoded := encodeActions("", decoded)

		result := make([]charmAction, 0, len(testCase.input))
		for _, action := range encoded {
			result = append(result, charmAction{
				Key:            action.Key,
				Description:    action.Description,
				Parallel:       action.Parallel,
				ExecutionGroup: action.ExecutionGroup,
				Params:         action.Params,
			})
		}
		c.Check(result, tc.DeepEquals, testCase.input)
	}
}
