// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"errors"
	"strings"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/testing"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v1"
)

type DoSuite struct {
	BaseActionSuite
	subcommand *action.DoCommand
}

var _ = gc.Suite(&DoSuite{})

func (s *DoSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
}

func (s *DoSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, s.subcommand)
}

func (s *DoSuite) TestInit(c *gc.C) {
	tests := []struct {
		should               string
		args                 []string
		expectUnit           names.UnitTag
		expectAction         string
		expectAsync          bool
		expectParamsYamlPath string
		expectOutput         string
		expectError          string
	}{{
		should:      "fail with missing args",
		args:        []string{},
		expectError: "no unit specified",
	}, {
		should:      "fail with no action specified",
		args:        []string{validUnitId},
		expectError: "no action specified",
	}, {
		should:      "fail with invalid unit tag",
		args:        []string{invalidUnitId, "valid-action-name"},
		expectError: "invalid unit name \"something-strange-\"",
	}, {
		should:      "fail with invalid action name",
		args:        []string{validUnitId, "BadName"},
		expectError: "invalid action name \"BadName\"",
	}, {
		should:      "fail with too many args",
		args:        []string{"1", "2", "3"},
		expectError: "unrecognized args: \\[\"2\" \"3\"\\]",
	}, {
		should:       "init properly with no params",
		args:         []string{validUnitId, "valid-action-name"},
		expectUnit:   names.NewUnitTag(validUnitId),
		expectAction: "valid-action-name",
	}, {
		should:       "handle --async properly",
		args:         []string{"--async", validUnitId, "valid-action-name"},
		expectAsync:  true,
		expectUnit:   names.NewUnitTag(validUnitId),
		expectAction: "valid-action-name",
	}, {
		should:       "handle --params properly",
		args:         []string{"--async", validUnitId, "valid-action-name"},
		expectAsync:  true,
		expectUnit:   names.NewUnitTag(validUnitId),
		expectAction: "valid-action-name",
	}, {
		should: "handle both --params and --async properly",
		args: []string{"--async", "--params=somefile.yaml",
			validUnitId, "valid-action-name"},
		expectAsync:          true,
		expectParamsYamlPath: "somefile.yaml",
		expectUnit:           names.NewUnitTag(validUnitId),
		expectAction:         "valid-action-name",
	}}

	for i, t := range tests {
		s.subcommand = &action.DoCommand{}
		c.Logf("test %d: it should %s: juju actions do %s", i,
			t.should, strings.Join(t.args, " "))
		err := testing.InitCommand(s.subcommand, t.args)
		if t.expectError == "" {
			c.Check(s.subcommand.UnitTag(), gc.Equals, t.expectUnit)
			c.Check(s.subcommand.ActionName(), gc.Equals, t.expectAction)
			c.Check(s.subcommand.IsAsync(), gc.Equals, t.expectAsync)
			c.Check(s.subcommand.ParamsYAMLPath(), gc.Equals, t.expectParamsYamlPath)
		} else {
			c.Check(err, gc.ErrorMatches, t.expectError)
		}
	}
}

func (s *DoSuite) TestRun(c *gc.C) {
	tests := []struct {
		should                 string
		withArgs               []string
		withParamsFileContents string
		withParamsFileError    string
		withAPIErr             string
		withActionResults      []params.ActionResult
		expectedErr            string
	}{{
		should:   "enqueue a basic action with no params",
		withArgs: []string{validUnitId, "some-action"},
		withActionResults: []params.ActionResult{{
			Action: &params.Action{
				Tag: validActionTagString,
			},
		}},
	}}

	for i, t := range tests {
		func() {
			c.Logf("test %d: it should %s: juju actions do %s", i,
				t.should, strings.Join(t.withArgs, " "))
			fakeClient := &fakeAPIClient{
				actionResults: t.withActionResults,
			}
			if t.withAPIErr != "" {
				fakeClient.apiErr = errors.New(t.withAPIErr)
			}
			restore := s.patchAPIClient(fakeClient)
			defer restore()

			s.subcommand = &action.DoCommand{}
			ctx, err := testing.RunCommand(c, s.subcommand, t.withArgs...)

			if t.expectedErr != "" || t.withAPIErr != "" {
				c.Check(err, gc.ErrorMatches, t.expectedErr)
			} else {
				c.Assert(err, gc.IsNil)
				// only one result?
				c.Assert(len(t.withActionResults), gc.Equals, 1)
				// result contains non-nil action?
				c.Assert(t.withActionResults[0].Action, gc.NotNil)
				// get the tag
				expectedTag, err := names.ParseActionTag(t.withActionResults[0].Action.Tag)
				c.Assert(err, gc.IsNil)
				sillyKey := "Action queued with id"
				expectedMap := map[string]string{sillyKey: expectedTag.Id()}
				result := ctx.Stdout.(*bytes.Buffer).Bytes()
				resultMap := make(map[string]string)
				err = yaml.Unmarshal(result, &resultMap)
				c.Assert(err, gc.IsNil)
				c.Check(resultMap, jc.DeepEquals, expectedMap)
			}
		}()
	}
}
