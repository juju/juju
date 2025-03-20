// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	gc "gopkg.in/check.v1"

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
	denied := IsAuthorisationFailure(s.maasError)
	c.Assert(denied, gc.Equals, handled)
}
