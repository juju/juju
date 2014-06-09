// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"bytes"

	gc "launchpad.net/gocheck"
)

type ActionsSuite struct{}

var _ = gc.Suite(&ActionsSuite{})

func (s *ActionsSuite) TestNewActions(c *gc.C) {
	emptyAction := NewActions()
	c.Assert(emptyAction, gc.DeepEquals, &Actions{})
}

func (s *ActionsSuite) TestCleanseOK(c *gc.C) {

	var goodInterfaceTests = []struct {
		description         string
		acceptableInterface map[string]interface{}
		expectedInterface   map[string]interface{}
	}{{
		description: "An interface requiring no changes.",
		acceptableInterface: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": map[string]interface{}{
				"foo1": "val1",
				"foo2": "val2"}},
		expectedInterface: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": map[string]interface{}{
				"foo1": "val1",
				"foo2": "val2"}},
	}, {
		description: "Substitute a single inner map[i]i.",
		acceptableInterface: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": map[interface{}]interface{}{
				"foo1": "val1",
				"foo2": "val2"}},
		expectedInterface: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": map[string]interface{}{
				"foo1": "val1",
				"foo2": "val2"}},
	}, {
		description: "Substitute nested inner map[i]i.",
		acceptableInterface: map[string]interface{}{
			"key1a": "val1a",
			"key2a": "val2a",
			"key3a": map[interface{}]interface{}{
				"key1b": "val1b",
				"key2b": map[interface{}]interface{}{
					"key1c": "val1c"}}},
		expectedInterface: map[string]interface{}{
			"key1a": "val1a",
			"key2a": "val2a",
			"key3a": map[string]interface{}{
				"key1b": "val1b",
				"key2b": map[string]interface{}{
					"key1c": "val1c"}}},
	}, {
		description: "Substitute nested map[i]i within []i.",
		acceptableInterface: map[string]interface{}{
			"key1a": "val1a",
			"key2a": []interface{}{5, "foo", map[string]interface{}{
				"key1b": "val1b",
				"key2b": map[interface{}]interface{}{
					"key1c": "val1c"}}}},
		expectedInterface: map[string]interface{}{
			"key1a": "val1a",
			"key2a": []interface{}{5, "foo", map[string]interface{}{
				"key1b": "val1b",
				"key2b": map[string]interface{}{
					"key1c": "val1c"}}}},
	}}

	for i, test := range goodInterfaceTests {
		c.Logf("test %d: %s", i, test.description)
		cleansedInterfaceMap, err := cleanse(test.acceptableInterface)
		c.Assert(err, gc.IsNil)
		c.Assert(cleansedInterfaceMap, gc.DeepEquals, test.expectedInterface)
	}
}

func (s *ActionsSuite) TestCleanseFail(c *gc.C) {

	var badInterfaceTests = []struct {
		description   string
		failInterface map[string]interface{}
		expectedError string
	}{{
		description: "An inner map[interface{}]interface{} with an int key.",
		failInterface: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": map[interface{}]interface{}{
				"foo1": "val1",
				5:      "val2"}},
		expectedError: "map keyed with non-string value",
	}, {
		description: "An inner []interface{} containing a map[i]i with an int key.",
		failInterface: map[string]interface{}{
			"key1a": "val1b",
			"key2a": "val2b",
			"key3a": []interface{}{"foo1", 5, map[interface{}]interface{}{
				"key1b": "val1b",
				"key2b": map[interface{}]interface{}{
					"key1c": "val1c",
					5:       "val2c"}}}},
		expectedError: "map keyed with non-string value",
	}}

	for i, test := range badInterfaceTests {
		c.Logf("test %d: %s", i, test.description)
		_, err := cleanse(test.failInterface)
		c.Assert(err, gc.NotNil)
		c.Assert(err.Error(), gc.Equals, test.expectedError)
	}
}

func (s *ActionsSuite) TestReadGoodActionsYaml(c *gc.C) {

	var goodActionsYamlTests = []struct {
		description     string
		yaml            string
		expectedActions *Actions
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
		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": ActionSpec{
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"outfile": map[string]interface{}{
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
		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": ActionSpec{
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"$schema": "http://json-schema.org/draft-03/schema#",
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string",
						"default":     "foo.bz2"}}}}},
	}, {
		description:     "An empty Actions definition.",
		yaml:            "",
		expectedActions: &Actions{},
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
		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": ActionSpec{
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string",
						"default":     "foo.bz2"},
					"compression-quality": map[string]interface{}{
						"description":      "The compression quality.",
						"type":             "number",
						"minimum":          0,
						"maximum":          9,
						"exclusiveMaximum": false}}},
			"remote-sync": ActionSpec{
				Description: "Sync a file to a remote host.",
				Params: map[string]interface{}{
					"file": map[string]interface{}{
						"description": "The file to send out.",
						"type":        "string",
						"format":      "uri",
						"optional":    false},
					"remote-uri": map[string]interface{}{
						"description": "The host to sync to.",
						"type":        "string",
						"format":      "uri",
						"optional":    false},
					"util": map[string]interface{}{
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

		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": ActionSpec{
				Description: "Take a snapshot of the database.",
				Params:      map[string]interface{}{}}}},
	}, {
		description: "A schema with no \"params\" key, implying no options.",
		yaml: `
actions:
   snapshot:
      description: Take a snapshot of the database.
`,

		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": ActionSpec{
				Description: "Take a snapshot of the database.",
				Params:      map[string]interface{}{}}}},
	}}

	// Beginning of testing loop
	for i, test := range goodActionsYamlTests {
		c.Logf("test %d: %s", i, test.description)
		reader := bytes.NewReader([]byte(test.yaml))
		loadedAction, err := ReadActionsYaml(reader)
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
		_, err := ReadActionsYaml(reader)
		c.Assert(err, gc.NotNil)
		c.Assert(err.Error(), gc.Equals, test.expectedError)
	}
}
