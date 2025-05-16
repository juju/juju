// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/http"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	maasError error
}

func TestErrorSuite(t *stdtesting.T) { tc.Run(t, &ErrorSuite{}) }
func (s *ErrorSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.maasError = gomaasapi.NewPermissionError("denial")
}

func (s *ErrorSuite) TestHandleCredentialErrorPermissionError(c *tc.C) {
	s.checkMaasPermissionHandling(c, true)

	s.maasError = errors.Trace(s.maasError)
	s.checkMaasPermissionHandling(c, true)

	s.maasError = errors.Annotatef(s.maasError, "more and more")
	s.checkMaasPermissionHandling(c, true)
}

func (s *ErrorSuite) TestHandleCredentialErrorAnotherError(c *tc.C) {
	s.maasError = errors.New("fluffy")
	s.checkMaasPermissionHandling(c, false)
}

func (s *ErrorSuite) TestNilError(c *tc.C) {
	s.maasError = nil
	s.checkMaasPermissionHandling(c, false)
}

func (s *ErrorSuite) TestGomaasError(c *tc.C) {
	// check accepted status codes
	s.maasError = gomaasapi.ServerError{StatusCode: http.StatusAccepted}
	s.checkMaasPermissionHandling(c, false)

	for t := range common.AuthorisationFailureStatusCodes {
		s.maasError = gomaasapi.ServerError{StatusCode: t}
		s.checkMaasPermissionHandling(c, true)
	}
}

func (s *ErrorSuite) checkMaasPermissionHandling(c *tc.C, handled bool) {
	denied := IsAuthorisationFailure(s.maasError)
	c.Assert(denied, tc.Equals, handled)
}
