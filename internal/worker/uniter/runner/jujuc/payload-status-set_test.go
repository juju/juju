// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/mocks"
)

type PayloadStatusSetSuiye struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&PayloadStatusSetSuiye{})

func (s *PayloadStatusSetSuiye) TestTooFewArgs(c *gc.C) {
	cmd := jujuc.PayloadStatusSetCmd{}
	err := cmd.Init([]string{})
	c.Check(err, gc.ErrorMatches, `missing .*`)

	err = cmd.Init([]string{payloads.StateRunning})
	c.Check(err, gc.ErrorMatches, `missing .*`)
}

func (s *PayloadStatusSetSuiye) TestInvalidStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)

	com, err := jujuc.NewCommand(hctx, "payload-status-set")
	c.Assert(err, jc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"class", "id", "created"})
	c.Assert(code, gc.Equals, 1)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `ERROR status "created" not supported; expected one of ["running", "starting", "stopped", "stopping"]`+"\n")
}

func (s *PayloadStatusSetSuiye) TestStatusSet(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)
	hctx.EXPECT().SetPayloadStatus("class", "id", "stopped").Return(nil)
	hctx.EXPECT().FlushPayloads()

	com, err := jujuc.NewCommand(hctx, "payload-status-set")
	c.Assert(err, jc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"class", "id", "stopped"})
	c.Assert(code, gc.Equals, 0)
}
