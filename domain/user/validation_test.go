// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	usererrors "github.com/juju/juju/domain/user/errors"
	usertesting "github.com/juju/juju/domain/user/testing"
)

type validationSuite struct{}

var _ = gc.Suite(&validationSuite{})

// TestUsernameValidation exists to assert the regex that is in use by
// ValidateUserName. We want to pass it a wide range of unicode names with weird
func (s *validationSuite) TestUsernameValidation(c *gc.C) {
	tests := []struct {
		Username   string
		ShouldPass bool
	}{}

	for _, valid := range usertesting.ValidUsernames {
		tests = append(tests, struct {
			Username   string
			ShouldPass bool
		}{valid, true})
	}

	for _, invalid := range usertesting.InvalidUsernames {
		tests = append(tests, struct {
			Username   string
			ShouldPass bool
		}{invalid, false})
	}

	for _, test := range tests {
		err := ValidateUserName(test.Username)
		if test.ShouldPass {
			c.Assert(err, jc.ErrorIsNil, gc.Commentf("test username %q", test.Username))
		} else {
			c.Assert(
				err, jc.ErrorIs, usererrors.UsernameNotValid,
				gc.Commentf("test username %q", test.Username),
			)
		}
	}
}
