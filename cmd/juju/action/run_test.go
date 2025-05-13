// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
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

type RunSuite struct {
	BaseActionSuite
	dir string
}

var _ = tc.Suite(&RunSuite{})

func (s *RunSuite) SetUpTest(c *tc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.dir = c.MkDir()
	c.Assert(utf8.ValidString(validParamsYaml), tc.IsTrue)
	c.Assert(utf8.ValidString(invalidParamsYaml), tc.IsTrue)
	c.Assert(utf8.ValidString(invalidUTFYaml), tc.IsFalse)
	setupValueFile(c, s.dir, "validParams.yml", validParamsYaml)
	setupValueFile(c, s.dir, "invalidParams.yml", invalidParamsYaml)
	setupValueFile(c, s.dir, "invalidUTF.yml", invalidUTFYaml)
}

func (s *RunSuite) TestInit(c *tc.C) {
	tests := []struct {
		should               string
		args                 []string
		expectWait           time.Duration
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
		should:      "fail with both --background and --wait",
		args:        []string{"--background", "--wait=60s", validUnitId, "action"},
		expectError: "cannot specify both --wait and --background",
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
		should:      "fail with units from different applications",
		args:        []string{validUnitId, validUnitId2, "different-application/0", "valid-action-name"},
		expectError: "all units must be of the same application",
	}, {
		should:      "fail with units from different applications, one using a leader identifier",
		args:        []string{"app1/leader", "app2/0", "valid-action-name"},
		expectError: "all units must be of the same application",
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
		should:       "use wait if specified",
		args:         []string{validUnitId, "action", "--wait", "20s"},
		expectUnits:  []string{validUnitId},
		expectAction: "action",
		expectWait:   20 * time.Second,
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
			wrappedCommand, command := action.NewRunCommandForTest(s.store, testClock(), nil)
			c.Logf("test %d: should %s:\n$ juju run (action) %s\n", i,
				t.should, strings.Join(t.args, " "))
			args := append([]string{modelFlag, "admin"}, t.args...)
			err := cmdtesting.InitCommand(wrappedCommand, args)
			if t.expectError == "" {
				c.Check(command.UnitNames(), tc.DeepEquals, t.expectUnits)
				c.Check(command.ActionName(), tc.Equals, t.expectAction)
				c.Check(command.ParamsYAML().Path, tc.Equals, t.expectParamsYamlPath)
				c.Check(command.Args(), tc.DeepEquals, t.expectKVArgs)
				c.Check(command.ParseStrings(), tc.Equals, t.expectParseStrings)
				if t.expectWait != 0 {
					c.Check(command.Wait(), tc.Equals, t.expectWait)
				} else {
					c.Check(command.Wait(), tc.Equals, 60*time.Second)
				}
			} else {
				c.Check(err, tc.ErrorMatches, t.expectError)
			}
		}
	}
}

func (s *RunSuite) TestRun(c *tc.C) {
	tests := []struct {
		should                 string
		clientSetup            func(client *fakeAPIClient)
		withArgs               []string
		withAPIErr             error
		withActionResults      []actionapi.ActionResult
		expectedActionEnqueued []actionapi.Action
		expectedOutput         string
		expectedErr            string
		expectedLogs           []string
	}{{
		should:   "fail with multiple results",
		withArgs: []string{validUnitId, "some-action"},
		withActionResults: []actionapi.ActionResult{
			{Action: &actionapi.Action{ID: validActionId}},
			{Action: &actionapi.Action{ID: validActionId}},
		},
		expectedErr: "illegal number of results returned",
	}, {
		should:   "fail with API error",
		withArgs: []string{validUnitId, "some-action"},
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{ID: validActionId}},
		},
		withAPIErr:  errors.New("something wrong in API"),
		expectedErr: "something wrong in API",
	}, {
		should:   "fail with error in result",
		withArgs: []string{validUnitId, "some-action"},
		withActionResults: []actionapi.ActionResult{{
			Error: errors.New("database error"),
		}},
		expectedErr: "database error",
		expectedOutput: `
Operation 1 failed to schedule any tasks:
database error`[1:],
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
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []actionapi.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
	}, {
		should:   "run a basic action with no params with output set to action-set data",
		withArgs: []string{validUnitId, "some-action"},
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
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
		expectedActionEnqueued: []actionapi.Action{{
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
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"return-code":     0,
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
		expectedActionEnqueued: []actionapi.Action{{
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
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"return-code":     0,
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
		expectedActionEnqueued: []actionapi.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
		expectedOutput: `
mysql/0:
  id: "1"
  results:
    outcome: success
    result-map:
      message: hello
    return-code: 0
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
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status: "completed",
			Output: map[string]interface{}{
				"return-code":     0,
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
		expectedActionEnqueued: []actionapi.Action{{
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
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Log: []actionapi.ActionMessage{{
				Timestamp: time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
				Message:   "log line 1",
			}, {
				Timestamp: time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
				Message:   "log line 2",
			}},
			Status: "completed",
			Output: map[string]interface{}{
				"return-code":     0,
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
		expectedActionEnqueued: []actionapi.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
		expectedLogs: []string{"log line 1", "log line 2"},
		expectedOutput: `
mysql/0:
  id: "1"
  log:
  - 2015-02-14 06:06:06 +0000 UTC log line 1
  - 2015-02-14 06:06:06 +0000 UTC log line 2
  results:
    outcome: success
    result-map:
      message: hello
    return-code: 0
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
		should:   "run action which fails with plain output selected",
		withArgs: []string{validUnitId, "some-action", "--format", "plain"},
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status:  params.ActionFailed,
			Message: "action failed msg",
			Output: map[string]interface{}{
				"outcome": "fail",
				"result-map": map[string]interface{}{
					"message": "failed :'(",
				},
			},
		}},
		expectedActionEnqueued: []actionapi.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   names.NewUnitTag(validUnitId).String(),
		}},
		expectedOutput: `
Action id 1 failed: action failed msg
outcome: fail
result-map:
  message: failed :'(`[1:],
	}, {
		should:   "run action on multiple units with stdout for each action",
		withArgs: []string{validUnitId, validUnitId2, "some-action", "--format", "yaml", "--utc"},
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
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
			Action: &actionapi.Action{
				ID:       validActionId2,
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
		expectedActionEnqueued: []actionapi.Action{{
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
  id: "1"
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
  id: "2"
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
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
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
			Action: &actionapi.Action{
				ID:       validActionId2,
				Receiver: names.NewUnitTag(validUnitId2).String(),
				Name:     "some-action",
			},
			Status: params.ActionCompleted,
			Output: map[string]interface{}{
				"outcome": "success",
				"result-map": map[string]interface{}{
					"message": "hello2",
				},
			},
		}},
		expectedActionEnqueued: []actionapi.Action{{
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
  id: "1"
  output: |
    outcome: success
    result-map:
      message: hello
  status: completed
mysql/1:
  id: "2"
  output: |
    outcome: success
    result-map:
      message: hello2
  status: completed`[1:],
	}, {
		should:   "run action on multiple units which fails with plain output selected",
		withArgs: []string{validUnitId, validUnitId2, "some-action", "--format", "plain"},
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status:  params.ActionFailed,
			Message: "action failed msg",
			Output: map[string]interface{}{
				"outcome": "fail",
				"result-map": map[string]interface{}{
					"message": "failed :'(",
				},
			},
		}, {
			Action: &actionapi.Action{
				ID:       validActionId2,
				Receiver: names.NewUnitTag(validUnitId2).String(),
				Name:     "some-action",
			},
			Status:  params.ActionFailed,
			Message: "action failed msg 2",
			Output: map[string]interface{}{
				"outcome": "fail",
				"result-map": map[string]interface{}{
					"message": "failed2 :'(",
				},
			},
		}},
		expectedActionEnqueued: []actionapi.Action{{
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
  id: "1"
  message: action failed msg
  output: |
    outcome: fail
    result-map:
      message: failed :'(
  status: failed
mysql/1:
  id: "2"
  message: action failed msg 2
  output: |
    outcome: fail
    result-map:
      message: failed2 :'(
  status: failed`[1:],
	}, {
		should:   "run action on multiple units which both pass and fail with plain output selected",
		withArgs: []string{validUnitId, validUnitId2, "some-action", "--format", "plain"},
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
				Name:     "some-action",
			},
			Status:  params.ActionFailed,
			Message: "action failed msg",
			Output: map[string]interface{}{
				"outcome": "fail",
				"result-map": map[string]interface{}{
					"message": "failed :'(",
				},
			},
		}, {
			Action: &actionapi.Action{
				ID:       validActionId2,
				Receiver: names.NewUnitTag(validUnitId2).String(),
				Name:     "some-action",
			},
			Status: params.ActionCompleted,
			Output: map[string]interface{}{
				"outcome": "success",
				"result-map": map[string]interface{}{
					"message": "pass",
				},
			},
		}},
		expectedActionEnqueued: []actionapi.Action{{
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
  id: "1"
  message: action failed msg
  output: |
    outcome: fail
    result-map:
      message: failed :'(
  status: failed
mysql/1:
  id: "2"
  output: |
    outcome: success
    result-map:
      message: pass
  status: completed`[1:],
	}, {
		should: "enqueue an action with some explicit params",
		withArgs: []string{validUnitId, "some-action", "--background",
			"out.name=bar",
			"out.kind=tmpfs",
			"out.num=3",
			"out.boolval=y",
		},
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []actionapi.Action{{
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
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []actionapi.Action{{
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
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []actionapi.Action{{
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
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []actionapi.Action{{
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
		withActionResults: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       validActionId,
				Receiver: names.NewUnitTag(validUnitId).String(),
			},
		}},
		expectedActionEnqueued: []actionapi.Action{{
			Name:       "some-action",
			Parameters: map[string]interface{}{},
			Receiver:   "mysql/leader",
		},
		}},
	}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			c.Logf("test %d: should %s:\n$ juju actions do %s\n", i, t.should, strings.Join(t.withArgs, " "))
			s.clock = testClock()

			fakeClient := &fakeAPIClient{
				actionResults: t.withActionResults,
				logMessageCh:  make(chan []string, len(t.expectedLogs)),
			}

			if len(t.expectedLogs) > 0 {
				fakeClient.waitForResults = make(chan bool)
			}
			if t.clientSetup != nil {
				t.clientSetup(fakeClient)
			}
			fakeClient.apiErr = t.withAPIErr
			fakeClient.actionResults = t.withActionResults

			s.testRunHelper(c,
				fakeClient,
				t.expectedErr,
				t.expectedOutput,
				modelFlag,
				t.withArgs,
				t.expectedActionEnqueued,
				t.expectedLogs)
		}
	}
}

func (s *RunSuite) TestVerbosity(c *tc.C) {
	tests := []struct {
		about   string
		verbose bool
		quiet   bool
		output  string
	}{{
		about: "normal output",
		output: `
Running operation 1 with 1 task
  - task 1 on unit-mysql-0

Waiting for task 1...

hello
`[1:],
	}, {
		about:   "verbose",
		verbose: true,
		output: `
Running operation 1 with 1 task
  - task 1 on unit-mysql-0

Waiting for task 1...

hello
`[1:],
	}, {
		about:  "quiet",
		quiet:  true,
		output: "\nhello\n",
	}}

	// Set up fake API client
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	fakeClient.actionResults = []actionapi.ActionResult{{
		Action: &actionapi.Action{
			ID:       validActionId,
			Receiver: names.NewUnitTag(validUnitId).String(),
			Name:     "some-action",
		},
		Output: map[string]interface{}{
			"stdout": "hello",
		},
	}}

	for i, t := range tests {
		c.Logf("test %d: %s", i, t.about)

		// Set up context
		output := bytes.Buffer{}
		ctx := &cmd.Context{
			Context: context.Background(),
			Stdout:  &output,
			Stderr:  &output,
		}
		log := cmd.Log{
			Verbose: t.verbose,
			Quiet:   t.quiet,
		}
		log.Start(ctx) // sets the verbose/quiet options in `ctx`

		// Run command
		runCmd, _ := action.NewRunCommandForTest(s.store, s.clock, nil)
		err := cmdtesting.InitCommand(runCmd, []string{"-m", "admin", validUnitId, "some-action"})
		c.Assert(err, tc.ErrorIsNil)
		err = runCmd.Run(ctx)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(output.String(), tc.Equals, t.output)
	}
}

func (s *RunSuite) testRunHelper(c *tc.C, client *fakeAPIClient,
	expectedErr, expectedOutput, modelFlag string, withArgs []string,
	expectedActionEnqueued []actionapi.Action,
	expectedLogs []string,
) {
	restore := s.patchAPIClient(client)
	defer restore()
	args := append([]string{modelFlag, "admin"}, withArgs...)

	if len(expectedLogs) > 0 {
		go func() {
			encodedLogs := make([]string, len(expectedLogs))
			for n, log := range expectedLogs {
				msg := actions.ActionMessage{
					Message:   log,
					Timestamp: time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
				}
				msgData, err := json.Marshal(msg)
				c.Assert(err, tc.ErrorIsNil)
				encodedLogs[n] = string(msgData)
			}
			client.logMessageCh <- encodedLogs
		}()
	}

	var receivedMessages []string
	var expectedLogMessages []string
	for _, msg := range expectedLogs {
		expectedLogMessages = append(expectedLogMessages, "06:06:06 "+msg)
	}
	runCmd, _ := action.NewRunCommandForTest(s.store, s.clock, func(_ *cmd.Context, msg string) {
		receivedMessages = append(receivedMessages, msg)
		if reflect.DeepEqual(receivedMessages, expectedLogMessages) {
			close(client.waitForResults)
		}
	})

	var (
		wg  sync.WaitGroup
		ctx *cmd.Context
		err error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, err = cmdtesting.RunCommand(c, runCmd, args...)
	}()

	wg.Wait()

	if len(expectedLogs) > 0 {
		select {
		case <-client.waitForResults:
		case <-time.After(testing.LongWait):
			c.Fatal("waiting for log messages to be consumed")
		}
	}

	if expectedErr != "" {
		if expectedOutput != "" {
			outputResult := ctx.Stderr.(*bytes.Buffer).Bytes()
			outString := strings.Trim(string(outputResult), "\n")
			c.Check(outString, tc.Equals, expectedOutput)
		} else {
			c.Check(err, tc.ErrorMatches, expectedErr)
		}
	} else {
		c.Assert(err, tc.IsNil)
		// Before comparing, double-check to avoid
		// panics in malformed tests.
		c.Assert(len(client.actionResults), tc.Not(tc.Equals), 0)
		// Make sure the test's expected actions were
		// non-nil and correct.
		for i := range client.actionResults {
			c.Assert(client.actionResults[i].Action, tc.NotNil)
		}
		// Make sure the action sent to the API to be
		// enqueued was indeed the expected map
		enqueued := client.enqueuedActions
		c.Assert(enqueued, tc.DeepEquals, expectedActionEnqueued)

		if expectedOutput == "" {
			outputResult := ctx.Stderr.(*bytes.Buffer).Bytes()
			outString := strings.Trim(string(outputResult), "\n")

			expectedID := client.actionResults[0].Action.ID
			valid := names.IsValidAction(expectedID)
			c.Assert(valid, tc.IsTrue)

			// Make sure the CLI responded with the expected tag
			c.Assert(outString, tc.Equals, fmt.Sprintf(`
Scheduled operation 1 with task %s
Check operation status with 'juju show-operation 1'
Check task status with 'juju show-task %s'`[1:],
				expectedID, expectedID))
		} else {
			outputResult := ctx.Stdout.(*bytes.Buffer).Bytes()
			outString := strings.Trim(string(outputResult), "\n")
			c.Assert(outString, tc.Equals, expectedOutput)
		}
	}
}
