// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/testing"
)

type ShowOperationSuite struct {
	BaseActionSuite
}

var _ = gc.Suite(&ShowOperationSuite{})

func (s *ShowOperationSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
}

func (s *ShowOperationSuite) TestInit(c *gc.C) {
	tests := []struct {
		should      string
		args        []string
		expectError string
	}{{
		should:      "fail with missing arg",
		args:        []string{},
		expectError: "no operation ID specified",
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
			cmd, _ := action.NewShowOperationCommandForTest(s.store, s.clock)
			args := append([]string{modelFlag, "admin"}, t.args...)
			err := cmdtesting.InitCommand(cmd, args)
			if t.expectError != "" {
				c.Check(err, gc.ErrorMatches, t.expectError)
			}
		}
	}
}

const operationId = "666"

func (s *ShowOperationSuite) TestRun(c *gc.C) {
	tests := []struct {
		should            string
		withClientWait    string
		withClientQueryID string
		withAPIDelay      time.Duration
		withAPITimeout    time.Duration
		withTicks         int
		withAPIResponse   []params.OperationResult
		withAPIError      string
		withFormat        string
		expectedErr       string
		expectedOutput    string
		watch             bool
	}{{
		should:            "timeout if result never comes",
		withClientWait:    "2s",
		withAPIDelay:      3 * time.Second,
		withAPITimeout:    5 * time.Second,
		withTicks:         1,
		withClientQueryID: operationId,
		withAPIResponse: []params.OperationResult{{
			OperationTag: names.NewOperationTag(operationId).String(),
			Status:       "running",
		}},
		expectedErr: "timeout reached",
		expectedOutput: `
status: pending
timing:
  enqueued: 2015-02-14 08:13:00 +0000 UTC
  started: 2015-02-14 08:15:00 +0000 UTC
`[1:],
	}, {
		should:            "pass api error through properly",
		withClientQueryID: operationId,
		withAPITimeout:    1 * time.Second,
		withAPIError:      "api call error",
		expectedErr:       "api call error",
	}, {
		should:            "fail with id not found",
		withClientQueryID: operationId,
		withAPITimeout:    1 * time.Second,
		expectedErr:       `operation "` + operationId + `" not found`,
	}, {
		should:            "pass through an error from the API server",
		withClientQueryID: operationId,
		withAPITimeout:    1 * time.Second,
		withAPIResponse: []params.OperationResult{{
			OperationTag: names.NewOperationTag(operationId).String(),
			Summary:      "an operation",
			Status:       "failed",
			Error:        apiservererrors.ServerError(errors.New("an apiserver error")),
		}},
		expectedOutput: `
summary: an operation
status: failed
error: an apiserver error
`[1:],
	}, {
		should:            "only return once status is no longer running or pending",
		withAPIDelay:      1 * time.Second,
		withClientWait:    "10s",
		withClientQueryID: operationId,
		withAPITimeout:    3 * time.Second,
		withTicks:         2,
		withAPIResponse: []params.OperationResult{{
			OperationTag: names.NewOperationTag(operationId).String(),
			Status:       "running",
			Actions: []params.ActionResult{{
				Output: map[string]interface{}{
					"foo": map[string]interface{}{
						"bar": "baz",
					},
				},
			}},
			Enqueued: time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:  time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		}},
		expectedErr: "test timed out before wait time",
	}, {
		should:            "pretty-print operation output",
		withClientQueryID: operationId,
		withAPITimeout:    1 * time.Second,
		withAPIResponse: []params.OperationResult{{
			OperationTag: names.NewOperationTag(operationId).String(),
			Summary:      "an operation",
			Status:       "complete",
			Actions: []params.ActionResult{{
				Action: &params.Action{
					Tag:        names.NewActionTag("69").String(),
					Receiver:   "foo/0",
					Name:       "backup",
					Parameters: map[string]interface{}{"hello": "world"},
				},
				Status:  "completed",
				Message: "oh dear",
				Output: map[string]interface{}{
					"foo": map[string]interface{}{
						"bar": "baz",
					},
				},
			}},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedOutput: `
summary: an operation
status: complete
action:
  name: backup
  parameters:
    hello: world
timing:
  enqueued: 2015-02-14 08:13:00 +0000 UTC
  started: 2015-02-14 08:15:00 +0000 UTC
  completed: 2015-02-14 08:15:30 +0000 UTC
tasks:
  "69":
    host: foo/0
    status: completed
    message: oh dear
    results:
      foo:
        bar: baz
`[1:],
	}, {
		should:            "pretty-print action output with no completed time",
		withClientQueryID: operationId,
		withClientWait:    "1s",
		withAPITimeout:    2 * time.Second,
		withTicks:         1,
		withAPIResponse: []params.OperationResult{{
			OperationTag: names.NewOperationTag(operationId).String(),
			Summary:      "an operation",
			Status:       "pending",
			Actions: []params.ActionResult{{
				Action: &params.Action{
					Tag:        names.NewActionTag("69").String(),
					Receiver:   "foo/0",
					Name:       "backup",
					Parameters: map[string]interface{}{"hello": "world"},
				},
				Status:  "pending",
				Message: "oh dear",
				Output: map[string]interface{}{
					"foo": map[string]interface{}{
						"bar": "baz",
					},
				},
			}},
			Enqueued: time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Started:  time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		}},
		expectedErr: "timeout reached",
		expectedOutput: `
summary: an operation
status: complete
action:
  name: backup
  parameters:
    hello: world
timing:
  enqueued: 2015-02-14 08:13:00 +0000 UTC
  started: 2015-02-14 08:15:00 +0000 UTC
tasks:
  "69":
    host: foo/0
    status: completed
    message: oh dear
    results:
      foo:
        bar: baz
`[1:],
	}, {
		should:            "set an appropriate timer and wait, get a result",
		withClientQueryID: operationId,
		withAPITimeout:    5 * time.Second,
		withClientWait:    "3s",
		withAPIDelay:      1 * time.Second,
		withTicks:         1,
		withAPIResponse: []params.OperationResult{{
			OperationTag: names.NewOperationTag(operationId).String(),
			Summary:      "an operation",
			Status:       "completed",
			Actions: []params.ActionResult{{
				Action: &params.Action{
					Tag:      names.NewActionTag("69").String(),
					Name:     "backup",
					Receiver: "foo/0",
				},
				Status: "completed",
				Output: map[string]interface{}{
					"foo": map[string]interface{}{
						"bar": "baz",
					},
				},
			}},
			Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedOutput: `
summary: an operation
status: completed
action:
  name: backup
  parameters: {}
timing:
  enqueued: 2015-02-14 08:13:00 +0000 UTC
  completed: 2015-02-14 08:15:30 +0000 UTC
tasks:
  "69":
    host: foo/0
    status: completed
    results:
      foo:
        bar: baz
`[1:],
	}, {
		should:            "watch, wait, get a result",
		withClientQueryID: operationId,
		watch:             true,
		withAPIResponse: []params.OperationResult{{
			OperationTag: names.NewOperationTag(operationId).String(),
			Summary:      "an operation",
			Status:       "completed",
			Enqueued:     time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
			Completed:    time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
		}},
		expectedOutput: `
summary: an operation
status: completed
timing:
  enqueued: 2015-02-14 08:13:00 +0000 UTC
  completed: 2015-02-14 08:15:30 +0000 UTC
`[1:],
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			c.Logf("test %d (model option %v): should %s", i, modelFlag, t.should)
			s.clock = testClock()
			fakeClient := s.makeFakeClient(
				t.withAPIDelay,
				t.withAPITimeout,
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
			)
		}
	}
}

func (s *ShowOperationSuite) testRunHelper(c *gc.C, client *fakeAPIClient,
	expectedErr, expectedOutput, format, wait, query, modelFlag string,
	watch bool, numTicks int, numExpectedTimers int,
) {
	unpatch := s.patchAPIClient(client)
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

	runCmd, _ := action.NewShowOperationCommandForTest(s.store, s.clock)

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

	if expectedErr != "" {
		c.Check(err, gc.ErrorMatches, expectedErr)
	} else {
		c.Assert(err, gc.IsNil)
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, expectedOutput)
	}
}

func (s *ShowOperationSuite) makeFakeClient(
	delay, timeout time.Duration,
	response []params.OperationResult,
	errStr string,
) *fakeAPIClient {
	var delayTimer clock.Timer
	if delay != 0 {
		delayTimer = s.clock.NewTimer(delay)
	}
	client := &fakeAPIClient{
		delay:            delayTimer,
		timeout:          s.clock.NewTimer(timeout),
		operationResults: response,
	}
	if errStr != "" {
		client.apiErr = errors.New(errStr)
	}
	return client
}
