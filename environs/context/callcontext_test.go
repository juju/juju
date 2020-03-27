// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type CallContextSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CallContextSuite{})

func (s *CallContextSuite) TestCallContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	invalidator := NewMockModelCredentialInvalidator(ctrl)
	invalidator.EXPECT().InvalidateModelCredential("call").Return(nil)

	ctx := CallContext(invalidator)
	c.Assert(ctx, gc.NotNil)

	err := ctx.InvalidateCredential("call")
	c.Assert(err, jc.ErrorIsNil)
}
