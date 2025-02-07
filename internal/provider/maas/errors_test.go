// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	maasError error
}

var _ = gc.Suite(&ErrorSuite{})

func (s *ErrorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.maasError = gomaasapi.NewPermissionError("denial")
}

func (s *ErrorSuite) TestNoValidation(c *gc.C) {
	denied := common.HandleCredentialError(
		context.Background(),
		environs.NoopModelCredentialInvalidator{},
		IsAuthorisationFailure, s.maasError)
	c.Assert(c.GetTestLog(), jc.DeepEquals, "")
	c.Assert(denied, jc.IsTrue)
}

func (s *ErrorSuite) TestInvalidationCallbackErrorOnlyLogs(c *gc.C) {
	invalidator := modelCredentialInvalidator(func(reason environs.InvalidationReason) error {
		return errors.New("kaboom")
	})
	denied := common.HandleCredentialError(context.Background(), invalidator, IsAuthorisationFailure, s.maasError)
	c.Assert(c.GetTestLog(), jc.Contains, "could not invalidate stored cloud credential on the controller")
	c.Assert(denied, jc.IsTrue)
}

func (s *ErrorSuite) TestHandleCredentialErrorPermissionError(c *gc.C) {
	s.checkMaasPermissionHandling(c, true)

	s.maasError = errors.Trace(s.maasError)
	s.checkMaasPermissionHandling(c, true)

	s.maasError = errors.Annotatef(s.maasError, "more and more")
	s.checkMaasPermissionHandling(c, true)
}

func (s *ErrorSuite) TestHandleCredentialErrorAnotherError(c *gc.C) {
	s.maasError = errors.New("fluffy")
	s.checkMaasPermissionHandling(c, false)
}

func (s *ErrorSuite) TestNilError(c *gc.C) {
	s.maasError = nil
	s.checkMaasPermissionHandling(c, false)
}

func (s *ErrorSuite) TestGomaasError(c *gc.C) {
	// check accepted status codes
	s.maasError = gomaasapi.ServerError{StatusCode: http.StatusAccepted}
	s.checkMaasPermissionHandling(c, false)

	for t := range common.AuthorisationFailureStatusCodes {
		s.maasError = gomaasapi.ServerError{StatusCode: t}
		s.checkMaasPermissionHandling(c, true)
	}
}

func (s *ErrorSuite) checkMaasPermissionHandling(c *gc.C, handled bool) {
	called := false
	invalidator := modelCredentialInvalidator(func(reason environs.InvalidationReason) error {
		c.Assert(reason.String(), gc.Matches, "cloud denied access:.*")
		called = true
		return nil
	})
	denied := common.HandleCredentialError(context.Background(), invalidator, IsAuthorisationFailure, s.maasError)
	c.Assert(called, gc.Equals, handled)
	c.Assert(denied, gc.Equals, handled)
}

type modelCredentialInvalidator func(reason environs.InvalidationReason) error

func (m modelCredentialInvalidator) InvalidateModelCredential(_ context.Context, reason environs.InvalidationReason) error {
	return m(reason)
}
