// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"errors"
	"strings"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/testing"
	"github.com/juju/names"
	gc "gopkg.in/check.v1"
)

type FetchSuite struct {
	BaseActionSuite
	subcommand *action.FetchCommand
}

var _ = gc.Suite(&FetchSuite{})

func (s *FetchSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
}

func (s *FetchSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, s.subcommand)
}

func (s *FetchSuite) TestInit(c *gc.C) {
	tests := []struct {
		should      string
		args        []string
		expectTag   names.ActionTag
		expectError string
	}{{
		should:      "fail with missing arg",
		args:        []string{},
		expectError: "no action UUID specified",
	}, {
		should:      "fail properly with a bad ID",
		args:        []string{invalidActionId},
		expectError: "invalid action ID \"" + invalidActionId + "\"",
	}, {
		should:    "init properly with single arg",
		args:      []string{validActionId},
		expectTag: names.NewActionTag(validActionId),
	}, {
		should:      "fail with multiple args",
		args:        []string{"12345", "54321"},
		expectError: `unrecognized args: \["54321"\]`,
	}}

	for i, t := range tests {
		s.subcommand = &action.FetchCommand{}
		c.Logf("test %d: it should %s: juju actions fetch %s", i,
			t.should, strings.Join(t.args, " "))
		err := testing.InitCommand(s.subcommand, t.args)
		if t.expectError == "" {
			c.Check(s.subcommand.ActionTag(), gc.Equals, t.expectTag)
		} else {
			c.Check(err, gc.ErrorMatches, t.expectError)
		}
	}
}

func (s *FetchSuite) TestRun(c *gc.C) {
	tests := []struct {
		should         string
		withResults    []params.ActionResult
		withAPIError   string
		expectedErr    string
		expectedOutput string
	}{{
		should:       "pass api error through properly",
		withAPIError: "api call error",
		expectedErr:  "api call error",
	}, {
		should:         "fail gracefully with no results",
		withResults:    []params.ActionResult{},
		expectedOutput: "No results for action " + validActionId + "\n",
	}, {
		should:      "error correctly with multiple results",
		withResults: []params.ActionResult{{}, {}},
		expectedErr: "too many results for action " + validActionId,
	}, {
		should: "pass through an error from the API server",
		withResults: []params.ActionResult{{
			Error: common.ServerError(errors.New("an apiserver error")),
		}},
		expectedErr: "an apiserver error",
	}, {
		should: "pretty-print action output",
		withResults: []params.ActionResult{{
			Status:  "complete",
			Message: "oh dear",
			Output: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "baz",
				},
			},
		}},
		expectedOutput: "message: oh dear\n" +
			"results:\n" +
			"  foo:\n" +
			"    bar: baz\n" +
			"status: complete\n",
	}}

	for i, t := range tests {
		func() { // for the defer of restoring patch function
			s.subcommand = &action.FetchCommand{}
			c.Logf("test %d: it should %s", i, t.should)
			client := &fakeAPIClient{
				actionResults: t.withResults,
			}
			if t.withAPIError != "" {
				client.apiErr = errors.New(t.withAPIError)
			}
			restore := s.BaseActionSuite.patchAPIClient(client)
			defer restore()
			//args := fmt.Sprintf("%s %s", s.subcommand.Info().Name, "some-action-id")
			ctx, err := testing.RunCommand(c, s.subcommand, validActionId)
			if t.expectedErr != "" || t.withAPIError != "" {
				c.Check(err, gc.ErrorMatches, t.expectedErr)
			} else {
				c.Assert(err, gc.IsNil)
				c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Matches, t.expectedOutput)
			}
		}()
	}
}
