// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"slices"
	"testing"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/tc"
)

// typesSuite is responsible for testing the types provides by this package.
type typesSuite struct {
}

// TestTypesSuite runs all of the tests contained within [typesSuite].
func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

// TestAgentBinaryCompactOnVersion ensures that [AgentBinaryCompactOnVersion]
// works by removing duplicate versions in a slice of [AgentBinary]s.
func (_ *typesSuite) TestAgentBinaryCompactOnVersion(c *tc.C) {
	version1, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)
	version2, err := semversion.Parse("4.0-beta1")
	c.Assert(err, tc.ErrorIsNil)
	version3, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)

	agentBinaries := []AgentBinary{
		{
			Architecture: AMD64,
			Stream:       AgentStreamReleased,
			Version:      version1,
		},
		{
			Architecture: ARM64,
			Stream:       AgentStreamReleased,
			Version:      version1,
		},
		{
			Architecture: S390X,
			Stream:       AgentStreamProposed,
			Version:      version2,
		},
		{
			Architecture: PPC64EL,
			Stream:       AgentStreamTesting,
			Version:      version2,
		},
		{
			Architecture: AMD64,
			Stream:       AgentStreamTesting,
			Version:      version3,
		},
	}
	agentBinaries = slices.CompactFunc(agentBinaries, AgentBinaryCompactOnVersion)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_[_].Architecture", tc.Ignore)
	mc.AddExpr("_[_].Stream", tc.Ignore)
	c.Check(agentBinaries, mc, []AgentBinary{
		{Version: version1},
		{Version: version2},
		{Version: version3},
	})
}

// TestAgentBinaryHighestVersion ensures that [AgentBinaryHighestVersion]
// works by returning the highest version in a slice of [AgentBinary]s.
func (_ *typesSuite) TestAgentBinaryHighestVersion(c *tc.C) {
	version1, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)
	version2, err := semversion.Parse("4.0-beta1")
	c.Assert(err, tc.ErrorIsNil)
	version3, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)

	agentBinaries := []AgentBinary{
		{
			Architecture: AMD64,
			Stream:       AgentStreamReleased,
			Version:      version3,
		},
		{
			Architecture: AMD64,
			Stream:       AgentStreamReleased,
			Version:      version2,
		},
		{
			Architecture: AMD64,
			Stream:       AgentStreamReleased,
			Version:      version1,
		},
	}

	ver := slices.MaxFunc(agentBinaries, AgentBinaryHighestVersion)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_[_].Architecture", tc.Ignore)
	mc.AddExpr("_[_].Stream", tc.Ignore)
	c.Check(ver, mc, AgentBinary{Version: version3})
}

// TestAgentBinaryNotMatchinVersion tests the [AgentBinaryNotMatchingVersion]
// with [slices.Delete] to confirm that it removes all [AgentBinaries] that do
// match a target version.
func (*typesSuite) TestAgentBinaryNotMatchinVersion(c *tc.C) {
	versionToMatch, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	version1 := versionToMatch
	version2, err := semversion.Parse("4.0-beta1")
	c.Assert(err, tc.ErrorIsNil)
	version3, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)

	agentBinaries := []AgentBinary{
		{
			Architecture: AMD64,
			Stream:       AgentStreamReleased,
			Version:      version1,
		},
		{
			Architecture: ARM64,
			Stream:       AgentStreamProposed,
			Version:      version1,
		},
		{
			Architecture: S390X,
			Stream:       AgentStreamReleased,
			Version:      version2,
		},
		{
			Architecture: S390X,
			Stream:       AgentStreamReleased,
			Version:      version3,
		},
		{
			Architecture: ARM64,
			Stream:       AgentStreamReleased,
			Version:      version3,
		},
	}

	agentBinaries = slices.DeleteFunc(
		agentBinaries, AgentBinaryNotMatchingVersion(versionToMatch),
	)
	c.Check(agentBinaries, tc.SameContents, []AgentBinary{
		{
			Architecture: AMD64,
			Stream:       AgentStreamReleased,
			Version:      version1,
		},
		{
			Architecture: ARM64,
			Stream:       AgentStreamProposed,
			Version:      version1,
		},
	})
}

// TestArchitectureFromStringNotRecognised ensures that if the string
// architecture is not recognised then the returned Architecture is 0 with a
// false conversion value.
func (_ *typesSuite) TestArchitectureFromStringNotRecognised(c *tc.C) {
	arch, converted := ArchitectureFromString("fakearch")
	c.Check(converted, tc.IsFalse)
	c.Check(arch, tc.Equals, Architecture(0))
}

// TestArchitectureFromString ensures that [ArchitectureFromString] correctly
// converts known architecture strings to their corresponding [Architecture]
// values.
func (_ *typesSuite) TestArchitectureFromString(c *tc.C) {
	tests := []struct {
		A Architecture
		S string
	}{
		{A: AMD64, S: "amd64"},
		{A: ARM64, S: "arm64"},
		{A: S390X, S: "s390x"},
		{A: PPC64EL, S: "ppc64el"},
	}

	for _, test := range tests {
		c.Run(test.S, func(t *testing.T) {
			arch, converted := ArchitectureFromString(test.S)
			c.Check(converted, tc.IsTrue)
			c.Check(arch, tc.Equals, test.A)
		})
	}
}

// TestArchitectureToStringNotRecognised ensures that if the [Architecture]
// value is not recognised then the returned string is empty.
func (_ *typesSuite) TestArchitectureToStringNotRecognised(c *tc.C) {
	c.Check(Architecture(999).String(), tc.Equals, "")
}

// TestArchitectureToString ensures that [Architecture.String] correctly
// converts known [Architecture] values to their corresponding string.
func (*typesSuite) TestArchitectureToString(c *tc.C) {
	tests := []struct {
		A Architecture
		S string
	}{
		{A: AMD64, S: "amd64"},
		{A: ARM64, S: "arm64"},
		{A: S390X, S: "s390x"},
		{A: PPC64EL, S: "ppc64el"},
	}

	for _, test := range tests {
		c.Run(test.S, func(t *testing.T) {
			val := test.A.String()
			c.Check(val, tc.Equals, test.S)
		})
	}
}
