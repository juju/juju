// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"errors"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/action"
	basetesting "github.com/juju/juju/api/base/testing"
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
		expectedResult map[string]params.ActionSpec
	}{{
		description: "result from wrong application",
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
				Actions: map[string]params.ActionSpec{
					"action": {
						Description: "description",
						Params: map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
		},
		expectedResult: map[string]params.ActionSpec{
			"action": {
				Description: "description",
				Params: map[string]interface{}{
					"foo": "bar",
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

func (s *actionSuite) TestWatchActionProgress(c *gc.C) {
	var called bool
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				called = true
				c.Assert(request, gc.Equals, "WatchActionsProgress")
				c.Assert(a, jc.DeepEquals, params.Entities{
					Entities: []params.Entity{{
						Tag: "action-666",
					}},
				})
				c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
				*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
					Results: []params.StringsWatchResult{{
						Error: &params.Error{Message: "FAIL"},
					}},
				}
				return nil
			},
		),
		BestVersion: 5,
	}
	client := action.NewClient(apiCaller)
	w, err := client.WatchActionProgress("666")
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
	c.Assert(called, jc.IsTrue)
}

func (s *actionSuite) TestWatchActionProgressArity(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Assert(request, gc.Equals, "WatchActionsProgress")
				c.Assert(a, jc.DeepEquals, params.Entities{
					Entities: []params.Entity{{
						Tag: "action-666",
					}},
				})
				c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
				*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
					Results: []params.StringsWatchResult{{
						Error: &params.Error{Message: "FAIL"},
					}, {
						Error: &params.Error{Message: "ANOTHER"},
					}},
				}
				return nil
			},
		),
		BestVersion: 5,
	}
	client := action.NewClient(apiCaller)
	_, err := client.WatchActionProgress("666")
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *actionSuite) TestWatchActionProgressNotSupported(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				return nil
			},
		),
		BestVersion: 4,
	}
	client := action.NewClient(apiCaller)
	_, err := client.WatchActionProgress("666")
	c.Assert(err, gc.ErrorMatches, "WatchActionProgress not supported by this version \\(4\\) of Juju")
}

func (s *actionSuite) TestListOperations(c *gc.C) {
	var args params.OperationQueryArgs
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Assert(request, gc.Equals, "ListOperations")
				c.Assert(a, jc.DeepEquals, args)
				c.Assert(result, gc.FitsTypeOf, &params.OperationResults{})
				*(result.(*params.OperationResults)) = params.OperationResults{
					Results: []params.OperationResult{{
						Summary: "hello",
					}},
				}
				return nil
			},
		),
		BestVersion: 6,
	}
	client := action.NewClient(apiCaller)
	result, err := client.ListOperations(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.OperationResults{
		Results: []params.OperationResult{{
			Summary: "hello",
		}},
	})
}

func (s *actionSuite) TestListOperationsNotSupported(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				return nil
			},
		),
		BestVersion: 4,
	}
	client := action.NewClient(apiCaller)
	_, err := client.ListOperations(params.OperationQueryArgs{})
	c.Assert(err, gc.ErrorMatches, "ListOperations not supported by this version \\(4\\) of Juju")
}

func (s *actionSuite) TestOperation(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Assert(request, gc.Equals, "Operations")
				c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "operation-666"}}})
				c.Assert(result, gc.FitsTypeOf, &params.OperationResults{})
				*(result.(*params.OperationResults)) = params.OperationResults{
					Results: []params.OperationResult{{
						Summary: "hello",
					}},
				}
				return nil
			},
		),
		BestVersion: 6,
	}
	client := action.NewClient(apiCaller)
	result, err := client.Operation("666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.OperationResult{
		Summary: "hello",
	})
}

func (s *actionSuite) TestOperationNotSupported(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				return nil
			},
		),
		BestVersion: 4,
	}
	client := action.NewClient(apiCaller)
	_, err := client.Operation("666")
	c.Assert(err, gc.ErrorMatches, "Operations not supported by this version \\(4\\) of Juju")
}

func (s *actionSuite) TestEnqueueOperation(c *gc.C) {
	args := params.Actions{
		Actions: []params.Action{{
			Receiver: "unit/0",
			Name:     "test",
			Parameters: map[string]interface{}{
				"foo": "bar",
			},
		}},
	}
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Assert(request, gc.Equals, "EnqueueOperation")
				c.Assert(a, jc.DeepEquals, args)
				c.Assert(result, gc.FitsTypeOf, &params.EnqueuedActions{})
				*(result.(*params.EnqueuedActions)) = params.EnqueuedActions{
					OperationTag: "operation-1",
					Actions: []params.StringResult{{
						Error: &params.Error{Message: "FAIL"},
					}},
				}
				return nil
			},
		),
		BestVersion: 6,
	}
	client := action.NewClient(apiCaller)
	result, err := client.EnqueueOperation(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.EnqueuedActions{
		Actions: []params.StringResult{{
			Error: &params.Error{Message: "FAIL"},
		}},
		OperationTag: "operation-1",
	})
}

func (s *actionSuite) TestEnqueueOperationNotSupported(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				return nil
			},
		),
		BestVersion: 5,
	}
	client := action.NewClient(apiCaller)
	_, err := client.EnqueueOperation(params.Actions{})
	c.Assert(err, gc.ErrorMatches, "EnqueueOperation not supported by this version \\(5\\) of Juju")
}
