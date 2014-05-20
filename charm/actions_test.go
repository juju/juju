// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"bytes"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
)

type ActionsSuite struct{}

var _ = gc.Suite(&ActionsSuite{})

func TestReadGoodActionsYaml(c *gc.C) {

	var goodActionsYamlTests = []struct {
		description string
		yaml        string
		actions     *charm.Actions
	}{
		{
			"A simple snapshot actions YAML with one parameter.",
			// YAML
			"actionspecs:\n" +
				"  snapshot:\n" +
				"    description: Take a snapshot of the database.\n" +
				"    params:\n" +
				"      outfile:\n" +
				"        description: The file to write out to.\n" +
				"        type: string\n" +
				"        default: foo.bz2\n",
			// Actions
			*charm.Actions{map[string]charm.ActionSpec{
				"snapshot": charm.ActionSpec{
					Description: "Take a snapshot of the database.",
					Params: map[string]interface{}{
						"outfile": map[interface{}]interface{}{
							"description": "The file to write out to.",
							"type":        "string",
							"default":     "foo.bz2"}}}}}},

		{
			"A more complex schema with hyphenated names and multiple parameters.",
			"placeholder",
			nil,
		},

		{
			"A schema with an empty \"params\" key, implying no options.",
			// YAML
			"actionspecs:\n" +
				"  snapshot:\n" +
				"    description: Take a snapshot of the database.\n" +
				"    params:\n",
			// Actions
			*charm.Actions{map[string]charm.ActionSpec{
				"snapshot": charm.ActionSpec{
					Description: "Take a snapshot of the database.",
					Params:      map[string]interface{}{}}}},
		}}

	// Beginning of actual test
	for i, test := range goodActionsYamlTests {
		c.Logf("test %d: %s", i, test.description)
		reader := bytes.NewReader([]byte(t.yaml))
		loadedAction, err := charm.ReadActionsYaml(reader)
		c.Assert(loadedAction, gc.DeepEquals, test.actions)
	}
}

func TestReadBadActionsYaml(c *gc.C) {

	var badActionsYamlTests = []struct {
		description string
		yaml        string
		actions     *charm.Actions
	}{
		{
			"Malformed YAML: missing key in \"outfile\".",
			// YAML
			"actionspecs:\n" +
				"  snapshot:\n" +
				"    description: Take a snapshot of the database.\n" +
				"    params:\n" +
				"      outfile:\n" +
				"        The file to write out to.\n" +
				"        type: string\n" +
				"        default: foo.bz2\n",
			// Actions
			nil,
		},
		{
			"Malformed JSON-Schema: $schema element misplaced.",
			// YAML
			"actionspecs:\n" +
				"  snapshot:\n" +
				"  description: Take a snapshot of the database.\n" +
				"    params:\n" +
				"      outfile:\n" +
				"        $schema: http://json-schema.org/draft-03/schema#\n" +
				"        description: The file to write out to.\n" +
				"        type: string\n" +
				"        default: foo.bz2\n",
			// Actions
			nil,
		},
		{
			"Malformed Actions: hyphen at beginning of action name.",
			// YAML
			"actionspecs:\n" +
				"  -snapshot:\n" +
				"    description: Take a snapshot of the database.\n" +
				"    params:\n" +
				"      outfile:\n" +
				"        description: The file to write out to.\n" +
				"        type: string\n" +
				"        default: foo.bz2\n",
			// Actions
			nil,
		},
		{
			"Malformed Actions: hyphen after action name.",
			// YAML
			"actionspecs:\n" +
				"  snapshot-:\n" +
				"    description: Take a snapshot of the database.\n" +
				"    params:\n" +
				"      outfile:\n" +
				"        description: The file to write out to.\n" +
				"        type: string\n" +
				"        default: foo.bz2\n",
			// Actions
			nil,
		},
	}

	for i, test := range badActionsYamlTests {
		c.Logf("test %d: %s", i, test.description)
		reader := bytes.NewReader([]byte(t.yaml))
		loadedAction, err := charm.ReadActionsYaml(reader)
		c.Assert(err, gc.NotNil)
	}
}
