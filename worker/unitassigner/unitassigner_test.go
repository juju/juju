// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"errors"

	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

var _ = gc.Suite(testsuite{})

type testsuite struct{}

func newHandler(api UnitAssigner) unitAssignerHandler {
	return unitAssignerHandler{api: api, logger: loggo.GetLogger("test")}
}

func (testsuite) TestSetup(c *gc.C) {
	f := &fakeAPI{}
	ua := newHandler(f)
	_, err := ua.SetUp()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.calledWatch, jc.IsTrue)

	f.err = errors.New("boo")
	_, err = ua.SetUp()
	c.Assert(err, gc.Equals, f.err)
}

func (testsuite) TestHandle(c *gc.C) {
	f := &fakeAPI{}
	ua := newHandler(f)
	ids := []string{"foo/0", "bar/0"}
	err := ua.Handle(nil, ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.assignTags, gc.DeepEquals, []names.UnitTag{
		names.NewUnitTag("foo/0"),
		names.NewUnitTag("bar/0"),
	})

	f.err = errors.New("boo")
	err = ua.Handle(nil, ids)
	c.Assert(err, gc.Equals, f.err)
}

func (testsuite) TestHandleError(c *gc.C) {
	e := errors.New("some error")
	f := &fakeAPI{assignErrs: []error{e}}
	ua := newHandler(f)
	ids := []string{"foo/0", "bar/0"}
	err := ua.Handle(nil, ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.assignTags, gc.DeepEquals, []names.UnitTag{
		names.NewUnitTag("foo/0"),
		names.NewUnitTag("bar/0"),
	})
	c.Assert(f.status.Entities, gc.NotNil)
	entities := f.status.Entities
	c.Assert(entities, gc.HasLen, 1)
	c.Assert(entities[0], gc.DeepEquals, params.EntityStatusArgs{
		Tag:    "unit-foo-0",
		Status: status.Error.String(),
		Info:   e.Error(),
	})
}

type fakeAPI struct {
	calledWatch bool
	assignTags  []names.UnitTag
	err         error
	status      params.SetStatus
	assignErrs  []error
}

func (f *fakeAPI) AssignUnits(tags []names.UnitTag) ([]error, error) {
	f.assignTags = tags
	return f.assignErrs, f.err
}

func (f *fakeAPI) WatchUnitAssignments() (watcher.StringsWatcher, error) {
	f.calledWatch = true
	return nil, f.err
}

func (f *fakeAPI) SetAgentStatus(args params.SetStatus) error {
	f.status = args
	return f.err
}
