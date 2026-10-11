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

func (s *recoverySuite) TestCheckProviderFamily(c *tc.C) {
	info := &recovery.ArchiveInfo{CloudType: "lxd"}
	c.Check(info.CheckProviderFamily("lxd"), tc.ErrorIsNil)

	err := info.CheckProviderFamily("ec2")
	c.Assert(err, tc.ErrorMatches,
		"archive comes from a .lxd. controller but the bootstrap cloud is .ec2.*")
}
