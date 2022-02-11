// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	actionapi "github.com/juju/juju/api/action"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type ShowOutputSuite struct {
	BaseActionSuite
}

var _ = gc.Suite(&ShowOutputSuite{})

func (s *ShowOutputSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
}

func (s *ShowOutputSuite) TestInit(c *gc.C) {
	tests := []struct {
		should      string
		args        []string
		expectError string
	}{{
		should:      "fail with missing arg",
		args:        []string{},
		expectError: "no action ID specified",
	}, {
		should:      "fail with multiple args",
		args:        []string{"12345", "54321"},
		expectError: `unrecognized args: \["54321"\]`,
	}, {
		should:      "fail with both wait and watch",
		args:        []string{"--wait", "0s", "--watch"},
		expectError: `specify either --watch or --wait but not both`,
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			c.Logf("test %d: it should %s: juju show-operation %s", i,
				t.should, strings.Join(t.args, " "))
			cmd, _ := action.NewShowOutputCommandForTest(s.store, nil)
			args := append([]string{modelFlag, "admin"}, t.args...)
			err := cmdtesting.InitCommand(cmd, args)
			if t.expectError != "" {
				c.Check(err, gc.ErrorMatches, t.expectError)
			}
		}
	}
}

func (s *ShowOutputSuite) TestRun(c *gc.C) {
	tests := []struct {
		should            string
		withClientWait    string
		withClientQueryID string
		withAPIDelay      time.Duration
		withAPITimeout    time.Duration
		withTags          params.FindTagsResults
		withAPIResponse   []actionapi.ActionResult
		withAPIError      string
		withFormat        string
		expectedErr       string
		expectedOutput    string
		expectedLogs      []string
		watch             bool
	}{{
		should:         "handle wait-time formatting errors",
		withClientWait: "not-a-duration-at-all",
		expectedErr:    `time: invalid duration "?not-a-duration-at-all"?`,
	}, {
		should:            "timeout if result never comes",
		withClientWait:    "2s",
		withAPIDelay:      3 * time.Second,
		withAPITimeout:    5 * time.Second,
		withClientQueryID: validActionId,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse:   []actionapi.ActionResult{{}},
		expectedErr:       "timeout reached",
		expectedOutput: `
status: pending
timing:
  enqueued: 2015-02-14 08:13:00 +0000 UTC
  started: 2015-02-14 08:15:00 +0000 UTC
`[1:],
	}, {
		should:            "pass api error through properly",
		withClientQueryID: validActionId,
		withAPITimeout:    1 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIError:      "api call error",
		expectedErr:       "api call error",
	}, {
		should:            "fail with no tag matches",
		withClientQueryID: validActionId,
		withAPITimeout:    1 * time.Second,
		withTags:          tagsForIdPrefix(validActionId),
		expectedErr:       `actions for identifier "` + validActionId + `" not found`,
	}, {
		should:            "fail with no results",
		withClientQueryID: validActionId,
		withAPITimeout:    1 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse:   []actionapi.ActionResult{},
		expectedErr:       "no results for action " + validActionId,
	}, {
		should:            "error correctly with multiple results",
		withClientQueryID: validActionId,
		withAPITimeout:    1 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse:   []actionapi.ActionResult{{}, {}},
		expectedErr:       "too many results for action " + validActionId,
	}, {
		should:            "pass through an error from the API server",
		withClientQueryID: validActionId,
		withAPITimeout:    1 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse: []actionapi.ActionResult{{
			Error: apiservererrors.ServerError(errors.New("an apiserver error")),
		}},
		expectedErr: "an apiserver error",
	}, {
		should:            "only return once status is no longer running or pending",
		withAPIDelay:      1 * time.Second,
		withClientWait:    "10s",
		withClientQueryID: validActionId,
		withAPITimeout:    3 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse: []actionapi.ActionResult{{
			Status: "running",
			Output: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "baz",
				},
			},
			Enqueued: time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:  time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		}},
		expectedErr: "test timed out before wait time",
	}, {
		should:            "pretty-print action output",
		withClientQueryID: validActionId,
		withAPITimeout:    1 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse: []actionapi.ActionResult{{
			Status:  "complete",
			Message: "oh dear",
			Output: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "baz",
				},
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedOutput: `
id: f47ac10b-58cc-4372-a567-0e02b2c3d479
message: oh dear
results:
  foo:
    bar: baz
status: complete
timing:
  completed: 2015-02-14 08:15:30 +0000 UTC
  enqueued: 2015-02-14 08:13:00 +0000 UTC
  started: 2015-02-14 08:15:00 +0000 UTC
`[1:],
	}, {
		should:            "pretty-print action output with no completed time",
		withClientQueryID: validActionId,
		withClientWait:    "1s",
		withAPITimeout:    2 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse: []actionapi.ActionResult{{
			Status: "pending",
			Output: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "baz",
				},
			},
			Enqueued: time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:  time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		}},
		expectedErr: "timeout reached",
		expectedOutput: `
results:
  foo:
    bar: baz
status: pending
timing:
  enqueued: 2015-02-14 08:13:00 +0000 UTC
  started: 2015-02-14 08:15:00 +0000 UTC
`[1:],
	}, {
		should:            "pretty-print action output with no enqueued time",
		withClientQueryID: validActionId,
		withClientWait:    "1s",
		withAPITimeout:    2 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse: []actionapi.ActionResult{{
			Status: "pending",
			Output: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "baz",
				},
			},
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		}},
		expectedErr: "timeout reached",
		expectedOutput: `
id: f47ac10b-58cc-4372-a567-0e02b2c3d479
results:
  foo:
    bar: baz
status: pending
timing:
  completed: 2015-02-14 08:15:30 +0000 UTC
  started: 2015-02-14 08:15:00 +0000 UTC
`[1:],
	}, {
		should:            "pretty-print action output with no started time",
		withClientQueryID: validActionId,
		withClientWait:    "1s",
		withAPITimeout:    2 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse: []actionapi.ActionResult{{
			Status: "pending",
			Output: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "baz",
				},
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedErr: "timeout reached",
		expectedOutput: `
id: f47ac10b-58cc-4372-a567-0e02b2c3d479
results:
  foo:
    bar: baz
status: pending
timing:
  completed: 2015-02-14 08:15:30 +0000 UTC
  enqueued: 2015-02-14 08:13:00 +0000 UTC
`[1:],
	}, {
		should:            "plain format action output",
		withClientQueryID: validActionId,
		withClientWait:    "1s",
		withAPITimeout:    2 * time.Second,
		withFormat:        "plain",
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse: []actionapi.ActionResult{{
			Status:  "complete",
			Message: "oh dear",
			Output: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "baz",
				},
				"stdout": "hello",
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedOutput: `
foo:
  bar: baz

hello

`[1:],
	}, {
		should:            "set an appropriate timer and wait, get a result",
		withClientQueryID: validActionId,
		withAPITimeout:    5 * time.Second,
		withClientWait:    "3s",
		withAPIDelay:      1 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		withAPIResponse: []actionapi.ActionResult{{
			Status: "completed",
			Output: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "baz",
				},
			},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedOutput: `
id: f47ac10b-58cc-4372-a567-0e02b2c3d479
results:
  foo:
    bar: baz
status: completed
timing:
  completed: 2015-02-14 08:15:30 +0000 UTC
  enqueued: 2015-02-14 08:13:00 +0000 UTC
`[1:],
	}, {
		should:            "watch, wait, get a result",
		withClientQueryID: validActionId,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		watch:             true,
		withAPIResponse: []actionapi.ActionResult{{
			Status:    "completed",
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedOutput: `
id: f47ac10b-58cc-4372-a567-0e02b2c3d479
status: completed
timing:
  completed: 2015-02-14 08:15:30 +0000 UTC
  enqueued: 2015-02-14 08:13:00 +0000 UTC
`[1:],
	}, {
		should:            "print log messages when watching",
		withClientQueryID: validActionId,
		withAPITimeout:    5 * time.Second,
		withAPIDelay:      1 * time.Second,
		withTags:          tagsForIdPrefix(validActionId, validActionTagString),
		watch:             true,
		expectedLogs:      []string{"log line 1", "log line 2"},
		withAPIResponse: []actionapi.ActionResult{{
			Status:    "completed",
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedOutput: `
id: f47ac10b-58cc-4372-a567-0e02b2c3d479
status: completed
timing:
  completed: 2015-02-14 08:15:30 +0000 UTC
  enqueued: 2015-02-14 08:13:00 +0000 UTC
`[1:],
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			c.Logf("test %d (model option %v): should %s", i, modelFlag, t.should)
			fakeClient := makeFakeClient(
				t.withAPIDelay,
				t.withAPITimeout,
				t.withTags,
				t.withAPIResponse,
				map[string][]actionapi.ActionResult{},
				t.withAPIError,
			)
			fakeClient.logMessageCh = make(chan []string, len(t.expectedLogs))
			if len(t.expectedLogs) > 0 {
				fakeClient.waitForResults = make(chan bool)
			}
			testRunHelper(
				c, s,
				fakeClient,
				t.expectedErr,
				t.expectedOutput,
				t.withFormat,
				t.withClientWait,
				t.withClientQueryID,
				modelFlag,
				t.watch,
				t.expectedLogs,
			)
		}
	}
}

func testRunHelper(c *gc.C, s *ShowOutputSuite, client *fakeAPIClient,
	expectedErr, expectedOutput, format, wait, query, modelFlag string,
	watch bool,
	expectedLogs []string,
) {
	unpatch := s.BaseActionSuite.patchAPIClient(client)
	defer unpatch()
	args := append([]string{modelFlag, "admin"}, query, "--utc")
	if wait != "" {
		args = append(args, "--wait", wait)
	}
	if format != "" {
		args = append(args, "--format", format)
	}
	if watch {
		args = append(args, "--watch")
	}

	if len(expectedLogs) > 0 {
		go func() {
			encodedLogs := make([]string, len(expectedLogs))
			for n, log := range expectedLogs {
				msg := actions.ActionMessage{
					Message:   log,
					Timestamp: time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
				}
				msgData, err := json.Marshal(msg)
				c.Assert(err, jc.ErrorIsNil)
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
	cmd, _ := action.NewShowOutputCommandForTest(s.store, func(_ *cmd.Context, msg string) {
		receivedMessages = append(receivedMessages, msg)
		if reflect.DeepEqual(receivedMessages, expectedLogMessages) {
			close(client.waitForResults)
		}
	})

	ctx, err := cmdtesting.RunCommand(c, cmd, args...)

	if len(expectedLogMessages) > 0 {
		select {
		case <-client.waitForResults:
		case <-time.After(testing.LongWait):
			c.Fatal("waiting for log messages to be consumed")
		}
	}

	if expectedErr != "" {
		c.Check(err, gc.ErrorMatches, expectedErr)
	} else {
		c.Assert(err, gc.IsNil)
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, expectedOutput)
	}
}

func makeFakeClient(
	delay, timeout time.Duration,
	tags params.FindTagsResults,
	response []actionapi.ActionResult,
	actionsByNames map[string][]actionapi.ActionResult,
	errStr string,
) *fakeAPIClient {
	var delayTimer *time.Timer
	if delay != 0 {
		delayTimer = time.NewTimer(delay)
	}
	client := &fakeAPIClient{
		delay:            delayTimer,
		timeout:          time.NewTimer(timeout),
		actionTagMatches: tags,
		actionResults:    response,
		actionsByNames:   actionsByNames,
		apiVersion:       5,
	}
	if errStr != "" {
		client.apiErr = errors.New(errStr)
	}
	return client
}
