// Copyright 2011-2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/juju/tc"
)

type ActionsSuite struct{}

func TestActionsSuite(t *testing.T) {
	tc.Run(t, &ActionsSuite{})
}

func (s *ActionsSuite) TestNewActions(c *tc.C) {
	emptyAction := NewActions()
	c.Assert(emptyAction, tc.DeepEquals, &Actions{})
}

func (s *ActionsSuite) TestValidateOk(c *tc.C) {
	for i, test := range []struct {
		description      string
		actionSpec       *ActionSpec
		objectToValidate map[string]interface{}
	}{{
		description: "Validation of an empty object is ok.",
		actionSpec: &ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"}},
				"additionalProperties": false}},
		objectToValidate: nil,
	}, {
		description: "Validation of one required value.",
		actionSpec: &ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"}},
				"additionalProperties": false,
				"required":             []interface{}{"outfile"}}},
		objectToValidate: map[string]interface{}{
			"outfile": "out-2014-06-12.bz2",
		},
	}, {
		description: "Validation of one required and one optional value.",
		actionSpec: &ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"},
					"quality": map[string]interface{}{
						"description": "Compression quality",
						"type":        "integer",
						"minimum":     0,
						"maximum":     9}},
				"additionalProperties": false,
				"required":             []interface{}{"outfile"}}},
		objectToValidate: map[string]interface{}{
			"outfile": "out-2014-06-12.bz2",
		},
	}, {
		description: "Validation of an optional, range limited value.",
		actionSpec: &ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"},
					"quality": map[string]interface{}{
						"description": "Compression quality",
						"type":        "integer",
						"minimum":     0,
						"maximum":     9}},
				"additionalProperties": false,
				"required":             []interface{}{"outfile"}}},
		objectToValidate: map[string]interface{}{
			"outfile": "out-2014-06-12.bz2",
			"quality": 5,
		},
	}, {
		description: "Validation of extra params with additionalProperties set to true",
		actionSpec: &ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"}},
				"additionalProperties": true}},
		objectToValidate: map[string]interface{}{
			"outfile":     "out-2014-06-12.bz2",
			"extraParam1": 1,
			"extraParam2": 2,
		},
	}} {
		c.Logf("test %d: %s", i, test.description)
		err := test.actionSpec.ValidateParams(test.objectToValidate)
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *ActionsSuite) TestValidateFail(c *tc.C) {
	var validActionTests = []struct {
		description   string
		actionSpec    *ActionSpec
		badActionJson string
		expectedError string
	}{{
		description: "Validation of one required value.",
		actionSpec: &ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"}},
				"additionalProperties": false,
				"required":             []interface{}{"outfile"}}},
		badActionJson: `{"outfile": 5}`,
		expectedError: "validation failed: (root).outfile : must be of type string, given 5",
	}, {
		description: "Restrict to only one property",
		actionSpec: &ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"}},
				"required":             []interface{}{"outfile"},
				"additionalProperties": false}},
		badActionJson: `{"outfile": "foo.bz", "bar": "foo"}`,
		expectedError: "validation failed: (root) : additional property \"bar\" is not allowed, given {\"bar\":\"foo\",\"outfile\":\"foo.bz\"}",
	}, {
		description: "Validation of one required and one optional value.",
		actionSpec: &ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"},
					"quality": map[string]interface{}{
						"description": "Compression quality",
						"type":        "integer",
						"minimum":     0,
						"maximum":     9}},
				"additionalProperties": false,
				"required":             []interface{}{"outfile"}}},
		badActionJson: `{"quality": 5}`,
		expectedError: "validation failed: (root) : \"outfile\" property is missing and required, given {\"quality\":5}",
	}, {
		description: "Validation of an optional, range limited value.",
		actionSpec: &ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"},
					"quality": map[string]interface{}{
						"description": "Compression quality",
						"type":        "integer",
						"minimum":     0,
						"maximum":     9}},
				"additionalProperties": false,
				"required":             []interface{}{"outfile"}}},
		badActionJson: `
{ "outfile": "out-2014-06-12.bz2", "quality": "two" }`,
		expectedError: "validation failed: (root).quality : must be of type integer, given \"two\"",
	}, {
		description: "Validation of misspelled value.",
		actionSpec: &ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"}},
				"additionalProperties": false,
			}},
		badActionJson: `{"oufile": 5}`,
		expectedError: `validation failed: (root) : additional property "oufile" is not allowed, given {"oufile":5}`,
	}}

	for i, test := range validActionTests {
		c.Logf("test %d: %s", i, test.description)
		var params map[string]interface{}
		jsonBytes := []byte(test.badActionJson)
		err := json.Unmarshal(jsonBytes, &params)
		c.Assert(err, tc.IsNil)
		err = test.actionSpec.ValidateParams(params)
		c.Assert(err.Error(), tc.Equals, test.expectedError)
	}
}

func (s *ActionsSuite) TestCleanseOk(c *tc.C) {

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
		c.Assert(err, tc.IsNil)
		c.Assert(cleansedInterfaceMap, tc.DeepEquals, test.expectedInterface)
	}
}

func (s *ActionsSuite) TestCleanseFail(c *tc.C) {

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
		c.Assert(err, tc.NotNil)
		c.Assert(err.Error(), tc.Equals, test.expectedError)
	}
}

func (s *ActionsSuite) TestGetActionNameRule(c *tc.C) {

	var regExCheck = []struct {
		description string
		regExString string
	}{{
		description: "Check returned actionNameRule regex",
		regExString: "^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$",
	}}

	for i, t := range regExCheck {
		c.Logf("test %d: %v: %#v\n", i, t.description, t.regExString)
		obtained := GetActionNameRule()
		c.Assert(obtained.String(), tc.Equals, t.regExString)
	}
}

func (s *ActionsSuite) TestReadGoodActionsYaml(c *tc.C) {
	var goodActionsYamlTests = []struct {
		description     string
		yaml            string
		expectedActions *Actions
	}{{
		description: "A simple snapshot actions YAML with one parameter.",
		yaml: `
snapshot:
   description: Take a snapshot of the database.
   params:
      outfile:
         description: "The file to write out to."
         type: string
   required: ["outfile"]
`,
		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": {
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"title":       "snapshot",
					"description": "Take a snapshot of the database.",
					"type":        "object",
					"properties": map[string]interface{}{
						"outfile": map[string]interface{}{
							"description": "The file to write out to.",
							"type":        "string"}},
					"additionalProperties": false,
					"required":             []interface{}{"outfile"}}}}},
	}, {
		description: "An empty Actions definition.",
		yaml:        "",
		expectedActions: &Actions{
			ActionSpecs: map[string]ActionSpec{},
		},
	}, {
		description: "A more complex schema with hyphenated names and multiple parameters.",
		yaml: `
snapshot:
   description: "Take a snapshot of the database."
   params:
      outfile:
         description: "The file to write out to."
         type: "string"
      compression-quality:
         description: "The compression quality."
         type: "integer"
         minimum: 0
         maximum: 9
         exclusiveMaximum: false
remote-sync:
   description: "Sync a file to a remote host."
   params:
      file:
         description: "The file to send out."
         type: "string"
         format: "uri"
      remote-uri:
         description: "The host to sync to."
         type: "string"
         format: "uri"
      util:
         description: "The util to perform the sync (rsync or scp.)"
         type: "string"
         enum: ["rsync", "scp"]
   required: ["file", "remote-uri"]
`,
		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": {
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"title":       "snapshot",
					"description": "Take a snapshot of the database.",
					"type":        "object",
					"properties": map[string]interface{}{
						"outfile": map[string]interface{}{
							"description": "The file to write out to.",
							"type":        "string"},
						"compression-quality": map[string]interface{}{
							"description":      "The compression quality.",
							"type":             "integer",
							"minimum":          0,
							"maximum":          9,
							"exclusiveMaximum": false}},
					"additionalProperties": false}},
			"remote-sync": {
				Description: "Sync a file to a remote host.",
				Params: map[string]interface{}{
					"title":       "remote-sync",
					"description": "Sync a file to a remote host.",
					"type":        "object",
					"properties": map[string]interface{}{
						"file": map[string]interface{}{
							"description": "The file to send out.",
							"type":        "string",
							"format":      "uri"},
						"remote-uri": map[string]interface{}{
							"description": "The host to sync to.",
							"type":        "string",
							"format":      "uri"},
						"util": map[string]interface{}{
							"description": "The util to perform the sync (rsync or scp.)",
							"type":        "string",
							"enum":        []interface{}{"rsync", "scp"}}},
					"additionalProperties": false,
					"required":             []interface{}{"file", "remote-uri"}}}}},
	}, {
		description: "A schema with other keys, e.g. \"definitions\"",
		yaml: `
snapshot:
   description: "Take a snapshot of the database."
   params:
      outfile:
         description: "The file to write out to."
         type: "string"
      compression-quality:
         description: "The compression quality."
         type: "integer"
         minimum: 0
         maximum: 9
         exclusiveMaximum: false
   definitions:
      diskdevice: {}
      something-else: {}
`,
		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": {
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"title":       "snapshot",
					"description": "Take a snapshot of the database.",
					"type":        "object",
					"properties": map[string]interface{}{
						"outfile": map[string]interface{}{
							"description": "The file to write out to.",
							"type":        "string",
						},
						"compression-quality": map[string]interface{}{
							"description":      "The compression quality.",
							"type":             "integer",
							"minimum":          0,
							"maximum":          9,
							"exclusiveMaximum": false,
						},
					},
					"additionalProperties": false,
					"definitions": map[string]interface{}{
						"diskdevice":     map[string]interface{}{},
						"something-else": map[string]interface{}{},
					},
				},
			},
		}},
	}, {
		description: "A schema with no \"params\" key, implying no options.",
		yaml: `
snapshot:
   description: Take a snapshot of the database.
`,

		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": {
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"description":          "Take a snapshot of the database.",
					"title":                "snapshot",
					"type":                 "object",
					"properties":           map[string]interface{}{},
					"additionalProperties": false,
				}}}},
	}, {
		description: "A schema with no values at all, implying no options.",
		yaml: `
snapshot:
`,

		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": {
				Description: "No description",
				Params: map[string]interface{}{
					"description":          "No description",
					"title":                "snapshot",
					"type":                 "object",
					"properties":           map[string]interface{}{},
					"additionalProperties": false,
				}}}},
	}, {
		description: "A simple snapshot actions YAML with names ending characters.",
		yaml: `
snapshot-01:
   description: Take database first snapshot.
   params:
      outfile-01:
         description: "The file to write out to."
         type: string
   required: ["outfile"]
`,
		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot-01": {
				Description: "Take database first snapshot.",
				Params: map[string]interface{}{
					"title":       "snapshot-01",
					"description": "Take database first snapshot.",
					"type":        "object",
					"properties": map[string]interface{}{
						"outfile-01": map[string]interface{}{
							"description": "The file to write out to.",
							"type":        "string"}},
					"additionalProperties": false,
					"required":             []interface{}{"outfile"}}}}},
	}, {
		description: "A simple snapshot actions YAML with names containing characters.",
		yaml: `
snapshot-0-foo:
   description: Take database first snapshot.
   params:
      outfile:
         description: "The file to write out to."
         type: string
   required: ["outfile"]
`,
		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot-0-foo": {
				Description: "Take database first snapshot.",
				Params: map[string]interface{}{
					"title":       "snapshot-0-foo",
					"description": "Take database first snapshot.",
					"type":        "object",
					"properties": map[string]interface{}{
						"outfile": map[string]interface{}{
							"description": "The file to write out to.",
							"type":        "string"}},
					"additionalProperties": false,
					"required":             []interface{}{"outfile"}}}}},
	}, {
		description: "A simple snapshot actions YAML with names starting characters.",
		yaml: `
01-snapshot:
   description: Take database first snapshot.
   params:
      01-outfile:
         description: "The file to write out to."
         type: string
   required: ["outfile"]
`,
		expectedActions: &Actions{map[string]ActionSpec{
			"01-snapshot": {
				Description: "Take database first snapshot.",
				Params: map[string]interface{}{
					"title":       "01-snapshot",
					"description": "Take database first snapshot.",
					"type":        "object",
					"properties": map[string]interface{}{
						"01-outfile": map[string]interface{}{
							"description": "The file to write out to.",
							"type":        "string"}},
					"additionalProperties": false,
					"required":             []interface{}{"outfile"}}}}},
	}, {
		description: "An action with parallel and execution group values set",
		yaml: `
snapshot:
   description: "Take a snapshot of the database."
   parallel: true
   execution-group: "exec group"
`,
		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": {
				Description:    "Take a snapshot of the database.",
				Parallel:       true,
				ExecutionGroup: "exec group",
				Params: map[string]interface{}{
					"title":                "snapshot",
					"description":          "Take a snapshot of the database.",
					"type":                 "object",
					"properties":           map[string]interface{}{},
					"additionalProperties": false},
			},
		}},
	}, {
		description: "An action with additional properties set to true",
		yaml: `
snapshot:
   description: "Take a snapshot of the database."
   additionalProperties: true
`,
		expectedActions: &Actions{map[string]ActionSpec{
			"snapshot": {
				Description: "Take a snapshot of the database.",
				Params: map[string]interface{}{
					"title":                "snapshot",
					"description":          "Take a snapshot of the database.",
					"type":                 "object",
					"properties":           map[string]interface{}{},
					"additionalProperties": true,
				},
			},
		}},
	}}

	// Beginning of testing loop
	for i, test := range goodActionsYamlTests {
		c.Logf("test %d: %s", i, test.description)
		reader := bytes.NewReader([]byte(test.yaml))
		loadedAction, err := ReadActionsYaml("somecharm", reader)
		c.Assert(err, tc.IsNil)
		c.Check(loadedAction, tc.DeepEquals, test.expectedActions)
	}
}

func (s *ActionsSuite) TestJujuCharmActionsYaml(c *tc.C) {
	actionsYaml := `
juju-snapshot:
   description: Take a snapshot of the database.
   params:
      outfile:
         description: "The file to write out to."
         type: string
   required: ["outfile"]
`
	expectedActions := &Actions{map[string]ActionSpec{
		"juju-snapshot": {
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"title":       "juju-snapshot",
				"description": "Take a snapshot of the database.",
				"type":        "object",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string"}},
				"required":             []interface{}{"outfile"},
				"additionalProperties": false}}}}

	reader := bytes.NewReader([]byte(actionsYaml))
	loadedAction, err := ReadActionsYaml("juju-charm", reader)
	c.Assert(err, tc.IsNil)
	c.Check(loadedAction, tc.DeepEquals, expectedActions)
}

func (s *ActionsSuite) TestReadBadActionsYaml(c *tc.C) {

	var badActionsYamlTests = []struct {
		description   string
		yaml          string
		expectedError string
	}{{
		description: "Reject JSON-Schema containing references.",
		yaml: `
snapshot:
   description: Take a snapshot of the database.
   params:
      $schema: "http://json-schema.org/draft-03/schema#"
`,
		expectedError: `schema key "\$schema" not compatible with this version of juju`,
	}, {
		description: "Reject JSON-Schema containing references.",
		yaml: `
snapshot:
   description: Take a snapshot of the database.
   params:
      outfile: { $ref: "http://json-schema.org/draft-03/schema#" }
`,
		expectedError: `schema key "\$ref" not compatible with this version of juju`,
	}, {
		description: "Malformed YAML: missing key in \"outfile\".",
		yaml: `
snapshot:
   description: Take a snapshot of the database.
   params:
      outfile:
         The file to write out to.
         type: string
         default: foo.bz2
`,

		expectedError: `yaml: line [0-9]: mapping values are not allowed in this context`,
	}, {
		description: "Malformed JSON-Schema: $schema element misplaced.",
		yaml: `
snapshot:
description: Take a snapshot of the database.
   params:
      outfile:
         $schema: http://json-schema.org/draft-03/schema#
         description: The file to write out to.
         type: string
         default: foo.bz2
`,

		expectedError: `yaml: line [0-9]: mapping values are not allowed in this context`,
	}, {
		description: "Malformed Actions: hyphen at beginning of action name.",
		yaml: `
-snapshot:
   description: Take a snapshot of the database.
`,

		expectedError: `bad action name -snapshot`,
	}, {
		description: "Malformed Actions: hyphen after action name.",
		yaml: `
snapshot-:
   description: Take a snapshot of the database.
`,

		expectedError: `bad action name snapshot-`,
	}, {
		description: "Malformed Actions: caps in action name.",
		yaml: `
Snapshot:
   description: Take a snapshot of the database.
`,

		expectedError: `bad action name Snapshot`,
	}, {
		description: `Reserved Action Name: "juju".`,
		yaml: `
juju:
   description: A reserved action.
`,
		expectedError: `cannot use action name juju: "juju" is a reserved name`,
	}, {
		description: `Reserved Action Name: "juju-run".`,
		yaml: `
juju-run:
   description: A reserved action.
`,
		expectedError: `cannot use action name juju-run: the "juju-" prefix is reserved`,
	}, {
		description: "A non-string description fails to parse",
		yaml: `
snapshot:
   description: ["Take a snapshot of the database."]
`,
		expectedError: `value for schema key "description" must be a string`,
	}, {
		description: "A non-string execution-group fails to parse",
		yaml: `
snapshot:
   execution-group: ["Exec group"]
`,
		expectedError: `value for schema key "execution-group" must be a string`,
	}, {
		description: "A non-bool parallel value fails to parse",
		yaml: `
snapshot:
   parallel: "not a bool"
`,
		expectedError: `value for schema key "parallel" must be a bool`,
	}, {
		description: "A non-list \"required\" key",
		yaml: `
snapshot:
   description: Take a snapshot of the database.
   params:
      outfile:
         description: "The file to write out to."
         type: string
   required: "outfile"
`,
		expectedError: `value for schema key "required" must be a YAML list`,
	}, {
		description: "A schema with an empty \"params\" key fails to parse",
		yaml: `
snapshot:
   description: Take a snapshot of the database.
   params:
`,
		expectedError: `params failed to parse as a map`,
	}, {
		description: "A schema with a non-map \"params\" value fails to parse",
		yaml: `
snapshot:
   description: Take a snapshot of the database.
   params: ["a", "b"]
`,
		expectedError: `params failed to parse as a map`,
	}, {
		description: "\"definitions\" goes against JSON-Schema definition",
		yaml: `
snapshot:
   description: "Take a snapshot of the database."
   params:
      outfile:
         description: "The file to write out to."
         type: "string"
   definitions:
      diskdevice: ["a"]
      something-else: {"a": "b"}
`,
		expectedError: `invalid params schema for action schema snapshot: definitions must be of type array of schemas`,
	}, {
		description: "excess keys not in the JSON-Schema spec will be rejected",
		yaml: `
snapshot:
   description: "Take a snapshot of the database."
   params:
      outfile:
         description: "The file to write out to."
         type: "string"
      compression-quality:
         description: "The compression quality."
         type: "integer"
         minimum: 0
         maximum: 9
         exclusiveMaximum: false
   definitions:
      diskdevice: {}
      something-else: {}
   other-key: ["some", "values"],
`,
		expectedError: `yaml: line [0-9]+: did not find expected key`,
	}}

	for i, test := range badActionsYamlTests {
		c.Logf("test %d: %s", i, test.description)
		reader := bytes.NewReader([]byte(test.yaml))
		_, err := ReadActionsYaml("somecharm", reader)
		c.Check(err, tc.ErrorMatches, test.expectedError)
	}
}

func (s *ActionsSuite) TestRecurseMapOnKeys(c *tc.C) {
	tests := []struct {
		should     string
		givenKeys  []string
		givenMap   map[string]interface{}
		expected   interface{}
		shouldFail bool
	}{{
		should:    "fail if the specified key was not in the map",
		givenKeys: []string{"key", "key2"},
		givenMap: map[string]interface{}{
			"key": map[string]interface{}{
				"key": "value",
			},
		},
		shouldFail: true,
	}, {
		should:    "fail if a key was not a string",
		givenKeys: []string{"key", "key2"},
		givenMap: map[string]interface{}{
			"key": map[interface{}]interface{}{
				5: "value",
			},
		},
		shouldFail: true,
	}, {
		should:    "fail if we have more keys but not a recursable val",
		givenKeys: []string{"key", "key2"},
		givenMap: map[string]interface{}{
			"key": []string{"a", "b", "c"},
		},
		shouldFail: true,
	}, {
		should:    "retrieve a good value",
		givenKeys: []string{"key", "key2"},
		givenMap: map[string]interface{}{
			"key": map[string]interface{}{
				"key2": "value",
			},
		},
		expected: "value",
	}, {
		should:    "retrieve a map",
		givenKeys: []string{"key"},
		givenMap: map[string]interface{}{
			"key": map[string]interface{}{
				"key": "value",
			},
		},
		expected: map[string]interface{}{
			"key": "value",
		},
	}, {
		should:    "retrieve a slice",
		givenKeys: []string{"key"},
		givenMap: map[string]interface{}{
			"key": []string{"a", "b", "c"},
		},
		expected: []string{"a", "b", "c"},
	}}

	for i, t := range tests {
		c.Logf("test %d: should %s\n  map: %#v\n  keys: %#v", i, t.should, t.givenMap, t.givenKeys)
		obtained, failed := recurseMapOnKeys(t.givenKeys, t.givenMap)
		c.Assert(!failed, tc.Equals, t.shouldFail)
		if !t.shouldFail {
			c.Check(obtained, tc.DeepEquals, t.expected)
		}
	}
}

func (s *ActionsSuite) TestInsertDefaultValues(c *tc.C) {
	schemas := map[string]string{
		"simple": `
act:
  params:
    val:
      type: string
      default: somestr
`[1:],
		"complicated": `
act:
  params:
    val:
      type: object
      properties:
        foo:
          type: string
        bar:
          type: object
          properties:
            baz:
              type: string
              default: boz
`[1:],
		"default-object": `
act:
  params:
    val:
      type: object
      default:
        foo: bar
        bar:
          baz: woz
`[1:],
		"none": `
act:
  params:
    val:
      type: object
      properties:
        var:
          type: object
          properties:
            x:
              type: string
`[1:]}

	for i, t := range []struct {
		should         string
		schema         string
		withParams     map[string]interface{}
		expectedResult map[string]interface{}
		expectedError  string
	}{{
		should:        "error with no schema",
		expectedError: "schema must be of type object",
	}, {
		should:         "create a map if handed nil",
		schema:         schemas["none"],
		withParams:     nil,
		expectedResult: map[string]interface{}{},
	}, {
		should:         "create and fill target if handed nil",
		schema:         schemas["simple"],
		withParams:     nil,
		expectedResult: map[string]interface{}{"val": "somestr"},
	}, {
		should:         "create a simple default value",
		schema:         schemas["simple"],
		withParams:     map[string]interface{}{},
		expectedResult: map[string]interface{}{"val": "somestr"},
	}, {
		should:         "do nothing for no default value",
		schema:         schemas["none"],
		withParams:     map[string]interface{}{},
		expectedResult: map[string]interface{}{},
	}, {
		should:     "insert a default value within a nested map",
		schema:     schemas["complicated"],
		withParams: map[string]interface{}{},
		expectedResult: map[string]interface{}{
			"val": map[string]interface{}{
				"bar": map[string]interface{}{
					"baz": "boz",
				}}},
	}, {
		should:     "create a default value which is an object",
		schema:     schemas["default-object"],
		withParams: map[string]interface{}{},
		expectedResult: map[string]interface{}{
			"val": map[string]interface{}{
				"foo": "bar",
				"bar": map[string]interface{}{
					"baz": "woz",
				}}},
	}, {
		should:         "not overwrite existing values with default objects",
		schema:         schemas["default-object"],
		withParams:     map[string]interface{}{"val": 5},
		expectedResult: map[string]interface{}{"val": 5},
	}, {
		should: "interleave defaults into existing objects",
		schema: schemas["complicated"],
		withParams: map[string]interface{}{
			"val": map[string]interface{}{
				"foo": "bar",
				"bar": map[string]interface{}{
					"faz": "foz",
				}}},
		expectedResult: map[string]interface{}{
			"val": map[string]interface{}{
				"foo": "bar",
				"bar": map[string]interface{}{
					"baz": "boz",
					"faz": "foz",
				}}},
	}} {
		c.Logf("test %d: should %s", i, t.should)
		schema := getSchemaForAction(c, t.schema)
		// Testing this method
		result, err := schema.InsertDefaults(t.withParams)
		if t.expectedError != "" {
			c.Check(err, tc.ErrorMatches, t.expectedError)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, t.expectedResult)
	}
}

func getSchemaForAction(c *tc.C, wholeSchema string) ActionSpec {
	// Load up the YAML schema definition.
	reader := bytes.NewReader([]byte(wholeSchema))
	loadedActions, err := ReadActionsYaml("somecharm", reader)
	c.Assert(err, tc.IsNil)
	// Same action name for all tests, "act".
	return loadedActions.ActionSpecs["act"]
}
