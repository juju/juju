// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/juju/tc"

	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type ListOperationsSuite struct {
	BaseActionSuite
	wrappedCommand cmd.Command
	command        *action.ListOperationsCommand
}

func TestListOperationsSuite(t *testing.T) {
	tc.Run(t, &ListOperationsSuite{})
}

func (s *ListOperationsSuite) SetUpTest(c *tc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
}

func (s *ListOperationsSuite) TestInit(c *tc.C) {
	tests := []struct {
		should      string
		args        []string
		expectedErr string
	}{{
		should:      "fail with invalid application name",
		args:        []string{"--apps", "valid," + invalidApplicationId},
		expectedErr: "invalid application name \"" + invalidApplicationId + "\"",
	}, {
		should:      "fail with invalid unit name",
		args:        []string{"--units", "valid/0," + invalidUnitId},
		expectedErr: "invalid unit name \"" + invalidUnitId + "\"",
	}, {
		should:      "fail with invalid machine id",
		args:        []string{"--machines", "0," + invalidMachineId},
		expectedErr: "invalid machine id \"" + invalidMachineId + "\"",
	}, {
		should:      "fail with invalid status value",
		args:        []string{"--status", "pending," + "foo"},
		expectedErr: `"foo" is not a valid task status, want one of \[pending running completed failed cancelled aborting aborted error\]`,
	}, {
		should:      "fail with multiple errors",
		args:        []string{"--units", "valid/0," + invalidUnitId, "--apps", "valid," + invalidApplicationId},
		expectedErr: "invalid application name \"" + invalidApplicationId + "\"\ninvalid unit name \"" + invalidUnitId + "\"",
	}, {
		should:      "fail with too many args",
		args:        []string{"any"},
		expectedErr: `unrecognized args: \["any"\]`,
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			c.Logf("test %d should %s: juju operations defined %s", i,
				t.should, strings.Join(t.args, " "))
			s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
			args := append([]string{modelFlag, "admin"}, t.args...)
			err := cmdtesting.InitCommand(s.wrappedCommand, args)
			if t.expectedErr == "" {
				c.Check(err, tc.ErrorIsNil)
			} else {
				c.Check(err, tc.ErrorMatches, t.expectedErr)
			}
		}
	}
}

func (s *ListOperationsSuite) TestRunQueryArgs(c *tc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListOperationsCommandForTest(s.store)
	args := []string{
		"--apps", "mysql,mediawiki",
		"--units", "mysql/1,mediawiki/0",
		"--machines", "0,1",
		"--actions", "backup",
		"--status", "completed,pending",
	}
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
		args = append([]string{modelFlag, "admin", "--utc"}, args...)

		_, err := cmdtesting.RunCommand(c, s.wrappedCommand, args...)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(fakeClient.operationQueryArgs, tc.DeepEquals, actionapi.OperationQueryArgs{
			Applications: []string{"mysql", "mediawiki"},
			Units:        []string{"mysql/1", "mediawiki/0"},
			Machines:     []string{"0", "1"},
			ActionNames:  []string{"backup"},
			Status:       []string{"completed", "pending"},
		})
	}
}

var listOperationResults = actionapi.Operations{
	Operations: []actionapi.Operation{{
		Actions: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       "2",
				Receiver: "unit-mysql-0",
				Name:     "backup",
			},
		}},
		Summary:   "operation 1",
		Fail:      "fail",
		ID:        "1",
		Enqueued:  time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
		Started:   time.Time{},
		Completed: time.Time{},
		Status:    "error",
	}, {
		Actions: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       "4",
				Receiver: "unit-mysql-1",
				Name:     "restore",
			},
		}},
		Summary:   "operation 3",
		ID:        "3",
		Enqueued:  time.Time{},
		Started:   time.Time{},
		Completed: time.Date(2014, time.February, 14, 6, 6, 6, 0, time.UTC),
		Status:    "running",
	}, {
		Actions: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       "6",
				Receiver: "machine-1",
				Name:     "juju-exec",
			},
		}},
		Summary:   "operation 5",
		ID:        "5",
		Enqueued:  time.Date(2013, time.February, 14, 6, 6, 6, 0, time.UTC),
		Started:   time.Time{},
		Completed: time.Time{},
		Status:    "pending",
	}, {
		Actions: []actionapi.ActionResult{{
			Error: errors.New("boom"),
		}},
		Summary: "operation 10",
		ID:      "10",
	}},
}

func (s *ListOperationsSuite) TestRunNoResults(c *tc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListOperationsCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin")
		c.Assert(err, tc.ErrorIsNil)
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, "")
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), tc.Equals, "no matching operations\n")
	}
}

func (s *ListOperationsSuite) TestRunPlain(c *tc.C) {
	fakeClient := &fakeAPIClient{
		operationResults: listOperationResults,
	}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListOperationsCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin", "--utc")
		c.Assert(err, tc.ErrorIsNil)
		expected := `
ID  Status   Started  Finished             Task IDs  Summary
 1  error                                  2         operation 1
 3  running           2014-02-14T06:06:06  4         operation 3
 5  pending                                6         operation 5
10  error                                            operation 10
`[1:]
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, expected)
	}
}

func (s *ListOperationsSuite) TestRunPlainTruncated(c *tc.C) {
	listOperationResults.Truncated = true
	fakeClient := &fakeAPIClient{
		operationResults: listOperationResults,
	}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListOperationsCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin", "--utc", "--offset=12", "--limit=4")
		c.Assert(err, tc.ErrorIsNil)
		expected := `
Displaying operation results 13 to 16.
Run the command again with --offset=16 --limit=4 to see the next batch.

ID  Status   Started  Finished             Task IDs  Summary
 1  error                                  2         operation 1
 3  running           2014-02-14T06:06:06  4         operation 3
 5  pending                                6         operation 5
10  error                                            operation 10
`[1:]
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, expected)
	}
}

var listOperationManyTasksResults = actionapi.Operations{
	Operations: []actionapi.Operation{{
		Actions: []actionapi.ActionResult{{
			Action: &actionapi.Action{
				ID:       "2",
				Receiver: "unit-mysql-0",
				Name:     "backup",
			}}, {
			Action: &actionapi.Action{
				ID:       "3",
				Receiver: "unit-mysql-1",
				Name:     "backup",
			}}, {
			Action: &actionapi.Action{
				ID:       "4",
				Receiver: "unit-mysql-2",
				Name:     "backup",
			}}, {
			Action: &actionapi.Action{
				ID:       "5",
				Receiver: "unit-mysql-3",
				Name:     "backup",
			}}, {
			Action: &actionapi.Action{
				ID:       "6",
				Receiver: "unit-mysql-4",
				Name:     "backup",
			}}, {
			Action: &actionapi.Action{
				ID:       "7",
				Receiver: "unit-mysql-5",
				Name:     "backup",
			},
		}},
		Summary:   "operation 1",
		Fail:      "fail",
		ID:        "1",
		Enqueued:  time.Time{},
		Started:   time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
		Completed: time.Time{},
		Status:    "completed",
	}},
}

func (s *ListOperationsSuite) TestRunPlainManyTasks(c *tc.C) {
	fakeClient := &fakeAPIClient{
		operationResults: listOperationManyTasksResults,
	}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListOperationsCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin", "--utc")
		c.Assert(err, tc.ErrorIsNil)
		expected := `
ID  Status     Started              Finished  Task IDs      Summary
 1  completed  2015-02-14T06:06:06            2,3,4,5,6...  operation 1
`[1:]
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, expected)
	}
}

func (s *ListOperationsSuite) TestRunYaml(c *tc.C) {
	fakeClient := &fakeAPIClient{
		operationResults: listOperationResults,
	}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListOperationsCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin", "--format", "yaml", "--utc")
		c.Assert(err, tc.ErrorIsNil)
		expected := `
"1":
  summary: operation 1
  status: error
  fail: fail
  action:
    name: backup
    parameters: {}
  timing:
    enqueued: 2015-02-14 06:06:06 +0000 UTC
  tasks:
    "2":
      host: mysql/0
      status: ""
"3":
  summary: operation 3
  status: running
  action:
    name: restore
    parameters: {}
  timing:
    completed: 2014-02-14 06:06:06 +0000 UTC
  tasks:
    "4":
      host: mysql/1
      status: ""
"5":
  summary: operation 5
  status: pending
  action:
    name: juju-exec
    parameters: {}
  timing:
    enqueued: 2013-02-14 06:06:06 +0000 UTC
  tasks:
    "6":
      host: "1"
      status: ""
"10":
  summary: operation 10
  status: error
`[1:]
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, expected)
	}
}
