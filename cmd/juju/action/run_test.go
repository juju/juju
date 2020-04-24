// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/core/actions"
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

type CallSuite struct {
	BaseActionSuite
	dir string
}

var _ = gc.Suite(&CallSuite{})

func (s *CallSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.dir = c.MkDir()
	c.Assert(utf8.ValidString(validParamsYaml), jc.IsTrue)
	c.Assert(utf8.ValidString(invalidParamsYaml), jc.IsTrue)
	c.Assert(utf8.ValidString(invalidUTFYaml), jc.IsFalse)
	setupValueFile(c, s.dir, "validParams.yml", validParamsYaml)
	setupValueFile(c, s.dir, "invalidParams.yml", invalidParamsYaml)
	setupValueFile(c, s.dir, "invalidUTF.yml", invalidUTFYaml)
}

func (s *CallSuite) TestInit(c *gc.C) {
	tests := []struct {
		should               string
		args                 []string
		expectMaxWait        time.Duration
		expectUnits          []string
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
		should:      "fail with both --background and --max-wait",
		args:        []string{"--background", "--max-wait=60s", validUnitId, "action"},
		expectError: "cannot specify both --max-wait and --background",
	}, {
		should:      "fail with no action specified",
		args:        []string{validUnitId},
		expectError: "no action specified",
	}, {
		should:      "fail with invalid unit ID",
		args:        []string{invalidUnitId, "valid-action-name"},
		expectError: "invalid unit or action name \"something-strange-\"",
	}, {
		should:      "fail with invalid unit ID first",
		args:        []string{validUnitId, invalidUnitId, "valid-action-name"},
		expectError: "invalid unit or action name \"something-strange-\"",
	}, {
		should:      "fail with invalid unit ID second",
		args:        []string{invalidUnitId, validUnitId, "valid-action-name"},
		expectError: "invalid unit or action name \"something-strange-\"",
	}, {
		should:       "work with multiple valid units",
		args:         []string{validUnitId, validUnitId2, "valid-action-name"},
		expectUnits:  []string{validUnitId, validUnitId2},
		expectAction: "valid-action-name",
		expectKVArgs: [][]string{},
	}, {
		should:      "fail with invalid action name",
		args:        []string{validUnitId, "BadName"},
		expectError: "invalid unit or action name \"BadName\"",
	}, {
		should:      "fail with invalid action name ending in \"-\"",
		args:        []string{validUnitId, "name-end-with-dash-"},
		expectError: "invalid unit or action name \"name-end-with-dash-\"",
	}, {
		should:      "fail with wrong formatting of k-v args",
		args:        []string{validUnitId, "valid-action-name", "uh"},
		expectError: "argument \"uh\" must be of the form key.key.key...=value",
	}, {
		should:      "fail with wrong formatting of k-v args",
		args:        []string{validUnitId, "valid-action-name", "foo.Baz=3"},
		expectError: "key \"Baz\" must start and end with lowercase alphanumeric, and contain only lowercase alphanumeric and hyphens",
	}, {
		should:      "fail with wrong formatting of k-v args",
		args:        []string{validUnitId, "valid-action-name", "no-go?od=3"},
		expectError: "key \"no-go\\?od\" must start and end with lowercase alphanumeric, and contain only lowercase alphanumeric and hyphens",
	}, {
		should:        "use max-wait if specified",
		args:          []string{validUnitId, "action", "--max-wait", "20s"},
		expectUnits:   []string{validUnitId},
		expectAction:  "action",
		expectMaxWait: 20 * time.Second,
	}, {
		should:       "work with action name ending in numeric values",
		args:         []string{validUnitId, "action-01"},
		expectUnits:  []string{validUnitId},
		expectAction: "action-01",
	}, {
		should:       "work with numeric values within action name",
		args:         []string{validUnitId, "action-00-foo"},
		expectUnits:  []string{validUnitId},
		expectAction: "action-00-foo",
	}, {
		should:       "work with action name starting with numeric values",
		args:         []string{validUnitId, "00-action"},
		expectUnits:  []string{validUnitId},
		expectAction: "00-action",
	}, {
		should:       "work with empty values",
		args:         []string{validUnitId, "valid-action-name", "ok="},
		expectUnits:  []string{validUnitId},
		expectAction: "valid-action-name",
		expectKVArgs: [][]string{{"ok", ""}},
	}, {
		should:             "handle --parse-strings",
		args:               []string{validUnitId, "valid-action-name", "--string-args"},
		expectUnits:        []string{validUnitId},
		expectAction:       "valid-action-name",
		expectParseStrings: true,
	}, {
		// cf. worker/uniter/runner/jujuc/action-set_test.go per @fwereade
		should:       "work with multiple '=' signs",
		args:         []string{validUnitId, "valid-action-name", "ok=this=is=weird="},
		expectUnits:  []string{validUnitId},
		expectAction: "valid-action-name",
		expectKVArgs: [][]string{{"ok", "this=is=weird="}},
	}, {
		should:       "init properly with no params",
		args:         []string{validUnitId, "valid-action-name"},
		expectUnits:  []string{validUnitId},
		expectAction: "valid-action-name",
	}, {
		should:               "handle --params properly",
		args:                 []string{validUnitId, "valid-action-name", "--params=foo.yml"},
		expectUnits:          []string{validUnitId},
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
		expectUnits:          []string{validUnitId},
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
		expectUnits:  []string{validUnitId},
		expectAction: "valid-action-name",
		expectKVArgs: [][]string{
			{"foo", "bar", "2"},
			{"foo", "baz", "bo", "y"},
			{"bar", "foo", "hello"},
		},
	}, {
		should:       "work with leader identifier",
		args:         []string{"mysql/leader", "valid-action-name"},
		expectUnits:  []string{"mysql/leader"},
		expectAction: "valid-action-name",
		expectKVArgs: [][]string{},
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			wrappedCommand, command := action.NewRunCommandForTest(s.store, nil)
			c.Logf("test %d: should %s:\n$ juju run (action) %s\n", i,
				t.should, strings.Join(t.args, " "))
			args := append([]string{modelFlag, "admin"}, t.args...)
			err := cmdtesting.InitCommand(wrappedCommand, args)
			if t.expectError == "" {
				c.Check(command.UnitNames(), gc.DeepEquals, t.expectUnits)
				c.Check(command.ActionName(), gc.Equals, t.expectAction)
				c.Check(command.ParamsYAML().Path, gc.Equals, t.expectParamsYamlPath)
				c.Check(command.Args(), jc.DeepEquals, t.expectKVArgs)
				c.Check(command.ParseStrings(), gc.Equals, t.expectParseStrings)
				if t.expectMaxWait != 0 {
					c.Check(command.MaxWait(), gc.Equals, t.expectMaxWait)
				} else {
					c.Check(command.MaxWait(), gc.Equals, 60*time.Second)
				}
			} else {
				c.Check(err, gc.ErrorMatches, t.expectError)
			}
		}
	}
}

func (s *CallSuite) TestRun(c *gc.C) {
	tests := []struct {
		should                 string
		clientSetup            func(client *fakeAPIClient)
		withArgs               []string
		withAPIErr             error
		withActionResults      []params.ActionResult
		withTags               params.FindTagsResults
		expectedActionEnqueued []params.Action
		expectedOutput         string
		expectedErr            string
		expectedLogs           []string
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
		withAPIErr:  errors.New("something wrong in API"),
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
		expectedErr: "yaml: line 4: mapping values are not allowed in this context",
	}, {
		should: "fail with invalid UTF in file",
		withArgs: []string{validUnitId, "some-action",
			"--params", s.dir + "/" + "invalidUTF.yml",
		},
		expectedErr: "yaml: invalid leading UTF-8 octet",
	}, {
		should:      "fail with invalid YAML passed as arg and no --string-args",
		withArgs:    []string{validUnitId, "some-action", "foo.bar=\""},
		expectedErr: "yaml: found unexpected end of stream",
	}, {
		should:   "enqueue a basic action with no params",
		withArgs: []string{validUnitId, "some-action", "--background"},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []params.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
	}, {
		should:   "run a basic action with no params with output set to action-set data",
		withArgs: []string{validUnitId, "some-action"},
		withTags: tagsForIdPrefix(validActionId, validActionTagString),
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"outcome": "success",
				"result-map": map[string]interface{}{
					"message": "hello",
				},
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
		}},
		expectedActionEnqueued: []params.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
		expectedOutput: `
outcome: success
result-map:
  message: hello`[1:],
	}, {
		should:   "run a basic action with no params with plain output including stdout, stderr",
		withArgs: []string{validUnitId, "some-action"},
		withTags: tagsForIdPrefix(validActionId, validActionTagString),
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"return-code":     "0",
				"stdout":          "hello",
				"stderr":          "world",
				"stdout-encoding": "utf-8",
				"stderr-encoding": "utf-8",
				"outcome":         "success",
				"result-map": map[string]interface{}{
					"message": "hello",
				},
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
		}},
		expectedActionEnqueued: []params.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
		expectedOutput: `
outcome: success
result-map:
  message: hello

hello
world`[1:],
	}, {
		should:   "run a basic action with no params with yaml output including stdout, stderr",
		withArgs: []string{validUnitId, "some-action", "--format", "yaml", "--utc"},
		withTags: tagsForIdPrefix(validActionId, validActionTagString),
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"return-code":     "0",
				"stdout":          "hello",
				"stderr":          "world",
				"stdout-encoding": "utf-8",
				"stderr-encoding": "utf-8",
				"outcome":         "success",
				"result-map": map[string]interface{}{
					"message": "hello",
				},
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
		}},
		expectedActionEnqueued: []params.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
		expectedOutput: `
mysql/0:
  id: f47ac10b-58cc-4372-a567-0e02b2c3d479
  results:
    outcome: success
    result-map:
      message: hello
    return-code: "0"
    stderr: world
    stderr-encoding: utf-8
    stdout: hello
    stdout-encoding: utf-8
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
  unit: mysql/0`[1:],
	}, {
		should:   "run a basic action with progress logs",
		withArgs: []string{validUnitId, "some-action", "--utc"},
		withTags: tagsForIdPrefix(validActionId, validActionTagString),

		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"return-code":     "0",
				"stdout":          "hello",
				"stderr":          "world",
				"stdout-encoding": "utf-8",
				"stderr-encoding": "utf-8",
				"outcome":         "success",
				"result-map": map[string]interface{}{
					"message": "hello",
				},
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
		}},
		expectedActionEnqueued: []params.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
		expectedLogs: []string{"log line 1", "log line 2"},
		expectedOutput: `
outcome: success
result-map:
  message: hello

hello
world`[1:],
	}, {
		should:   "run a basic action with progress logs with yaml output",
		withArgs: []string{validUnitId, "some-action", "--format", "yaml", "--utc"},
		withTags: tagsForIdPrefix(validActionId, validActionTagString),

		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Log: []params.ActionMessage{{
				Timestamp: time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
				Message:   "log line 1",
			}, {
				Timestamp: time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
				Message:   "log line 2",
			}},
			Status: "completed",
			Output: map[string]interface{}{
				"return-code":     "0",
				"stdout":          "hello",
				"stderr":          "world",
				"stdout-encoding": "utf-8",
				"stderr-encoding": "utf-8",
				"outcome":         "success",
				"result-map": map[string]interface{}{
					"message": "hello",
				},
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
		}},
		expectedActionEnqueued: []params.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
		expectedLogs: []string{"log line 1", "log line 2"},
		expectedOutput: `
mysql/0:
  id: f47ac10b-58cc-4372-a567-0e02b2c3d479
  log:
  - 2015-02-14 06:06:06 +0000 UTC log line 1
  - 2015-02-14 06:06:06 +0000 UTC log line 2
  results:
    outcome: success
    result-map:
      message: hello
    return-code: "0"
    stderr: world
    stderr-encoding: utf-8
    stdout: hello
    stdout-encoding: utf-8
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
  unit: mysql/0`[1:],
	}, {
		should:   "run action on multiple units with stdout for each action",
		withArgs: []string{validUnitId, validUnitId2, "some-action", "--format", "yaml", "--utc"},
		withTags: params.FindTagsResults{Matches: map[string][]params.Entity{
			validActionId:  {{Tag: validActionTagString}},
			validActionId2: {{Tag: validActionTagString2}},
		}},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"outcome": "success",
				"result-map": map[string]interface{}{
					"message": "hello",
				},
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
		}, {
			Action: &params.Action{
				Tag:      validActionTagString2,
				Receiver: names.NewUnitTag(validUnitId2).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"outcome": "success",
				"result-map": map[string]interface{}{
					"message": "hello2",
				},
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
		}},
		expectedActionEnqueued: []params.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}, {
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId2).String(),
		}},
		expectedOutput: `
mysql/0:
  id: f47ac10b-58cc-4372-a567-0e02b2c3d479
  results:
    outcome: success
    result-map:
      message: hello
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
  unit: mysql/0
mysql/1:
  id: f47ac10b-58cc-4372-a567-0e02b2c3d478
  results:
    outcome: success
    result-map:
      message: hello2
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
  unit: mysql/1`[1:],
	}, {
		should:   "run action on multiple units with plain output selected",
		withArgs: []string{validUnitId, validUnitId2, "some-action", "--format", "plain"},
		withTags: params.FindTagsResults{Matches: map[string][]params.Entity{
			validActionId:  {{Tag: validActionTagString}},
			validActionId2: {{Tag: validActionTagString2}},
		}},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"outcome": "success",
				"result-map": map[string]interface{}{
					"message": "hello",
				},
			},
		}, {
			Action: &params.Action{
				Tag:      validActionTagString2,
				Receiver: names.NewUnitTag(validUnitId2).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"outcome": "success",
				"result-map": map[string]interface{}{
					"message": "hello2",
				},
			},
		}},
		expectedActionEnqueued: []params.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}, {
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId2).String(),
		}},
		expectedOutput: `
mysql/0:
  id: f47ac10b-58cc-4372-a567-0e02b2c3d479
  output: |
    outcome: success
    result-map:
      message: hello
mysql/1:
  id: f47ac10b-58cc-4372-a567-0e02b2c3d478
  output: |
    outcome: success
    result-map:
      message: hello2`[1:],
	}, {
		should: "enqueue an action with some explicit params",
		withArgs: []string{validUnitId, "some-action", "--background",
			"out.name=bar",
			"out.kind=tmpfs",
			"out.num=3",
			"out.boolval=y",
		},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []params.Action{{
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
		}},
	}, {
		should: "enqueue an action with some raw string params",
		withArgs: []string{validUnitId, "some-action", "--background", "--string-args",
			"out.name=bar",
			"out.kind=tmpfs",
			"out.num=3",
			"out.boolval=y",
		},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []params.Action{{
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
		}},
	}, {
		should: "enqueue an action with file params plus CLI args",
		withArgs: []string{validUnitId, "some-action", "--background",
			"--params", s.dir + "/" + "validParams.yml",
			"compression.kind=gz",
			"compression.fast=true",
		},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []params.Action{{
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
		}},
	}, {
		should: "enqueue an action with file params and explicit params",
		withArgs: []string{validUnitId, "some-action", "--background",
			"out.name=bar",
			"out.kind=tmpfs",
			"compression.quality.speed=high",
			"compression.quality.size=small",
			"--params", s.dir + "/" + "validParams.yml",
		},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []params.Action{{
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
		}},
	}, {
		should:   "enqueue a basic action on the leader",
		withArgs: []string{"mysql/leader", "some-action", "--background"},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []params.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   "mysql/leader",
		},
		}},
	}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			func() {
				c.Logf("test %d: should %s:\n$ juju actions do %s\n", i, t.should, strings.Join(t.withArgs, " "))

				fakeClient := &fakeAPIClient{
					actionResults:    t.withActionResults,
					actionTagMatches: t.withTags,
					apiVersion:       6,
					logMessageCh:     make(chan []string, len(t.expectedLogs)),
				}
				if len(t.expectedLogs) > 0 {
					fakeClient.waitForResults = make(chan bool)
				}
				if t.clientSetup != nil {
					t.clientSetup(fakeClient)
				}

				fakeClient.apiErr = t.withAPIErr
				restore := s.patchAPIClient(fakeClient)
				defer restore()

				if len(t.expectedLogs) > 0 {
					go func() {
						encodedLogs := make([]string, len(t.expectedLogs))
						for n, log := range t.expectedLogs {
							msg := actions.ActionMessage{
								Message:   log,
								Timestamp: time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
							}
							msgData, err := json.Marshal(msg)
							c.Assert(err, jc.ErrorIsNil)
							encodedLogs[n] = string(msgData)
						}
						fakeClient.logMessageCh <- encodedLogs
					}()
				}

				var receivedMessages []string
				var expectedLogs []string
				for _, msg := range t.expectedLogs {
					expectedLogs = append(expectedLogs, "06:06:06 "+msg)
				}
				wrappedCommand, _ := action.NewRunCommandForTest(s.store, func(_ *cmd.Context, msg string) {
					receivedMessages = append(receivedMessages, msg)
					if reflect.DeepEqual(receivedMessages, expectedLogs) {
						close(fakeClient.waitForResults)
					}
				})
				args := append([]string{modelFlag, "admin"}, t.withArgs...)
				ctx, err := cmdtesting.RunCommand(c, wrappedCommand, args...)

				if len(t.expectedLogs) > 0 {
					select {
					case <-fakeClient.waitForResults:
					case <-time.After(testing.LongWait):
						c.Fatal("waiting for log messages to be consumed")
					}
				}

				if t.expectedErr != "" || t.withAPIErr != nil {
					c.Check(err, gc.ErrorMatches, t.expectedErr)
				} else {
					c.Assert(err, gc.IsNil)
					// Before comparing, double-check to avoid
					// panics in malformed tests.
					c.Assert(len(t.withActionResults), gc.Not(gc.Equals), 0)
					// Make sure the test's expected actions were
					// non-nil and correct.
					for i := range t.withActionResults {
						c.Assert(t.withActionResults[i].Action, gc.NotNil)
					}
					// Make sure the action sent to the API to be
					// enqueued was indeed the expected map
					enqueued := fakeClient.EnqueuedActions()
					c.Assert(enqueued.Actions, jc.DeepEquals, t.expectedActionEnqueued)

					if t.expectedOutput == "" {
						outputResult := ctx.Stderr.(*bytes.Buffer).Bytes()
						outString := strings.Trim(string(outputResult), "\n")

						expectedTag, err := names.ParseActionTag(t.withActionResults[0].Action.Tag)
						c.Assert(err, gc.IsNil)

						// Make sure the CLI responded with the expected tag
						c.Assert(outString, gc.Equals, fmt.Sprintf(`
Scheduled operation 1 with task %s
Check operation status with 'juju show-operation 1'
Check task status with 'juju show-task %s'`[1:],
							expectedTag.Id(), expectedTag.Id()))
					} else {
						outputResult := ctx.Stdout.(*bytes.Buffer).Bytes()
						outString := strings.Trim(string(outputResult), "\n")
						c.Assert(outString, gc.Equals, t.expectedOutput)
					}
				}
			}()
		}
	}
}
