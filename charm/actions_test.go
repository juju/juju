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

func (s *ActionsSuite) TestReadGoodActionsYaml(c *gc.C) {

	var goodActionsYamlTests = []struct {
		description string
		yaml        string
		actions     *charm.Actions
	}{
		{
			"A simple snapshot actions YAML with one parameter.",
			// YAML
			`
actionspecs:
  snapshot:
    description: Take a snapshot of the database.
    params:
      outfile:
        description: The file to write out to.
        type: string
        default: foo.bz2
`,
			// Actions
			&charm.Actions{map[string]charm.ActionSpec{
				"snapshot": charm.ActionSpec{
					Description: "Take a snapshot of the database.",
					Params: map[string]interface{}{
						"outfile": map[interface{}]interface{}{
							"description": "The file to write out to.",
							"type":        "string",
							"default":     "foo.bz2"}}}}}},

		//{
		//	"A more complex schema with hyphenated names and multiple parameters.",
		//	"placeholder",
		//	nil,
		//},

		{
			"A schema with an empty \"params\" key, implying no options.",
			// YAML
			`
actionspecs:
  snapshot:
    description: Take a snapshot of the database.
    params:
`,
			// Actions
			&charm.Actions{map[string]charm.ActionSpec{
				"snapshot": charm.ActionSpec{
					Description: "Take a snapshot of the database.",
					Params:      map[string]interface{}(nil)}}},
		}}

	// Beginning of actual test
	for i, test := range goodActionsYamlTests {
		c.Logf("test %d: %s", i, test.description)
		reader := bytes.NewReader([]byte(test.yaml))
		loadedAction, err := charm.ReadActionsYaml(reader)
		c.Assert(err, gc.IsNil)
		c.Assert(loadedAction, gc.DeepEquals, test.actions)
	}
}

func (s *ActionsSuite) TestReadBadActionsYaml(c *gc.C) {

	var badActionsYamlTests = []struct {
		description string
		yaml        string
		actions     *charm.Actions
	}{
		{
			"Malformed YAML: missing key in \"outfile\".",
			// YAML
			`
actionspecs:
  snapshot:
    description: Take a snapshot of the database.
    params:
      outfile:
        The file to write out to.
        type: string
        default: foo.bz2
`,
			// Actions
			nil,
		},
		{
			"Malformed JSON-Schema: $schema element misplaced.",
			// YAML
			`
actionspecs:
  snapshot:
  description: Take a snapshot of the database.
    params:
      outfile:
        $schema: http://json-schema.org/draft-03/schema#
        description: The file to write out to.
        type: string
        default: foo.bz2
`,
			// Actions
			nil,
		},
		{
			"Malformed Actions: hyphen at beginning of action name.",
			// YAML
			`
actionspecs:
  -snapshot:
    description: Take a snapshot of the database.
    params:
      outfile:
        description: The file to write out to.
        type: string
        default: foo.bz2
`,
			// Actions
			nil,
		},
		{
			"Malformed Actions: hyphen after action name.",
			// YAML
			`
actionspecs:
  snapshot-:
    description: Take a snapshot of the database.
    params:
      outfile:
        description: The file to write out to.
        type: string
        default: foo.bz2
`,
			// Actions
			nil,
		},
	}

	for i, test := range badActionsYamlTests {
		c.Logf("test %d: %s", i, test.description)
		reader := bytes.NewReader([]byte(test.yaml))
		_, err := charm.ReadActionsYaml(reader)
		c.Assert(err, gc.NotNil)
	}
}
