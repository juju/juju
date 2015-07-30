// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"io/ioutil"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type UserCommandSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&UserCommandSuite{})

var expectedUserCommmandNames = []string{
	"add",
	"change-password",
	"credentials",
	"disable",
	"enable",
	"help",
	"info",
	"list",
}

func (s *UserCommandSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := testing.RunCommand(c, user.NewSuperCommand(), "--help")
	c.Assert(err, jc.ErrorIsNil)
	namesFound := testing.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, gc.DeepEquals, expectedUserCommmandNames)
}

type BaseSuite struct {
	testing.FakeJujuHomeSuite
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	memstore := configstore.NewMem()
	s.PatchValue(&configstore.Default, func() (configstore.Storage, error) {
		return memstore, nil
	})
	os.Setenv(osenv.JujuEnvEnvKey, "testing")
	info := memstore.CreateInfo("testing")
	info.SetBootstrapConfig(map[string]interface{}{"random": "extra data"})
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:   []string{"127.0.0.1:12345"},
		Hostnames:   []string{"localhost:12345"},
		CACert:      testing.CACert,
		EnvironUUID: "env-uuid",
	})
	info.SetAPICredentials(configstore.APICredentials{
		User:     "user-test",
		Password: "password",
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(user.ReadPassword, func() (string, error) {
		return "sekrit", nil
	})
	err = envcmd.WriteCurrentSystem("testing")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BaseSuite) assertServerFileMatches(c *gc.C, serverfile, username, password string) {
	yaml, err := ioutil.ReadFile(serverfile)
	c.Assert(err, jc.ErrorIsNil)
	var content envcmd.ServerFile
	err = goyaml.Unmarshal(yaml, &content)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(content.Username, gc.Equals, username)
	c.Assert(content.Password, gc.Equals, password)
	c.Assert(content.CACert, gc.Equals, testing.CACert)
	c.Assert(content.Addresses, jc.DeepEquals, []string{"127.0.0.1:12345"})
}
