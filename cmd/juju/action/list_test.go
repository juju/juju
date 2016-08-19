// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"errors"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type ListSuite struct {
	BaseActionSuite
	svc            *state.Application
	wrappedCommand cmd.Command
	command        *action.ListCommand
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.wrappedCommand, s.command = action.NewListCommandForTest(s.store)
}

func (s *ListSuite) TestInit(c *gc.C) {
	tests := []struct {
		should               string
		args                 []string
		expectedSvc          names.ApplicationTag
		expectedOutputSchema bool
		expectedErr          string
	}{{
		should:      "fail with missing application name",
		args:        []string{},
		expectedErr: "no application name specified",
	}, {
		should:      "fail with invalid application name",
		args:        []string{invalidServiceId},
		expectedErr: "invalid application name \"" + invalidServiceId + "\"",
	}, {
		should:      "fail with too many args",
		args:        []string{"two", "things"},
		expectedErr: "unrecognized args: \\[\"things\"\\]",
	}, {
		should:      "init properly with valid application name",
		args:        []string{validServiceId},
		expectedSvc: names.NewApplicationTag(validServiceId),
	}, {
		should:      "schema with tabular output",
		args:        []string{"--format=tabular", "--schema", validServiceId},
		expectedErr: "full schema not compatible with tabular output",
	}, {
		should:               "init properly with valid application name and --schema",
		args:                 []string{"--format=yaml", "--schema", validServiceId},
		expectedOutputSchema: true,
		expectedSvc:          names.NewApplicationTag(validServiceId),
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			c.Logf("test %d should %s: juju actions defined %s", i,
				t.should, strings.Join(t.args, " "))
			s.wrappedCommand, s.command = action.NewListCommandForTest(s.store)
			args := append([]string{modelFlag, "admin"}, t.args...)
			err := testing.InitCommand(s.wrappedCommand, args)
			if t.expectedErr == "" {
				c.Check(err, jc.ErrorIsNil)
				c.Check(s.command.ApplicationTag(), gc.Equals, t.expectedSvc)
				c.Check(s.command.FullSchema(), gc.Equals, t.expectedOutputSchema)
			} else {
				c.Check(err, gc.ErrorMatches, t.expectedErr)
			}
		}
	}
}

func (s *ListSuite) TestRun(c *gc.C) {
	simpleOutput := `
ACTION          DESCRIPTION
kill            Kill the database.
no-description  No description
no-params       An action with no parameters.
snapshot        Take a snapshot of the database.

`[1:]

	tests := []struct {
		should           string
		expectFullSchema bool
		expectNoResults  bool
		expectMessage    string
		withArgs         []string
		withAPIErr       string
		withCharmActions map[string]params.ActionSpec
		expectedErr      string
	}{{
		should:      "pass back API error correctly",
		withArgs:    []string{validServiceId},
		withAPIErr:  "an API error",
		expectedErr: "an API error",
	}, {
		should:           "get short results properly",
		withArgs:         []string{validServiceId},
		withCharmActions: someCharmActions,
	}, {
		should:           "get full schema results properly",
		withArgs:         []string{"--format=yaml", "--schema", validServiceId},
		expectFullSchema: true,
		withCharmActions: someCharmActions,
	}, {
		should:          "work properly when no results found",
		withArgs:        []string{validServiceId},
		expectNoResults: true,
		expectMessage:   "No actions defined for " + validServiceId,
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			func() {
				c.Logf("test %d should %s", i, t.should)

				fakeClient := &fakeAPIClient{charmActions: t.withCharmActions}
				if t.withAPIErr != "" {
					fakeClient.apiErr = errors.New(t.withAPIErr)
				}
				restore := s.patchAPIClient(fakeClient)
				defer restore()

				args := append([]string{modelFlag, "admin"}, t.withArgs...)
				s.wrappedCommand, s.command = action.NewListCommandForTest(s.store)
				ctx, err := testing.RunCommand(c, s.wrappedCommand, args...)

				if t.expectedErr != "" || t.withAPIErr != "" {
					c.Check(err, gc.ErrorMatches, t.expectedErr)
				} else {
					c.Assert(err, gc.IsNil)
					result := ctx.Stdout.(*bytes.Buffer).Bytes()
					if t.expectFullSchema {
						checkFullSchema(c, t.withCharmActions, result)
					} else if t.expectNoResults {
						c.Check(string(result), gc.Matches, t.expectMessage+"(?sm).*")
					} else {
						c.Check(testing.Stdout(ctx), gc.Equals, simpleOutput)
					}
				}

			}()
		}
	}
}

func checkFullSchema(c *gc.C, expected map[string]params.ActionSpec, actual []byte) {
	expectedOutput := make(map[string]interface{})
	for k, v := range expected {
		expectedOutput[k] = v.Params
	}
	c.Check(string(actual), jc.YAMLEquals, expectedOutput)
}
