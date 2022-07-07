// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"errors"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/juju/action"
)

type ShowSuite struct {
	BaseActionSuite
	wrappedCommand cmd.Command
	command        *action.ShowCommand
}

var _ = gc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.wrappedCommand, s.command = action.NewShowCommandForTest(s.store)
}

func (s *ShowSuite) TestInit(c *gc.C) {
	tests := []struct {
		should         string
		args           []string
		expectedApp    string
		expectedAction string
		expectedErr    string
	}{{
		should:      "fail with missing application name",
		args:        []string{},
		expectedErr: "no application specified",
	}, {
		should:      "fail with missing action name",
		args:        []string{validApplicationId},
		expectedErr: "no action specified",
	}, {
		should:      "fail with invalid application name",
		args:        []string{invalidApplicationId, "doIt"},
		expectedErr: "invalid application name \"" + invalidApplicationId + "\"",
	}, {
		should:      "fail with too many args",
		args:        []string{"one", "two", "things"},
		expectedErr: "unrecognized args: \\[\"things\"\\]",
	}, {
		should:         "init properly with valid application name",
		args:           []string{validApplicationId, "doIt"},
		expectedApp:    validApplicationId,
		expectedAction: "doIt",
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			c.Logf("test %d should %s: juju show-action defined %s", i,
				t.should, strings.Join(t.args, " "))
			s.wrappedCommand, s.command = action.NewShowCommandForTest(s.store)
			args := append([]string{modelFlag, "admin"}, t.args...)
			err := cmdtesting.InitCommand(s.wrappedCommand, args)
			if t.expectedErr == "" {
				c.Check(err, jc.ErrorIsNil)
				c.Check(s.command.ApplicationName(), gc.Equals, t.expectedApp)
				c.Check(s.command.ActionName(), gc.Equals, t.expectedAction)
			} else {
				c.Check(err, gc.ErrorMatches, t.expectedErr)
			}
		}
	}
}

func (s *ShowSuite) TestShow(c *gc.C) {
	simpleOutput := `
Take a snapshot of the database.

Arguments
full:
  type: boolean
  description: take a full backup
  default: true
name:
  type: string
  description: snapshot name
prefix:
  type: string
  description: prefix to snapshot name
  default: ""

`[1:]

	tests := []struct {
		should           string
		expectNoResults  bool
		expectMessage    string
		withArgs         []string
		withAPIErr       string
		withCharmActions map[string]actionapi.ActionSpec
		expectedErr      string
	}{{
		should:      "pass back API error correctly",
		withArgs:    []string{validApplicationId, "doIt"},
		withAPIErr:  "an API error",
		expectedErr: "an API error",
	}, {
		should:          "work properly when no results found",
		withArgs:        []string{validApplicationId, "snapshot"},
		expectNoResults: true,
		expectedErr:     "cmd: error out silently",
		expectMessage:   `unknown action "snapshot"`,
	}, {
		should:           "error when unknown action specified",
		withArgs:         []string{validApplicationId, "something"},
		withCharmActions: someCharmActions,
		expectMessage:    `unknown action "something"`,
		expectedErr:      "cmd: error out silently",
	}, {
		should:           "get results properly",
		withArgs:         []string{validApplicationId, "snapshot"},
		withCharmActions: someCharmActions,
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
				s.wrappedCommand, s.command = action.NewShowCommandForTest(s.store)
				ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, args...)

				if t.expectedErr != "" || t.withAPIErr != "" {
					c.Check(err, gc.ErrorMatches, t.expectedErr)
					if t.expectMessage != "" {
						msg := cmdtesting.Stderr(ctx)
						msg = strings.Replace(msg, "\n", "", -1)
						c.Check(msg, gc.Matches, t.expectMessage)
					}
				} else {
					c.Assert(err, gc.IsNil)
					c.Check(cmdtesting.Stdout(ctx), gc.Equals, simpleOutput)
				}

			}()
		}
	}
}
