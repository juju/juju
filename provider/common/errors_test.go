// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

type ErrorsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ErrorsSuite{})

func (*ErrorsSuite) TestWrapZoneIndependentError(c *gc.C) {
	err1 := errors.New("foo")
	err2 := errors.Annotate(err1, "bar")
	wrapped := common.ZoneIndependentError(err2)
	c.Assert(wrapped, jc.Satisfies, environs.IsAvailabilityZoneIndependent)
	c.Assert(wrapped, gc.ErrorMatches, "bar: foo")

	stack := errors.ErrorStack(wrapped)
	c.Assert(stack, gc.Matches, `
.*github.com/juju/juju/provider/common/errors_test.go:.*: foo
.*github.com/juju/juju/provider/common/errors_test.go:.*: bar
.*github.com/juju/juju/provider/common/errors_test.go:.*: bar: foo`[1:])
}

func (s *ErrorsSuite) TestInvalidCredentialWrapped(c *gc.C) {
	err1 := errors.New("foo")
	err2 := errors.Annotate(err1, "bar")
	err := common.CredentialNotValid(err2)

	// This is to confirm that IsCredentialNotValid is correct.
	c.Assert(err2, gc.Not(jc.Satisfies), common.IsCredentialNotValid)
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	c.Assert(err, gc.ErrorMatches, "bar: foo")

	stack := errors.ErrorStack(err)
	c.Assert(stack, gc.Matches, `
.*github.com/juju/juju/provider/common/errors_test.go:.*: foo
.*github.com/juju/juju/provider/common/errors_test.go:.*: bar
.*github.com/juju/juju/provider/common/errors_test.go:.*: bar: foo`[1:])
}

func (s *ErrorsSuite) TestInvalidCredentialNew(c *gc.C) {
	err := common.NewCredentialNotValid("Your account is blocked.")
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	c.Assert(err, gc.ErrorMatches, "credential not valid: Your account is blocked.")
}

func (s *ErrorsSuite) TestInvalidCredentialf(c *gc.C) {
	err1 := errors.New("foo")
	err := common.CredentialNotValidf(err1, "bar")
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	c.Assert(err, gc.ErrorMatches, "bar: foo")
}

var authFailureError = errors.New("auth failure")

func (s *ErrorsSuite) TestNilContext(c *gc.C) {
	isAuthF := func(e error) bool {
		return true
	}
	denied := common.MaybeHandleCredentialError(isAuthF, authFailureError, nil)
	c.Assert(c.GetTestLog(), jc.DeepEquals, "")
	c.Assert(denied, jc.IsTrue)
}

func (s *ErrorsSuite) TestInvalidationCallbackErrorOnlyLogs(c *gc.C) {
	isAuthF := func(e error) bool {
		return true
	}
	ctx := context.NewCloudCallContext()
	ctx.InvalidateCredentialFunc = func(msg string) error {
		return errors.New("kaboom")
	}
	denied := common.MaybeHandleCredentialError(isAuthF, authFailureError, ctx)
	c.Assert(c.GetTestLog(), jc.Contains, "could not invalidate stored cloud credential on the controller")
	c.Assert(denied, jc.IsTrue)
}

func (s *ErrorsSuite) TestHandleCredentialErrorPermissionError(c *gc.C) {
	s.checkPermissionHandling(c, authFailureError, true)

	e := errors.Trace(authFailureError)
	s.checkPermissionHandling(c, e, true)

	e = errors.Annotatef(authFailureError, "more and more")
	s.checkPermissionHandling(c, e, true)
}

func (s *ErrorsSuite) TestHandleCredentialErrorAnotherError(c *gc.C) {
	s.checkPermissionHandling(c, errors.New("fluffy"), false)
}

func (s *ErrorsSuite) TestNilError(c *gc.C) {
	s.checkPermissionHandling(c, nil, false)
}

func (s *ErrorsSuite) checkPermissionHandling(c *gc.C, e error, handled bool) {
	isAuthF := func(e error) bool {
		return handled
	}
	ctx := context.NewCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		c.Assert(msg, gc.DeepEquals, "cloud denied access")
		called = true
		return nil
	}

	denied := common.MaybeHandleCredentialError(isAuthF, e, ctx)
	c.Assert(called, gc.Equals, handled)
	c.Assert(denied, gc.Equals, handled)
}
