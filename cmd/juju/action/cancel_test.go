// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/juju/action"
)

type CancelSuite struct {
	BaseActionSuite
	subcommand cmd.Command
}

var _ = gc.Suite(&CancelSuite{})

func (s *CancelSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.subcommand, _ = action.NewCancelCommandForTest(s.store)
}

func (s *CancelSuite) TestInit(c *gc.C) {
	for _, modelFlag := range s.modelFlags {
		cmd, _ := action.NewCancelCommandForTest(s.store)
		args := append([]string{modelFlag, "admin"}, "test")
		err := cmdtesting.InitCommand(cmd, args)
		c.Check(err, jc.ErrorIs, errors.NotValid)
	}
}

func (s *CancelSuite) TestRun(c *gc.C) {
	result1 := []actionapi.ActionResult{{Action: &actionapi.Action{ID: "1"}, Status: "some-random-status"}}
	result2 := []actionapi.ActionResult{{Action: &actionapi.Action{ID: "2"}, Status: "a status"}, {Action: &actionapi.Action{ID: "3"}, Status: "another status"}}

	tests := []cancelTestCase{
		{expectError: "no task IDs specified"},
		{args: []string{}, expectError: "no task IDs specified"},
		{args: []string{"3"}, expectError: "no tasks found, no tasks have been canceled"},
		{args: []string{"1"}, results: result1},
		{args: []string{"2", "3"}, results: result2},
	}

	for i, test := range tests {
		c.Logf("iteration %d, test case %+v", i, test)
		s.runTestCase(c, test)
	}
}

func (s *CancelSuite) runTestCase(c *gc.C, tc cancelTestCase) {
	for _, modelFlag := range s.modelFlags {
		fakeClient := &fakeAPIClient{
			timeout:       s.clock.NewTimer(5 * time.Second), // 5 second test wait
			actionResults: tc.results,
		}

		restore := s.patchAPIClient(fakeClient)
		defer restore()

		s.subcommand, _ = action.NewCancelCommandForTest(s.store)
		args := append([]string{modelFlag, "admin"}, tc.args...)
		ctx, err := cmdtesting.RunCommand(c, s.subcommand, args...)
		if tc.expectError == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, tc.expectError)
		}
		if len(tc.results) > 0 {
			out := &bytes.Buffer{}
			err := cmd.FormatYaml(out, action.ActionResultsToMap(tc.results))
			c.Check(err, jc.ErrorIsNil)
			c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, out.String())
			c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
		}
	}
}

type cancelTestCase struct {
	args        []string
	expectError string
	results     []actionapi.ActionResult
}
