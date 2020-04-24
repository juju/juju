// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/charm/v7"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
)

type registerSuite struct {
	testing.IsolationSuite

	hookCtx *stubRegisterContext
	command RegisterCmd
}

var _ = gc.Suite(&registerSuite{})

func (s *registerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.hookCtx = &stubRegisterContext{}

	s.command = RegisterCmd{
		hookContextFunc: func() (Component, error) {
			return s.hookCtx, nil
		},
	}
}

func (s *registerSuite) TestInitNilArgs(c *gc.C) {
	err := s.command.Init(nil)
	c.Assert(err, gc.NotNil)
}

func (s *registerSuite) TestInitTooFewArgs(c *gc.C) {
	err := s.command.Init([]string{"foo", "bar"})
	c.Assert(err, gc.NotNil)
}

func (s *registerSuite) TestInit(c *gc.C) {
	err := s.command.Init([]string{"type", "class", "id"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.command.typ, gc.Equals, "type")
	c.Assert(s.command.class, gc.Equals, "class")
	c.Assert(s.command.id, gc.Equals, "id")
	c.Assert(s.command.labels, gc.HasLen, 0)
}

func (s *registerSuite) TestInitWithLabels(c *gc.C) {
	err := s.command.Init([]string{"type", "class", "id", "tag1", "tag 2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.command.typ, gc.Equals, "type")
	c.Assert(s.command.class, gc.Equals, "class")
	c.Assert(s.command.id, gc.Equals, "id")
	c.Assert(s.command.labels, gc.DeepEquals, []string{"tag1", "tag 2"})
}

func (s *registerSuite) TestRun(c *gc.C) {
	err := s.command.Init([]string{"type", "class", "id", "tag1", "tag 2"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := setupMetadata(c)
	err = s.command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.hookCtx.flushed, jc.IsTrue)
	c.Check(s.hookCtx.payload, jc.DeepEquals, payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "class",
			Type: "type",
		},
		ID:     "id",
		Status: payload.StateRunning,
		Labels: []string{"tag1", "tag 2"},
		Unit:   "a-application/0",
	})
	// TODO (natefinch): we need to do something with the labels
}

func (s *registerSuite) TestRunUnknownClass(c *gc.C) {
	err := s.command.Init([]string{"type", "badclass", "id"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := setupMetadata(c)
	err = s.command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "payload \"badclass\" not found in metadata.yaml")
}

func (s *registerSuite) TestRunUnknownType(c *gc.C) {
	err := s.command.Init([]string{"badtype", "class", "id"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := setupMetadata(c)
	err = s.command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "incorrect type \"badtype\" for payload \"class\", expected \"type\"")
}

func (s *registerSuite) TestRunTrackErr(c *gc.C) {
	s.hookCtx.trackerr = errors.Errorf("boo")
	err := s.command.Init([]string{"type", "class", "id", "tag1", "tag 2"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := setupMetadata(c)
	err = s.command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "boo")
}

func (s *registerSuite) TestRunFlushErr(c *gc.C) {
	s.hookCtx.flusherr = errors.Errorf("boo")
	err := s.command.Init([]string{"type", "class", "id", "tag1", "tag 2"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := setupMetadata(c)
	err = s.command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "boo")
}

type stubRegisterContext struct {
	Component
	payload  payload.Payload
	flushed  bool
	trackerr error
	flusherr error
}

func (f *stubRegisterContext) Track(pl payload.Payload) error {
	f.payload = pl
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
summary: Testing charm payload management
maintainer: juju@canonical.com <Juju>
description: |
  Testing payloads
subordinate: false
payloads:
  class:
    type: type
    lifecycle: ["restart"]
`
