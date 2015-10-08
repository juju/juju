// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
)

type registerSuite struct{}

var _ = gc.Suite(&registerSuite{})

func (registerSuite) TestInitNilArgs(c *gc.C) {
	r := RegisterCmd{}
	err := r.Init(nil)
	c.Assert(err, gc.NotNil)
}

func (registerSuite) TestInitTooFewArgs(c *gc.C) {
	r := RegisterCmd{}
	err := r.Init([]string{"foo", "bar"})
	c.Assert(err, gc.NotNil)
}

func (registerSuite) TestInit(c *gc.C) {
	r := RegisterCmd{}
	err := r.Init([]string{"type", "class", "id"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.typ, gc.Equals, "type")
	c.Assert(r.class, gc.Equals, "class")
	c.Assert(r.id, gc.Equals, "id")
	c.Assert(r.tags, gc.HasLen, 0)
}

func (registerSuite) TestInitWithTags(c *gc.C) {
	r := RegisterCmd{}
	err := r.Init([]string{"type", "class", "id", "tag1", "tag 2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.typ, gc.Equals, "type")
	c.Assert(r.class, gc.Equals, "class")
	c.Assert(r.id, gc.Equals, "id")
	c.Assert(r.tags, gc.DeepEquals, []string{"tag1", "tag 2"})
}

func (registerSuite) TestRun(c *gc.C) {
	f := &fakeComponent{}
	r := RegisterCmd{Comp: f}
	err := r.Init([]string{"type", "class", "id", "tag1", "tag 2"})
	c.Assert(err, jc.ErrorIsNil)

	err = r.Run(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.flushed, jc.IsTrue)
	c.Assert(f.info.Workload.Name, gc.Equals, "class")
	c.Assert(f.info.Workload.Type, gc.Equals, "type")
	c.Assert(f.info.Status.State, gc.Equals, workload.StateRunning)
	c.Assert(f.info.Details.ID, gc.Equals, "id")

	// TODO (natefinch): we need to do something with the tags
}

func (registerSuite) TestRunTrackErr(c *gc.C) {
	f := &fakeComponent{trackerr: errors.Errorf("boo")}
	r := RegisterCmd{Comp: f}
	err := r.Run(nil)
	c.Assert(err, gc.ErrorMatches, "boo")
}

func (registerSuite) TestRunFlushErr(c *gc.C) {
	f := &fakeComponent{flusherr: errors.Errorf("boo")}
	r := RegisterCmd{Comp: f}
	err := r.Run(nil)
	c.Assert(err, gc.ErrorMatches, "boo")
}

type fakeComponent struct {
	Component
	info     workload.Info
	flushed  bool
	trackerr error
	flusherr error
}

func (f *fakeComponent) Track(info workload.Info) error {
	f.info = info
	return f.trackerr
}

func (f *fakeComponent) Flush() error {
	f.flushed = true
	return f.flusherr
}
