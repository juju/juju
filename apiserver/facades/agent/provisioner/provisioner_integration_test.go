// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type provisionerSuite struct {
	testing.ApiServerSuite

	machines []*state.Machine

	authorizer     apiservertesting.FakeAuthorizer
	resources      *common.Resources
	provisioner    *provisioner.ProvisionerAPIV11
	serviceFactory servicefactory.ServiceFactory
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.setUpTest(c, false)
}

func (s *provisionerSuite) setUpTest(c *gc.C, withController bool) {
	if s.ApiServerSuite.ControllerModelConfigAttrs == nil {
		s.ApiServerSuite.ControllerModelConfigAttrs = make(map[string]any)
	}
	s.ApiServerSuite.ControllerConfigAttrs = map[string]any{
		controller.SystemSSHKeys: "testSystemSSH",
	}
	s.ApiServerSuite.ControllerModelConfigAttrs["image-stream"] = "daily"
	s.ApiServerSuite.SetUpTest(c)

	controllerServiceFactory := s.ControllerServiceFactory(c)
	err := controllerServiceFactory.Config().UpdateModelConfig(context.Background(),
		map[string]any{
			"image-stream": "daily",
		},
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Reset previous machines (if any) and create 3 machines
	// for the tests, plus an optional controller machine.
	s.machines = nil
	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	st := s.ControllerModel(c).State()

	if withController {
		controllerConfigService := controllerServiceFactory.ControllerConfig()
		controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
		c.Assert(err, jc.ErrorIsNil)

		s.machines = append(s.machines, testing.AddControllerMachine(c, st, controllerConfig))
	}

	s.serviceFactory = s.ControllerServiceFactory(c)

	for i := 0; i < 5; i++ {
		m, err := st.AddMachine(state.NoopInstancePrechecker{}, state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Check(err, jc.ErrorIsNil)
		_, err = s.serviceFactory.Machine().CreateMachine(context.Background(), machine.Name(m.Id()))
		c.Assert(err, jc.ErrorIsNil)
		s.machines = append(s.machines, m)
	}

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming we logged in as the controller.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Controller: true,
	}

	// Create the resource registry separately to track invocations to
	// Register, and to register the root for tools URLs.
	s.resources = common.NewResources()

	// Create a provisioner API for the machine.
	provisionerAPI, err := provisioner.NewProvisionerAPIV11(context.Background(), facadetest.ModelContext{
		Auth_:           s.authorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		ServiceFactory_: s.serviceFactory,
		Logger_:         loggertesting.WrapCheckLog(c),
		ControllerUUID_: coretesting.ControllerTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.provisioner = provisionerAPI
}

type withoutControllerSuite struct {
	provisionerSuite
}

var _ = gc.Suite(&withoutControllerSuite{})

func (s *withoutControllerSuite) SetUpTest(c *gc.C) {
	s.setUpTest(c, false)
}

func (s *withoutControllerSuite) TestProvisionerFailsWithNonMachineAgentNonManagerUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = true
	// Works with a controller, which is not a machine agent.
	st := s.ControllerModel(c).State()
	aProvisioner, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		ServiceFactory_: s.ControllerServiceFactory(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(aProvisioner, gc.NotNil)

	// But fails with neither a machine agent or a controller.
	anAuthorizer.Controller = false
	aProvisioner, err = provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		ServiceFactory_: s.ControllerServiceFactory(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, gc.NotNil)
	c.Assert(aProvisioner, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *withoutControllerSuite) TestSetPasswords(c *gc.C) {
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: s.machines[0].Tag().String(), Password: "xxx0-1234567890123457890"},
			{Tag: s.machines[1].Tag().String(), Password: "xxx1-1234567890123457890"},
			{Tag: s.machines[2].Tag().String(), Password: "xxx2-1234567890123457890"},
			{Tag: s.machines[3].Tag().String(), Password: "xxx3-1234567890123457890"},
			{Tag: s.machines[4].Tag().String(), Password: "xxx4-1234567890123457890"},
			{Tag: "machine-42", Password: "foo"},
			{Tag: "unit-foo-0", Password: "zzz"},
			{Tag: "application-bar", Password: "abc"},
		},
	}
	results, err := s.provisioner.SetPasswords(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: nil},
			{Error: nil},
			{Error: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes to both machines succeeded.
	for i, machine := range s.machines {
		c.Logf("trying %q password", machine.Tag())
		err = machine.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		changed := machine.PasswordValid(fmt.Sprintf("xxx%d-1234567890123457890", i))
		c.Assert(changed, jc.IsTrue)
	}
}

func (s *withoutControllerSuite) TestShortSetPasswords(c *gc.C) {
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: s.machines[1].Tag().String(), Password: "xxx1"},
		},
	}
	results, err := s.provisioner.SetPasswords(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"password is only 4 bytes long, and is not a valid Agent password")
}

func (s *withoutControllerSuite) TestLifeAsMachineAgent(c *gc.C) {
	// NOTE: This and the next call serve to test the two
	// different authorization schemes:
	// 1. Machine agents can access their own machine and
	// any container that has their own machine as parent;
	// 2. Controllers can access any machine without
	// a parent.
	// There's no need to repeat this test for each method,
	// because the authorization logic is common.

	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = s.machines[0].Tag()
	st := s.ControllerModel(c).State()
	aProvisioner, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		ServiceFactory_: s.ControllerServiceFactory(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(aProvisioner, gc.NotNil)

	// Make the machine dead before trying to add containers.
	err = s.machines[0].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Create some containers to work on.
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	var containers []*state.Machine
	for i := 0; i < 3; i++ {
		container, err := st.AddMachineInsideMachine(template, s.machines[0].Id(), instance.LXD)
		c.Check(err, jc.ErrorIsNil)
		containers = append(containers, container)
	}
	// Make one container dead.
	err = containers[1].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: containers[0].Tag().String()},
		{Tag: containers[1].Tag().String()},
		{Tag: containers[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := aProvisioner.Life(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "alive"},
			{Life: "dead"},
			{Life: "alive"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestLifeAsController(c *gc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machines[0].Life(), gc.Equals, state.Alive)
	c.Assert(s.machines[1].Life(), gc.Equals, state.Dead)
	c.Assert(s.machines[2].Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Life(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "alive"},
			{Life: "dead"},
			{Life: "alive"},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Remove the subordinate and make sure it's detected.
	err = s.machines[1].Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	result, err = s.provisioner.Life(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: s.machines[1].Tag().String()},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.NotFoundError("machine 1")},
		},
	})
}

func (s *withoutControllerSuite) TestRemove(c *gc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	s.assertLife(c, 0, state.Alive)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Remove(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: `cannot remove machine "machine-0": still alive`}},
			{Error: nil},
			{Error: &params.Error{Message: `cannot remove machine "machine-2": still alive`}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertLife(c, 0, state.Alive)
	err = s.machines[2].Refresh()
	c.Assert(err, jc.ErrorIsNil)
	s.assertLife(c, 2, state.Alive)
}

func (s *withoutControllerSuite) TestSetStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Stopped,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	}
	err = s.machines[2].SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: s.machines[0].Tag().String(), Status: status.Error.String(), Info: "not really",
				Data: map[string]any{"foo": "bar"}},
			{Tag: s.machines[1].Tag().String(), Status: status.Stopped.String(), Info: "foobar"},
			{Tag: s.machines[2].Tag().String(), Status: status.Started.String(), Info: "again"},
			{Tag: "machine-42", Status: status.Started.String(), Info: "blah"},
			{Tag: "unit-foo-0", Status: status.Stopped.String(), Info: "foobar"},
			{Tag: "application-bar", Status: status.Stopped.String(), Info: "foobar"},
		}}
	result, err := s.provisioner.SetStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertStatus(c, 0, status.Error, "not really", map[string]any{"foo": "bar"})
	s.assertStatus(c, 1, status.Stopped, "foobar", map[string]any{})
	s.assertStatus(c, 2, status.Started, "again", map[string]any{})
}

func (s *withoutControllerSuite) TestSetInstanceStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Provisioning,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Running,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: s.machines[0].Tag().String(), Status: status.Provisioning.String(), Info: "not really",
				Data: map[string]any{"foo": "bar"}},
			{Tag: s.machines[1].Tag().String(), Status: status.Running.String(), Info: "foobar"},
			{Tag: s.machines[2].Tag().String(), Status: status.ProvisioningError.String(), Info: "again"},
			{Tag: "machine-42", Status: status.Provisioning.String(), Info: "blah"},
			{Tag: "unit-foo-0", Status: status.Error.String(), Info: "foobar"},
			{Tag: "application-bar", Status: status.ProvisioningError.String(), Info: "foobar"},
		}}
	result, err := s.provisioner.SetInstanceStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertInstanceStatus(c, 0, status.Provisioning, "not really", map[string]any{"foo": "bar"})
	s.assertInstanceStatus(c, 1, status.Running, "foobar", map[string]any{})
	s.assertInstanceStatus(c, 2, status.ProvisioningError, "again", map[string]any{})
	// ProvisioningError also has a special case which is to set the machine to Error
	s.assertStatus(c, 2, status.Error, "again", map[string]any{})
}

func (s *withoutControllerSuite) TestSetModificationStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetModificationStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Applied,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetModificationStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	}
	err = s.machines[2].SetModificationStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: s.machines[0].Tag().String(), Status: status.Pending.String(), Info: "not really",
				Data: map[string]any{"foo": "bar"}},
			{Tag: s.machines[1].Tag().String(), Status: status.Applied.String(), Info: "foobar"},
			{Tag: s.machines[2].Tag().String(), Status: status.Error.String(), Info: "again"},
			{Tag: "machine-42", Status: status.Pending.String(), Info: "blah"},
			{Tag: "unit-foo-0", Status: status.Error.String(), Info: "foobar"},
			{Tag: "application-bar", Status: status.Error.String(), Info: "foobar"},
		}}
	result, err := s.provisioner.SetModificationStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertModificationStatus(c, 0, status.Pending, "not really", map[string]any{"foo": "bar"})
	s.assertModificationStatus(c, 1, status.Applied, "foobar", map[string]any{})
	s.assertModificationStatus(c, 2, status.Error, "again", map[string]any{})
}

func (s *withoutControllerSuite) TestMachinesWithTransientErrors(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Provisioning,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "transient error",
		Data:    map[string]any{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Data:    map[string]any{"transient": false},
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "error",
		Since:   &now,
	}
	err = s.machines[3].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	// Machine 4 is provisioned but error not reset yet.
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "transient error",
		Data:    map[string]any{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[4].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	hwChars := instance.MustParseHardware("arch=arm64", "mem=4G")
	err = s.machines[4].SetProvisioned("i-am", "", "fake_nonce", &hwChars)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.provisioner.MachinesWithTransientErrors(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Id: "1", Life: "alive", Status: "provisioning error", Info: "transient error",
				Data: map[string]any{"transient": true, "foo": "bar"}},
		},
	})
}

func (s *withoutControllerSuite) TestMachinesWithTransientErrorsPermission(c *gc.C) {
	// Machines where there's permission issues are omitted.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = names.NewMachineTag("1")
	aProvisioner, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		ServiceFactory_: s.ControllerServiceFactory(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	},
	)
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Running,
		Message: "blah",
		Since:   &now,
	}
	err = s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "transient error",
		Data:    map[string]any{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Data:    map[string]any{"transient": false},
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Since:   &now,
	}
	err = s.machines[3].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	result, err := aProvisioner.MachinesWithTransientErrors(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{{
			Id: "1", Life: "alive", Status: "provisioning error",
			Info: "transient error",
			Data: map[string]any{"transient": true, "foo": "bar"},
		},
		},
	})
}

func (s *withoutControllerSuite) TestEnsureDead(c *gc.C) {
	machineName0 := machine.Name(s.machines[0].Id())
	machineName1 := machine.Name(s.machines[1].Id())
	machineName2 := machine.Name(s.machines[2].Id())

	err := s.serviceFactory.Machine().SetMachineLife(context.Background(), machineName0, life.Alive)
	c.Assert(err, jc.ErrorIsNil)
	err = s.serviceFactory.Machine().SetMachineLife(context.Background(), machineName1, life.Dead)
	c.Assert(err, jc.ErrorIsNil)
	err = s.serviceFactory.Machine().SetMachineLife(context.Background(), machineName2, life.Alive)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.EnsureDead(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	obtainedLife, err := s.serviceFactory.Machine().GetMachineLife(context.Background(), machine.Name(s.machines[0].Id()))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*obtainedLife, gc.Equals, life.Dead)
	obtainedLife, err = s.serviceFactory.Machine().GetMachineLife(context.Background(), machine.Name(s.machines[1].Id()))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*obtainedLife, gc.Equals, life.Dead)
	obtainedLife, err = s.serviceFactory.Machine().GetMachineLife(context.Background(), machine.Name(s.machines[2].Id()))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*obtainedLife, gc.Equals, life.Dead)
}

func (s *withoutControllerSuite) assertLife(c *gc.C, index int, expectLife state.Life) {
	err := s.machines[index].Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machines[index].Life(), gc.Equals, expectLife)
}

func (s *withoutControllerSuite) assertStatus(c *gc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]any) {

	statusInfo, err := s.machines[index].Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, expectStatus)
	c.Assert(statusInfo.Message, gc.Equals, expectInfo)
	c.Assert(statusInfo.Data, gc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) assertInstanceStatus(c *gc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]any) {

	statusInfo, err := s.machines[index].InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, expectStatus)
	c.Assert(statusInfo.Message, gc.Equals, expectInfo)
	c.Assert(statusInfo.Data, gc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) assertModificationStatus(c *gc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]any) {

	statusInfo, err := s.machines[index].ModificationStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, expectStatus)
	c.Assert(statusInfo.Message, gc.Equals, expectInfo)
	c.Assert(statusInfo.Data, gc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) TestWatchContainers(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.WatchContainers{Params: []params.WatchContainer{
		{MachineTag: s.machines[0].Tag().String(), ContainerType: string(instance.LXD)},
		{MachineTag: s.machines[1].Tag().String(), ContainerType: string(instance.LXD)},
		{MachineTag: "machine-42", ContainerType: ""},
		{MachineTag: "unit-foo-0", ContainerType: ""},
		{MachineTag: "application-bar", ContainerType: ""},
	}}
	result, err := s.provisioner.WatchContainers(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{}},
			{StringsWatcherId: "2", Changes: []string{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 2)
	m0Watcher := s.resources.Get("1")
	defer workertest.CleanKill(c, m0Watcher)
	m1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, m1Watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc0 := statetesting.NewStringsWatcherC(c, m0Watcher.(state.StringsWatcher))
	wc0.AssertNoChange()
	wc1 := statetesting.NewStringsWatcherC(c, m1Watcher.(state.StringsWatcher))
	wc1.AssertNoChange()
}

func (s *withoutControllerSuite) TestWatchAllContainers(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.WatchContainers{Params: []params.WatchContainer{
		{MachineTag: s.machines[0].Tag().String()},
		{MachineTag: s.machines[1].Tag().String()},
		{MachineTag: "machine-42"},
		{MachineTag: "unit-foo-0"},
		{MachineTag: "application-bar"},
	}}
	result, err := s.provisioner.WatchAllContainers(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{}},
			{StringsWatcherId: "2", Changes: []string{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 2)
	m0Watcher := s.resources.Get("1")
	defer workertest.CleanKill(c, m0Watcher)
	m1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, m1Watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc0 := statetesting.NewStringsWatcherC(c, m0Watcher.(state.StringsWatcher))
	wc0.AssertNoChange()
	wc1 := statetesting.NewStringsWatcherC(c, m1Watcher.(state.StringsWatcher))
	wc1.AssertNoChange()
}

func (s *withoutControllerSuite) TestStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Stopped,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	}
	err = s.machines[2].SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Status(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	// Zero out the updated timestamps so we can easily check the results.
	for i, statusResult := range result.Results {
		r := statusResult
		if r.Status != "" {
			c.Assert(r.Since, gc.NotNil)
		}
		r.Since = nil
		result.Results[i] = r
	}
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Status: status.Started.String(), Info: "blah", Data: map[string]any{}},
			{Status: status.Stopped.String(), Info: "foo", Data: map[string]any{}},
			{Status: status.Error.String(), Info: "not really", Data: map[string]any{"foo": "bar"}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestInstanceStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Provisioning,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Running,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "not really",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.InstanceStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	// Zero out the updated timestamps so we can easily check the results.
	for i, statusResult := range result.Results {
		r := statusResult
		if r.Status != "" {
			c.Assert(r.Since, gc.NotNil)
		}
		r.Since = nil
		result.Results[i] = r
	}
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Status: status.Provisioning.String(), Info: "blah", Data: map[string]any{}},
			{Status: status.Running.String(), Info: "foo", Data: map[string]any{}},
			{Status: status.ProvisioningError.String(), Info: "not really", Data: map[string]any{"foo": "bar"}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestAvailabilityZone(c *gc.C) {
	availabilityZone := "ru-north-siberia"
	emptyAz := ""
	hcWithAZ := instance.HardwareCharacteristics{AvailabilityZone: &availabilityZone}
	hcWithEmptyAZ := instance.HardwareCharacteristics{AvailabilityZone: &emptyAz}
	hcWithNilAz := instance.HardwareCharacteristics{AvailabilityZone: nil}

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	// add machines with different availability zones: string, empty string, nil
	azMachine, _ := f.MakeMachineReturningPassword(c, &factory.MachineParams{
		Characteristics: &hcWithAZ,
	})

	emptyAzMachine, _ := f.MakeMachineReturningPassword(c, &factory.MachineParams{
		Characteristics: &hcWithEmptyAZ,
	})

	nilAzMachine, _ := f.MakeMachineReturningPassword(c, &factory.MachineParams{
		Characteristics: &hcWithNilAz,
	})
	args := params.Entities{Entities: []params.Entity{
		{Tag: azMachine.Tag().String()},
		{Tag: emptyAzMachine.Tag().String()},
		{Tag: nilAzMachine.Tag().String()},
	}}
	result, err := s.provisioner.AvailabilityZone(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: availabilityZone},
			{Result: emptyAz},
			{Result: emptyAz},
		},
	})
}

func (s *withoutControllerSuite) TestKeepInstance(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	// Add a machine with keep-instance = true.
	foobarMachine := f.MakeMachine(c, &factory.MachineParams{InstanceId: "1234"})
	err := foobarMachine.SetKeepInstance(true)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: foobarMachine.Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.KeepInstance(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: false},
			{Result: true},
			{Result: false},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroup(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	addUnits := func(name string, machines ...*state.Machine) (units []*state.Unit) {
		app := f.MakeApplication(c, &factory.ApplicationParams{
			Name:  name,
			Charm: f.MakeCharm(c, &factory.CharmParams{Name: name}),
		})
		for _, m := range machines {
			unit, err := app.AddUnit(state.AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			err = unit.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			units = append(units, unit)
		}
		return units
	}
	setProvisioned := func(id string) {
		m, err := s.ControllerModel(c).State().Machine(id)
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetProvisioned(instance.Id("machine-"+id+"-inst"), "", "nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	mysqlUnit := addUnits("mysql", s.machines[0], s.machines[3])[0]
	wordpressUnits := addUnits("wordpress", s.machines[0], s.machines[1], s.machines[2])

	// Unassign wordpress/1 from machine-1.
	// The unit should not show up in the results.
	err := wordpressUnits[1].UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)

	// Provision machines 1, 2 and 3. Machine-0 remains
	// unprovisioned, and machine-1 has no units, and so
	// neither will show up in the results.
	setProvisioned("1")
	setProvisioned("2")
	setProvisioned("3")

	// Add a few controllers, provision two of them.
	st := s.ControllerModel(c).State()
	_, _, err = st.EnableHA(state.NoopInstancePrechecker{}, 3, constraints.Value{}, state.UbuntuBase("12.10"), nil)
	c.Assert(err, jc.ErrorIsNil)
	setProvisioned("5")
	setProvisioned("7")

	// Create a logging service, subordinate to mysql.
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})
	eps, err := st.InferEndpoints("mysql", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: s.machines[3].Tag().String()},
		{Tag: "machine-5"},
	}}
	result, err := s.provisioner.DistributionGroup(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.DistributionGroupResults{
		Results: []params.DistributionGroupResult{
			{Result: []instance.Id{"machine-2-inst", "machine-3-inst"}},
			{Result: []instance.Id{}},
			{Result: []instance.Id{"machine-2-inst"}},
			{Result: []instance.Id{"machine-3-inst"}},
			{Result: []instance.Id{"machine-5-inst", "machine-7-inst"}},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroupControllerAuth(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.DistributionGroup(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.DistributionGroupResults{
		Results: []params.DistributionGroupResult{
			// controller may access any top-level machines.
			{Result: []instance.Id{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			// only a machine agent for the container or its
			// parent may access it.
			{Error: apiservertesting.ErrUnauthorized},
			// non-machines always unauthorized
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroupMachineAgentAuth(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	provisioner, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		ServiceFactory_: s.ControllerServiceFactory(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Check(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "machine-1-lxd-99"},
		{Tag: "machine-1-lxd-99-lxd-100"},
	}}
	result, err := provisioner.DistributionGroup(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.DistributionGroupResults{
		Results: []params.DistributionGroupResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: []instance.Id{}},
			{Error: apiservertesting.ErrUnauthorized},
			// only a machine agent for the container or its
			// parent may access it.
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError("machine 1/lxd/99")},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroupByMachineId(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	addUnits := func(name string, machines ...*state.Machine) (units []*state.Unit) {
		app := f.MakeApplication(c, &factory.ApplicationParams{
			Name:  name,
			Charm: f.MakeCharm(c, &factory.CharmParams{Name: name}),
		})
		for _, m := range machines {
			unit, err := app.AddUnit(state.AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			err = unit.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			units = append(units, unit)
		}
		return units
	}
	setProvisioned := func(id string) {
		m, err := s.ControllerModel(c).State().Machine(id)
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetProvisioned(instance.Id("machine-"+id+"-inst"), "", "nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	_ = addUnits("mysql", s.machines[0], s.machines[3])[0]
	wordpressUnits := addUnits("wordpress", s.machines[0], s.machines[1], s.machines[2])

	// Unassign wordpress/1 from machine-1.
	// The unit should not show up in the results.
	err := wordpressUnits[1].UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)

	// Provision machines 1, 2 and 3. Machine-0 remains
	// unprovisioned.
	setProvisioned("1")
	setProvisioned("2")
	setProvisioned("3")

	// Add a few controllers, provision two of them.
	_, _, err = s.ControllerModel(c).State().EnableHA(s.InstancePrechecker(c, s.ControllerModel(c).State()), 3, constraints.Value{}, state.UbuntuBase("12.10"), nil)
	c.Assert(err, jc.ErrorIsNil)
	setProvisioned("5")
	setProvisioned("7")

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: s.machines[3].Tag().String()},
		{Tag: "machine-5"},
	}}
	result, err := s.provisioner.DistributionGroupByMachineId(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{"2", "3"}},
			{Result: []string{}},
			{Result: []string{"0"}},
			{Result: []string{"0"}},
			{Result: []string{"6", "7"}},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroupByMachineIdControllerAuth(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.DistributionGroupByMachineId(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			// controller may access any top-level machines.
			{Result: []string{}, Error: nil},
			{Result: nil, Error: apiservertesting.NotFoundError("machine 42")},
			// only a machine agent for the container or its
			// parent may access it.
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
			// non-machines always unauthorized
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroupByMachineIdMachineAgentAuth(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	provisioner, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		ServiceFactory_: s.ControllerServiceFactory(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Check(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "machine-1-lxd-99"},
		{Tag: "machine-1-lxd-99-lxd-100"},
	}}
	result, err := provisioner.DistributionGroupByMachineId(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
			{Result: []string{}, Error: nil},
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
			// only a machine agent for the container or its
			// parent may access it.
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
			{Result: nil, Error: apiservertesting.NotFoundError("machine 1/lxd/99")},
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestConstraints(c *gc.C) {
	// Add a machine with some constraints.
	cons := constraints.MustParse("cores=123", "mem=8G")
	template := state.MachineTemplate{
		Base:        state.UbuntuBase("12.10"),
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
	}
	consMachine, err := s.ControllerModel(c).State().AddOneMachine(s.InstancePrechecker(c, s.ControllerModel(c).State()), template)
	c.Assert(err, jc.ErrorIsNil)

	machine0Constraints, err := s.machines[0].Constraints()
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: consMachine.Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Constraints(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ConstraintsResults{
		Results: []params.ConstraintsResult{
			{Constraints: machine0Constraints},
			{Constraints: template.Constraints},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestSetInstanceInfo(c *gc.C) {
	registry := storage.ChainedProviderRegistry{
		dummystorage.StorageProviders(),
		provider.CommonStorageProviders(),
	}
	serviceFactoryGetter := s.ServiceFactoryGetter(c)

	st := s.ControllerModel(c).State()
	storageService := serviceFactoryGetter.FactoryForModel(model.UUID(st.ModelUUID())).Storage(registry)
	err := storageService.CreateStoragePool(context.Background(), "static-pool", "static", map[string]any{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.ControllerModel(c).UpdateModelConfig(s.ConfigSchemaSourceGetter(c), map[string]any{
		"storage-default-block-source": "static-pool",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Provision machine 0 first.
	hwChars := instance.MustParseHardware("arch=arm64", "mem=4G")
	err = s.machines[0].SetInstanceInfo("i-am", "", "fake_nonce", &hwChars, nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	volumesMachine, err := st.AddOneMachine(s.InstancePrechecker(c, s.ControllerModel(c).State()), state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{Size: 1000},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	args := params.InstancesInfo{Machines: []params.InstanceInfo{{
		Tag:        s.machines[0].Tag().String(),
		InstanceId: "i-was",
		Nonce:      "fake_nonce",
	}, {
		Tag:             s.machines[1].Tag().String(),
		InstanceId:      "i-will",
		Nonce:           "fake_nonce",
		Characteristics: &hwChars,
	}, {
		Tag:             s.machines[2].Tag().String(),
		InstanceId:      "i-am-too",
		Nonce:           "fake",
		Characteristics: nil,
	}, {
		Tag:        volumesMachine.Tag().String(),
		InstanceId: "i-am-also",
		Nonce:      "fake",
		Volumes: []params.Volume{{
			VolumeTag: "volume-0",
			Info: params.VolumeInfo{
				VolumeId: "vol-0",
				Size:     1234,
			},
		}},
		VolumeAttachments: map[string]params.VolumeAttachmentInfo{
			"volume-0": {
				DeviceName: "sda",
			},
		},
	},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.SetInstanceInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{
				Message: `cannot record provisioning info for "i-was": cannot set instance data for machine "0": already set`,
			}},
			{Error: nil},
			{Error: nil},
			{Error: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify machine 1 and 2 were provisioned.
	c.Assert(s.machines[1].Refresh(), gc.IsNil)
	c.Assert(s.machines[2].Refresh(), gc.IsNil)

	instanceId, err := s.machines[1].InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instanceId, gc.Equals, instance.Id("i-will"))
	instanceId, err = s.machines[2].InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instanceId, gc.Equals, instance.Id("i-am-too"))
	c.Check(s.machines[1].CheckProvisioned("fake_nonce"), jc.IsTrue)
	c.Check(s.machines[2].CheckProvisioned("fake"), jc.IsTrue)
	gotHardware, err := s.machines[1].HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotHardware, gc.DeepEquals, &hwChars)

	// Verify the machine with requested volumes was provisioned, and the
	// volume information recorded in state.
	sb, err := state.NewStorageBackend(st)
	c.Assert(err, jc.ErrorIsNil)
	volumeAttachments, err := sb.MachineVolumeAttachments(volumesMachine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)
	volumeAttachmentInfo, err := volumeAttachments[0].Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachmentInfo, gc.Equals, state.VolumeAttachmentInfo{DeviceName: "sda"})
	volume, err := sb.Volume(volumeAttachments[0].Volume())
	c.Assert(err, jc.ErrorIsNil)
	volumeInfo, err := volume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeInfo, gc.Equals, state.VolumeInfo{VolumeId: "vol-0", Pool: "static-pool", Size: 1234})

	// Verify the machine without requested volumes still has no volume
	// attachments recorded in state.
	volumeAttachments, err = sb.MachineVolumeAttachments(s.machines[1].MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 0)
}

func (s *withoutControllerSuite) TestInstanceId(c *gc.C) {
	// Provision 2 machines first.
	err := s.machines[0].SetProvisioned("i-am", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	hwChars := instance.MustParseHardware("arch=arm64", "mem=4G")
	err = s.machines[1].SetProvisioned("i-am-not", "", "fake_nonce", &hwChars)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.InstanceId(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "i-am"},
			{Result: "i-am-not"},
			{Error: apiservertesting.NotProvisionedError("2")},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestWatchModelMachines(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	got, err := s.provisioner.WatchModelMachines(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	want := params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0", "1", "2", "3", "4"},
	}
	c.Assert(got.StringsWatcherId, gc.Equals, want.StringsWatcherId)
	c.Assert(got.Changes, jc.SameContents, want.Changes)

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Make sure WatchModelMachines fails with a machine agent login.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	aProvisioner, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		ServiceFactory_: s.ControllerServiceFactory(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := aProvisioner.WatchModelMachines(context.Background())
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{})
}

func (s *withoutControllerSuite) TestWatchMachineErrorRetry(c *gc.C) {
	s.PatchValue(&provisioner.ErrorRetryWaitDelay, 2*coretesting.ShortWait)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	_, err := s.provisioner.WatchMachineErrorRetry(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, resource.(state.NotifyWatcher))
	wc.AssertNoChange()

	// We should now get a time triggered change.
	wc.AssertOneChange()

	// Make sure WatchMachineErrorRetry fails with a machine agent login.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	aProvisioner, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		ServiceFactory_: s.ControllerServiceFactory(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := aProvisioner.WatchMachineErrorRetry(context.Background())
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{})
}

func (s *withoutControllerSuite) TestMarkMachinesForRemoval(c *gc.C) {
	err := s.machines[0].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines[2].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.provisioner.MarkMachinesForRemoval(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-2"},         // ok
			{Tag: "machine-100"},       // not found
			{Tag: "machine-0"},         // ok
			{Tag: "machine-1"},         // not dead
			{Tag: "machine-0-lxd-5"},   // unauthorised
			{Tag: "application-thing"}, // only machines allowed
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	results := res.Results
	c.Assert(results, gc.HasLen, 6)
	c.Check(results[0].Error, gc.IsNil)
	c.Check(*results[1].Error, jc.DeepEquals,
		*apiservererrors.ServerError(errors.NotFoundf("machine 100")))
	c.Check(*results[1].Error, jc.Satisfies, params.IsCodeNotFound)
	c.Check(results[2].Error, gc.IsNil)
	c.Check(*results[3].Error, jc.DeepEquals,
		*apiservererrors.ServerError(errors.New("cannot remove machine 1: machine is not dead")))
	c.Check(*results[4].Error, jc.DeepEquals, *apiservertesting.ErrUnauthorized)
	c.Check(*results[5].Error, jc.DeepEquals,
		*apiservererrors.ServerError(errors.New(`"application-thing" is not a valid machine tag`)))

	removals, err := s.ControllerModel(c).State().AllMachineRemovals()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(removals, jc.SameContents, []string{"0", "2"})
}

func (s *withoutControllerSuite) TestSetSupportedContainers(c *gc.C) {
	args := params.MachineContainersParams{Params: []params.MachineContainers{{
		MachineTag:     "machine-0",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}, {
		MachineTag:     "machine-1",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}}}
	results, err := s.provisioner.SetSupportedContainers(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, gc.IsNil)
	}
	st := s.ControllerModel(c).State()
	m0, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{instance.LXD})
	m1, err := st.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	containers, ok = m1.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{instance.LXD})
}

func (s *withoutControllerSuite) TestSetSupportedContainersPermissions(c *gc.C) {
	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		ServiceFactory_: s.ControllerServiceFactory(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(aProvisioner, gc.NotNil)

	args := params.MachineContainersParams{
		Params: []params.MachineContainers{{
			MachineTag:     "machine-0",
			ContainerTypes: []instance.ContainerType{instance.LXD},
		}, {
			MachineTag:     "machine-1",
			ContainerTypes: []instance.ContainerType{instance.LXD},
		}, {
			MachineTag:     "machine-42",
			ContainerTypes: []instance.ContainerType{instance.LXD},
		},
		},
	}
	// Only machine 0 can have it's containers updated.
	results, err := aProvisioner.SetSupportedContainers(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestSupportedContainers(c *gc.C) {
	setArgs := params.MachineContainersParams{Params: []params.MachineContainers{{
		MachineTag:     "machine-0",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}, {
		MachineTag:     "machine-1",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}}}
	_, err := s.provisioner.SetSupportedContainers(context.Background(), setArgs)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "machine-1",
	}}}
	results, err := s.provisioner.SupportedContainers(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, gc.IsNil)
	}
	st := s.ControllerModel(c).State()
	m0, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, results.Results[0].ContainerTypes)
	m1, err := st.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	containers, ok = m1.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, results.Results[1].ContainerTypes)
}

func (s *withoutControllerSuite) TestSupportedContainersWithoutBeingSet(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "machine-1",
	}}}
	results, err := s.provisioner.SupportedContainers(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, gc.IsNil)
		c.Assert(result.ContainerTypes, gc.HasLen, 0)
	}
}

func (s *withoutControllerSuite) TestSupportedContainersWithInvalidTag(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: "user-0",
	}}}
	results, err := s.provisioner.SupportedContainers(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	for _, result := range results.Results {
		c.Assert(result.Error, gc.ErrorMatches, "permission denied")
	}
}

func (s *withoutControllerSuite) TestSupportsNoContainers(c *gc.C) {
	args := params.MachineContainersParams{
		Params: []params.MachineContainers{
			{
				MachineTag: "machine-0",
			},
		},
	}
	results, err := s.provisioner.SetSupportedContainers(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	m0, err := s.ControllerModel(c).State().Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{})
}

var _ = gc.Suite(&withControllerSuite{})

type withControllerSuite struct {
	provisionerSuite
}

func (s *withControllerSuite) SetUpTest(c *gc.C) {
	s.provisionerSuite.setUpTest(c, true)
}

func (s *withControllerSuite) TestAPIAddresses(c *gc.C) {
	hostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}
	st := s.ControllerModel(c).State()

	controllerCfg := coretesting.FakeControllerConfig()

	err := st.SetAPIHostPorts(controllerCfg, hostPorts, hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.provisioner.APIAddresses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *withControllerSuite) TestCACert(c *gc.C) {
	result, err := s.provisioner.CACert(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.BytesResult{
		Result: []byte(coretesting.CACert),
	})
}
