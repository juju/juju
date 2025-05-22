// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors_test

import (
	stderrors "errors"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/proxy/errors"
	"github.com/juju/juju/internal/testhelpers"
)

func TestErrorsSuite(t *testing.T) {
	tc.Run(t, &ErrorsSuite{})
}

type ErrorsSuite struct {
	testhelpers.IsolationSuite
}

func (*ErrorsSuite) TestIsProxyConnectError(c *tc.C) {
	c.Assert(errors.IsProxyConnectError(nil), tc.IsFalse)
	err := stderrors.New("foo")
	c.Assert(errors.IsProxyConnectError(err), tc.IsFalse)
	err = errors.NewProxyConnectError(stderrors.New("foo"), "")
	c.Assert(errors.IsProxyConnectError(err), tc.IsTrue)
}

func (*ErrorsSuite) TestProxyType(c *tc.C) {
	err := errors.NewProxyConnectError(stderrors.New("foo"), "bar")
	c.Assert(errors.ProxyType(err), tc.Equals, "bar")
}
