// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v27 = version.MustParse("2.7.0")

type steps27Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps27Suite{})

func (s *steps27Suite) TestCreateControllerNodes(c *gc.C) {
	step := findStateStep(c, v27, `add controller node docs`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestAddSpaceIdToSpaceDocs(c *gc.C) {
	step := findStateStep(c, v27, `recreate spaces with IDs`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestChangeSubnetAZtoSlice(c *gc.C) {
	step := findStateStep(c, v27, `change subnet AvailabilityZone to AvailabilityZones`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestChangeSubnetSpaceNameToSpaceID(c *gc.C) {
	step := findStateStep(c, v27, `change subnet SpaceName to SpaceID`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestAddSubnetIdToSubnetDocs(c *gc.C) {
	step := findStateStep(c, v27, `recreate subnets with IDs`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestReplacePortsDocSubnetIDCIDR(c *gc.C) {
	step := findStateStep(c, v27, `replace portsDoc.SubnetID as a CIDR with an ID.`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestEnsureRelationApplicationSettings(c *gc.C) {
	step := findStateStep(c, v27, `ensure application settings exist for all relations`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestConvertAddressSpaceIDs(c *gc.C) {
	step := findStateStep(c, v27, `ensure stored addresses refer to space by ID, and remove old space name/provider ID`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestReplaceSpaceNameWithIDEndpointBindings(c *gc.C) {
	step := findStateStep(c, v27, `replace space name in endpointBindingDoc bindings with an space ID`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestRemoveControllerConfigMaxLogAgeAndSize(c *gc.C) {
	step := findStateStep(c, v27, `remove controller config for max-logs-age and max-logs-size if set`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestLogfilePermissions(c *gc.C) {
	// This test is to primarily test the walking of the log directory and the
	// calling of the SetOwnership call. Skipped on windows because this will
	// never be run there.
	if runtime.GOOS == "windows" {
		c.Skip("never running upgrade steps on windows")
	}

	// Set up a test directory, with a few files, and a directory within with a few
	// files. We aren't actually able to test that the files do get changed ownership
	// to "syslog:adm" because normal users running these tests don't have permission
	// to do that. What we are testing here is the path that is passed into the Chown
	// command.

	var calls []string
	s.PatchValue(&paths.Chown, func(name string, uid, gid int) error {
		calls = append(calls, name)
		return nil
	})

	base := c.MkDir()
	c.Assert(ioutil.WriteFile(filepath.Join(base, "one"), []byte("test"), 0600), jc.ErrorIsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(base, "two"), []byte("test"), 0600), jc.ErrorIsNil)
	c.Assert(os.Mkdir(filepath.Join(base, "nested"), 0755), jc.ErrorIsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(base, "nested", "three"), []byte("test"), 0600), jc.ErrorIsNil)

	err := upgrades.SetJujuFolderPermissionsToAdm(base)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(calls, jc.SameContents, []string{
		filepath.Join(base, "one"),
		filepath.Join(base, "two"),
		filepath.Join(base, "nested", "three"),
	})
}
