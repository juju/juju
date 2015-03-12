// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/testing"
)

var (
	validParamsYaml = `
out: name
compression:
  kind: xz
  quality: high
`[1:]
	invalidParamsYaml = `
broken-map:
  foo:
    foo
    bar: baz
`[1:]
	invalidUTFYaml = "out: ok" + string([]byte{0xFF, 0xFF})
)

type DoSuite struct {
	BaseActionSuite
	subcommand *action.DoCommand
	dir        string
}

var _ = gc.Suite(&DoSuite{})

func (s *DoSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.dir = c.MkDir()
	c.Assert(utf8.ValidString(validParamsYaml), jc.IsTrue)
	c.Assert(utf8.ValidString(invalidParamsYaml), jc.IsTrue)
	c.Assert(utf8.ValidString(invalidUTFYaml), jc.IsFalse)
	setupValueFile(c, s.dir, "validParams.yml", validParamsYaml)
	setupValueFile(c, s.dir, "invalidParams.yml", invalidParamsYaml)
	setupValueFile(c, s.dir, "invalidUTF.yml", invalidUTFYaml)
}

func (s *DoSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, s.subcommand)
}

func (s *DoSuite) TestInit(c *gc.C) {
	tests := []struct {
		should               string
		args                 []string
		expectUnit           names.UnitTag
		expectAction         string
		expectParamsYamlPath string
		expectParseStrings   bool
		expectKVArgs         [][]string
		expectOutput         string
		expectError          string
	}{{
		should:      "fail with missing args",
		args:        []string{},
		expectError: "no unit specified",
	}, {
		should:      "fail with no action specified",
		args:        []string{validUnitId},
		expectError: "no action specified",
	}, {
		should:      "fail with invalid unit tag",
		args:        []string{invalidUnitId, "valid-action-name"},
		expectError: "invalid unit name \"something-strange-\"",
	}, {
		should:      "fail with invalid action name",
		args:        []string{validUnitId, "BadName"},
		expectError: "invalid action name \"BadName\"",
	}, {
		should:      "fail with wrong formatting of k-v args",
		args:        []string{validUnitId, "valid-action-name", "uh"},
		expectError: "argument \"uh\" must be of the form key...=value",
	}, {
		should:      "fail with wrong formatting of k-v args",
		args:        []string{validUnitId, "valid-action-name", "foo.Baz=3"},
		expectError: "key \"Baz\" must start and end with lowercase alphanumeric, and contain only lowercase alphanumeric and hyphens",
	}, {
		should:      "fail with wrong formatting of k-v args",
		args:        []string{validUnitId, "valid-action-name", "no-go?od=3"},
		expectError: "key \"no-go\\?od\" must start and end with lowercase alphanumeric, and contain only lowercase alphanumeric and hyphens",
	}, {
		should:       "work with empty values",
		args:         []string{validUnitId, "valid-action-name", "ok="},
		expectUnit:   names.NewUnitTag(validUnitId),
		expectAction: "valid-action-name",
		expectKVArgs: [][]string{{"ok", ""}},
	}, {
		should:             "handle --parse-strings",
		args:               []string{validUnitId, "valid-action-name", "--string-args"},
		expectUnit:         names.NewUnitTag(validUnitId),
		expectAction:       "valid-action-name",
		expectParseStrings: true,
	}, {
		// cf. worker/uniter/runner/jujuc/action-set_test.go per @fwereade
		should:       "work with multiple '=' signs",
		args:         []string{validUnitId, "valid-action-name", "ok=this=is=weird="},
		expectUnit:   names.NewUnitTag(validUnitId),
		expectAction: "valid-action-name",
		expectKVArgs: [][]string{{"ok", "this=is=weird="}},
	}, {
		should:       "init properly with no params",
		args:         []string{validUnitId, "valid-action-name"},
		expectUnit:   names.NewUnitTag(validUnitId),
		expectAction: "valid-action-name",
	}, {
		should:               "handle --params properly",
		args:                 []string{validUnitId, "valid-action-name", "--params=foo.yml"},
		expectUnit:           names.NewUnitTag(validUnitId),
		expectAction:         "valid-action-name",
		expectParamsYamlPath: "foo.yml",
	}, {
		should: "handle --params and key-value args",
		args: []string{
			validUnitId,
			"valid-action-name",
			"--params=foo.yml",
			"foo.bar=2",
			"foo.baz.bo=3",
			"bar.foo=hello",
		},
		expectUnit:           names.NewUnitTag(validUnitId),
		expectAction:         "valid-action-name",
		expectParamsYamlPath: "foo.yml",
		expectKVArgs: [][]string{
			{"foo", "bar", "2"},
			{"foo", "baz", "bo", "3"},
			{"bar", "foo", "hello"},
		},
	}, {
		should: "handle key-value args with no --params",
		args: []string{
			validUnitId,
			"valid-action-name",
			"foo.bar=2",
			"foo.baz.bo=y",
			"bar.foo=hello",
		},
		expectUnit:   names.NewUnitTag(validUnitId),
		expectAction: "valid-action-name",
		expectKVArgs: [][]string{
			{"foo", "bar", "2"},
			{"foo", "baz", "bo", "y"},
			{"bar", "foo", "hello"},
		},
	}}

	for i, t := range tests {
		s.subcommand = &action.DoCommand{}
		c.Logf("test %d: should %s:\n$ juju actions do %s\n", i,
			t.should, strings.Join(t.args, " "))
		err := testing.InitCommand(s.subcommand, t.args)
		if t.expectError == "" {
			c.Check(s.subcommand.UnitTag(), gc.Equals, t.expectUnit)
			c.Check(s.subcommand.ActionName(), gc.Equals, t.expectAction)
			c.Check(s.subcommand.ParamsYAMLPath(), gc.Equals, t.expectParamsYamlPath)
			c.Check(s.subcommand.KeyValueDoArgs(), jc.DeepEquals, t.expectKVArgs)
			c.Check(s.subcommand.ParseStrings(), gc.Equals, t.expectParseStrings)
		} else {
			c.Check(err, gc.ErrorMatches, t.expectError)
		}
	}
}

func (s *DoSuite) TestRun(c *gc.C) {
	tests := []struct {
		should                 string
		withArgs               []string
		withAPIErr             string
		withActionResults      []params.ActionResult
		expectedActionEnqueued params.Action
		expectedErr            string
	}{{
		should:   "fail with multiple results",
		withArgs: []string{validUnitId, "some-action"},
		withActionResults: []params.ActionResult{
			{Action: &params.Action{Tag: validActionTagString}},
			{Action: &params.Action{Tag: validActionTagString}},
		},
		expectedErr: "illegal number of results returned",
	}, {
		should:   "fail with API error",
		withArgs: []string{validUnitId, "some-action"},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{Tag: validActionTagString}},
		},
		withAPIErr:  "something wrong in API",
		expectedErr: "something wrong in API",
	}, {
		should:   "fail with error in result",
		withArgs: []string{validUnitId, "some-action"},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{Tag: validActionTagString},
			Error:  common.ServerError(errors.New("database error")),
		}},
		expectedErr: "database error",
	}, {
		should:   "fail with invalid tag in result",
		withArgs: []string{validUnitId, "some-action"},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{Tag: invalidActionTagString},
		}},
		expectedErr: "\"" + invalidActionTagString + "\" is not a valid action tag",
	}, {
		should: "fail with missing file passed",
		withArgs: []string{validUnitId, "some-action",
			"--params", s.dir + "/" + "missing.yml",
		},
		expectedErr: "open .*missing.yml: " + utils.NoSuchFileErrRegexp,
	}, {
		should: "fail with invalid yaml in file",
		withArgs: []string{validUnitId, "some-action",
			"--params", s.dir + "/" + "invalidParams.yml",
		},
		expectedErr: "YAML error: line 3: mapping values are not allowed in this context",
	}, {
		should: "fail with invalid UTF in file",
		withArgs: []string{validUnitId, "some-action",
			"--params", s.dir + "/" + "invalidUTF.yml",
		},
		expectedErr: "YAML error: invalid leading UTF-8 octet",
	}, {
		should:      "fail with invalid YAML passed as arg and no --string-args",
		withArgs:    []string{validUnitId, "some-action", "foo.bar=\""},
		expectedErr: "YAML error: found unexpected end of stream",
	}, {
		should:   "enqueue a basic action with no params",
		withArgs: []string{validUnitId, "some-action"},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{Tag: validActionTagString},
		}},
		expectedActionEnqueued: params.Action{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		},
	}, {
		should: "enqueue an action with some explicit params",
		withArgs: []string{validUnitId, "some-action",
			"out.name=bar",
			"out.kind=tmpfs",
			"out.num=3",
			"out.boolval=y",
		},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{Tag: validActionTagString},
		}},
		expectedActionEnqueued: params.Action{
			Name:     "some-action",
			Receiver: names.NewUnitTag(validUnitId).String(),
			Parameters: map[string]interface{}{
				"out": map[string]interface{}{
					"name":    "bar",
					"kind":    "tmpfs",
					"num":     3,
					"boolval": true,
				},
			},
		},
	}, {
		should: "enqueue an action with some raw string params",
		withArgs: []string{validUnitId, "some-action", "--string-args",
			"out.name=bar",
			"out.kind=tmpfs",
			"out.num=3",
			"out.boolval=y",
		},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{Tag: validActionTagString},
		}},
		expectedActionEnqueued: params.Action{
			Name:     "some-action",
			Receiver: names.NewUnitTag(validUnitId).String(),
			Parameters: map[string]interface{}{
				"out": map[string]interface{}{
					"name":    "bar",
					"kind":    "tmpfs",
					"num":     "3",
					"boolval": "y",
				},
			},
		},
	}, {
		should: "enqueue an action with file params plus CLI args",
		withArgs: []string{validUnitId, "some-action",
			"--params", s.dir + "/" + "validParams.yml",
			"compression.kind=gz",
			"compression.fast=true",
		},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{Tag: validActionTagString},
		}},
		expectedActionEnqueued: params.Action{
			Name:     "some-action",
			Receiver: names.NewUnitTag(validUnitId).String(),
			Parameters: map[string]interface{}{
				"out": "name",
				"compression": map[string]interface{}{
					"kind":    "gz",
					"quality": "high",
					"fast":    true,
				},
			},
		},
	}, {
		should: "enqueue an action with file params and explicit params",
		withArgs: []string{validUnitId, "some-action",
			"out.name=bar",
			"out.kind=tmpfs",
			"compression.quality.speed=high",
			"compression.quality.size=small",
			"--params", s.dir + "/" + "validParams.yml",
		},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{Tag: validActionTagString},
		}},
		expectedActionEnqueued: params.Action{
			Name:     "some-action",
			Receiver: names.NewUnitTag(validUnitId).String(),
			Parameters: map[string]interface{}{
				"out": map[string]interface{}{
					"name": "bar",
					"kind": "tmpfs",
				},
				"compression": map[string]interface{}{
					"kind": "xz",
					"quality": map[string]interface{}{
						"speed": "high",
						"size":  "small",
					},
				},
			},
		},
	}}

	for i, t := range tests {
		func() {
			c.Logf("test %d: should %s:\n$ juju actions do %s\n", i,
				t.should, strings.Join(t.withArgs, " "))
			fakeClient := &fakeAPIClient{
				actionResults: t.withActionResults,
			}
			if t.withAPIErr != "" {
				fakeClient.apiErr = errors.New(t.withAPIErr)
			}
			restore := s.patchAPIClient(fakeClient)
			defer restore()

			s.subcommand = &action.DoCommand{}
			ctx, err := testing.RunCommand(c, s.subcommand, t.withArgs...)

			if t.expectedErr != "" || t.withAPIErr != "" {
				c.Check(err, gc.ErrorMatches, t.expectedErr)
			} else {
				c.Assert(err, gc.IsNil)
				// Before comparing, double-check to avoid
				// panics in malformed tests.
				c.Assert(len(t.withActionResults), gc.Equals, 1)
				// Make sure the test's expected Action was
				// non-nil and correct.
				c.Assert(t.withActionResults[0].Action, gc.NotNil)
				expectedTag, err := names.ParseActionTag(t.withActionResults[0].Action.Tag)
				c.Assert(err, gc.IsNil)
				// Make sure the CLI responded with the expected tag
				keyToCheck := "Action queued with id"
				expectedMap := map[string]string{keyToCheck: expectedTag.Id()}
				outputResult := ctx.Stdout.(*bytes.Buffer).Bytes()
				resultMap := make(map[string]string)
				err = yaml.Unmarshal(outputResult, &resultMap)
				c.Assert(err, gc.IsNil)
				c.Check(resultMap, jc.DeepEquals, expectedMap)
				// Make sure the Action sent to the API to be
				// enqueued was indeed the expected map
				enqueued := fakeClient.EnqueuedActions()
				c.Assert(enqueued.Actions, gc.HasLen, 1)
				c.Check(enqueued.Actions[0], jc.DeepEquals, t.expectedActionEnqueued)
			}
		}()
	}
}
