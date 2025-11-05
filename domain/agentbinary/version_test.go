// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"testing"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/tc"
)

// versionSuite represents a set of tests for validation [Version].
type versionSuite struct {
}

// TestVersionSuite runs all of the tests contained within [versionSuite].
func TestVersionSuite(t *testing.T) {
	tc.Run(t, versionSuite{})
}

// TestVersionValid tests that a valid [Version] does not return an error from
// [Version.Validate].
func (versionSuite) TestVersionValid(c *tc.C) {
	v := Version{
		Architecture: RISCV64,
		Number:       tc.Must1(c, semversion.Parse, "4.0.0"),
	}
	c.Check(v.Validate(), tc.ErrorIsNil)
}

// TestInvalidVersion tests that an invalid [Version] returns the expected
// [coreerrors.NotValid] error.
func (versionSuite) TestInvalidVersion(c *tc.C) {
	c.Run("Number", func(t *testing.T) {
		v := Version{
			Architecture: RISCV64,
			Number:       semversion.Zero,
		}
		c.Check(v.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("Architecture", func(t *testing.T) {
		v := Version{
			Architecture: Architecture(-1),
			Number:       tc.Must1(c, semversion.Parse, "4.0.0"),
		}
		c.Check(v.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("ArchitectureAndNumber", func(t *testing.T) {
		v := Version{
			Architecture: Architecture(-1),
			Number:       semversion.Zero,
		}
		c.Check(v.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})
}
