// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type restrictNewerClientSuite struct {
	testing.BaseSuite

	callerVersion version.Number
}

var _ = gc.Suite(&restrictNewerClientSuite{})

func (r *restrictNewerClientSuite) SetUpTest(c *gc.C) {
	r.BaseSuite.SetUpTest(c)
	// Patch to a big version so we avoid the whitelisted compatible
	// versions by default.
	r.PatchValue(&jujuversion.Current, version.MustParse("666.1.0"))
	r.callerVersion = jujuversion.Current
}

func (r *restrictNewerClientSuite) TestOldClientAllowedMethods(c *gc.C) {
	r.callerVersion.Major = jujuversion.Current.Major - 1
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.callerVersion)
	checkAllowed := func(facade, method string, version int) {
		caller, err := root.FindMethod(facade, version, method)
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, gc.NotNil)
	}
	checkAllowed("Client", "FullStatus", clientFacadeVersion)
	checkAllowed("Pinger", "Ping", pingerFacadeVersion)
	// Worker calls for migrations.
	checkAllowed("MigrationTarget", "Prechecks", 1)
	checkAllowed("UserManager", "UserInfo", userManagerFacadeVersion)
}

func (r *restrictNewerClientSuite) TestRecentClientAllowedAll(c *gc.C) {
	r.PatchValue(&jujuversion.Current, version.MustParse("3.0.0"))
	r.callerVersion = jujuversion.Current
	r.callerVersion.Major--
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.callerVersion)
	checkAllowed := func(facade, method string, version int) {
		caller, err := root.FindMethod(facade, version, method)
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, gc.NotNil)
	}
	checkAllowed("ModelManager", "CreateModel", modelManagerFacadeVersion)
}

func (r *restrictNewerClientSuite) TestRecentNewerClientAllowedMethods(c *gc.C) {
	r.assertNewerClientAllowedMethods(c, 0, true)
	r.assertNewerClientAllowedMethods(c, 1, true)
}

func (r *restrictNewerClientSuite) assertNewerClientAllowedMethods(c *gc.C, minor int, allowed bool) {
	r.callerVersion.Major = jujuversion.Current.Major + 1
	r.callerVersion.Minor = minor
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.callerVersion)
	checkAllowed := func(facade, method string, version int) {
		caller, err := root.FindMethod(facade, version, method)
		if allowed {
			c.Check(err, jc.ErrorIsNil)
			c.Check(caller, gc.NotNil)
		} else {
			c.Check(err, gc.NotNil)
			c.Check(caller, gc.IsNil)
		}
	}
	checkAllowed("Client", "FullStatus", clientFacadeVersion)
	checkAllowed("Pinger", "Ping", pingerFacadeVersion)
	// For migrations.
	checkAllowed("MigrationTarget", "Prechecks", 1)
	checkAllowed("UserManager", "UserInfo", userManagerFacadeVersion)
	// For upgrades.
	checkAllowed("ModelUpgrader", "UpgradeModel", 1)
}

func (r *restrictNewerClientSuite) TestOldClientUpgradeMethodDisallowed(c *gc.C) {
	r.callerVersion.Major = jujuversion.Current.Major - 1
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.callerVersion)
	caller, err := root.FindMethod("ModelUpgrader", 1, "UpgradeModel")
	c.Assert(errors.HasType[*params.IncompatibleClientError](err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictNewerClientSuite) TestReallyOldClientDisallowedMethod(c *gc.C) {
	r.callerVersion.Major = jujuversion.Current.Major - 2
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.callerVersion)
	caller, err := root.FindMethod("Client", 3, "FullStatus")
	c.Assert(errors.HasType[*params.IncompatibleClientError](err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictNewerClientSuite) TestReallyNewClientDisallowedMethod(c *gc.C) {
	r.callerVersion.Major = jujuversion.Current.Major + 2
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.callerVersion)
	caller, err := root.FindMethod("Client", 3, "FullStatus")
	c.Assert(errors.HasType[*params.IncompatibleClientError](err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictNewerClientSuite) TestAlwaysDisallowedMethod(c *gc.C) {
	r.callerVersion.Major = jujuversion.Current.Major - 1
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.callerVersion)
	caller, err := root.FindMethod("ModelConfig", 3, "ModelSet")
	c.Assert(errors.HasType[*params.IncompatibleClientError](err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictNewerClientSuite) TestAllowedListedClient(c *gc.C) {
	// Ensure we're allowed to migrate from 3.1.x min release to 4.0.0.
	r.assertAllowedListedClient(c, "2.9.0", "4.0.0", false)
	r.assertAllowedListedClient(c, "3.0.0", "4.0.0", true)
	r.assertAllowedListedClient(c, "3.1.0", "4.0.0", true)
	r.assertAllowedListedClient(c, "3.1.0", "4.0.9", true)
	r.assertAllowedListedClient(c, "3.1.0", "4.1.0", true)
}

func (r *restrictNewerClientSuite) assertAllowedListedClient(c *gc.C, callerVers, serverVers string, allowed bool) {
	r.PatchValue(&jujuversion.Current, version.MustParse(serverVers))
	r.callerVersion = version.MustParse(callerVers)
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.callerVersion)
	caller, err := root.FindMethod("ModelConfig", 3, "ModelSet")
	if allowed {
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, gc.NotNil)
	} else {
		c.Check(err, gc.NotNil)
		c.Check(caller, gc.IsNil)
	}
}

func (r *restrictNewerClientSuite) TestAgentMethod(c *gc.C) {
	// Ensure we're allowed to migrate from 3.1.x min release to 4.0.0.
	r.assertAgentMethod(c, "3.0.0", "4.0.0", false)
	r.assertAgentMethod(c, "3.1.0", "4.0.0", true)
	r.assertAgentMethod(c, "3.1.0", "4.0.9", true)
	r.assertAgentMethod(c, "3.1.0", "4.1.0", true)
}

func (r *restrictNewerClientSuite) assertAgentMethod(c *gc.C, agentVers, serverVers string, allowed bool) {
	r.PatchValue(&jujuversion.Current, version.MustParse(serverVers))
	r.callerVersion = version.MustParse(agentVers)
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(false, r.callerVersion)
	caller, err := root.FindMethod("Uniter", 18, "CurrentModel")
	if allowed {
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, gc.NotNil)
	} else {
		c.Check(err, gc.NotNil)
		c.Check(caller, gc.IsNil)
	}
}
