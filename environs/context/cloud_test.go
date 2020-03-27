// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type CloudCallContextSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CloudCallContextSuite{})

func (s *CloudCallContextSuite) TestCloudCallContext(c *gc.C) {
	ctx := NewCloudCallContext()
	c.Assert(ctx, gc.NotNil)

	err := ctx.InvalidateCredential("call")
	c.Assert(err, gc.FitsTypeOf, errors.NewNotImplemented(nil, ""))
}
