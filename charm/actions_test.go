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
		// Test 1
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
		// Test 2
		{
			"A more complex schema with hyphenated names and multiple parameters.",
			`
actionspecs:
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
			// Actions
			&charm.Actions{map[string]charm.ActionSpec{
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
		},

		// Test 3
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

	// Beginning of testing loop
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
		// Test 1
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

		// Test 2
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

		// Test 3
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

		// Test 4
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
