// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"os"
	"path/filepath"

	"github.com/juju/charm/v12"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/mocks"
)

type registerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&registerSuite{})

func (s *registerSuite) TestInitNilArgs(c *gc.C) {
	cmd := jujuc.PayloadRegisterCmd{}
	err := cmd.Init(nil)
	c.Assert(err, gc.NotNil)
}

func (s *registerSuite) TestInitTooFewArgs(c *gc.C) {
	cmd := jujuc.PayloadRegisterCmd{}
	err := cmd.Init([]string{"foo", "bar"})
	c.Assert(err, gc.NotNil)
}

func (s *registerSuite) TestRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)
	payload := payloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "class",
			Type: "type",
		},
		ID:     "id",
		Status: payloads.StateRunning,
		Labels: []string{"tag1", "tag2"},
		Unit:   "a-application/0",
	}
	hctx.EXPECT().TrackPayload(payload).Return(nil)
	hctx.EXPECT().FlushPayloads()

	com, err := jujuc.NewCommand(hctx, "payload-register")
	c.Assert(err, jc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"type", "class", "id", "tag1", "tag2"})
	c.Assert(code, gc.Equals, 0)
}

func (s *registerSuite) TestRunUnknownClass(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)

	com, err := jujuc.NewCommand(hctx, "payload-register")
	c.Assert(err, jc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"type", "badclass", "id", "tag1", "tag2"})
	c.Assert(code, gc.Equals, 1)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `ERROR payload "badclass" not found in metadata.yaml`+"\n")
}

func (s *registerSuite) TestRunUnknownType(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)

	com, err := jujuc.NewCommand(hctx, "payload-register")
	c.Assert(err, jc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"badtype", "class", "id", "tag1", "tag2"})
	c.Assert(code, gc.Equals, 1)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `ERROR incorrect type "badtype" for payload "class", expected "type"`+"\n")
}

func (s *registerSuite) TestRunError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)
	payload := payloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "class",
			Type: "type",
		},
		ID:     "id",
		Status: payloads.StateRunning,
		Labels: []string{"tag1", "tag2"},
		Unit:   "a-application/0",
	}
	hctx.EXPECT().TrackPayload(payload).Return(errors.New("boom"))

	com, err := jujuc.NewCommand(hctx, "payload-register")
	c.Assert(err, jc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"type", "class", "id", "tag1", "tag2"})
	c.Assert(code, gc.Equals, 1)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `ERROR boom`+"\n")
}

func setupMetadata(c *gc.C) *cmd.Context {
	dir := c.MkDir()
	path := filepath.Join(dir, "metadata.yaml")
	err := os.WriteFile(path, []byte(metadataContents), 0660)
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	ctx.Dir = dir
	return ctx
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
