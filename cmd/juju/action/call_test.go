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
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

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
		expectFunction       string
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
		args:        []string{"--background", "--max-wait=60s", validUnitId, "function"},
		expectError: "cannot specify both --max-wait and --background",
	}, {
		should:      "fail with no function specified",
		args:        []string{validUnitId},
		expectError: "no function specified",
	}, {
		should:      "fail with invalid unit ID",
		args:        []string{invalidUnitId, "valid-function-name"},
		expectError: "invalid unit or function name \"something-strange-\"",
	}, {
		should:      "fail with invalid unit ID first",
		args:        []string{validUnitId, invalidUnitId, "valid-function-name"},
		expectError: "invalid unit or function name \"something-strange-\"",
	}, {
		should:      "fail with invalid unit ID second",
		args:        []string{invalidUnitId, validUnitId, "valid-function-name"},
		expectError: "invalid unit or function name \"something-strange-\"",
	}, {
		should:         "work with multiple valid units",
		args:           []string{validUnitId, validUnitId2, "valid-function-name"},
		expectUnits:    []string{validUnitId, validUnitId2},
		expectFunction: "valid-function-name",
		expectKVArgs:   [][]string{},
	}, {
		should:      "fail with invalid function name",
		args:        []string{validUnitId, "BadName"},
		expectError: "invalid unit or function name \"BadName\"",
	}, {
		should:      "fail with invalid function name ending in \"-\"",
		args:        []string{validUnitId, "name-end-with-dash-"},
		expectError: "invalid unit or function name \"name-end-with-dash-\"",
	}, {
		should:      "fail with wrong formatting of k-v args",
		args:        []string{validUnitId, "valid-function-name", "uh"},
		expectError: "argument \"uh\" must be of the form key.key.key...=value",
	}, {
		should:      "fail with wrong formatting of k-v args",
		args:        []string{validUnitId, "valid-function-name", "foo.Baz=3"},
		expectError: "key \"Baz\" must start and end with lowercase alphanumeric, and contain only lowercase alphanumeric and hyphens",
	}, {
		should:      "fail with wrong formatting of k-v args",
		args:        []string{validUnitId, "valid-function-name", "no-go?od=3"},
		expectError: "key \"no-go\\?od\" must start and end with lowercase alphanumeric, and contain only lowercase alphanumeric and hyphens",
	}, {
		should:         "use max-wait if specified",
		args:           []string{validUnitId, "function", "--max-wait", "20s"},
		expectUnits:    []string{validUnitId},
		expectFunction: "function",
		expectMaxWait:  20 * time.Second,
	}, {
		should:         "work with function name ending in numeric values",
		args:           []string{validUnitId, "function-01"},
		expectUnits:    []string{validUnitId},
		expectFunction: "function-01",
	}, {
		should:         "work with numeric values within function name",
		args:           []string{validUnitId, "function-00-foo"},
		expectUnits:    []string{validUnitId},
		expectFunction: "function-00-foo",
	}, {
		should:         "work with function name starting with numeric values",
		args:           []string{validUnitId, "00-function"},
		expectUnits:    []string{validUnitId},
		expectFunction: "00-function",
	}, {
		should:         "work with empty values",
		args:           []string{validUnitId, "valid-function-name", "ok="},
		expectUnits:    []string{validUnitId},
		expectFunction: "valid-function-name",
		expectKVArgs:   [][]string{{"ok", ""}},
	}, {
		should:             "handle --parse-strings",
		args:               []string{validUnitId, "valid-function-name", "--string-args"},
		expectUnits:        []string{validUnitId},
		expectFunction:     "valid-function-name",
		expectParseStrings: true,
	}, {
		// cf. worker/uniter/runner/jujuc/function-set_test.go per @fwereade
		should:         "work with multiple '=' signs",
		args:           []string{validUnitId, "valid-function-name", "ok=this=is=weird="},
		expectUnits:    []string{validUnitId},
		expectFunction: "valid-function-name",
		expectKVArgs:   [][]string{{"ok", "this=is=weird="}},
	}, {
		should:         "init properly with no params",
		args:           []string{validUnitId, "valid-function-name"},
		expectUnits:    []string{validUnitId},
		expectFunction: "valid-function-name",
	}, {
		should:               "handle --params properly",
		args:                 []string{validUnitId, "valid-function-name", "--params=foo.yml"},
		expectUnits:          []string{validUnitId},
		expectFunction:       "valid-function-name",
		expectParamsYamlPath: "foo.yml",
	}, {
		should: "handle --params and key-value args",
		args: []string{
			validUnitId,
			"valid-function-name",
			"--params=foo.yml",
			"foo.bar=2",
			"foo.baz.bo=3",
			"bar.foo=hello",
		},
		expectUnits:          []string{validUnitId},
		expectFunction:       "valid-function-name",
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
			"valid-function-name",
			"foo.bar=2",
			"foo.baz.bo=y",
			"bar.foo=hello",
		},
		expectUnits:    []string{validUnitId},
		expectFunction: "valid-function-name",
		expectKVArgs: [][]string{
			{"foo", "bar", "2"},
			{"foo", "baz", "bo", "y"},
			{"bar", "foo", "hello"},
		},
	}, {
		should:         "work with leader identifier",
		args:           []string{"mysql/leader", "valid-function-name"},
		expectUnits:    []string{"mysql/leader"},
		expectFunction: "valid-function-name",
		expectKVArgs:   [][]string{},
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			wrappedCommand, command := action.NewCallCommandForTest(s.store, nil)
			c.Logf("test %d: should %s:\n$ juju call (function) %s\n", i,
				t.should, strings.Join(t.args, " "))
			args := append([]string{modelFlag, "admin"}, t.args...)
			err := cmdtesting.InitCommand(wrappedCommand, args)
			if t.expectError == "" {
				c.Check(command.UnitNames(), gc.DeepEquals, t.expectUnits)
				c.Check(command.FunctionName(), gc.Equals, t.expectFunction)
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

func (s *CallSuite) TestCall(c *gc.C) {
	tests := []struct {
		should                   string
		clientSetup              func(client *fakeAPIClient)
		withArgs                 []string
		withAPIErr               error
		withFunctionResults      []params.ActionResult
		withTags                 params.FindTagsResults
		expectedFunctionEnqueued []params.Action
		expectedOutput           string
		expectedErr              string
		expectedLogs             []string
	}{{
		should:   "fail with multiple results",
		withArgs: []string{validUnitId, "some-function"},
		withFunctionResults: []params.ActionResult{
			{Action: &params.Action{Tag: validActionTagString}},
			{Action: &params.Action{Tag: validActionTagString}},
		},
		expectedErr: "illegal number of results returned",
	}, {
		should:   "fail with API error",
		withArgs: []string{validUnitId, "some-function"},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{Tag: validActionTagString}},
		},
		withAPIErr:  errors.New("something wrong in API"),
		expectedErr: "something wrong in API",
	}, {
		should:   "fail with error in result",
		withArgs: []string{validUnitId, "some-function"},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{Tag: validActionTagString},
			Error:  common.ServerError(errors.New("database error")),
		}},
		expectedErr: "database error",
	}, {
		should:   "fail with invalid tag in result",
		withArgs: []string{validUnitId, "some-function"},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{Tag: invalidActionTagString},
		}},
		expectedErr: "\"" + invalidActionTagString + "\" is not a valid action tag",
	}, {
		should: "fail with missing file passed",
		withArgs: []string{validUnitId, "some-function",
			"--params", s.dir + "/" + "missing.yml",
		},
		expectedErr: "open .*missing.yml: " + utils.NoSuchFileErrRegexp,
	}, {
		should: "fail with invalid yaml in file",
		withArgs: []string{validUnitId, "some-function",
			"--params", s.dir + "/" + "invalidParams.yml",
		},
		expectedErr: "yaml: line 4: mapping values are not allowed in this context",
	}, {
		should: "fail with invalid UTF in file",
		withArgs: []string{validUnitId, "some-function",
			"--params", s.dir + "/" + "invalidUTF.yml",
		},
		expectedErr: "yaml: invalid leading UTF-8 octet",
	}, {
		should:      "fail with invalid YAML passed as arg and no --string-args",
		withArgs:    []string{validUnitId, "some-function", "foo.bar=\""},
		expectedErr: "yaml: found unexpected end of stream",
	}, {
		should:   "enqueue a basic function with no params",
		withArgs: []string{validUnitId, "some-function", "--background"},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedFunctionEnqueued: []params.Action{{
			Name:       "some-function",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
	}, {
		should:   "call a basic function with no params with output set to action-set data",
		withArgs: []string{validUnitId, "some-function"},
		withTags: tagsForIdPrefix(validActionId, validActionTagString),
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-function",
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
		expectedFunctionEnqueued: []params.Action{{
			Name:       "some-function",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
		expectedOutput: `
outcome: success
result-map:
  message: hello`[1:],
	}, {
		should:   "call a basic function with no params with plain output including stdout, stderr",
		withArgs: []string{validUnitId, "some-function"},
		withTags: tagsForIdPrefix(validActionId, validActionTagString),
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-function",
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
		expectedFunctionEnqueued: []params.Action{{
			Name:       "some-function",
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
		should:   "call a basic function with no params with yaml output including stdout, stderr",
		withArgs: []string{validUnitId, "some-function", "--format", "yaml", "--utc"},
		withTags: tagsForIdPrefix(validActionId, validActionTagString),
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-function",
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
		expectedFunctionEnqueued: []params.Action{{
			Name:       "some-function",
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
		should:   "call a basic function with progress logs",
		withArgs: []string{validUnitId, "some-function", "--utc"},
		withTags: tagsForIdPrefix(validActionId, validActionTagString),

		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-function",
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
		expectedFunctionEnqueued: []params.Action{{
			Name:       "some-function",
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
		should:   "call a basic action with progress logs with yaml output",
		withArgs: []string{validUnitId, "some-function", "--format", "yaml", "--utc"},
		withTags: tagsForIdPrefix(validActionId, validActionTagString),

		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-function",
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
		expectedFunctionEnqueued: []params.Action{{
			Name:       "some-function",
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
		should:   "call action on multiple units with stdout for each action",
		withArgs: []string{validUnitId, validUnitId2, "some-function", "--format", "yaml", "--utc"},
		withTags: params.FindTagsResults{Matches: map[string][]params.Entity{
			validActionId:  {{Tag: validActionTagString}},
			validActionId2: {{Tag: validActionTagString2}},
		}},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-function",
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
				Name:     "some-function",
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
		expectedFunctionEnqueued: []params.Action{{
			Name:       "some-function",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}, {
			Name:       "some-function",
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
		should:   "call function on multiple units with plain output selected",
		withArgs: []string{validUnitId, validUnitId2, "some-function", "--format", "plain"},
		withTags: params.FindTagsResults{Matches: map[string][]params.Entity{
			validActionId:  {{Tag: validActionTagString}},
			validActionId2: {{Tag: validActionTagString2}},
		}},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-function",
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
				Name:     "some-function",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"outcome": "success",
				"result-map": map[string]interface{}{
					"message": "hello2",
				},
			},
		}},
		expectedFunctionEnqueued: []params.Action{{
			Name:       "some-function",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}, {
			Name:       "some-function",
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
		should: "enqueue an function with some explicit params",
		withArgs: []string{validUnitId, "some-function", "--background",
			"out.name=bar",
			"out.kind=tmpfs",
			"out.num=3",
			"out.boolval=y",
		},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedFunctionEnqueued: []params.Action{{
			Name:     "some-function",
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
		should: "enqueue an function with some raw string params",
		withArgs: []string{validUnitId, "some-function", "--background", "--string-args",
			"out.name=bar",
			"out.kind=tmpfs",
			"out.num=3",
			"out.boolval=y",
		},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedFunctionEnqueued: []params.Action{{
			Name:     "some-function",
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
		should: "enqueue an function with file params plus CLI args",
		withArgs: []string{validUnitId, "some-function", "--background",
			"--params", s.dir + "/" + "validParams.yml",
			"compression.kind=gz",
			"compression.fast=true",
		},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedFunctionEnqueued: []params.Action{{
			Name:     "some-function",
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
		should: "enqueue an function with file params and explicit params",
		withArgs: []string{validUnitId, "some-function", "--background",
			"out.name=bar",
			"out.kind=tmpfs",
			"compression.quality.speed=high",
			"compression.quality.size=small",
			"--params", s.dir + "/" + "validParams.yml",
		},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedFunctionEnqueued: []params.Action{{
			Name:     "some-function",
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
		should:      "fail with not implemented Leaders method",
		clientSetup: func(api *fakeAPIClient) { api.apiVersion = 2 },
		withArgs:    []string{"mysql/leader", "some-function", "--background"},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedErr: "unable to determine leader for application \"mysql\"" +
			"\nleader determination is unsupported by this API" +
			"\neither upgrade your controller, or explicitly specify a unit",
	}, {
		should:      "enqueue a basic function on the leader",
		clientSetup: func(api *fakeAPIClient) { api.apiVersion = 3 },
		withArgs:    []string{"mysql/leader", "some-function", "--background"},
		withFunctionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag:      validActionTagString,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedFunctionEnqueued: []params.Action{{
			Name:       "some-function",
			Parameters: map[string]interface{}{},
			Receiver:   "mysql/leader",
		},
		}},
	}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			func() {
				c.Logf("test %d: should %s:\n$ juju functions do %s\n", i, t.should, strings.Join(t.withArgs, " "))

				fakeClient := &fakeAPIClient{
					actionResults:    t.withFunctionResults,
					actionTagMatches: t.withTags,
					apiVersion:       5,
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
				wrappedCommand, _ := action.NewCallCommandForTest(s.store, func(_ *cmd.Context, msg string) {
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
					c.Assert(len(t.withFunctionResults), gc.Not(gc.Equals), 0)
					// Make sure the test's expected Functions were
					// non-nil and correct.
					for i := range t.withFunctionResults {
						c.Assert(t.withFunctionResults[i].Action, gc.NotNil)
					}
					// Make sure the Function sent to the API to be
					// enqueued was indeed the expected map
					enqueued := fakeClient.EnqueuedActions()
					c.Assert(enqueued.Actions, jc.DeepEquals, t.expectedFunctionEnqueued)

					if t.expectedOutput == "" {
						outputResult := ctx.Stderr.(*bytes.Buffer).Bytes()
						outString := strings.Trim(string(outputResult), "\n")

						expectedTag, err := names.ParseActionTag(t.withFunctionResults[0].Action.Tag)
						c.Assert(err, gc.IsNil)

						// Make sure the CLI responded with the expected tag
						c.Assert(outString, gc.Equals, fmt.Sprintf(`
Scheduled Operation %s
Check status with 'juju show-operation %s'`[1:],
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
