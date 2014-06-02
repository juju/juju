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
		description     string
		yaml            string
		expectedActions *charm.Actions
	}{{
		description: "A simple snapshot actions YAML with one parameter.",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
      params:
         outfile:
            description: The file to write out to.
            type: string
            default: foo.bz2
`,
		expectedActions: &charm.Actions{map[string]charm.ActionSpec{
			"snapshot": charm.ActionSpec{
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"outfile": map[interface{}]interface{}{
						"description": "The file to write out to.",
						"type":        "string",
						"default":     "foo.bz2"}}}}},
	}, {
		description: "An Actions YAML with one parameter and a $schema.",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
      params:
         $schema: http://json-schema.org/draft-03/schema#
         outfile:
            description: The file to write out to.
            type: string
            default: foo.bz2
`,
		expectedActions: &charm.Actions{map[string]charm.ActionSpec{
			"snapshot": charm.ActionSpec{
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"$schema": "http://json-schema.org/draft-03/schema#",
					"outfile": map[interface{}]interface{}{
						"description": "The file to write out to.",
						"type":        "string",
						"default":     "foo.bz2"}}}}},
	}, {
		description:     "An empty Actions definition.",
		yaml:            "",
		expectedActions: &charm.Actions{},
	}, {
		description: "A more complex schema with hyphenated names and multiple parameters.",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
      params:
         outfile:
            description: The file to write out to.
            type: string
            default: foo.bz2
         compression-quality:
            description: The compression quality.
            type: number
            minimum: 0
            maximum: 9
            exclusiveMaximum: false
   remote-sync:
      description: Sync a file to a remote host.
      params:
         file:
            description: The file to send out.
            type: string
            format: uri
            optional: false
         remote-uri:
            description: The host to sync to.
            type: string
            format: uri
            optional: false
         util:
            description: The util to perform the sync (rsync or scp.)
            type: string
            enum: [rsync, scp]
            default: rsync
`,
		expectedActions: &charm.Actions{map[string]charm.ActionSpec{
			"snapshot": charm.ActionSpec{
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"outfile": map[interface{}]interface{}{
						"description": "The file to write out to.",
						"type":        "string",
						"default":     "foo.bz2"},
					"compression-quality": map[interface{}]interface{}{
						"description":      "The compression quality.",
						"type":             "number",
						"minimum":          0,
						"maximum":          9,
						"exclusiveMaximum": false}}},
			"remote-sync": charm.ActionSpec{
				Description: "Sync a file to a remote host.",
				Params: map[string]interface{}{
					"file": map[interface{}]interface{}{
						"description": "The file to send out.",
						"type":        "string",
						"format":      "uri",
						"optional":    false},
					"remote-uri": map[interface{}]interface{}{
						"description": "The host to sync to.",
						"type":        "string",
						"format":      "uri",
						"optional":    false},
					"util": map[interface{}]interface{}{
						"description": "The util to perform the sync (rsync or scp.)",
						"type":        "string",
						"enum":        []interface{}{"rsync", "scp"},
						"default":     "rsync"}}}}},
	}, {
		description: "A schema with an empty \"params\" key, implying no options.",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
      params:
`,

		expectedActions: &charm.Actions{map[string]charm.ActionSpec{
			"snapshot": charm.ActionSpec{
				Description: "Take a snapshot of the database.",
				Params:      nil}}},
	}, {
		description: "A schema with no \"params\" key, implying no options.",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
`,

		expectedActions: &charm.Actions{map[string]charm.ActionSpec{
			"snapshot": charm.ActionSpec{
				Description: "Take a snapshot of the database.",
				Params:      nil}}},
	}}

	// Beginning of testing loop
	for i, test := range goodActionsYamlTests {
		c.Logf("test %d: %s", i, test.description)
		reader := bytes.NewReader([]byte(test.yaml))
		loadedAction, err := charm.ReadActionsYaml(reader)
		c.Assert(err, gc.IsNil)
		c.Assert(loadedAction, gc.DeepEquals, test.expectedActions)
	}
}

func (s *ActionsSuite) TestReadBadActionsYaml(c *gc.C) {

	var badActionsYamlTests = []struct {
		description   string
		yaml          string
		expectedError string
	}{{
		description: "Invalid JSON-Schema: $schema key not a string.",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
      params:
         $schema: 5
         outfile:
            description: The file to write out to.
            type: string
            default: foo.bz2
`,
		expectedError: "invalid params schema for action schema snapshot: $schema must be of type string",
	}, {
		description: "Malformed YAML: missing key in \"outfile\".",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
      params:
         outfile:
            The file to write out to.
            type: string
            default: foo.bz2
`,

		expectedError: "YAML error: line 7: mapping values are not allowed in this context",
	}, {
		description: "Malformed JSON-Schema: $schema element misplaced.",
		yaml: `
actions:
   snapshot:
   description: Take a snapshot of the database.
      params:
         outfile:
            $schema: http://json-schema.org/draft-03/schema#
            description: The file to write out to.
            type: string
            default: foo.bz2
`,

		expectedError: "YAML error: line 4: mapping values are not allowed in this context",
	}, {
		description: "Malformed Actions: hyphen at beginning of action name.",
		yaml: `
actions:
   -snapshot:
      description: Take a snapshot of the database.
`,

		expectedError: "bad action name -snapshot",
	}, {
		description: "Malformed Actions: hyphen after action name.",
		yaml: `
actions:
   snapshot-:
      description: Take a snapshot of the database.
`,

		expectedError: "bad action name snapshot-",
	}, {
		description: "Malformed Actions: caps in action name.",
		yaml: `
actions:
   Snapshot:
      description: Take a snapshot of the database.
`,

		expectedError: "bad action name Snapshot",
	}, {
		description: "Malformed Params: hyphen before param name.",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
      params:
        -outfile:
          description: The file to write out to.
`,

		expectedError: "bad param name -outfile",
	}, {
		description: "Malformed Params: hyphen after param name.",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
      params:
        outfile-:
          description: The file to write out to.
`,

		expectedError: "bad param name outfile-",
	}, {
		description: "Malformed Params: caps in param name.",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
      params:
        Outfile:
          description: The file to write out to.
`,

		expectedError: "bad param name Outfile",
	}}

	for i, test := range badActionsYamlTests {
		c.Logf("test %d: %s", i, test.description)
		reader := bytes.NewReader([]byte(test.yaml))
		_, err := charm.ReadActionsYaml(reader)
		c.Assert(err, gc.Not(gc.IsNil))
		c.Assert(err.Error(), gc.Equals, test.expectedError)
	}
}
