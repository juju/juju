// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
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
github.com/juju/juju/provider/common/errors_test.go:.*: foo
github.com/juju/juju/provider/common/errors_test.go:.*: bar
github.com/juju/juju/provider/common/errors_test.go:.*: bar: foo`[1:])
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
github.com/juju/juju/provider/common/errors_test.go:.*: foo
github.com/juju/juju/provider/common/errors_test.go:.*: bar
github.com/juju/juju/provider/common/errors_test.go:.*: bar: foo`[1:])
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
