// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/recovery"
)

func (s *recoverySuite) TestCheckAgentVersion(c *tc.C) {
	info := &recovery.ArchiveInfo{AgentVersion: semversion.MustParse("4.1.0")}
	c.Check(info.CheckAgentVersion(semversion.MustParse("4.1.0")), tc.ErrorIsNil)

	err := info.CheckAgentVersion(semversion.MustParse("4.1.1"))
	c.Assert(err, tc.ErrorMatches,
		"archive was created by agent version 4.1.0 but this binary is 4.1.1.*")
	err = info.CheckAgentVersion(semversion.MustParse("4.1-beta1"))
	c.Assert(err, tc.ErrorMatches,
		"archive was created by agent version 4.1.0 but this binary is 4.1-beta1.*")
}

// TestCheckAgentVersionBuildMismatch pins the build number as part of
// the gate: an archive recovers only onto the exact build it was taken
// from, not onto other packaging of the same version.
func (s *recoverySuite) TestCheckAgentVersionBuildMismatch(c *tc.C) {
	info := &recovery.ArchiveInfo{AgentVersion: semversion.MustParse("4.1-beta3.1")}
	c.Check(info.CheckAgentVersion(semversion.MustParse("4.1-beta3.1")), tc.ErrorIsNil)

	err := info.CheckAgentVersion(semversion.MustParse("4.1-beta3"))
	c.Assert(err, tc.ErrorMatches,
		"archive was created by agent version 4.1-beta3.1 but this binary is 4.1-beta3.*")

	info.AgentVersion = semversion.MustParse("4.1.0")
	err = info.CheckAgentVersion(semversion.MustParse("4.1.0.1"))
	c.Assert(err, tc.ErrorMatches,
		"archive was created by agent version 4.1.0 but this binary is 4.1.0.1.*")
}

// TestCheckAgentCompatibility pins the version-line gate: patch and build
// numbers may differ within one version line, but major.minor and the
// pre-release tag may not. The tag's number is part of the tag, so
// 4.1-beta3 and 4.1-beta4 are different lines.
func (s *recoverySuite) TestCheckAgentCompatibility(c *tc.C) {
	for _, test := range []struct {
		summary  string
		archived string
		current  string
	}{{
		summary:  "same release",
		archived: "4.1.0",
		current:  "4.1.0",
	}, {
		summary:  "release patch",
		archived: "4.1.0",
		current:  "4.1.7",
	}, {
		summary:  "release build",
		archived: "4.1.0.1",
		current:  "4.1.0",
	}, {
		summary:  "pre-release build",
		archived: "4.1-beta3.1",
		current:  "4.1-beta3",
	}} {
		c.Logf("%s", test.summary)
		info := &recovery.ArchiveInfo{AgentVersion: semversion.MustParse(test.archived)}
		c.Check(info.CheckAgentCompatibility(semversion.MustParse(test.current)), tc.ErrorIsNil)
	}

	for _, test := range []struct {
		summary  string
		archived string
		current  string
	}{{
		summary:  "different minor",
		archived: "4.1.0",
		current:  "4.2.0",
	}, {
		summary:  "different major",
		archived: "4.1.0",
		current:  "5.1.0",
	}, {
		summary:  "different pre-release tag",
		archived: "4.1-beta3",
		current:  "4.1-beta4",
	}, {
		summary:  "pre-release and release",
		archived: "4.1-beta3",
		current:  "4.1.0",
	}} {
		c.Logf("%s", test.summary)
		info := &recovery.ArchiveInfo{AgentVersion: semversion.MustParse(test.archived)}
		err := info.CheckAgentCompatibility(semversion.MustParse(test.current))
		c.Check(err, tc.ErrorMatches,
			"archive was created by agent version .* but this binary is .*; "+
				"recovery requires the same major.minor version line and pre-release tag")
	}
}

func (s *recoverySuite) TestCheckProviderFamily(c *tc.C) {
	info := &recovery.ArchiveInfo{CloudType: "lxd"}
	c.Check(info.CheckProviderFamily("lxd"), tc.ErrorIsNil)

	err := info.CheckProviderFamily("ec2")
	c.Assert(err, tc.ErrorMatches,
		"archive comes from a .lxd. controller but the bootstrap cloud is .ec2.*")
}
