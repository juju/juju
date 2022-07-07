// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"errors"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/client/action"
	"github.com/juju/juju/rpc/params"
)

type actionSuite struct {
}

var _ = gc.Suite(&actionSuite{})

func (s *actionSuite) TestApplicationCharmActions(c *gc.C) {
	tests := []struct {
		description    string
		patchResults   []params.ApplicationCharmActionsResult
		patchErr       string
		expectedErr    string
		expectedResult map[string]action.ActionSpec
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
		expectedResult: map[string]action.ActionSpec{
			"action": {
				Description: "description",
				Params: map[string]interface{}{
					"foo": "bar",
				},
			},
		},
	}}

	for i, t := range tests {
		c.Logf("test %d: %s", i, t.description)
		apiCaller := basetesting.BestVersionCaller{
			APICallerFunc: basetesting.APICallerFunc(
				func(objType string,
					version int,
					id, request string,
					a, result interface{},
				) error {
					c.Assert(request, gc.Equals, "ApplicationsCharmsActions")
					c.Assert(a, gc.FitsTypeOf, params.Entities{})
					p := a.(params.Entities)
					c.Check(p.Entities, gc.HasLen, 1)
					c.Assert(result, gc.FitsTypeOf, &params.ApplicationsCharmActionsResults{})
					*(result.(*params.ApplicationsCharmActionsResults)) = params.ApplicationsCharmActionsResults{
						Results: t.patchResults,
					}
					if t.patchErr != "" {
						return errors.New(t.patchErr)
					}
					return nil
				},
			),
			BestVersion: 5,
		}
		client := action.NewClient(apiCaller)
		result, err := client.ApplicationCharmActions("foo")
		if t.expectedErr != "" {
			c.Check(err, gc.ErrorMatches, t.expectedErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(result, jc.DeepEquals, t.expectedResult)
		}
	}
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

func (s *actionSuite) TestListOperations(c *gc.C) {
	offset := 100
	limit := 200
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Assert(request, gc.Equals, "ListOperations")
				c.Assert(a, jc.DeepEquals, params.OperationQueryArgs{
					Applications: []string{"app"},
					Units:        []string{"unit/0"},
					Machines:     []string{"0"},
					ActionNames:  []string{"backup"},
					Status:       []string{"running"},
					Offset:       &offset,
					Limit:        &limit,
				})
				c.Assert(result, gc.FitsTypeOf, &params.OperationResults{})
				*(result.(*params.OperationResults)) = params.OperationResults{
					Results: []params.OperationResult{{
						OperationTag: "operation-1",
						Summary:      "hello",
						Fail:         "fail",
						Status:       "error",
						Actions: []params.ActionResult{{
							Action: &params.Action{Tag: "action-666", Name: "test", Receiver: "unit-mysql-0"},
						}},
					}},
					Truncated: true,
				}
				return nil
			},
		),
		BestVersion: 6,
	}
	client := action.NewClient(apiCaller)
	result, err := client.ListOperations(action.OperationQueryArgs{
		Applications: []string{"app"},
		Units:        []string{"unit/0"},
		Machines:     []string{"0"},
		ActionNames:  []string{"backup"},
		Status:       []string{"running"},
		Offset:       &offset,
		Limit:        &limit,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, action.Operations{
		Operations: []action.Operation{{
			ID:      "1",
			Summary: "hello",
			Status:  "error",
			Fail:    "fail",
			Actions: []action.ActionResult{{
				Action: &action.Action{ID: "666", Name: "test", Receiver: "unit-mysql-0"},
			}},
		}},
		Truncated: true,
	})
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
						OperationTag: "operation-1",
						Summary:      "hello",
						Fail:         "fail",
						Actions: []params.ActionResult{{
							Action: &params.Action{Tag: "action-666", Name: "test", Receiver: "unit-mysql-0"},
						}},
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
	c.Assert(result, jc.DeepEquals, action.Operation{
		ID:      "1",
		Summary: "hello",
		Fail:    "fail",
		Actions: []action.ActionResult{{
			Action: &action.Action{ID: "666", Name: "test", Receiver: "unit-mysql-0"},
		}},
	})
}

func (s *actionSuite) TestEnqueueOperation(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Assert(request, gc.Equals, "EnqueueOperation")
				c.Assert(a, jc.DeepEquals, params.Actions{
					Actions: []params.Action{{
						Receiver: "unit/0",
						Name:     "test",
						Parameters: map[string]interface{}{
							"foo": "bar",
						},
					}},
				})
				c.Assert(result, gc.FitsTypeOf, &params.EnqueuedActions{})
				*(result.(*params.EnqueuedActions)) = params.EnqueuedActions{
					OperationTag: "operation-1",
					Actions: []params.ActionResult{{
						Error: &params.Error{Message: "FAIL"},
					}},
				}
				return nil
			},
		),
		BestVersion: 6,
	}
	args := []action.Action{{
		Receiver: "unit/0",
		Name:     "test",
		Parameters: map[string]interface{}{
			"foo": "bar",
		}},
	}
	client := action.NewClient(apiCaller)
	result, err := client.EnqueueOperation(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, action.EnqueuedActions{
		Actions: []action.ActionResult{{
			Error: &params.Error{Message: "FAIL"},
		}},
		OperationID: "1",
	})
}
