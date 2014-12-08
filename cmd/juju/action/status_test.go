// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"

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
	fakestatus := "bloobered"

	args := []string{prefix}
	result := []params.ActionResult{{Status: fakestatus}}

	errNotSpecified := "no action UUID specified"
	errNotFound := `actions for id "` + prefix + `" not found`
	errNotRecognized := `id "` + prefix + `" got unrecognized entity tags .*`
	errMultipleMatches := `id "` + prefix + `" matched multiple actions .*`
	errNoResults := `id "` + prefix + `" matched action "` + fakeid + `", but found no results`

	tests := []statusTestCase{
		{expectError: errNotSpecified},
		{args: args, expectError: errNotFound},
		{args: args, expectError: errNotFound, tags: tagsForId(prefix)},
		{args: args, expectError: errNotRecognized, tags: tagsForId(prefix, "bb", "bc")},
		{args: args, expectError: errMultipleMatches, tags: tagsForId(prefix, faketag, faketag2)},
		{args: args, expectError: errNoResults, tags: tagsForId(prefix, faketag)},
		{args: args, tags: tagsForId(prefix, faketag), results: result},
	}

	for _, test := range tests {
		s.runTestCase(c, test)
	}
}

func (s *StatusSuite) runTestCase(c *gc.C, tc statusTestCase) {
	fakeClient := &fakeAPIClient{actionTagMatches: tc.tags, actionResults: tc.results}
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
		expected := "id: .*\nstatus: " + tc.results[0].Status + "\n"
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Matches, expected)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	}
}

type statusTestCase struct {
	args        []string
	expectError string
	tags        map[string][]params.Entity
	results     []params.ActionResult
}

func tagsForId(prefix string, tags ...string) map[string][]params.Entity {
	var entities []params.Entity
	for _, t := range tags {
		entities = append(entities, params.Entity{Tag: t})
	}
	return map[string][]params.Entity{prefix: entities}
}
