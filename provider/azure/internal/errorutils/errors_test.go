// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils_test

import (
	"net/http"

	"github.com/Azure/go-autorest/autorest"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure/internal/errorutils"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	azureError autorest.DetailedError
}

var _ = gc.Suite(&ErrorSuite{})

func (s *ErrorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.azureError = autorest.DetailedError{
		StatusCode: http.StatusUnauthorized,
	}
}

func (s *ErrorSuite) TestNilContext(c *gc.C) {
	err := errorutils.HandleCredentialError(s.azureError, nil)
	c.Assert(err, gc.DeepEquals, s.azureError)

	invalidated := errorutils.MaybeInvalidateCredential(s.azureError, nil)
	c.Assert(invalidated, jc.IsFalse)

	c.Assert(c.GetTestLog(), jc.DeepEquals, "")
}

func (s *ErrorSuite) TestInvalidationCallbackErrorOnlyLogs(c *gc.C) {
	ctx := context.NewCloudCallContext()
	ctx.InvalidateCredentialFunc = func(msg string) error {
		return errors.New("kaboom")
	}
	errorutils.MaybeInvalidateCredential(s.azureError, ctx)
	c.Assert(c.GetTestLog(), jc.Contains, "could not invalidate stored azure cloud credential on the controller")
}

func (s *ErrorSuite) TestAuthRelatedStatusCodes(c *gc.C) {
	ctx := context.NewCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		c.Assert(msg, gc.DeepEquals, "azure cloud denied access")
		called = true
		return nil
	}

	// First test another status code.
	s.azureError.StatusCode = http.StatusAccepted
	errorutils.HandleCredentialError(s.azureError, ctx)
	c.Assert(called, jc.IsFalse)

	for t := range common.AuthorisationFailureStatusCodes {
		called = false
		s.azureError.StatusCode = t
		errorutils.HandleCredentialError(s.azureError, ctx)
		c.Assert(called, jc.IsTrue)
	}
}

func (*ErrorSuite) TestNilAzureError(c *gc.C) {
	ctx := context.NewCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		called = true
		return nil
	}
	returnedErr := errorutils.HandleCredentialError(nil, ctx)
	c.Assert(called, jc.IsFalse)
	c.Assert(returnedErr, jc.ErrorIsNil)
}
