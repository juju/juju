// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var _ = gc.Suite(testsuite{})

type testsuite struct{}

func (testsuite) TestAssignUnits(c *gc.C) {
	f := &fakeAPICaller{}
	api := New(f)
	ids := []string{"foo", "bar"}
	res, err := api.AssignUnits(ids)
	c.Assert(f.request, gc.Equals, "AssignUnits")
	c.Assert(f.params, gc.DeepEquals, params.AssignUnitsParams{IDs: ids})
	c.Assert(err, jc.ErrorIsNil)
	if r, ok := f.response.(*params.AssignUnitsResults); !ok {
		c.Errorf("Expected &params.AssignUnitsResults, got %#v", f.response)
	} else {
		c.Assert(res, gc.DeepEquals, *r)
	}
}

func (testsuite) TestWatchUnitAssignment(c *gc.C) {
	f := &fakeAPICaller{}
	api := New(f)
	w, err := api.WatchUnitAssignments()
	c.Assert(f.request, gc.Equals, "WatchUnitAssignments")
	c.Assert(f.params, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
	if _, ok := f.response.(*params.StringsWatchResult); !ok {
		c.Errorf("Expected &params.StringsWatchResult, got %#v", f.response)
	}
	c.Assert(w, gc.NotNil)
}

type fakeAPICaller struct {
	base.APICaller
	request  string
	params   interface{}
	response interface{}
	err      error
}

func (f *fakeAPICaller) APICall(objType string, version int, id, request string, params, response interface{}) error {
	f.request = request
	f.params = params
	f.response = response
	return f.err

}

func (fakeAPICaller) BestFacadeVersion(facade string) int {
	return 0
}
