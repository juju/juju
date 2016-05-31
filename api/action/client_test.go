// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/action"
	"github.com/juju/juju/apiserver/params"
)

type actionSuite struct {
	baseSuite
}

var _ = gc.Suite(&actionSuite{})

func (s *actionSuite) TestClient(c *gc.C) {
	facade := action.ExposeFacade(s.client)

	c.Check(facade.Name(), gc.Equals, "Action")
}

func (s *actionSuite) TestApplicationCharmActions(c *gc.C) {
	tests := []struct {
		description    string
		patchResults   []params.ApplicationCharmActionsResult
		patchErr       string
		expectedErr    string
		expectedResult *charm.Actions
	}{{
		description: "result from wrong service",
		patchResults: []params.ApplicationCharmActionsResult{
			{
				ApplicationTag: names.NewApplicationTag("bar").String(),
			},
		},
		expectedErr: `action results received for wrong application "application-bar"`,
	}, {
		description: "some other error",
		patchResults: []params.ApplicationCharmActionsResult{
			{
				ApplicationTag: names.NewApplicationTag("foo").String(),
				Error: &params.Error{
					Message: "something bad",
				},
			},
		},
		expectedErr: `something bad`,
	}, {
		description: "more than one result",
		patchResults: []params.ApplicationCharmActionsResult{
			{},
			{},
		},
		expectedErr: "2 results, expected 1",
	}, {
		description:  "no results",
		patchResults: []params.ApplicationCharmActionsResult{},
		expectedErr:  "0 results, expected 1",
	}, {
		description: "error on facade call",
		patchErr:    "something went wrong",
		expectedErr: "something went wrong",
	}, {
		description: "normal result",
		patchResults: []params.ApplicationCharmActionsResult{
			{
				ApplicationTag: names.NewApplicationTag("foo").String(),
				Actions: &charm.Actions{
					ActionSpecs: map[string]charm.ActionSpec{
						"action": {
							Description: "description",
							Params: map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		expectedResult: &charm.Actions{
			ActionSpecs: map[string]charm.ActionSpec{
				"action": {
					Description: "description",
					Params: map[string]interface{}{
						"foo": "bar",
					},
				},
			},
		},
	}}

	for i, t := range tests {
		// anonymous func to properly trigger defer
		func() {
			c.Logf("test %d: %s", i, t.description)
			cleanup := patchApplicationCharmActions(c, s.client, t.patchResults, t.patchErr)
			defer cleanup()
			result, err := s.client.ApplicationCharmActions(params.Entity{Tag: names.NewApplicationTag("foo").String()})
			if t.expectedErr != "" {
				c.Check(err, gc.ErrorMatches, t.expectedErr)
			} else {
				c.Check(err, jc.ErrorIsNil)
				c.Check(result, jc.DeepEquals, t.expectedResult)
			}
		}()
	}
}

// replace sCharmActions" facade call with required results and error
// if desired
func patchApplicationCharmActions(c *gc.C, apiCli *action.Client, patchResults []params.ApplicationCharmActionsResult, err string) func() {
	return action.PatchClientFacadeCall(apiCli,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Assert(req, gc.Equals, "ApplicationsCharmsActions")
			c.Assert(paramsIn, gc.FitsTypeOf, params.Entities{})
			p := paramsIn.(params.Entities)
			c.Check(p.Entities, gc.HasLen, 1)
			result := resp.(*params.ApplicationsCharmActionsResults)
			result.Results = patchResults
			if err != "" {
				return errors.New(err)
			}
			return nil
		},
	)
}
