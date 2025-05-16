// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
)

type ShowTaskSuite struct {
	BaseActionSuite
}

func TestShowTaskSuite(t *stdtesting.T) { tc.Run(t, &ShowTaskSuite{}) }
func (s *ShowTaskSuite) TestInit(c *tc.C) {
	tests := []struct {
		should      string
		args        []string
		expectError string
	}{{
		should:      "fail with missing arg",
		args:        []string{},
		expectError: "no task ID specified",
	}, {
		should:      "fail with invalid task ID",
		args:        []string{"test"},
		expectError: "task ID \"test\" not valid",
	}, {
		should:      "fail with multiple args",
		args:        []string{"12345", "54321"},
		expectError: `unrecognized args: \["54321"\]`,
	}, {
		should:      "fail with both wait and watch",
		args:        []string{"--wait", "0s", "--watch"},
		expectError: `specify either --watch or --wait but not both`,
	}, {
		should:      "invalid wait time",
		args:        []string{"--wait", "not-a-duration-at-all"},
		expectError: `.*time: invalid duration "?not-a-duration-at-all"?`,
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			c.Logf("test %d: it should %s: juju show-operation %s", i,
				t.should, strings.Join(t.args, " "))
			cmd, _ := action.NewShowTaskCommandForTest(s.store, s.clock, nil)
			args := append([]string{modelFlag, "admin"}, t.args...)
			err := cmdtesting.InitCommand(cmd, args)
			if t.expectError != "" {
				c.Check(err, tc.ErrorMatches, t.expectError)
			}
		}
	}
}

func (s *ShowTaskSuite) TestRun(c *tc.C) {
	tests := []struct {
		should            string
		withClientWait    string
		withClientQueryID string
		withAPIDelay      time.Duration
		withAPITimeout    time.Duration
		withAPIResponse   []actionapi.ActionResult
		withAPIError      string
		withFormat        string
		expectedErr       string
		expectedOutput    string
		expectedLogs      []string
		watch             bool
	}{{
		should:            "timeout if result never comes",
		withClientWait:    "2s",
		withAPIDelay:      10 * time.Second,
		withClientQueryID: validActionId,
		withAPIResponse:   []actionapi.ActionResult{{Status: "pending"}},
		expectedErr:       "maximum wait time reached",
		expectedOutput: `
status: pending
timing:
  enqueued: 2015-02-14 08:13:00 +0000 UTC
  started: 2015-02-14 08:15:00 +0000 UTC
`[1:],
	}, {
		should:            "pass api error through properly",
		withClientQueryID: validActionId,
		withAPIError:      "api call error",
		expectedErr:       "api call error",
	}, {
		should:            "fail with no results",
		withClientQueryID: validActionId,
		expectedErr:       "task " + validActionId + " not found",
	}, {
		should:            "error correctly with multiple results",
		withClientQueryID: validActionId,
		withAPIResponse:   []actionapi.ActionResult{{}, {}},
		expectedErr:       "too many results for task " + validActionId,
	}, {
		should:            "pass through an error from the API server",
		withClientQueryID: validActionId,
		withAPIResponse: []actionapi.ActionResult{{
			Error: errors.New("an apiserver error"),
		}},
		expectedErr: "an apiserver error",
	}, {
		should:            "pretty-print action output",
		withClientQueryID: validActionId,
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
id: "1"
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
		expectedErr: "maximum wait time reached",
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
		expectedErr: "maximum wait time reached",
		expectedOutput: `
id: "1"
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
		expectedErr: "maximum wait time reached",
		expectedOutput: `
id: "1"
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
		withFormat:        "plain",
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
		withClientWait:    "10s",
		withAPIDelay:      1 * time.Second,
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
id: "1"
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
		watch:             true,
		withAPIResponse: []actionapi.ActionResult{{
			Status:    "completed",
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedOutput: `
id: "1"
status: completed
timing:
  completed: 2015-02-14 08:15:30 +0000 UTC
  enqueued: 2015-02-14 08:13:00 +0000 UTC
`[1:],
	}, {
		should:            "print log messages when watching",
		withClientQueryID: validActionId,
		withAPIDelay:      1 * time.Second,
		watch:             true,
		expectedLogs:      []string{"log line 1", "log line 2"},
		withAPIResponse: []actionapi.ActionResult{{
			Status:    "completed",
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedOutput: `
id: "1"
status: completed
timing:
  completed: 2015-02-14 08:15:30 +0000 UTC
  enqueued: 2015-02-14 08:13:00 +0000 UTC
`[1:],
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			c.Logf("test %d (model option %v): should %s", i, modelFlag, t.should)
			s.clock = testClock()
			fakeClient := s.makeFakeClient(
				t.withAPIDelay,
				t.withAPITimeout, // test timeout
				t.withAPIResponse,
				t.withAPIError,
			)

			fakeClient.logMessageCh = make(chan []string, len(t.expectedLogs))
			if len(t.expectedLogs) > 0 {
				fakeClient.waitForResults = make(chan bool)
			}
			s.testRunHelper(
				c,
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

func (s *ShowTaskSuite) testRunHelper(c *tc.C, client *fakeAPIClient,
	expectedErr, expectedOutput, format, wait, query, modelFlag string,
	watch bool, expectedLogs []string,
) {
	unpatch := s.patchAPIClient(client)
	defer unpatch()
	args := append([]string{modelFlag, "admin"}, query, "--utc")
	if wait != "" {
		args = append(args, "--wait", wait)
	}
	if format != "" {
		args = append(args, "--format", format)
	} else {
		args = append(args, "--format", "yaml")
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
	runCmd, _ := action.NewShowTaskCommandForTest(s.store, s.clock, func(_ *cmd.Context, msg string) {
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

	if len(expectedLogMessages) > 0 {
		select {
		case <-client.waitForResults:
		case <-time.After(testing.LongWait):
			c.Fatal("waiting for log messages to be consumed")
		}
	}

	if expectedErr != "" {
		c.Check(err, tc.ErrorMatches, expectedErr)
	} else {
		c.Assert(err, tc.IsNil)
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, expectedOutput)
	}
}

func (s *ShowTaskSuite) makeFakeClient(
	delay, timeout time.Duration,
	response []actionapi.ActionResult,
	errStr string,
) *fakeAPIClient {
	var delayTimer clock.Timer
	if delay != 0 {
		delayTimer = s.clock.NewTimer(delay)
	}
	if timeout == 0 {
		timeout = testing.LongWait
	}
	client := &fakeAPIClient{
		delay:         delayTimer,
		timeout:       clock.WallClock.NewTimer(timeout),
		actionResults: response,
	}
	if errStr != "" {
		client.apiErr = errors.New(errStr)
	}
	return client
}
