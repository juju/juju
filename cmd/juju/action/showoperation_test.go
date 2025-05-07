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
	"github.com/juju/tc"

	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
)

type ShowOperationSuite struct {
	BaseActionSuite
}

var _ = tc.Suite(&ShowOperationSuite{})

func (s *ShowOperationSuite) SetUpTest(c *tc.C) {
	s.BaseActionSuite.SetUpTest(c)
}

func (s *ShowOperationSuite) TestInit(c *tc.C) {
	tests := []struct {
		should      string
		args        []string
		expectError string
	}{{
		should:      "fail with missing arg",
		args:        []string{},
		expectError: "no operation ID specified",
	}, {
		should:      "fail with invalid operation ID",
		args:        []string{"test"},
		expectError: "operation ID \"test\" not valid",
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
				c.Check(err, tc.ErrorMatches, t.expectError)
			}
		}
	}
}

const operationId = "666"

func (s *ShowOperationSuite) TestRun(c *tc.C) {
	tests := []struct {
		should            string
		withClientWait    string
		withClientQueryID string
		withAPIDelay      time.Duration
		withAPITimeout    time.Duration
		withAPIResponse   actionapi.Operations
		withAPIError      string
		withFormat        string
		expectedErr       string
		expectedOutput    string
		watch             bool
	}{{
		should:            "timeout if result never comes",
		withClientWait:    "2s",
		withAPIDelay:      10 * time.Second,
		withClientQueryID: operationId,
		withAPIResponse: actionapi.Operations{
			Operations: []actionapi.Operation{{
				ID:     operationId,
				Status: "running",
			}},
		},
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
		withAPIError:      "api call error",
		expectedErr:       "api call error",
	}, {
		should:            "fail with id not found",
		withClientQueryID: operationId,
		expectedErr:       `operation "` + operationId + `" not found`,
	}, {
		should:            "pass through an error from the API server",
		withClientQueryID: operationId,
		withAPIResponse: actionapi.Operations{
			Operations: []actionapi.Operation{{
				ID:      operationId,
				Summary: "an operation",
				Status:  "failed",
				Error:   errors.New("an apiserver error"),
			}},
		},
		expectedOutput: `
summary: an operation
status: failed
error: an apiserver error
`[1:],
	}, {
		should:            "pretty-print operation output",
		withClientQueryID: operationId,
		withAPIResponse: actionapi.Operations{
			Operations: []actionapi.Operation{{
				ID:      operationId,
				Summary: "an operation",
				Status:  "complete",
				Actions: []actionapi.ActionResult{{
					Action: &actionapi.Action{
						ID:         "69",
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
		},
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
		withAPIResponse: actionapi.Operations{
			Operations: []actionapi.Operation{{
				ID:      operationId,
				Summary: "an operation",
				Status:  "pending",
				Actions: []actionapi.ActionResult{{
					Action: &actionapi.Action{
						ID:         "69",
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
		},
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
		withClientWait:    "10s",
		withAPIDelay:      1 * time.Second,
		withAPIResponse: actionapi.Operations{
			Operations: []actionapi.Operation{{
				ID:      operationId,
				Summary: "an operation",
				Status:  "completed",
				Actions: []actionapi.ActionResult{{
					Action: &actionapi.Action{
						ID:       "69",
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
		},
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
		withAPIResponse: actionapi.Operations{
			Operations: []actionapi.Operation{{
				ID:        operationId,
				Summary:   "an operation",
				Status:    "completed",
				Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
				Completed: time.Date(2015, time.February, 14, 8, 15, 30, 0, time.UTC),
			}},
		},
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
			)
		}
	}
}

func (s *ShowOperationSuite) testRunHelper(c *tc.C, client *fakeAPIClient,
	expectedErr, expectedOutput, format, wait, query, modelFlag string,
	watch bool,
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

	wg.Wait()

	if expectedErr != "" {
		c.Check(err, tc.ErrorMatches, expectedErr)
	} else {
		c.Assert(err, tc.IsNil)
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, expectedOutput)
	}
}

func (s *ShowOperationSuite) makeFakeClient(
	delay, timeout time.Duration,
	response actionapi.Operations,
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
		delay:            delayTimer,
		timeout:          clock.WallClock.NewTimer(timeout),
		operationResults: response,
	}
	if errStr != "" {
		client.apiErr = errors.New(errStr)
	}
	return client
}
