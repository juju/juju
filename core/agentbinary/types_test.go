// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
)

type typeSuite struct{}

var _ = tc.Suite(&typeSuite{})

// TestVersionValidation verifies that validation succeeds when given valid
// version attributes.
func (s *typeSuite) TestVersionValidation(c *tc.C) {
	v := Version{
		Number: semversion.MustParse("4.1.1"),
		Arch:   arch.AMD64,
	}
	c.Check(v.Validate(), tc.ErrorIsNil)
}

// TestVersionValidationFailsWithZeroVersion checks that if we specify the zero
// value for the agent version number, we get a validation error that satisfies
// [coreerrors.NotValid]
func (s *typeSuite) TestVersionValidationFailsWithZeroVersion(c *tc.C) {
	v := Version{
		Number: semversion.Zero,
		Arch:   arch.ARM64,
	}
	err := v.Validate()
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestVersionValidationFailsWithUnsupportedArch checks that if we specify an
// architecture that is unsupported, we get back a validation error that
// satisfies [coreerrors.NotValid].
func (s *typeSuite) TestVersionValidationFailsWithUnsupportedArch(c *tc.C) {
	v := Version{
		Number: semversion.MustParse("2.0.0"),
		Arch:   arch.Arch("unsupported"),
	}
	err := v.Validate()
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}
