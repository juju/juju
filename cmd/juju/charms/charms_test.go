// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/charms"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type CharmsCommandSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&CharmsCommandSuite{})

var expectedCharmsCommmandNames = []string{
	"help",
	"list",
}

func (s *CharmsCommandSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := testing.RunCommand(c, charms.NewSuperCommand(), "--help")
	c.Assert(err, jc.ErrorIsNil)
	namesFound := testing.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, gc.DeepEquals, expectedCharmsCommmandNames)
}

type BaseSuite struct {
	testing.BaseSuite
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
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
}
