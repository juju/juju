// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var _ = gc.Suite(testsuite{})

type testsuite struct{}

func (testsuite) TestAssignUnits(c *gc.C) {
	f := &fakeAssignCaller{c: c, response: params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{},
		}}}
	api := New(f)
	ids := []names.UnitTag{names.NewUnitTag("mysql/0"), names.NewUnitTag("mysql/1")}
	errs, err := api.AssignUnits(ids)
	c.Assert(f.request, gc.Equals, "AssignUnits")
	c.Assert(f.params, gc.DeepEquals,
		params.Entities{[]params.Entity{
			{Tag: "unit-mysql-0"},
			{Tag: "unit-mysql-1"},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []error{nil, nil})
}

func (testsuite) TestAssignUnitsNotFound(c *gc.C) {
	f := &fakeAssignCaller{c: c, response: params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Code: params.CodeNotFound}},
		}}}
	api := New(f)
	ids := []names.UnitTag{names.NewUnitTag("mysql/0")}
	errs, err := api.AssignUnits(ids)
	f.Lock()
	c.Assert(f.request, gc.Equals, "AssignUnits")
	c.Assert(f.params, gc.DeepEquals,
		params.Entities{[]params.Entity{
			{Tag: "unit-mysql-0"},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 1)
	c.Assert(errs[0], jc.Satisfies, errors.IsNotFound)
}

func (testsuite) TestWatchUnitAssignment(c *gc.C) {
	f := &fakeWatchCaller{
		c:        c,
		response: params.StringsWatchResult{},
	}
	api := New(f)
	w, err := api.WatchUnitAssignments()
	f.Lock()
	c.Assert(f.request, gc.Equals, "WatchUnitAssignments")
	c.Assert(f.params, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

type fakeAssignCaller struct {
	base.APICaller
	sync.Mutex
	request  string
	params   interface{}
	response params.ErrorResults
	err      error
	c        *gc.C
}

func (f *fakeAssignCaller) APICall(objType string, version int, id, request string, param, response interface{}) error {
	f.Lock()
	defer f.Unlock()
	f.request = request
	f.params = param
	res, ok := response.(*params.ErrorResults)
	if !ok {
		f.c.Errorf("Expected *params.ErrorResults as response, but was %#v", response)
	} else {
		*res = f.response
	}
	return f.err

}

func (*fakeAssignCaller) BestFacadeVersion(facade string) int {
	return 1
}

type fakeWatchCaller struct {
	base.APICaller
	sync.Mutex
	request  string
	params   interface{}
	response params.StringsWatchResult
	err      error
	c        *gc.C
}

func (f *fakeWatchCaller) APICall(objType string, version int, id, request string, param, response interface{}) error {
	f.Lock()
	defer f.Unlock()
	f.request = request
	f.params = param
	_, ok := response.(*params.StringsWatchResult)
	if !ok {
		f.c.Errorf("Expected *params.StringsWatchResult as response, but was %#v", response)
	}
	return f.err
}

func (*fakeWatchCaller) BestFacadeVersion(facade string) int {
	return 1
}
