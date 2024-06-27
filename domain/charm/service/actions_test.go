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

type actionsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&actionsSuite{})

var actionsTestCases = [...]struct {
	name   string
	input  charm.Actions
	output internalcharm.Actions
}{
	{
		name:   "empty",
		input:  charm.Actions{},
		output: internalcharm.Actions{},
	},
	{
		name: "empty params",
		input: charm.Actions{
			Actions: map[string]charm.Action{
				"action1": {
					Description:    "description1",
					Parallel:       true,
					ExecutionGroup: "group1",
				},
			},
		},
		output: internalcharm.Actions{
			ActionSpecs: map[string]internalcharm.ActionSpec{
				"action1": {
					Description:    "description1",
					Parallel:       true,
					ExecutionGroup: "group1",
				},
			},
		},
	},
	{
		name: "params",
		input: charm.Actions{
			Actions: map[string]charm.Action{
				"remote-sync": {
					Description:    "description1",
					Parallel:       true,
					ExecutionGroup: "group1",
					Params:         []byte(`{"remote-sync":{"description":"Sync a file to a remote host.","params":{"file":{"description":"The file to send out.","type":"string","format":"uri"},"remote-uri":{"description":"The host to sync to.","type":"string","format":"uri"},"util":{"description":"The util to perform the sync (rsync or scp.)","type":"string","enum":["rsync","scp"]}},"required":["file","remote-uri"]}}`),
				},
			},
		},
		output: internalcharm.Actions{
			ActionSpecs: map[string]internalcharm.ActionSpec{
				"remote-sync": {
					Description:    "description1",
					Parallel:       true,
					ExecutionGroup: "group1",
					Params: map[string]any{
						"remote-sync": map[string]any{
							"description": "Sync a file to a remote host.",
							"params": map[string]any{
								"file": map[string]any{
									"description": "The file to send out.",
									"type":        "string",
									"format":      "uri",
								},
								"remote-uri": map[string]any{
									"description": "The host to sync to.",
									"type":        "string",
									"format":      "uri",
								},
								"util": map[string]any{
									"description": "The util to perform the sync (rsync or scp.)",
									"type":        "string",
									"enum":        []any{"rsync", "scp"},
								},
							},
							"required": []any{"file", "remote-uri"},
						},
					},
				},
			},
		},
	},
}

func (s *metadataSuite) TestConvertActions(c *gc.C) {
	for _, tc := range actionsTestCases {
		c.Logf("Running test case %q", tc.name)

		result, err := decodeActions(tc.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, gc.DeepEquals, tc.output)
	}
}
