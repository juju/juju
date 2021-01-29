// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"errors"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	actionapi "github.com/juju/juju/api/action"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
)

type StatusSuite struct {
	BaseActionSuite
	subcommand cmd.Command
}

var _ = gc.Suite(&StatusSuite{})

func (s *StatusSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.subcommand, _ = action.NewStatusCommandForTest(s.store)
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
	result1 := []actionapi.ActionResult{{Status: "some-random-status", Action: &actionapi.Action{ID: fakeid, Name: "fakeName"}}}
	result2 := []actionapi.ActionResult{{Status: "a status", Action: &actionapi.Action{ID: fakeid2, Name: "fakeName2"}}, {Status: "another status"}}
	errResult := []actionapi.ActionResult{{Status: "", Error: &params.Error{Message: "an error"}}}

	errNotFound := "no actions found"
	errNotFoundForPrefix := `no actions found matching prefix "` + prefix + `"`
	errFoundTagButNoResults := `identifier "` + prefix + `" matched action\(s\) \[.*\], but found no results`

	nameArgs := []string{"--name", "action_name"}
	resultMany := map[string][]actionapi.ActionResult{
		"1": {},
		"2": {},
	}

	resultOne := map[string][]actionapi.ActionResult{
		"action_name": result1,
	}

	resultOneError := map[string][]actionapi.ActionResult{
		"action_name": {{
			Error: errors.New("whoops"),
		}},
	}

	resultNoneFound := map[string][]actionapi.ActionResult{
		"action_name": {},
	}

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
		{args: nameArgs, actionsByNames: resultMany, expectError: "expected one result got 2"},
		{args: nameArgs, actionsByNames: resultOneError, expectError: "whoops"},
		{args: nameArgs, actionsByNames: resultNoneFound, expectError: "no actions were found for name action_name"},
		{args: nameArgs, actionsByNames: resultOne, results: result1},
		{args: prefixArgs, tags: tagsForIdPrefix(prefix, faketag), results: errResult},
	}

	for i, test := range tests {
		c.Logf("iteration %d, test case %+v", i, test)
		s.runTestCase(c, test)
	}
}

func (s *StatusSuite) runTestCase(c *gc.C, tc statusTestCase) {
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

		s.subcommand, _ = action.NewStatusCommandForTest(s.store)
		args := append([]string{modelFlag, "admin"}, tc.args...)
		ctx, err := cmdtesting.RunCommand(c, s.subcommand, args...)
		if tc.expectError == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, tc.expectError)
		}
		if len(tc.results) > 0 {
			out := &bytes.Buffer{}
			checkActionResultsMap(c, tc.results)
			err := cmd.FormatYaml(out, action.ActionResultsToMap(tc.results))
			c.Check(err, jc.ErrorIsNil)
			c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, out.String())
			c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
		}
	}
}

func checkActionResultsMap(c *gc.C, results []actionapi.ActionResult) {
	requiredOutputFields := []string{"status", "completed at"}
	actionFields := []string{"action", "id", "unit"}

	actionResults := action.ActionResultsToMap(results)

	for i, result := range results {
		a := actionResults["actions"].([]map[string]interface{})[i]

		for _, field := range requiredOutputFields {
			c.Logf("checking for presence of result output field: %s", field)
			c.Check(a[field], gc.NotNil)
		}

		if result.Action != nil {
			for _, field := range actionFields {
				c.Logf("checking for presence of action output field: %s", field)
				c.Check(a[field], gc.NotNil)
			}
		}

		if result.Error != nil {
			c.Check(a["error"], gc.NotNil)
		}
	}
}

type statusTestCase struct {
	args           []string
	expectError    string
	tags           params.FindTagsResults
	results        []actionapi.ActionResult
	actionsByNames map[string][]actionapi.ActionResult
}
