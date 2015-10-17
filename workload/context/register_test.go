// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd"
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
	f := &stubRegisterContext{}
	r := RegisterCmd{api: f}
	err := r.Init([]string{"type", "class", "id", "tag1", "tag 2"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := setupMetadata(c)
	err = r.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.flushed, jc.IsTrue)
	c.Assert(f.info.Workload.Name, gc.Equals, "class")
	c.Assert(f.info.Workload.Type, gc.Equals, "type")
	c.Assert(f.info.Status.State, gc.Equals, workload.StateRunning)
	c.Assert(f.info.Details.ID, gc.Equals, "id")

	// TODO (natefinch): we need to do something with the tags
}

func (registerSuite) TestRunUnknownClass(c *gc.C) {
	f := &stubRegisterContext{}
	r := RegisterCmd{api: f}
	err := r.Init([]string{"type", "badclass", "id"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := setupMetadata(c)
	err = r.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "payload \"badclass\" not found in metadata.yaml")
}

func (registerSuite) TestRunUnknownType(c *gc.C) {
	f := &stubRegisterContext{}
	r := RegisterCmd{api: f}
	err := r.Init([]string{"badtype", "class", "id"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := setupMetadata(c)
	err = r.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "incorrect type \"badtype\" for payload \"class\", expected \"type\"")
}

func (registerSuite) TestRunTrackErr(c *gc.C) {
	f := &stubRegisterContext{trackerr: errors.Errorf("boo")}
	r := RegisterCmd{api: f}
	err := r.Init([]string{"type", "class", "id", "tag1", "tag 2"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := setupMetadata(c)
	err = r.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "boo")
}

func (registerSuite) TestRunFlushErr(c *gc.C) {
	f := &stubRegisterContext{flusherr: errors.Errorf("boo")}
	r := RegisterCmd{api: f}
	err := r.Init([]string{"type", "class", "id", "tag1", "tag 2"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := setupMetadata(c)
	err = r.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "boo")
}

type stubRegisterContext struct {
	Component
	info     workload.Info
	flushed  bool
	trackerr error
	flusherr error
}

func (f *stubRegisterContext) Track(info workload.Info) error {
	f.info = info
	return f.trackerr
}

func (f *stubRegisterContext) Flush() error {
	f.flushed = true
	return f.flusherr
}

func setupMetadata(c *gc.C) *cmd.Context {
	dir := c.MkDir()
	path := filepath.Join(dir, "metadata.yaml")
	ioutil.WriteFile(path, []byte(metadataContents), 0660)
	return &cmd.Context{Dir: dir}
}

const metadataContents = `name: ducksay
summary: Testing workload processes
maintainer: juju@canonical.com <Juju>
description: |
  Testing workloads
subordinate: false
payloads:
  class:
    type: type
    lifecycle: ["restart"]
`
