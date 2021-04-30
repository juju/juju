// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
)

type ListOperationsSuite struct {
	BaseActionSuite
	wrappedCommand cmd.Command
	command        *action.ListOperationsCommand
}

var _ = gc.Suite(&ListOperationsSuite{})

func (s *ListOperationsSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
}

func (s *ListOperationsSuite) TestInit(c *gc.C) {
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
				c.Check(err, jc.ErrorIsNil)
			} else {
				c.Check(err, gc.ErrorMatches, t.expectedErr)
			}
		}
	}
}

func (s *ListOperationsSuite) TestRunQueryArgs(c *gc.C) {
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
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(fakeClient.operationQueryArgs, jc.DeepEquals, params.OperationQueryArgs{
			Applications: []string{"mysql", "mediawiki"},
			Units:        []string{"mysql/1", "mediawiki/0"},
			Machines:     []string{"0", "1"},
			ActionNames:  []string{"backup"},
			Status:       []string{"completed", "pending"},
		})
	}
}

var listOperationResults = []params.OperationResult{
	{
		Actions: []params.ActionResult{{
			Action: &params.Action{
				Tag:      "action-2",
				Receiver: "unit-mysql-0",
				Name:     "backup",
			},
		}},
		Summary:      "operation 1",
		Fail:         "fail",
		OperationTag: "operation-1",
		Enqueued:     time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
		Started:      time.Time{},
		Completed:    time.Time{},
		Status:       "error",
	}, {
		Actions: []params.ActionResult{{
			Action: &params.Action{
				Tag:      "action-4",
				Receiver: "unit-mysql-1",
				Name:     "restore",
			},
		}},
		Summary:      "operation 3",
		OperationTag: "operation-3",
		Enqueued:     time.Time{},
		Started:      time.Time{},
		Completed:    time.Date(2014, time.February, 14, 6, 6, 6, 0, time.UTC),
		Status:       "running",
	}, {
		Actions: []params.ActionResult{{
			Action: &params.Action{
				Tag:      "action-6",
				Receiver: "machine-1",
				Name:     "juju-run",
			},
		}},
		Summary:      "operation 5",
		OperationTag: "operation-5",
		Enqueued:     time.Date(2013, time.February, 14, 6, 6, 6, 0, time.UTC),
		Started:      time.Time{},
		Completed:    time.Time{},
		Status:       "pending",
	}, {
		Actions: []params.ActionResult{{
			Error: &params.Error{Message: "boom"},
		}},
		Summary:      "operation 7",
		OperationTag: "operation-7",
	},
}

func (s *ListOperationsSuite) TestRunNoResults(c *gc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListOperationsCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "")
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "no matching operations\n")
	}
}

func (s *ListOperationsSuite) TestRunPlain(c *gc.C) {
	fakeClient := &fakeAPIClient{
		operationResults: listOperationResults,
	}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListOperationsCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin", "--utc")
		c.Assert(err, jc.ErrorIsNil)
		expected := `
Id  Status   Started  Finished             Task IDs  Summary
 1  error                                  2         operation 1
 3  running           2014-02-14T06:06:06  4         operation 3
 5  pending                                6         operation 5
 7  error                                            operation 7

`[1:]
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, expected)
	}
}

var listOperationManyTasksResults = []params.OperationResult{
	{
		Actions: []params.ActionResult{{
			Action: &params.Action{
				Tag:      "action-2",
				Receiver: "unit-mysql-0",
				Name:     "backup",
			}}, {
			Action: &params.Action{
				Tag:      "action-3",
				Receiver: "unit-mysql-1",
				Name:     "backup",
			}}, {
			Action: &params.Action{
				Tag:      "action-4",
				Receiver: "unit-mysql-2",
				Name:     "backup",
			}}, {
			Action: &params.Action{
				Tag:      "action-5",
				Receiver: "unit-mysql-3",
				Name:     "backup",
			}}, {
			Action: &params.Action{
				Tag:      "action-6",
				Receiver: "unit-mysql-4",
				Name:     "backup",
			}}, {
			Action: &params.Action{
				Tag:      "action-7",
				Receiver: "unit-mysql-5",
				Name:     "backup",
			},
		}},
		Summary:      "operation 1",
		OperationTag: "operation-1",
		Fail:         "fail",
		Enqueued:     time.Time{},
		Started:      time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
		Completed:    time.Time{},
		Status:       "completed",
	},
}

func (s *ListOperationsSuite) TestRunPlainManyTasks(c *gc.C) {
	fakeClient := &fakeAPIClient{
		operationResults: listOperationManyTasksResults,
	}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListOperationsCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin", "--utc")
		c.Assert(err, jc.ErrorIsNil)
		expected := `
Id  Status     Started              Finished  Task IDs      Summary
 1  completed  2015-02-14T06:06:06            2,3,4,5,6...  operation 1

`[1:]
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, expected)
	}
}

func (s *ListOperationsSuite) TestRunYaml(c *gc.C) {
	fakeClient := &fakeAPIClient{
		operationResults: listOperationResults,
	}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListOperationsCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListOperationsCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin", "--format", "yaml", "--utc")
		c.Assert(err, jc.ErrorIsNil)
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
    name: juju-run
    parameters: {}
  timing:
    enqueued: 2013-02-14 06:06:06 +0000 UTC
  tasks:
    "6":
      host: "1"
      status: ""
"7":
  summary: operation 7
  status: error
`[1:]
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, expected)
	}
}
