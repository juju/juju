// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	actionapi "github.com/juju/juju/api/action"
	"github.com/juju/juju/apiserver/params"
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

func (s *CancelSuite) TestRun(c *gc.C) {
	prefix := "deadbeef"
	fakeid := prefix + "-0000-4000-8000-feedfacebeef"
	fakeid2 := prefix + "-0001-4000-8000-feedfacebeef"
	faketag := "action-" + fakeid
	faketag2 := "action-" + fakeid2

	emptyArgs := []string{}
	emptyPrefixArgs := []string{}
	prefixArgs := []string{prefix}
	result1 := []actionapi.ActionResult{{Action: &actionapi.Action{}, Status: "some-random-status"}}
	result2 := []actionapi.ActionResult{{Action: &actionapi.Action{}, Status: "a status"}, {Action: &actionapi.Action{}, Status: "another status"}}

	errNotFound := "no actions specified"
	errNotFoundForPrefix := `no actions found matching prefix ` + prefix + `, no actions have been canceled`
	errFoundTagButNotCanceled := `identifier\(s\) \["` + prefix + `"\] matched action\(s\) \[.*\], but no actions were canceled`

	tests := []cancelTestCase{
		{expectError: errNotFound},
		{args: emptyArgs, expectError: errNotFound},
		{args: emptyArgs, tags: tagsForIdPrefix("", faketag, faketag2), expectError: errNotFound},
		{args: emptyPrefixArgs, expectError: errNotFound},
		{args: emptyPrefixArgs, tags: tagsForIdPrefix("", faketag, faketag2), expectError: errNotFound},
		{args: prefixArgs, expectError: errNotFoundForPrefix},
		{args: prefixArgs, expectError: errNotFoundForPrefix, tags: tagsForIdPrefix(prefix)},
		{args: prefixArgs, expectError: errNotFoundForPrefix, tags: tagsForIdPrefix(prefix, "bb", "bc")},
		{args: prefixArgs, expectError: errFoundTagButNotCanceled, tags: tagsForIdPrefix(prefix, faketag, faketag2)},
		{args: prefixArgs, expectError: errFoundTagButNotCanceled, tags: tagsForIdPrefix(prefix, faketag)},
		{args: prefixArgs, tags: tagsForIdPrefix(prefix, faketag), results: result1},
		{args: prefixArgs, tags: tagsForIdPrefix(prefix, faketag, faketag2), results: result2},
	}

	for i, test := range tests {
		c.Logf("iteration %d, test case %+v", i, test)
		s.runTestCase(c, test)
	}
}

func (s *CancelSuite) runTestCase(c *gc.C, tc cancelTestCase) {
	for _, modelFlag := range s.modelFlags {
		fakeClient := makeFakeClient(
			0*time.Second, // No API delay
			5*time.Second, // 5 second test timeout
			tc.tags,
			tc.results,
			tc.actionsByNames,
			"", // No API error
		)

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
	args           []string
	expectError    string
	tags           params.FindTagsResults
	results        []actionapi.ActionResult
	actionsByNames map[string][]actionapi.ActionResult
}
