// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ErrorSuite{})

func (s *ErrorSuite) TestIsAuthorisationFailurePermissionStatus(c *gc.C) {
	ok := IsAuthorisationFailure(gomaasapi.NewPermissionError("denial"))
	c.Assert(ok, jc.IsTrue)
}

func (s *ErrorSuite) TestIsAuthorisationFailureServerError(c *gc.C) {
	ok := IsAuthorisationFailure(gomaasapi.ServerError{
		StatusCode: http.StatusUnauthorized,
	})
	c.Assert(ok, jc.IsTrue)
}

func (s *ErrorSuite) TestIsAuthorisationFailureNil(c *gc.C) {
	ok := IsAuthorisationFailure(nil)
	c.Assert(ok, jc.IsFalse)
}

func (s *ErrorSuite) TestIsAuthorisationFailureGeneric(c *gc.C) {
	ok := IsAuthorisationFailure(errors.New("foo"))
	c.Assert(ok, jc.IsTrue)
}
