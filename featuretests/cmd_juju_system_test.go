// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/juju/testing/factory"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/environmentmanager"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

type cmdSystemSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdSystemSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.JES)
}

func (s *cmdSystemSuite) createEnv(c *gc.C, envname string, isServer bool) {
	conn, err := juju.NewAPIState(s.AdminUserTag(c), s.Environ, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { conn.Close() })
	s.SetFeatureFlags(feature.JES)
	envManager := environmentmanager.NewClient(conn)
	_, err = envManager.CreateEnvironment(s.AdminUserTag(c).Id(), nil, map[string]interface{}{
		"name":            envname,
		"authorized-keys": "ssh-key",
		"state-server":    isServer,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cmdSystemSuite) TestSystemListCommand(c *gc.C) {
	context, err := testing.RunCommand(c, &system.ListCommand{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "dummyenv\n")
}

func (s *cmdSystemSuite) TestSystemEnvironmentsCommand(c *gc.C) {
	s.createEnv(c, "new-env", false)
	envcmd.WriteCurrentSystem("dummyenv")
	context, err := testing.RunCommand(c, envcmd.WrapSystem(&system.EnvironmentsCommand{}))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME      OWNER                   LAST CONNECTION\n"+
		"dummyenv  user-dummy-admin@local  just now\n"+
		"new-env   user-dummy-admin@local  never connected\n"+
		"\n")
}

func (s *cmdSystemSuite) TestSystemLoginCommand(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		NoEnvUser: true,
		Password:  "super-secret",
	})
	apiInfo := s.APIInfo(c)
	serverFile := envcmd.ServerFile{
		Addresses: apiInfo.Addrs,
		CACert:    apiInfo.CACert,
		Username:  user.Name(),
		Password:  "super-secret",
	}
	serverFilePath := filepath.Join(c.MkDir(), "server.yaml")
	content, err := goyaml.Marshal(serverFile)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(serverFilePath, []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = testing.RunCommand(c, &system.LoginCommand{}, "--server", serverFilePath, "--new-password", "just-a-system")
	c.Assert(err, jc.ErrorIsNil)
}
