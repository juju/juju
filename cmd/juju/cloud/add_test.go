// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	"github.com/pkg/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type addSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&addSuite{})

func (s *addSuite) TestAddBadArgs(c *gc.C) {
	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(), "cloud", "cloud.yaml", "extra")
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
  aws:
    type: ec2
    auth-types: [ access-key ]
    regions:
      us-east-1:
        endpoint: https://us-east-1.aws.amazon.com/v1.2/
  localhost:
    type: lxd
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
	c.Assert(err, gc.ErrorMatches, `"homestack" already exists; use --replace to override this definition`)
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

func (s *addSuite) TestAddExistingPublic(c *gc.C) {
	sourceFile := s.createTestCloudData(c)
	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(), "aws", sourceFile)
	c.Assert(err, gc.ErrorMatches, `"aws" is the name of a public cloud; use --replace to override this definition`)
}

func (s *addSuite) TestAddExistingBuiltin(c *gc.C) {
	sourceFile := s.createTestCloudData(c)
	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(), "localhost", sourceFile)
	c.Assert(err, gc.ErrorMatches, `"localhost" is the name of a built-in cloud; use --replace to override this definition`)
}

func (s *addSuite) TestAddExistingPublicReplace(c *gc.C) {
	sourceFile := s.createTestCloudData(c)
	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(), "aws", sourceFile, "--replace")
	c.Assert(err, jc.ErrorIsNil)
	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("clouds.yaml"))
	c.Assert(string(data), gc.Equals, `
clouds:
  aws:
    type: ec2
    auth-types: [access-key]
    regions:
      us-east-1:
        endpoint: https://us-east-1.aws.amazon.com/v1.2/
  homestack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
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

func (s *addSuite) TestInteractive(c *gc.C) {
	s.createTestCloudData(c)
	command := cloud.NewAddCloudCommand()
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	out := &bytes.Buffer{}

	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: ioutil.Discard,
		Stdin:  &bytes.Buffer{},
	}
	err = command.Run(ctx)
	c.Check(errors.Cause(err), gc.Equals, io.EOF)

	c.Assert(out.String(), gc.Equals, `
Cloud Types
  maas
  manual
  openstack
  vsphere

Select cloud type: 
`[1:])
}

func (s *addSuite) TestInteractiveOpenstack(c *gc.C) {
	s.createTestCloudData(c)
	command := cloud.NewAddCloudCommand()
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader(`
openstack
os1
http://myopenstack
userpass,access-key
regionone
http://boston/1.0
n
`[1:])}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

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
  os1:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://myopenstack
    regions:
      regionone:
        endpoint: http://boston/1.0
`[1:])
}

func (s *addSuite) TestInteractiveMaas(c *gc.C) {
	s.createTestCloudData(c)
	command := cloud.NewAddCloudCommand()
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader(`
maas
m1
http://mymaas
`[1:])}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

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
  m1:
    type: maas
    auth-types: [oauth1]
    endpoint: http://mymaas
`[1:])
}

func (s *addSuite) TestInteractiveManual(c *gc.C) {
	s.createTestCloudData(c)
	command := cloud.NewAddCloudCommand()
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader(`
manual
man
192.168.1.6
`[1:])}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

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
  man:
    type: manual
    endpoint: 192.168.1.6
`[1:])
}

func (s *addSuite) TestInteractiveVSphere(c *gc.C) {
	s.createTestCloudData(c)
	command := cloud.NewAddCloudCommand()
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader(`
vsphere
mvs
192.168.1.6
foo
y
bar
n
`[1:])}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("clouds.yaml"))

	// Yes, the conversion copies the endpoint to each region, for some reason.
	c.Assert(string(data), gc.Equals, `
clouds:
  homestack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
  mvs:
    type: vsphere
    auth-types: [userpass]
    endpoint: 192.168.1.6
    regions:
      bar:
        endpoint: 192.168.1.6
      foo:
        endpoint: 192.168.1.6
`[1:])
}

func (s *addSuite) TestInteractiveExistingNameOverride(c *gc.C) {
	s.createTestCloudData(c)
	command := cloud.NewAddCloudCommand()
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader(`
manual
homestack
y
192.168.1.6
`[1:])}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("clouds.yaml"))
	c.Assert(string(data), gc.Equals, `
clouds:
  homestack:
    type: manual
    endpoint: 192.168.1.6
`[1:])
}

func (s *addSuite) TestInteractiveExistingNameNoOverride(c *gc.C) {
	s.createTestCloudData(c)
	command := cloud.NewAddCloudCommand()
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader(`
manual
homestack
n
homestack2
192.168.1.6
`[1:])}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

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
  homestack2:
    type: manual
    endpoint: 192.168.1.6
`[1:])
}
