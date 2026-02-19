// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"slices"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
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
			Architecture: AMD64,
			Stream:       AgentStreamDevel,
			Version:      version3,
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
	slices.SortFunc(agentBinaries, AgentBinaryCompareOnVersion)
	agentBinaries = slices.CompactFunc(agentBinaries, AgentBinaryCompactOnVersion)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_[_].Architecture", tc.Ignore)
	mc.AddExpr("_[_].Stream", tc.Ignore)
	c.Check(agentBinaries, mc, []AgentBinary{
		{Version: version2},
		{Version: version1},
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

// TestAgentBinaryNotMatchingVersion tests the [AgentBinaryNotMatchingVersion]
// with [slices.Delete] to confirm that it removes all [AgentBinaries] that do
// match a target version.
func (*typesSuite) TestAgentBinaryNotMatchingVersion(c *tc.C) {
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

// TestAgentBinaryArchitectures ensures that [AgentBinaryArchitectures]
// correctly returns in order all of the architectures that are present in a
// slice of [AgentBinary]s.
func (*typesSuite) TestAgentBinaryArchitectures(c *tc.C) {
	agentBinaries := []AgentBinary{
		{Architecture: AMD64},
		{Architecture: ARM64},
		{Architecture: S390X},
		{Architecture: S390X},
		{Architecture: ARM64},
	}

	vals := slices.Collect(AgentBinaryArchitectures(agentBinaries))
	c.Check(vals, tc.DeepEquals, []Architecture{
		AMD64, ARM64, S390X, S390X, ARM64,
	})
}
