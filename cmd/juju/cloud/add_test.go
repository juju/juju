// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type addSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&addSuite{})

func (s *addSuite) SetUpTest(c *gc.C) {
	origHome := osenv.SetJujuXDGDataHome(c.MkDir())
	s.AddCleanup(func(*gc.C) { osenv.SetJujuXDGDataHome(origHome) })
}

func (s *addSuite) TestAddBadArgs(c *gc.C) {
	addCmd := cloud.NewAddCloudCommand()
	_, err := testing.RunCommand(c, addCmd)
	c.Assert(err, gc.ErrorMatches, "Usage: juju add-cloud <cloud-name> <cloud.yaml>")
	_, err = testing.RunCommand(c, addCmd, "cloud", "cloud.yaml", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *addSuite) createTestCloudData(c *gc.C) string {
	current := `
clouds:
  homestack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
`[1:]
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(current), 0600)
	c.Assert(err, jc.ErrorIsNil)

	sourceDir := c.MkDir()
	sourceFile := filepath.Join(sourceDir, "someclouds.yaml")
	source := `
clouds:
  homestack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
      new-york:
        endpoint: http://newyork/1.0
  garage-maas:
    type: mass
    auth-types: [oauth]
    endpoint: http://garagemaas
`[1:]
	err = ioutil.WriteFile(sourceFile, []byte(source), 0600)
	c.Assert(err, jc.ErrorIsNil)
	return sourceFile
}

func (s *addSuite) TestAddBadFilename(c *gc.C) {
	addCmd := cloud.NewAddCloudCommand()
	_, err := testing.RunCommand(c, addCmd, "cloud", "somefile.yaml")
	c.Assert(err, gc.ErrorMatches, "open somefile.yaml: .*")
}

func (s *addSuite) TestAddBadCloudName(c *gc.C) {
	sourceFile := s.createTestCloudData(c)
	addCmd := cloud.NewAddCloudCommand()
	_, err := testing.RunCommand(c, addCmd, "cloud", sourceFile)
	c.Assert(err, gc.ErrorMatches, `cloud "cloud" not found in file .*`)
}

func (s *addSuite) TestAddExisting(c *gc.C) {
	sourceFile := s.createTestCloudData(c)
	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(), "homestack", sourceFile)
	c.Assert(err, gc.ErrorMatches, `cloud called \"homestack\" already exists; use --replace to replace this existing cloud`)
}

func (s *addSuite) TestAddExistingReplace(c *gc.C) {
	sourceFile := s.createTestCloudData(c)
	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(), "homestack", sourceFile, "--replace")
	c.Assert(err, jc.ErrorIsNil)
	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("clouds.yaml"))
	c.Assert(string(data), gc.Equals, `
clouds:
  homestack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
      new-york:
        endpoint: http://newyork/1.0
`[1:])
}

func (s *addSuite) TestAddNew(c *gc.C) {
	sourceFile := s.createTestCloudData(c)
	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(), "garage-maas", sourceFile)
	c.Assert(err, jc.ErrorIsNil)
	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("clouds.yaml"))
	c.Assert(string(data), gc.Equals, `
clouds:
  garage-maas:
    type: mass
    auth-types: [oauth]
    endpoint: http://garagemaas
  homestack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
`[1:])
}
