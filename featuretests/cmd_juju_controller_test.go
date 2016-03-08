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
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/modelmanager"
	undertakerapi "github.com/juju/juju/api/undertaker"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/modelcmd"
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
	modelManager := modelmanager.NewClient(s.APIState)
	_, err := modelManager.CreateModel(s.AdminUserTag(c).Id(), nil, map[string]interface{}{
		"name":            envname,
		"authorized-keys": "ssh-key",
		"controller":      isServer,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cmdControllerSuite) TestControllerListCommand(c *gc.C) {
	context := s.run(c, "list-controllers")
	expectedOutput := `
CONTROLLER   MODEL       USER         SERVER
dummymodel*  dummymodel  admin@local  

`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expectedOutput)
}

func (s *cmdControllerSuite) TestControllerModelsCommand(c *gc.C) {
	c.Assert(modelcmd.WriteCurrentController("dummymodel"), jc.ErrorIsNil)
	s.createEnv(c, "new-model", false)
	context := s.run(c, "list-models")
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME        OWNER        LAST CONNECTION\n"+
		"dummymodel  admin@local  just now\n"+
		"new-model   admin@local  never connected\n"+
		"\n")
}

func (s *cmdControllerSuite) TestCreateModel(c *gc.C) {
	c.Assert(modelcmd.WriteCurrentController("dummymodel"), jc.ErrorIsNil)
	// The JujuConnSuite doesn't set up an ssh key in the fake home dir,
	// so fake one on the command line.  The dummy provider also expects
	// a config value for 'controller'.
	context := s.run(c, "create-model", "new-model", "authorized-keys=fake-key", "controller=false")
	c.Check(testing.Stdout(context), gc.Equals, "")
	c.Check(testing.Stderr(context), gc.Equals, "created model \"new-model\"\n")

	// Make sure that the saved server details are sufficient to connect
	// to the api server.
	api, err := juju.NewAPIConnection(s.ControllerStore, "dummymodel", "admin@local", "new-model", nil)
	c.Assert(err, jc.ErrorIsNil)
	api.Close()
}

func (s *cmdControllerSuite) TestControllerDestroy(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:        "just-a-controller",
		ConfigAttrs: testing.Attrs{"controller": true},
	})
	defer st.Close()

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

	s.run(c, "destroy-controller", "dummymodel", "-y", "--destroy-all-models", "--debug")
	close(stop)
	<-done

	store, err := configstore.Default()
	_, err = store.ReadInfo("dummymodel")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cmdControllerSuite) TestRemoveBlocks(c *gc.C) {
	c.Assert(modelcmd.WriteCurrentController("dummymodel"), jc.ErrorIsNil)
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

	s.run(c, "kill-controller", "dummymodel", "-y")

	store, err := configstore.Default()
	_, err = store.ReadInfo("dummymodel")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cmdControllerSuite) TestListBlocks(c *gc.C) {
	c.Assert(modelcmd.WriteCurrentController("dummymodel"), jc.ErrorIsNil)
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	ctx := s.run(c, "list-all-blocks", "--format", "json")
	expected := fmt.Sprintf(`[{"name":"dummymodel","model-uuid":"%s","owner-tag":"%s","blocks":["BlockDestroy","BlockChange"]}]`,
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

	client := undertakerapi.NewClient(s.APIState)

	startTime := time.Date(2015, time.September, 1, 17, 2, 1, 0, time.UTC)
	mClock := testing.NewClock(startTime)
	undertaker.NewUndertaker(client, mClock)

	store, err := configstore.Default()
	_, err = store.ReadInfo("dummymodel:dummymodel")
	c.Assert(err, jc.ErrorIsNil)

	s.run(c, "kill-controller", "dummymodel", "-y")

	// Ensure that Destroy was called on the hosted model ...
	opRecvTimeout(c, st, opc, dummy.OpDestroy{})

	// ... and that the configstore was removed.
	_, err = store.ReadInfo("dummymodel")
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
