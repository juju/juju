// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors_test

import (
	stderrors "errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/proxy/errors"
)

var _ = gc.Suite(&ErrorsSuite{})

type ErrorsSuite struct {
	testing.IsolationSuite
}

func (*ErrorsSuite) TestIsProxyConnectError(c *gc.C) {
	c.Assert(errors.IsProxyConnectError(nil), jc.IsFalse)
	err := stderrors.New("foo")
	c.Assert(errors.IsProxyConnectError(err), jc.IsFalse)
	err = errors.NewProxyConnectError(stderrors.New("foo"), "")
	c.Assert(errors.IsProxyConnectError(err), jc.IsTrue)
}

func (*ErrorsSuite) TestProxyType(c *gc.C) {
	err := errors.NewProxyConnectError(stderrors.New("foo"), "bar")
	c.Assert(errors.ProxyType(err), gc.Equals, "bar")
}
