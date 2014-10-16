// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions_test

import (
	"errors"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api/actions"
	"github.com/juju/juju/apiserver/params"
)

type actionsSuite struct {
	baseSuite
}

var _ = gc.Suite(&actionsSuite{})

func (s *actionsSuite) TestClient(c *gc.C) {
	facade := actions.ExposeFacade(s.client)

	c.Check(facade.Name(), gc.Equals, "Actions")
}

func (s *actionsSuite) TestServiceCharmActions(c *gc.C) {
	// Things to test:
	//  - errors on facade call
	tests := []struct {
		description    string
		patchResults   []params.ServiceCharmActionsResult
		patchErr       string
		expectedErr    string
		expectedResult params.ServiceCharmActionsResult
	}{{
		description: "more than one result",
		patchResults: []params.ServiceCharmActionsResult{
			params.ServiceCharmActionsResult{},
			params.ServiceCharmActionsResult{},
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
			params.ServiceCharmActionsResult{
				ServiceTag: names.NewServiceTag("foo"),
				Actions: &charm.Actions{
					ActionSpecs: map[string]charm.ActionSpec{
						"action": charm.ActionSpec{
							Description: "description",
							Params: map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		expectedResult: params.ServiceCharmActionsResult{
			ServiceTag: names.NewServiceTag("foo"),
			Actions: &charm.Actions{
				ActionSpecs: map[string]charm.ActionSpec{
					"action": charm.ActionSpec{
						Description: "description",
						Params: map[string]interface{}{
							"foo": "bar",
						},
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
			result, err := s.client.ServiceCharmActions(names.NewServiceTag("foo"))
			if t.expectedErr != "" {
				c.Check(err, gc.ErrorMatches, t.expectedErr)
			} else {
				c.Check(err, gc.IsNil)
			}
			c.Check(result, jc.DeepEquals, t.expectedResult)
		}()
	}
}

// replace "ServicesCharmActions" facade call with required results and error
// if desired
func patchServiceCharmActions(c *gc.C, apiCli *actions.Client, patchResults []params.ServiceCharmActionsResult, err string) func() {
	return actions.PatchClientFacadeCall(apiCli,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Assert(req, gc.Equals, "ServicesCharmActions")
			c.Assert(paramsIn, gc.FitsTypeOf, params.ServiceTags{})
			p := paramsIn.(params.ServiceTags)
			c.Check(p.ServiceTags, gc.HasLen, 1)
			result := resp.(*params.ServicesCharmActionsResults)
			result.Results = patchResults
			if err != "" {
				return errors.New(err)
			}
			return nil
		},
	)
}
