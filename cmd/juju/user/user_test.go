// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"io/ioutil"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	memstore := configstore.NewMem()
	s.PatchValue(&configstore.Default, func() (configstore.Storage, error) {
		return memstore, nil
	})
	os.Setenv(osenv.JujuModelEnvKey, "testing")
	info := memstore.CreateInfo("testing")
	info.SetBootstrapConfig(map[string]interface{}{"random": "extra data"})
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses: []string{"127.0.0.1:12345"},
		Hostnames: []string{"localhost:12345"},
		CACert:    testing.CACert,
		ModelUUID: testing.ModelTag.Id(),
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
	err = modelcmd.WriteCurrentController("testing")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BaseSuite) assertServerFileMatches(c *gc.C, serverfile, username, password string) {
	yaml, err := ioutil.ReadFile(serverfile)
	c.Assert(err, jc.ErrorIsNil)
	var content modelcmd.ServerFile
	err = goyaml.Unmarshal(yaml, &content)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(content.Username, gc.Equals, username)
	c.Assert(content.Password, gc.Equals, password)
	c.Assert(content.CACert, gc.Equals, testing.CACert)
	c.Assert(content.Addresses, jc.DeepEquals, []string{"127.0.0.1:12345"})
}
