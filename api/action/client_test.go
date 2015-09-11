// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"errors"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

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

func (s *actionSuite) TestServiceCharmActions(c *gc.C) {
	tests := []struct {
		description    string
		patchResults   []params.ServiceCharmActionsResult
		patchErr       string
		expectedErr    string
		expectedResult *charm.Actions
	}{{
		description: "result from wrong service",
		patchResults: []params.ServiceCharmActionsResult{
			{
				ServiceTag: names.NewServiceTag("bar").String(),
			},
		},
		expectedErr: `action results received for wrong service "service-bar"`,
	}, {
		description: "some other error",
		patchResults: []params.ServiceCharmActionsResult{
			{
				ServiceTag: names.NewServiceTag("foo").String(),
				Error: &params.Error{
					Message: "something bad",
				},
			},
		},
		expectedErr: `something bad`,
	}, {
		description: "more than one result",
		patchResults: []params.ServiceCharmActionsResult{
			{},
			{},
		},
		expectedErr: "2 results, expected 1",
	}, {
		description:  "no results",
		patchResults: []params.ServiceCharmActionsResult{},
		expectedErr:  "0 results, expected 1",
	}, {
		description: "error on facade call",
		patchErr:    "something went wrong",
		expectedErr: "something went wrong",
	}, {
		description: "normal result",
		patchResults: []params.ServiceCharmActionsResult{
			{
				ServiceTag: names.NewServiceTag("foo").String(),
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
			cleanup := patchServiceCharmActions(c, s.client, t.patchResults, t.patchErr)
			defer cleanup()
			result, err := s.client.ServiceCharmActions(params.Entity{Tag: names.NewServiceTag("foo").String()})
			if t.expectedErr != "" {
				c.Check(err, gc.ErrorMatches, t.expectedErr)
			} else {
				c.Check(err, jc.ErrorIsNil)
				c.Check(result, jc.DeepEquals, t.expectedResult)
			}
		}()
	}
}

// replace "ServicesCharmActions" facade call with required results and error
// if desired
func patchServiceCharmActions(c *gc.C, apiCli *action.Client, patchResults []params.ServiceCharmActionsResult, err string) func() {
	return action.PatchClientFacadeCall(apiCli,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Assert(req, gc.Equals, "ServicesCharmActions")
			c.Assert(paramsIn, gc.FitsTypeOf, params.Entities{})
			p := paramsIn.(params.Entities)
			c.Check(p.Entities, gc.HasLen, 1)
			result := resp.(*params.ServicesCharmActionsResults)
			result.Results = patchResults
			if err != "" {
				return errors.New(err)
			}
			return nil
		},
	)
}
