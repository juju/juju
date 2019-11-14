// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/action"
)

type ListTasksSuite struct {
	BaseActionSuite
	wrappedCommand cmd.Command
	command        *action.ListTasksCommand
}

var _ = gc.Suite(&ListTasksSuite{})

func (s *ListTasksSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.wrappedCommand, s.command = action.NewListTasksCommandForTest(s.store)
}

func (s *ListTasksSuite) TestInit(c *gc.C) {
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
		should:      "fail with invalid status value",
		args:        []string{"--status", "pending," + "error"},
		expectedErr: `"error" is not a valid function status, want one of \[pending running completed\]`,
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
			c.Logf("test %d should %s: juju tasks defined %s", i,
				t.should, strings.Join(t.args, " "))
			s.wrappedCommand, s.command = action.NewListTasksCommandForTest(s.store)
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

func (s *ListTasksSuite) TestRunQueryArgs(c *gc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListTasksCommandForTest(s.store)
	args := []string{
		"--apps", "mysql,mediawiki",
		"--units", "mysql/1,mediawiki/0",
		"--functions", "backup",
		"--status", "completed,pending",
	}
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListTasksCommandForTest(s.store)
		args = append([]string{modelFlag, "admin", "--utc"}, args...)

		_, err := cmdtesting.RunCommand(c, s.wrappedCommand, args...)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(fakeClient.taskQueryArgs, jc.DeepEquals, params.TaskQueryArgs{
			Applications:  []string{"mysql", "mediawiki"},
			Units:         []string{"mysql/1", "mediawiki/0"},
			FunctionNames: []string{"backup"},
			Status:        []string{"completed", "pending"},
		})
	}
}

var listTaskResults = []params.ActionResult{
	{
		Action: &params.Action{
			Tag:      "action-1",
			Receiver: "unit-mysql-0",
			Name:     "backup",
		},
		Enqueued:  time.Time{},
		Started:   time.Date(2015, time.February, 14, 6, 6, 6, 0, time.UTC),
		Completed: time.Time{},
		Status:    "completed",
	}, {
		Action: &params.Action{
			Tag:      "action-2",
			Receiver: "unit-mysql-1",
			Name:     "restore",
		},
		Enqueued:  time.Time{},
		Started:   time.Time{},
		Completed: time.Date(2014, time.February, 14, 6, 6, 6, 0, time.UTC),
		Status:    "running",
	}, {
		Action: &params.Action{
			Tag:      "action-3",
			Receiver: "unit-mysql-1",
			Name:     "vacuum",
		},
		Enqueued:  time.Date(2013, time.February, 14, 6, 6, 6, 0, time.UTC),
		Started:   time.Time{},
		Completed: time.Time{},
		Status:    "pending",
	}, {
		Error: &params.Error{Message: "boom"},
	},
}

func (s *ListTasksSuite) TestRunNoResults(c *gc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListTasksCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListTasksCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "")
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "no matching tasks\n")
	}
}

func (s *ListTasksSuite) TestRunPlain(c *gc.C) {
	fakeClient := &fakeAPIClient{
		actionResults: listTaskResults,
	}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListTasksCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListTasksCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin", "--utc")
		c.Assert(err, jc.ErrorIsNil)
		expected := `
Id  Task     Status     Unit     Time
 3  vacuum   pending    mysql/1  2013-02-14T06:06:06
 2  restore  running    mysql/1  2014-02-14T06:06:06
 1  backup   completed  mysql/0  2015-02-14T06:06:06

`[1:]
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, expected)
	}
}

func (s *ListTasksSuite) TestRunYaml(c *gc.C) {
	fakeClient := &fakeAPIClient{
		actionResults: listTaskResults,
	}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.wrappedCommand, _ = action.NewListTasksCommandForTest(s.store)
	for _, modelFlag := range s.modelFlags {
		s.wrappedCommand, s.command = action.NewListTasksCommandForTest(s.store)
		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, modelFlag, "admin", "--format", "yaml", "--utc")
		c.Assert(err, jc.ErrorIsNil)
		expected := `
"1":
  status: completed
  timing:
    started: 2015-02-14 06:06:06 +0000 UTC
  unit: mysql/0
"2":
  status: running
  timing:
    completed: 2014-02-14 06:06:06 +0000 UTC
  unit: mysql/1
"3":
  status: pending
  timing:
    enqueued: 2013-02-14 06:06:06 +0000 UTC
  unit: mysql/1
`[1:]
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, expected)
	}
}
