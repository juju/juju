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
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/testing"
)

type ShowTaskSuite struct {
	BaseActionSuite
}

var _ = gc.Suite(&ShowTaskSuite{})

func (s *ShowTaskSuite) TestInit(c *gc.C) {
	tests := []struct {
		should      string
		args        []string
		expectError string
	}{{
		should:      "fail with missing arg",
		args:        []string{},
		expectError: "no task ID specified",
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
				c.Check(err, gc.ErrorMatches, t.expectError)
			}
		}
	}
}

func (s *ShowTaskSuite) TestRun(c *gc.C) {
	tests := []struct {
		should            string
		withClientWait    string
		withClientQueryID string
		withAPIDelay      time.Duration
		withAPITimeout    time.Duration
		withTicks         int
		withAPIResponse   []params.ActionResult
		withAPIError      string
		withFormat        string
		expectedErr       string
		expectedOutput    string
		expectedLogs      []string
		watch             bool
	}{{
		should:            "timeout if result never comes",
		withClientWait:    "2s",
		withAPIDelay:      3 * time.Second,
		withAPITimeout:    5 * time.Second,
		withTicks:         1,
		withClientQueryID: validActionId,
		withAPIResponse:   []params.ActionResult{{Status: "pending"}},
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
		withAPITimeout:    1 * time.Second,
		withAPIError:      "api call error",
		expectedErr:       "api call error",
	}, {
		should:            "fail with no results",
		withClientQueryID: validActionId,
		withAPITimeout:    1 * time.Second,
		expectedErr:       "task " + validActionId + " not found",
	}, {
		should:            "error correctly with multiple results",
		withClientQueryID: validActionId,
		withAPITimeout:    1 * time.Second,
		withAPIResponse:   []params.ActionResult{{}, {}},
		expectedErr:       "too many results for task " + validActionId,
	}, {
		should:            "pass through an error from the API server",
		withClientQueryID: validActionId,
		withAPITimeout:    1 * time.Second,
		withAPIResponse: []params.ActionResult{{
			Error: apiservererrors.ServerError(errors.New("an apiserver error")),
		}},
		expectedErr: "an apiserver error",
	}, {
		should:            "only return once status is no longer running or pending",
		withAPIDelay:      1 * time.Second,
		withClientWait:    "10s",
		withTicks:         2,
		withClientQueryID: validActionId,
		withAPITimeout:    3 * time.Second,
		withAPIResponse: []params.ActionResult{{
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
		withAPIResponse: []params.ActionResult{{
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
		withAPITimeout:    2 * time.Second,
		withTicks:         1,
		withAPIResponse: []params.ActionResult{{
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
		withAPITimeout:    2 * time.Second,
		withTicks:         1,
		withAPIResponse: []params.ActionResult{{
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
		withAPITimeout:    2 * time.Second,
		withTicks:         1,
		withAPIResponse: []params.ActionResult{{
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
		withAPITimeout:    2 * time.Second,
		withFormat:        "plain",
		withAPIResponse: []params.ActionResult{{
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
		withTicks:         1,
		withAPIResponse: []params.ActionResult{{
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
		withAPIResponse: []params.ActionResult{{
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
		withAPITimeout:    5 * time.Second,
		withAPIDelay:      1 * time.Second,
		watch:             true,
		expectedLogs:      []string{"log line 1", "log line 2"},
		withTicks:         1,
		withAPIResponse: []params.ActionResult{{
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

			numExpectedTimers := 0
			// Ensure the api timeout timer is registered.
			if t.withAPITimeout > 0 {
				numExpectedTimers++
			}
			// And the api delay timer.
			if t.withAPIDelay > 0 {
				numExpectedTimers++
			}
			err := s.clock.WaitAdvance(0*time.Second, testing.ShortWait, numExpectedTimers)
			c.Assert(err, jc.ErrorIsNil)

			// Ensure the cmd max wait timer is registered. But this only happens
			// during Run() so check for it later.
			if t.withClientWait != "" {
				numExpectedTimers++
			}

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
				t.withTicks,
				numExpectedTimers,
				t.expectedLogs,
			)
		}
	}
}

func (s *ShowTaskSuite) testRunHelper(c *gc.C, client *fakeAPIClient,
	expectedErr, expectedOutput, format, wait, query, modelFlag string,
	watch bool,
	numTicks int,
	numExpectedTimers int,
	expectedLogs []string,
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

	if numTicks > 0 {
		numExpectedTimers += 1
	}
	for t := 0; t < numTicks; t++ {
		err2 := s.clock.WaitAdvance(2*time.Second, testing.ShortWait, numExpectedTimers)
		c.Assert(err2, jc.ErrorIsNil)
		numExpectedTimers--
	}
	wg.Wait()

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

func (s *ShowTaskSuite) makeFakeClient(
	delay, timeout time.Duration,
	response []params.ActionResult,
	errStr string,
) *fakeAPIClient {
	var delayTimer clock.Timer
	if delay != 0 {
		delayTimer = s.clock.NewTimer(delay)
	}
	client := &fakeAPIClient{
		delay:         delayTimer,
		timeout:       s.clock.NewTimer(timeout),
		actionResults: response,
	}
	if errStr != "" {
		client.apiErr = errors.New(errStr)
	}
	return client
}
