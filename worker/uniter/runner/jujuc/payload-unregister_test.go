// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/worker/uniter/runner/jujuc"
	"github.com/juju/juju/v2/worker/uniter/runner/jujuc/mocks"
)

type PayloadUnregisterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&PayloadUnregisterSuite{})

func (s *PayloadUnregisterSuite) TestRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)
	hctx.EXPECT().UntrackPayload("class", "id").Return(nil)
	hctx.EXPECT().FlushPayloads()

	com, err := jujuc.NewCommand(hctx, cmdString("payload-unregister"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"class", "id"})
	c.Assert(code, gc.Equals, 0)
}
