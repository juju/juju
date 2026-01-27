// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
)

// providerTypeSuite is a test suite for asserting the contracts of [ProviderType].
type providerTypeSuite struct{}

// TestProviderTypeSuite runs the tests contained within [providerTypeSuite].
func TestProviderTypeSuite(t *testing.T) {
	tc.Run(t, providerTypeSuite{})
}

// TestIsValid checks using a set of valid provider type values that
// [ProviderType.IsValid] reports true for each of them.
func (providerTypeSuite) TestIsValid(c *tc.C) {
	validProviderTypes := []string{
		"a",
		"A",
		"aaaA",
		"a12-12",
		"multi--hypen",
		"a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z",
	}

	for _, validProviderType := range validProviderTypes {
		c.Run(validProviderType, func(c *testing.T) {
			p := ProviderType(validProviderType)
			tc.Check(c, p.IsValid(), tc.IsTrue)
		})
	}
}

// TestIsNotValid checks using a set of invalid provider type values that
// [ProviderType.IsValid] reports false for each of them.
func (providerTypeSuite) TestIsNotValid(c *tc.C) {
	invalidProviderTypes := []string{
		"1a",  // Cannot start with a digit
		"-aa", // cannot start with a hyphen
		"",    // cannot be empty
		// cannot be greater than 63 characters long
		"a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6fewffaeffewweffwwf",
		"Mix日edÆ", // cannot have non-ASCII characters
	}

	for _, invalidProviderType := range invalidProviderTypes {
		c.Run(invalidProviderType, func(c *testing.T) {
			p := ProviderType(invalidProviderType)
			tc.Check(c, p.IsValid(), tc.IsFalse)
		})
	}
}
