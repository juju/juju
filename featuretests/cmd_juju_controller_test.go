// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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

func (s *cmdControllerSuite) createModelAdminUser(c *gc.C, modelname string, isServer bool) {
	modelManager := modelmanager.NewClient(s.APIState)
	_, err := modelManager.CreateModel(s.AdminUserTag(c).Id(), nil, map[string]interface{}{
		"name":       modelname,
		"controller": isServer,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cmdControllerSuite) createModelNormalUser(c *gc.C, modelname string, isServer bool) {
	s.run(c, "add-user", "test")
	modelManager := modelmanager.NewClient(s.APIState)
	_, err := modelManager.CreateModel(names.NewLocalUserTag("test").Id(), nil, map[string]interface{}{
		"name":            modelname,
		"authorized-keys": "ssh-key",
		"controller":      isServer,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cmdControllerSuite) TestControllerListCommand(c *gc.C) {
	context := s.run(c, "list-controllers")
	expectedOutput := fmt.Sprintf(`
CONTROLLER  MODEL  USER         SERVER
kontroll*   admin  admin@local  %s

`[1:], s.APIState.Addr())
	c.Assert(testing.Stdout(context), gc.Equals, expectedOutput)
}

func (s *cmdControllerSuite) TestCreateModelAdminUser(c *gc.C) {
	s.createModelAdminUser(c, "new-model", false)
	context := s.run(c, "list-models")
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME       OWNER        LAST CONNECTION\n"+
		"admin*     admin@local  just now\n"+
		"new-model  admin@local  never connected\n"+
		"\n")
}

func (s *cmdControllerSuite) TestCreateModelNormalUser(c *gc.C) {
	s.createModelAdminUser(c, "new-model", false)
	context := s.run(c, "list-models")
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME       OWNER        LAST CONNECTION\n"+
		"admin*     admin@local  just now\n"+
		"new-model  admin@local  never connected\n"+
		"\n")
}

func (s *cmdControllerSuite) TestCreateModel(c *gc.C) {
	// The JujuConnSuite doesn't set up an ssh key in the fake home dir,
	// so fake one on the command line.  The dummy provider also expects
	// a config value for 'controller'.
	context := s.run(c, "create-model", "new-model", "authorized-keys=fake-key", "controller=false")
	c.Check(testing.Stdout(context), gc.Equals, "")
	c.Check(testing.Stderr(context), gc.Equals, "created model \"new-model\"\n")

	// Make sure that the saved server details are sufficient to connect
	// to the api server.
	accountDetails, err := s.ControllerStore.AccountByName("kontroll", "admin@local")
	c.Assert(err, jc.ErrorIsNil)
	modelDetails, err := s.ControllerStore.ModelByName("kontroll", "admin@local", "new-model")
	c.Assert(err, jc.ErrorIsNil)
	api, err := juju.NewAPIConnection(juju.NewAPIConnectionParams{
		Store:           s.ControllerStore,
		ControllerName:  "kontroll",
		AccountDetails:  accountDetails,
		ModelUUID:       modelDetails.ModelUUID,
		BootstrapConfig: noBootstrapConfig,
		DialOpts:        api.DefaultDialOpts(),
	})
	c.Assert(err, jc.ErrorIsNil)
	api.Close()
}

func (s *cmdControllerSuite) TestControllerDestroy(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:        "just-a-controller",
		ConfigAttrs: testing.Attrs{"controller": true},
	})
	defer st.Close()
	factory.NewFactory(st).MakeService(c, nil)

	stop := make(chan struct{})
	done := make(chan struct{})
	// In order for the destroy controller command to complete we need to run
	// the code that the cleaner and undertaker workers would be running in
	// the agent in order to progress the lifecycle of the hosted model,
	// and cleanup the documents.
	go func() {
		defer close(done)
		a := testing.LongAttempt.Start()
		for a.Next() {
			err := s.State.Cleanup()
			c.Check(err, jc.ErrorIsNil)
			err = st.Cleanup()
			c.Check(err, jc.ErrorIsNil)
			err = st.ProcessDyingModel()
			if errors.Cause(err) != state.ErrModelNotDying {
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

	s.run(c, "destroy-controller", "kontroll", "-y", "--destroy-all-models", "--debug")
	close(stop)
	<-done

	store := jujuclient.NewFileClientStore()
	_, err := store.ControllerByName("kontroll")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cmdControllerSuite) TestRemoveBlocks(c *gc.C) {
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	s.run(c, "remove-all-blocks")

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *cmdControllerSuite) TestControllerKill(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "foo",
	})

	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.Close()

	s.run(c, "kill-controller", "kontroll", "-y")

	store := jujuclient.NewFileClientStore()
	_, err := store.ControllerByName("kontroll")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cmdControllerSuite) TestListBlocks(c *gc.C) {
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	ctx := s.run(c, "list-all-blocks", "--format", "json")
	expected := fmt.Sprintf(`[{"name":"admin","model-uuid":"%s","owner-tag":"%s","blocks":["BlockDestroy","BlockChange"]}]`,
		s.State.ModelUUID(), s.AdminUserTag(c).String())

	strippedOut := strings.Replace(testing.Stdout(ctx), "\n", "", -1)
	c.Check(strippedOut, gc.Equals, expected)
}

func (s *cmdControllerSuite) TestSystemKillCallsEnvironDestroyOnHostedEnviron(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "foo",
	})
	defer st.Close()

	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.Close()

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	store := jujuclient.NewFileClientStore()
	_, err := store.ControllerByName("kontroll")
	c.Assert(err, jc.ErrorIsNil)

	s.run(c, "kill-controller", "kontroll", "-y")

	// Ensure that Destroy was called on the hosted environ ...
	// TODO(fwereade): how do we know it's the hosted environ?
	// what actual interactions made it ok to destroy any environ
	// here? (there used to be an undertaker that didn't work...)
	opRecvTimeout(c, st, opc, dummy.OpDestroy{})

	// ... and that the details were removed removed from
	// the client store.
	_, err = store.ControllerByName("kontroll")
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

func noBootstrapConfig(controllerName string) (*config.Config, error) {
	return nil, errors.NotFoundf("bootstrap config for controller %s", controllerName)
}
