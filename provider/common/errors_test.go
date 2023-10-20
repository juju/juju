// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/provider/common"
)

type ErrorsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ErrorsSuite{})

func (*ErrorsSuite) TestWrapZoneIndependentError(c *gc.C) {
	err1 := errors.New("foo")
	err2 := errors.Annotate(err1, "bar")
	wrapped := environs.ZoneIndependentError(err2)
	c.Assert(wrapped, jc.ErrorIs, environs.ErrAvailabilityZoneIndependent)
	c.Assert(wrapped, gc.ErrorMatches, "bar: foo")
}

func (s *ErrorsSuite) TestInvalidCredentialWrapped(c *gc.C) {
	err1 := errors.New("foo")
	err2 := errors.Annotate(err1, "bar")
	err := common.CredentialNotValidError(err2)

	// This is to confirm that Is(err, ErrorCredentialNotValid) is correct.
	c.Assert(err, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Assert(err, gc.ErrorMatches, "bar: foo")
}

func (s *ErrorsSuite) TestCredentialNotValidErrorLocationer(c *gc.C) {
	err := errors.New("some error")
	err = common.CredentialNotValidError(err)
	_, ok := err.(errors.Locationer)
	c.Assert(ok, jc.IsTrue)
}

func (s *ErrorsSuite) TestInvalidCredentialNew(c *gc.C) {
	err := fmt.Errorf("%w: Your account is blocked.", common.ErrorCredentialNotValid)
	c.Assert(err, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Assert(err, gc.ErrorMatches, "credential not valid: Your account is blocked.")
}

func (s *ErrorsSuite) TestInvalidCredentialf(c *gc.C) {
	err1 := errors.New("foo")
	err := fmt.Errorf("bar: %w", common.CredentialNotValidError(err1))
	c.Assert(err, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Assert(err, gc.ErrorMatches, "bar: foo")
}

var authFailureError = errors.New("auth failure")

func (s *ErrorsSuite) TestNoValidation(c *gc.C) {
	isAuthF := func(e error) bool {
		return true
	}
	denied := common.MaybeHandleCredentialError(isAuthF, authFailureError, envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(c.GetTestLog(), jc.DeepEquals, "")
	c.Assert(denied, jc.IsTrue)
}

func (s *ErrorsSuite) TestInvalidationCallbackErrorOnlyLogs(c *gc.C) {
	isAuthF := func(e error) bool {
		return true
	}
	ctx := envcontext.WithCredentialInvalidator(context.Background(), func(msg string) error {
		return errors.New("kaboom")
	})
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
	called := false
	ctx := envcontext.WithCredentialInvalidator(context.Background(), func(msg string) error {
		c.Assert(msg, gc.Matches, "cloud denied access:.*auth failure")
		called = true
		return nil
	})

	denied := common.MaybeHandleCredentialError(isAuthF, e, ctx)
	c.Assert(called, gc.Equals, handled)
	c.Assert(denied, gc.Equals, handled)
}
