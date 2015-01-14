// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

type errorSuite struct{}

var _ = gc.Suite(&errorSuite{})

func (*errorSuite) TestErrCode(c *gc.C) {
	var err error
	err = &params.Error{Code: params.CodeDead, Message: "brain dead test"}
	c.Check(params.ErrCode(err), gc.Equals, params.CodeDead)

	err = errors.Trace(err)
	c.Check(params.ErrCode(err), gc.Equals, params.CodeDead)
}
