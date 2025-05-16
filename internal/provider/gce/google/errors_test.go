// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"fmt"
	"net/http"
	"net/url"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/gce/google"
	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	googleError   *url.Error
	internalError *googlyError
}

func TestErrorSuite(t *stdtesting.T) { tc.Run(t, &ErrorSuite{}) }
func (s *ErrorSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.internalError = &googlyError{"400 Bad Request"}
	s.googleError = &url.Error{"Get", "http://notforreal.com/", s.internalError}
}

func (s *ErrorSuite) TestAuthRelatedStatusCodes(c *tc.C) {
	// First test another status code.
	s.internalError.SetMessage(http.StatusAccepted, "Accepted")
	denied := google.IsAuthorisationFailure(s.internalError)
	c.Assert(denied, tc.IsFalse)

	for code, descs := range google.AuthorisationFailureStatusCodes {
		for _, desc := range descs {
			s.internalError.SetMessage(code, desc)
			denied = google.IsAuthorisationFailure(s.googleError)
			c.Assert(denied, tc.IsTrue)
		}
	}

	for code := range google.AuthorisationFailureStatusCodes {
		s.internalError.SetMessage(code, "Some strange error")
		denied = google.IsAuthorisationFailure(s.googleError)
		c.Assert(denied, tc.IsFalse)
	}
}

type googlyError struct {
	msg string
}

func (e *googlyError) Error() string { return e.msg }

func (e *googlyError) SetMessage(code int, desc string) {
	e.msg = fmt.Sprintf("%v %v", code, desc)
}
