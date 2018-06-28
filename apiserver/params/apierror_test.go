// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
)

type errorSuite struct{}

var _ rpc.ErrorCoder = (*params.Error)(nil)

var _ = gc.Suite(&errorSuite{})

func (*errorSuite) TestErrCode(c *gc.C) {
	err0 := &params.Error{Code: params.CodeDead, Message: "brain dead test"}
	c.Check(params.ErrCode(err0), gc.Equals, params.CodeDead)

	err1 := errors.Trace(err0)
	c.Check(params.ErrCode(err1), gc.Equals, params.CodeDead)
}
