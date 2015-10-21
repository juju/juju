// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/watcher"
)

var _ = gc.Suite(testsuite{})

type testsuite struct{}

func (testsuite) TestSetup(c *gc.C) {
	f := &fakeAPI{}
	ua := unitAssigner{api: f}
	_, err := ua.SetUp()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.calledWatch, jc.IsTrue)

	f.err = errors.New("boo")
	_, err = ua.SetUp()
	c.Assert(err, gc.Equals, f.err)
}

func (testsuite) TestHandle(c *gc.C) {
	f := &fakeAPI{}
	ua := unitAssigner{api: f}
	ids := []string{"foo", "bar"}
	err := ua.Handle(ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.assignIds, gc.DeepEquals, ids)

	f.err = errors.New("boo")
	err = ua.Handle(ids)
	c.Assert(err, gc.Equals, f.err)
}

type fakeAPI struct {
	calledWatch bool
	assignIds   []string
	err         error
}

func (f *fakeAPI) AssignUnits(ids []string) ([]error, error) {
	f.assignIds = ids
	return nil, f.err
}

func (f *fakeAPI) WatchUnitAssignments() (watcher.StringsWatcher, error) {
	f.calledWatch = true
	return nil, f.err
}
