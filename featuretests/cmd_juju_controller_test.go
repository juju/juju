// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

type cmdControllerSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdControllerSuite) run(c *gc.C, args ...string) *cmd.Context {
	context := cmdtesting.Context(c)
	command := commands.NewJujuCommand(context, "")
	c.Assert(cmdtesting.InitCommand(command, args), jc.ErrorIsNil)
	c.Assert(command.Run(context), jc.ErrorIsNil)
	loggo.RemoveWriter("warning")
	return context
}

func (s *cmdControllerSuite) createModelAdminUser(c *gc.C, modelname string, isServer bool) base.ModelInfo {
	modelManager := modelmanager.NewClient(s.OpenControllerAPI(c))
	defer modelManager.Close()
	model, err := modelManager.CreateModel(
		modelname, s.AdminUserTag(c).Id(), "", "", names.CloudCredentialTag{}, map[string]interface{}{
			"controller": isServer,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return model
}

func (s *cmdControllerSuite) createModelNormalUser(c *gc.C, modelname string, isServer bool) {
	s.run(c, "add-user", "test")
	modelManager := modelmanager.NewClient(s.OpenControllerAPI(c))
	defer modelManager.Close()
	_, err := modelManager.CreateModel(
		modelname, names.NewLocalUserTag("test").Id(), "", "", names.CloudCredentialTag{}, map[string]interface{}{
			"authorized-keys": "ssh-key",
			"controller":      isServer,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cmdControllerSuite) TestControllerListCommand(c *gc.C) {
	context := s.run(c, "list-controllers")
	expectedOutput := `
Use --refresh option with this command to see the latest information.

Controller  Model       User   Access     Cloud/Region        Models  Nodes  HA  Version
kontroll*   controller  admin  superuser  dummy/dummy-region       1      -   -  (unknown)  

`[1:]
	c.Assert(cmdtesting.Stdout(context), gc.Equals, expectedOutput)
}

func (s *cmdControllerSuite) TestCreateModelAdminUser(c *gc.C) {
	s.createModelAdminUser(c, "new-model", false)
	context := s.run(c, "list-models")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: kontroll\n"+
		"\n"+
		"Model        Cloud/Region        Type   Status     Access  Last connection\n"+
		"controller*  dummy/dummy-region  dummy  available  admin   just now\n"+
		"new-model    dummy/dummy-region  dummy  available  admin   never connected\n"+
		"\n")
}

func (s *cmdControllerSuite) TestAddModelNormalUser(c *gc.C) {
	s.createModelNormalUser(c, "new-model", false)
	context := s.run(c, "list-models", "--all")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: kontroll\n"+
		"\n"+
		"Model           Cloud/Region        Type   Status     Access  Last connection\n"+
		"controller*     dummy/dummy-region  dummy  available  admin   just now\n"+
		"test/new-model  dummy/dummy-region  dummy  available  -       never connected\n"+
		"\n")
}

func (s *cmdControllerSuite) TestListModelsExactTimeFlag(c *gc.C) {
	s.createModelNormalUser(c, "new-model", false)
	context := s.run(c, "list-models", "--exact-time")
	c.Assert(cmdtesting.Stdout(context), gc.Matches, ""+
		"Controller: kontroll\n"+
		"\n"+
		"Model        Cloud/Region        Type   Status     Access  Last connection\n"+
		`controller\*  dummy/dummy-region  dummy  available  admin   20[0-9-]{8} [0-9:]{8} [-+][0-9]+ [A-Z]+\n`+
		"\n") // 2019-04-15 16:37:43 +1200 NZST
}

func (s *cmdControllerSuite) TestListModelsYAML(c *gc.C) {
	s.Factory.MakeMachine(c, nil)
	two := uint64(2)
	s.Factory.MakeMachine(c, &factory.MachineParams{Characteristics: &instance.HardwareCharacteristics{CpuCores: &two}})
	context := s.run(c, "list-models", "--format=yaml")
	expectedOutput := `
models:
- name: admin/controller
  short-name: controller
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: kontroll
  is-controller: true
  owner: admin
  cloud: dummy
  region: dummy-region
  credential:
    name: cred
    owner: admin
    cloud: dummy
  type: dummy
  life: alive
  status:
    current: available
    since: .*
  access: admin
  last-connection: just now
  sla-owner: admin
  agent-version: %v
current-model: controller
`[1:]
	c.Assert(cmdtesting.Stdout(context), gc.Matches, fmt.Sprintf(expectedOutput, version.Current))
}

func (s *cmdControllerSuite) TestShowModel(c *gc.C) {
	s.Factory.MakeMachine(c, nil)
	two := uint64(2)
	s.Factory.MakeMachine(c, &factory.MachineParams{Characteristics: &instance.HardwareCharacteristics{CpuCores: &two}})
	context := s.run(c, "show-model")
	expectedOutput := `
controller:
  name: admin/controller
  short-name: controller
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: kontroll
  is-controller: true
  owner: admin
  cloud: dummy
  region: dummy-region
  type: dummy
  life: alive
  status:
    current: available
    since: .*
  users:
    admin:
      display-name: admin
      access: admin
      last-connection: just now
  machines:
    "0":
      cores: 0
    "1":
      cores: 2
  sla: unsupported
  agent-version: %v
  credential:
    name: cred
    owner: admin
    cloud: dummy
    validity-check: valid
`[1:]
	c.Assert(cmdtesting.Stdout(context), gc.Matches, fmt.Sprintf(expectedOutput, version.Current))
}

func (s *cmdControllerSuite) TestListModelsYAMLWithExactTime(c *gc.C) {
	s.Factory.MakeMachine(c, nil)
	two := uint64(2)
	s.Factory.MakeMachine(c, &factory.MachineParams{Characteristics: &instance.HardwareCharacteristics{CpuCores: &two}})
	context := s.run(c, "list-models", "--exact-time", "--format=yaml")
	expectedOutput := `
models:
- name: admin/controller
  short-name: controller
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: kontroll
  is-controller: true
  owner: admin
  cloud: dummy
  region: dummy-region
  credential:
    name: cred
    owner: admin
    cloud: dummy
  type: dummy
  life: alive
  status:
    current: available
    since: 20[0-9-]{8} [0-9:]{8}[.0-9]* [-+][0-9]+ [A-Z]+
  access: admin
  last-connection: 20[0-9-]{8} [0-9:]{8} [-+][0-9]+ [A-Z]+
  sla-owner: admin
  agent-version: %v
current-model: controller
`[1:]
	c.Assert(cmdtesting.Stdout(context), gc.Matches, fmt.Sprintf(expectedOutput, version.Current))
}

func (s *cmdControllerSuite) TestListDeadModels(c *gc.C) {
	modelInfo := s.createModelAdminUser(c, "new-model", false)
	st, err := s.StatePool.Get(modelInfo.UUID)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = m.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Destroying,
		Message: "",
		Since:   &now,
	}
	err = m.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	// Dead models still show up in the list. It's a lie to pretend they
	// don't exist, and they will go away quickly.
	context := s.run(c, "list-models")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: kontroll\n"+
		"\n"+
		"Model        Cloud/Region        Type   Status      Access  Last connection\n"+
		"controller*  dummy/dummy-region  dummy  available   admin   just now\n"+
		"new-model    dummy/dummy-region  dummy  destroying  admin   never connected\n"+
		"\n")
}

func (s *cmdControllerSuite) TestAddModel(c *gc.C) {
	s.testAddModel(c)
}

func (s *cmdControllerSuite) TestAddModelWithCloudAndRegion(c *gc.C) {
	s.testAddModel(c, "dummy/dummy-region")
}

func (s *cmdControllerSuite) testAddModel(c *gc.C, args ...string) {
	// The JujuConnSuite doesn't set up an ssh key in the fake home dir,
	// so fake one on the command line.  The dummy provider also expects
	// a config value for 'controller'.
	args = append([]string{"add-model", "new-model"}, args...)
	args = append(args,
		"--config", "authorized-keys=fake-key",
		"--config", "controller=false",
	)
	context := s.run(c, args...)
	c.Check(cmdtesting.Stdout(context), gc.Equals, "")
	c.Check(cmdtesting.Stderr(context), jc.Contains, `
Added 'new-model' model on dummy/dummy-region with credential 'cred' for user 'admin'
`[1:])

	// Make sure that the saved server details are sufficient to connect
	// to the api server.
	accountDetails, err := s.ControllerStore.AccountDetails("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	modelDetails, err := s.ControllerStore.ModelByName("kontroll", "admin/new-model")
	c.Assert(err, jc.ErrorIsNil)
	api, err := juju.NewAPIConnection(juju.NewAPIConnectionParams{
		Store:          s.ControllerStore,
		ControllerName: "kontroll",
		AccountDetails: accountDetails,
		ModelUUID:      modelDetails.ModelUUID,
		DialOpts:       api.DefaultDialOpts(),
		OpenAPI:        api.Open,
	})
	c.Assert(err, jc.ErrorIsNil)
	api.Close()
}

func (s *cmdControllerSuite) TestControllerDestroy(c *gc.C) {
	s.testControllerDestroy(c, false)
}

func (s *cmdControllerSuite) TestControllerDestroyUsingAPI(c *gc.C) {
	s.testControllerDestroy(c, true)
}

func (s *cmdControllerSuite) testControllerDestroy(c *gc.C, forceAPI bool) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:        "just-a-hosted-model",
		ConfigAttrs: testing.Attrs{"controller": true},
		CloudRegion: "dummy-region",
	})
	defer st.Close()
	factory.NewFactory(st, s.StatePool).MakeApplication(c, nil)

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
			if errors.Cause(err) != state.ErrModelNotDying && !state.IsModelNotEmptyError(err) {
				c.Check(err, jc.ErrorIsNil)
				err2 := st.RemoveDyingModel()
				c.Check(err2, jc.ErrorIsNil)
				if err == nil && err2 == nil {
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

	if forceAPI {
		// Remove bootstrap config from the client store,
		// forcing the command to use the API.
		err := os.Remove(jujuclient.JujuBootstrapConfigPath())
		c.Assert(err, jc.ErrorIsNil)
	}

	ops := make(chan dummy.Operation, 1)
	dummy.Listen(ops)

	s.run(c, "destroy-controller", "kontroll", "-y", "--destroy-all-models", "--debug")
	close(stop)
	<-done

	destroyOp := (<-ops).(dummy.OpDestroy)
	c.Assert(destroyOp.Env, gc.Equals, "controller")
	c.Assert(destroyOp.Cloud, gc.Equals, "dummy")
	c.Assert(destroyOp.CloudRegion, gc.Equals, "dummy-region")

	store := jujuclient.NewFileClientStore()
	_, err := store.ControllerByName("kontroll")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cmdControllerSuite) TestEnableDestroyController(c *gc.C) {
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	s.run(c, "enable-destroy-controller")

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *cmdControllerSuite) TestControllerKill(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:        "foo",
		CloudRegion: "dummy-region",
	})

	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.Close()

	s.run(c, "kill-controller", "kontroll", "-y")

	store := jujuclient.NewFileClientStore()
	_, err := store.ControllerByName("kontroll")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
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

func (s *cmdControllerSuite) TestGetControllerConfigYAML(c *gc.C) {
	context := s.run(c, "controller-config", "--format=yaml")
	controllerCfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	cfgYaml, err := yaml.Marshal(controllerCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, string(cfgYaml))
}
