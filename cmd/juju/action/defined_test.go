// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"errors"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type DefinedSuite struct {
	BaseActionSuite
	svc            *state.Service
	wrappedCommand cmd.Command
	command        *action.DefinedCommand
}

var _ = gc.Suite(&DefinedSuite{})

func (s *DefinedSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.wrappedCommand, s.command = action.NewDefinedCommand(s.store)
}

func (s *DefinedSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, s.wrappedCommand)
}

func (s *DefinedSuite) TestInit(c *gc.C) {
	tests := []struct {
		should               string
		args                 []string
		expectedSvc          names.ServiceTag
		expectedOutputSchema bool
		expectedErr          string
	}{{
		should:      "fail with missing service name",
		args:        []string{},
		expectedErr: "no service name specified",
	}, {
		should:      "fail with invalid service name",
		args:        []string{invalidServiceId},
		expectedErr: "invalid service name \"" + invalidServiceId + "\"",
	}, {
		should:      "fail with too many args",
		args:        []string{"two", "things"},
		expectedErr: "unrecognized args: \\[\"things\"\\]",
	}, {
		should:      "init properly with valid service name",
		args:        []string{validServiceId},
		expectedSvc: names.NewServiceTag(validServiceId),
	}, {
		should:               "init properly with valid service name and --schema",
		args:                 []string{"--schema", validServiceId},
		expectedOutputSchema: true,
		expectedSvc:          names.NewServiceTag(validServiceId),
	}}

	for i, t := range tests {
		for _, modelFlag := range s.modelFlags {
			c.Logf("test %d should %s: juju actions defined %s", i,
				t.should, strings.Join(t.args, " "))
			s.wrappedCommand, s.command = action.NewDefinedCommand(s.store)
			args := append([]string{modelFlag, "dummymodel"}, t.args...)
			err := testing.InitCommand(s.wrappedCommand, args)
			if t.expectedErr == "" {
				c.Check(s.command.ServiceTag(), gc.Equals, t.expectedSvc)
				c.Check(s.command.FullSchema(), gc.Equals, t.expectedOutputSchema)
			} else {
				c.Check(err, gc.ErrorMatches, t.expectedErr)
			}
		}
	}
}

func (s *DefinedSuite) TestRun(c *gc.C) {
	tests := []struct {
		should           string
		expectFullSchema bool
		expectNoResults  bool
		expectMessage    string
		withArgs         []string
		withAPIErr       string
		withCharmActions *charm.Actions
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
		withArgs:         []string{"--schema", validServiceId},
		expectFullSchema: true,
		withCharmActions: someCharmActions,
	}, {
		should:           "work properly when no results found",
		withArgs:         []string{validServiceId},
		expectNoResults:  true,
		expectMessage:    "No actions defined for " + validServiceId,
		withCharmActions: &charm.Actions{ActionSpecs: map[string]charm.ActionSpec{}},
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

				args := append([]string{modelFlag, "dummymodel"}, t.withArgs...)
				s.wrappedCommand, s.command = action.NewDefinedCommand(s.store)
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
						checkSimpleSchema(c, t.withCharmActions, result)
					}
				}

			}()
		}
	}
}

func checkFullSchema(c *gc.C, expected *charm.Actions, actual []byte) {
	expectedOutput := make(map[string]interface{})
	for k, v := range expected.ActionSpecs {
		expectedOutput[k] = v.Params
	}
	c.Check(string(actual), jc.YAMLEquals, expectedOutput)
}

func checkSimpleSchema(c *gc.C, expected *charm.Actions, actualOutput []byte) {
	specs := expected.ActionSpecs
	expectedSpecs := make(map[string]string)
	for name, spec := range specs {
		expectedSpecs[name] = spec.Description
		if expectedSpecs[name] == "" {
			expectedSpecs[name] = "No description"
		}
	}
	actual := make(map[string]string)
	err := yaml.Unmarshal(actualOutput, &actual)
	c.Assert(err, gc.IsNil)
	c.Check(actual, jc.DeepEquals, expectedSpecs)
}
