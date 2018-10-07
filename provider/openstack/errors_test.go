// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	gooseerrors "gopkg.in/goose.v2/errors"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/testing"
)

type ErrorSuite struct {
	testing.BaseSuite
	openstackError error
}

var _ = gc.Suite(&ErrorSuite{})

func (s *ErrorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.openstackError = gooseerrors.NewUnauthorisedf(nil, "", "permision denial")
}

func (s *ErrorSuite) TestNilContext(c *gc.C) {
	err, denied := MaybeHandleCredentialError(s.openstackError, nil)
	c.Assert(err, gc.DeepEquals, s.openstackError)
	c.Assert(c.GetTestLog(), jc.DeepEquals, "")
	c.Assert(denied, jc.IsTrue)
}

func (s *ErrorSuite) TestInvalidationCallbackErrorOnlyLogs(c *gc.C) {
	ctx := context.NewCloudCallContext()
	ctx.InvalidateCredentialFunc = func(msg string) error {
		return errors.New("kaboom")
	}
	_, denied := MaybeHandleCredentialError(s.openstackError, ctx)
	c.Assert(c.GetTestLog(), jc.Contains, "could not invalidate stored openstack cloud credential on the controller")
	c.Assert(denied, jc.IsTrue)
}

func (s *ErrorSuite) TestNilError(c *gc.C) {
	s.openstackError = nil
	s.checkOpenstackPermissionHandling(c, false)
}

func (s *ErrorSuite) TestHandleCredentialErrorPermissionError(c *gc.C) {
	s.checkOpenstackPermissionHandling(c, true)

	s.openstackError = gooseerrors.NewUnauthorisedf(nil, "", "token is empty")
	s.checkOpenstackPermissionHandling(c, true)
}

func (s *ErrorSuite) TestHandleCredentialErrorAnotherError(c *gc.C) {
	s.openstackError = errors.New("fluffy")
	s.checkOpenstackPermissionHandling(c, false)
}

func (s *ErrorSuite) checkOpenstackPermissionHandling(c *gc.C, handled bool) {
	ctx := context.NewCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		c.Assert(msg, gc.DeepEquals, "openstack cloud denied access")
		called = true
		return nil
	}

	_, denied := MaybeHandleCredentialError(s.openstackError, ctx)
	c.Assert(called, gc.Equals, handled)
	c.Assert(denied, gc.Equals, handled)
}
