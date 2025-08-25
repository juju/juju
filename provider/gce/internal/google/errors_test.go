// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/gce/internal/google"
	"github.com/juju/juju/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	googleError   *url.Error
	internalError *googlyError
}

var _ = gc.Suite(&ErrorSuite{})

func (s *ErrorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.internalError = &googlyError{"400 Bad Request"}
	s.googleError = &url.Error{"Get", "http://notforreal.com/", s.internalError}
}

func (s *ErrorSuite) TestNilContext(c *gc.C) {
	err := google.HandleCredentialError(s.googleError, nil)
	c.Assert(err, gc.DeepEquals, s.googleError)
	c.Assert(c.GetTestLog(), jc.DeepEquals, "")
}

func (s *ErrorSuite) TestInvalidationCallbackErrorOnlyLogs(c *gc.C) {
	ctx := context.NewEmptyCloudCallContext()
	ctx.InvalidateCredentialFunc = func(msg string) error {
		return errors.New("kaboom")
	}
	google.HandleCredentialError(s.googleError, ctx)
	c.Assert(c.GetTestLog(), jc.Contains, "could not invalidate stored google cloud credential on the controller")
}

func (s *ErrorSuite) TestAuthRelatedStatusCodes(c *gc.C) {
	ctx := context.NewEmptyCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		c.Assert(msg, gc.Matches,
			regexp.QuoteMeta(`google cloud denied access: Get "http://notforreal.com/": 40`)+".*")
		called = true
		return nil
	}

	// First test another status code.
	s.internalError.SetMessage(http.StatusAccepted, "Accepted")
	google.HandleCredentialError(s.googleError, ctx)
	c.Assert(called, jc.IsFalse)

	for code, descs := range google.AuthorisationFailureStatusCodes {
		for _, desc := range descs {
			called = false
			s.internalError.SetMessage(code, desc)
			google.HandleCredentialError(s.googleError, ctx)
			c.Assert(called, jc.IsTrue)
		}
	}

	called = false
	for code := range google.AuthorisationFailureStatusCodes {
		s.internalError.SetMessage(code, "Some strange error")
		google.HandleCredentialError(s.googleError, ctx)
		c.Assert(called, jc.IsFalse)
	}
}

func (*ErrorSuite) TestNilGoogleError(c *gc.C) {
	ctx := context.NewEmptyCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		called = true
		return nil
	}
	returnedErr := google.HandleCredentialError(nil, ctx)
	c.Assert(called, jc.IsFalse)
	c.Assert(returnedErr, jc.ErrorIsNil)
}

func (*ErrorSuite) TestAnyOtherError(c *gc.C) {
	ctx := context.NewEmptyCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		called = true
		return nil
	}

	notinterestingErr := errors.New("not kaboom")
	returnedErr := google.HandleCredentialError(notinterestingErr, ctx)
	c.Assert(called, jc.IsFalse)
	c.Assert(returnedErr, gc.DeepEquals, notinterestingErr)
}

type googlyError struct {
	msg string
}

func (e *googlyError) Error() string { return e.msg }

func (e *googlyError) SetMessage(code int, desc string) {
	e.msg = fmt.Sprintf("%v %v", code, desc)
}
