// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/testing"
)

type StatusSuite struct {
	BaseActionSuite
	subcommand *action.StatusCommand
}

var _ = gc.Suite(&StatusSuite{})

func (s *StatusSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.subcommand = &action.StatusCommand{}
}

func (s *StatusSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, s.subcommand)
}

func (s *StatusSuite) TestRun(c *gc.C) {
	prefix := "deadbeef"
	fakeid := prefix + "-0000-4000-8000-feedfacebeef"
	fakeid2 := prefix + "-0001-4000-8000-feedfacebeef"
	faketag := "action-" + fakeid
	faketag2 := "action-" + fakeid2

	emptyArgs := []string{}
	emptyPrefixArgs := []string{}
	prefixArgs := []string{prefix}
	result1 := []params.ActionResult{{Status: "some-random-status"}}
	result2 := []params.ActionResult{{Status: "a status"}, {Status: "another status"}}

	errNotFound := "no actions found"
	errNotFoundForPrefix := `no actions found matching prefix "` + prefix + `"`
	errFoundTagButNoResults := `identifier "` + prefix + `" matched action\(s\) \[.*\], but found no results`

	tests := []statusTestCase{
		{expectError: errNotFound},
		{args: emptyArgs, expectError: errNotFound},
		{args: emptyArgs, tags: tagsForIdPrefix("", faketag, faketag2), results: result2},
		{args: emptyPrefixArgs, expectError: errNotFound},
		{args: emptyPrefixArgs, tags: tagsForIdPrefix("", faketag, faketag2), results: result2},
		{args: prefixArgs, expectError: errNotFoundForPrefix},
		{args: prefixArgs, expectError: errNotFoundForPrefix, tags: tagsForIdPrefix(prefix)},
		{args: prefixArgs, expectError: errNotFoundForPrefix, tags: tagsForIdPrefix(prefix, "bb", "bc")},
		{args: prefixArgs, expectError: errFoundTagButNoResults, tags: tagsForIdPrefix(prefix, faketag, faketag2)},
		{args: prefixArgs, expectError: errFoundTagButNoResults, tags: tagsForIdPrefix(prefix, faketag)},
		{args: prefixArgs, tags: tagsForIdPrefix(prefix, faketag), results: result1},
		{args: prefixArgs, tags: tagsForIdPrefix(prefix, faketag, faketag2), results: result2},
	}

	for i, test := range tests {
		c.Logf("iteration %d, test case %+v", i, test)
		s.runTestCase(c, test)
	}
}

func (s *StatusSuite) runTestCase(c *gc.C, tc statusTestCase) {
	fakeClient := makeFakeClient(
		0*time.Second, // No API delay
		5*time.Second, // 5 second test timeout
		tc.tags,
		tc.results,
		"", // No API error
	)

	restore := s.patchAPIClient(fakeClient)
	defer restore()

	s.subcommand = &action.StatusCommand{}
	ctx, err := testing.RunCommand(c, s.subcommand, tc.args...)
	if tc.expectError == "" {
		c.Check(err, jc.ErrorIsNil)
	} else {
		c.Check(err, gc.ErrorMatches, tc.expectError)
	}
	if len(tc.results) > 0 {
		buf, err := cmd.DefaultFormatters["yaml"](action.ActionResultsToMap(tc.results))
		c.Check(err, jc.ErrorIsNil)
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, string(buf)+"\n")
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	}
}

type statusTestCase struct {
	args        []string
	expectError string
	tags        params.FindTagsResults
	results     []params.ActionResult
}
