// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"context"
	"errors"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(testsuite{})

type testsuite struct{}

func newHandler(c *tc.C, api UnitAssigner) unitAssignerHandler {
	return unitAssignerHandler{api: api, logger: loggertesting.WrapCheckLog(c)}
}

func (testsuite) TestSetup(c *tc.C) {
	f := &fakeAPI{}
	ua := newHandler(c, f)
	_, err := ua.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.calledWatch, tc.IsTrue)

	f.err = errors.New("boo")
	_, err = ua.SetUp(c.Context())
	c.Assert(err, tc.Equals, f.err)
}

func (testsuite) TestHandle(c *tc.C) {
	f := &fakeAPI{}
	ua := newHandler(c, f)
	ids := []string{"foo/0", "bar/0"}
	err := ua.Handle(nil, ids)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.assignTags, tc.DeepEquals, []names.UnitTag{
		names.NewUnitTag("foo/0"),
		names.NewUnitTag("bar/0"),
	})

	f.err = errors.New("boo")
	err = ua.Handle(nil, ids)
	c.Assert(err, tc.Equals, f.err)
}

func (testsuite) TestHandleError(c *tc.C) {
	e := errors.New("some error")
	f := &fakeAPI{assignErrs: []error{e}}
	ua := newHandler(c, f)
	ids := []string{"foo/0", "bar/0"}
	err := ua.Handle(nil, ids)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.assignTags, tc.DeepEquals, []names.UnitTag{
		names.NewUnitTag("foo/0"),
		names.NewUnitTag("bar/0"),
	})
	c.Assert(f.status.Entities, tc.NotNil)
	entities := f.status.Entities
	c.Assert(entities, tc.HasLen, 1)
	c.Assert(entities[0], tc.DeepEquals, params.EntityStatusArgs{
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

func (f *fakeAPI) AssignUnits(ctx context.Context, tags []names.UnitTag) ([]error, error) {
	f.assignTags = tags
	return f.assignErrs, f.err
}

func (f *fakeAPI) WatchUnitAssignments(ctx context.Context) (watcher.StringsWatcher, error) {
	f.calledWatch = true
	return nil, f.err
}

func (f *fakeAPI) SetAgentStatus(ctx context.Context, args params.SetStatus) error {
	f.status = args
	return f.err
}
