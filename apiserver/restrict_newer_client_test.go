// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type restrictNewerClientSuite struct {
	testing.BaseSuite

	olderVersion version.Number
}

var _ = gc.Suite(&restrictNewerClientSuite{})

func (r *restrictNewerClientSuite) SetUpTest(c *gc.C) {
	r.BaseSuite.SetUpTest(c)
	r.PatchValue(&jujuversion.Current, version.MustParse("3.0.0"))
	r.olderVersion = jujuversion.Current
	r.olderVersion.Major--
}

func (r *restrictNewerClientSuite) TestOldClientAllowedMethods(c *gc.C) {
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.olderVersion)
	checkAllowed := func(facade, method string, version int) {
		caller, err := root.FindMethod(facade, version, method)
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, gc.NotNil)
	}
	checkAllowed("Client", "FullStatus", 1)
	checkAllowed("Pinger", "Ping", 1)
	// Worker calls for migrations.
	checkAllowed("MigrationTarget", "Prechecks", 1)
	checkAllowed("UserManager", "UserInfo", 1)
}

func (r *restrictNewerClientSuite) TestNewClientAllowedMethods(c *gc.C) {
	r.olderVersion.Major = jujuversion.Current.Major + 1
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.olderVersion)
	checkAllowed := func(facade, method string, version int) {
		caller, err := root.FindMethod(facade, version, method)
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, gc.NotNil)
	}
	checkAllowed("Client", "FullStatus", 1)
	checkAllowed("Pinger", "Ping", 1)
	// For migrations.
	checkAllowed("MigrationTarget", "Prechecks", 1)
	checkAllowed("UserManager", "UserInfo", 1)
	// For upgrades.
	checkAllowed("Client", "SetModelAgentVersion", 1)
}

func (r *restrictNewerClientSuite) TestOldClientDisallowedMethod(c *gc.C) {
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.olderVersion)
	caller, err := root.FindMethod("Client", 1, "SetModelAgentVersion")
	c.Assert(err, jc.Satisfies, params.IsIncompatibleClientError)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictNewerClientSuite) TestReallyOldClientDisallowedMethod(c *gc.C) {
	r.olderVersion.Major--
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.olderVersion)
	caller, err := root.FindMethod("Client", 1, "FullStatus")
	c.Assert(err, jc.Satisfies, params.IsIncompatibleClientError)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictNewerClientSuite) TestReallyNewClientDisallowedMethod(c *gc.C) {
	r.olderVersion.Major = jujuversion.Current.Major + 2
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.olderVersion)
	caller, err := root.FindMethod("Client", 1, "FullStatus")
	c.Assert(err, jc.Satisfies, params.IsIncompatibleClientError)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictNewerClientSuite) TestAlwaysDisallowedMethod(c *gc.C) {
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.olderVersion)
	caller, err := root.FindMethod("Client", 1, "ModelSet")
	c.Assert(err, jc.Satisfies, params.IsIncompatibleClientError)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictNewerClientSuite) TestAgentAllowedMethod(c *gc.C) {
	r.olderVersion.Major = 2
	r.olderVersion.Minor = 9
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(false, r.olderVersion)
	checkAllowed := func(facade, method string, version int) {
		caller, err := root.FindMethod(facade, version, method)
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, gc.NotNil)
	}
	checkAllowed("Uniter", "CurrentModel", 17)
}

func (r *restrictNewerClientSuite) TestReallyOldAgentDisallowedMethod(c *gc.C) {
	r.olderVersion.Minor = 0
	root := apiserver.TestingUpgradeOrMigrationOnlyRoot(true, r.olderVersion)
	caller, err := root.FindMethod("Uniter", 15, "CurrentModel")
	c.Assert(err, jc.Satisfies, params.IsIncompatibleClientError)
	c.Assert(caller, gc.IsNil)
}
