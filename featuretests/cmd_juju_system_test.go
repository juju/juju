// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"strings"

	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/environmentmanager"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type cmdSystemSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdSystemSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.JES)
	s.JujuConnSuite.SetUpTest(c)
}

func (s *cmdSystemSuite) run(c *gc.C, args ...string) *cmd.Context {
	command := system.NewSuperCommand()
	context, err := testing.RunCommand(c, command, args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdSystemSuite) createEnv(c *gc.C, envname string, isServer bool) {
	conn, err := juju.NewAPIState(s.AdminUserTag(c), s.Environ, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { conn.Close() })
	envManager := environmentmanager.NewClient(conn)
	_, err = envManager.CreateEnvironment(s.AdminUserTag(c).Id(), nil, map[string]interface{}{
		"name":            envname,
		"authorized-keys": "ssh-key",
		"state-server":    isServer,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cmdSystemSuite) TestSystemListCommand(c *gc.C) {
	context := s.run(c, "list")
	c.Assert(testing.Stdout(context), gc.Equals, "dummyenv\n")
}

func (s *cmdSystemSuite) TestSystemEnvironmentsCommand(c *gc.C) {
	c.Assert(envcmd.WriteCurrentSystem("dummyenv"), jc.ErrorIsNil)
	s.createEnv(c, "new-env", false)
	context := s.run(c, "environments")
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME      OWNER              LAST CONNECTION\n"+
		"dummyenv  dummy-admin@local  just now\n"+
		"new-env   dummy-admin@local  never connected\n"+
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

	s.run(c, "login", "--server", serverFilePath, "just-a-system")

	// Make sure that the saved server details are sufficient to connect
	// to the api server.
	api, err := juju.NewAPIFromName("just-a-system")
	c.Assert(err, jc.ErrorIsNil)
	api.Close()
}

func (s *cmdSystemSuite) TestCreateEnvironment(c *gc.C) {
	c.Assert(envcmd.WriteCurrentSystem("dummyenv"), jc.ErrorIsNil)
	// The JujuConnSuite doesn't set up an ssh key in the fake home dir,
	// so fake one on the command line.  The dummy provider also expects
	// a config value for 'state-server'.
	context := s.run(c, "create-environment", "new-env", "authorized-keys=fake-key", "state-server=false")
	c.Check(testing.Stdout(context), gc.Equals, "")
	c.Check(testing.Stderr(context), gc.Equals, `
created environment "new-env"
dummyenv (system) -> new-env
`[1:])

	// Make sure that the saved server details are sufficient to connect
	// to the api server.
	api, err := juju.NewAPIFromName("new-env")
	c.Assert(err, jc.ErrorIsNil)
	api.Close()
}

func (s *cmdSystemSuite) TestSystemDestroy(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name:        "just-a-system",
		ConfigAttrs: testing.Attrs{"state-server": true},
	})

	st.Close()
	s.run(c, "destroy", "dummyenv", "-y", "--destroy-all-environments")

	store, err := configstore.Default()
	_, err = store.ReadInfo("dummyenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cmdSystemSuite) TestRemoveBlocks(c *gc.C) {
	c.Assert(envcmd.WriteCurrentSystem("dummyenv"), jc.ErrorIsNil)
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	s.run(c, "remove-blocks")

	blocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *cmdSystemSuite) TestSystemKill(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "foo",
	})
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	st.Close()

	s.run(c, "kill", "dummyenv", "-y")

	store, err := configstore.Default()
	_, err = store.ReadInfo("dummyenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cmdSystemSuite) TestListBlocks(c *gc.C) {
	c.Assert(envcmd.WriteCurrentSystem("dummyenv"), jc.ErrorIsNil)
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	ctx := s.run(c, "list-blocks", "--format", "json")
	expected := fmt.Sprintf(`[{"name":"dummyenv","env-uuid":"%s","owner-tag":"%s","blocks":["BlockDestroy","BlockChange"]}]`,
		s.State.EnvironUUID(), s.AdminUserTag(c).String())

	strippedOut := strings.Replace(testing.Stdout(ctx), "\n", "", -1)
	c.Check(strippedOut, gc.Equals, expected)
}
