// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/environmentmanager"
	undertakerapi "github.com/juju/juju/api/undertaker"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/undertaker"
)

type cmdControllerSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdControllerSuite) run(c *gc.C, args ...string) *cmd.Context {
	context := testing.Context(c)
	command := commands.NewJujuCommand(context)
	c.Assert(testing.InitCommand(command, args), jc.ErrorIsNil)
	c.Assert(command.Run(context), jc.ErrorIsNil)
	return context
}

func (s *cmdControllerSuite) createEnv(c *gc.C, envname string, isServer bool) {
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

func (s *cmdControllerSuite) TestControllerListCommand(c *gc.C) {
	context := s.run(c, "list-controllers")
	c.Assert(testing.Stdout(context), gc.Equals, "dummyenv\n")
}

func (s *cmdControllerSuite) TestControllerEnvironmentsCommand(c *gc.C) {
	c.Assert(envcmd.WriteCurrentController("dummyenv"), jc.ErrorIsNil)
	s.createEnv(c, "new-env", false)
	context := s.run(c, "list-environments")
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME      OWNER              LAST CONNECTION\n"+
		"dummyenv  dummy-admin@local  just now\n"+
		"new-env   dummy-admin@local  never connected\n"+
		"\n")
}

func (s *cmdControllerSuite) TestControllerLoginCommand(c *gc.C) {
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

	s.run(c, "login", "--server", serverFilePath, "just-a-controller")

	// Make sure that the saved server details are sufficient to connect
	// to the api server.
	api, err := juju.NewAPIFromName("just-a-controller", nil)
	c.Assert(err, jc.ErrorIsNil)
	api.Close()
}

func (s *cmdControllerSuite) TestCreateEnvironment(c *gc.C) {
	c.Assert(envcmd.WriteCurrentController("dummyenv"), jc.ErrorIsNil)
	// The JujuConnSuite doesn't set up an ssh key in the fake home dir,
	// so fake one on the command line.  The dummy provider also expects
	// a config value for 'state-server'.
	context := s.run(c, "create-environment", "new-env", "authorized-keys=fake-key", "state-server=false")
	c.Check(testing.Stdout(context), gc.Equals, "")
	c.Check(testing.Stderr(context), gc.Equals, `
created environment "new-env"
dummyenv (controller) -> new-env
`[1:])

	// Make sure that the saved server details are sufficient to connect
	// to the api server.
	api, err := juju.NewAPIFromName("new-env", nil)
	c.Assert(err, jc.ErrorIsNil)
	api.Close()
}

func (s *cmdControllerSuite) TestControllerDestroy(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name:        "just-a-controller",
		ConfigAttrs: testing.Attrs{"state-server": true},
	})
	defer st.Close()

	stop := make(chan struct{})
	done := make(chan struct{})
	// In order for the destroy controller command to complete we need to run
	// the code that the cleaner and undertaker workers would be running in
	// the agent in order to progress the lifecycle of the hosted environment,
	// and cleanup the documents.
	go func() {
		defer close(done)
		a := testing.LongAttempt.Start()
		for a.Next() {
			err := s.State.Cleanup()
			c.Check(err, jc.ErrorIsNil)
			err = st.ProcessDyingEnviron()
			if errors.Cause(err) != state.ErrEnvironmentNotDying {
				c.Check(err, jc.ErrorIsNil)
				if err == nil {
					// success!
					return
				}
			}
			select {
			case <-stop:
				return
			default:
				// retry
			}
		}
	}()

	s.run(c, "destroy-controller", "dummyenv", "-y", "--destroy-all-environments", "--debug")
	close(stop)
	<-done

	store, err := configstore.Default()
	_, err = store.ReadInfo("dummyenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cmdControllerSuite) TestRemoveBlocks(c *gc.C) {
	c.Assert(envcmd.WriteCurrentController("dummyenv"), jc.ErrorIsNil)
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	s.run(c, "remove-all-blocks")

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *cmdControllerSuite) TestControllerKill(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "foo",
	})

	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	st.Close()

	s.run(c, "kill-controller", "dummyenv", "-y")

	store, err := configstore.Default()
	_, err = store.ReadInfo("dummyenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cmdControllerSuite) TestListBlocks(c *gc.C) {
	c.Assert(envcmd.WriteCurrentController("dummyenv"), jc.ErrorIsNil)
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	ctx := s.run(c, "list-all-blocks", "--format", "json")
	expected := fmt.Sprintf(`[{"name":"dummyenv","env-uuid":"%s","owner-tag":"%s","blocks":["BlockDestroy","BlockChange"]}]`,
		s.State.EnvironUUID(), s.AdminUserTag(c).String())

	strippedOut := strings.Replace(testing.Stdout(ctx), "\n", "", -1)
	c.Check(strippedOut, gc.Equals, expected)
}

func (s *cmdControllerSuite) TestSystemKillCallsEnvironDestroyOnHostedEnviron(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "foo",
	})
	defer st.Close()

	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	st.Close()

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	conn, err := juju.NewAPIState(s.AdminUserTag(c), s.Environ, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { conn.Close() })
	client := undertakerapi.NewClient(conn)

	startTime := time.Date(2015, time.September, 1, 17, 2, 1, 0, time.UTC)
	mClock := testing.NewClock(startTime)
	undertaker.NewUndertaker(client, mClock)

	store, err := configstore.Default()
	_, err = store.ReadInfo("dummyenv")
	c.Assert(err, jc.ErrorIsNil)

	s.run(c, "kill-controller", "dummyenv", "-y")

	// Ensure that Destroy was called on the hosted environment ...
	opRecvTimeout(c, st, opc, dummy.OpDestroy{})

	// ... and that the configstore was removed.
	_, err = store.ReadInfo("dummyenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

// opRecvTimeout waits for any of the given kinds of operation to
// be received from ops, and times out if not.
func opRecvTimeout(c *gc.C, st *state.State, opc <-chan dummy.Operation, kinds ...dummy.Operation) dummy.Operation {
	st.StartSync()
	for {
		select {
		case op := <-opc:
			for _, k := range kinds {
				if reflect.TypeOf(op) == reflect.TypeOf(k) {
					return op
				}
			}
			c.Logf("discarding unknown event %#v", op)
		case <-time.After(testing.LongWait):
			c.Fatalf("time out wating for operation")
		}
	}
}
