// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type provisionerSuite struct {
	testing.ApiServerSuite

	machines []*state.Machine

	authorizer     apiservertesting.FakeAuthorizer
	resources      *common.Resources
	provisioner    *provisioner.ProvisionerAPIV11
	domainServices services.DomainServices
}

func TestProvisionerSuite(t *stdtesting.T) { tc.Run(t, &provisionerSuite{}) }
func (s *provisionerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:

 - Test the distribution group is correctly distributed
 - Test the distribution group is correctly distributed are grouped by machine ids
	`)
}

func (s *provisionerSuite) SetUpTest(c *tc.C) {
	s.setUpTest(c, false)
}

func (s *provisionerSuite) setUpTest(c *tc.C, withController bool) {
	if s.ApiServerSuite.ControllerModelConfigAttrs == nil {
		s.ApiServerSuite.ControllerModelConfigAttrs = make(map[string]any)
	}
	s.ApiServerSuite.ControllerConfigAttrs = map[string]any{
		controller.SystemSSHKeys: "testSystemSSH",
	}
	s.ApiServerSuite.ControllerModelConfigAttrs["image-stream"] = "daily"
	s.ApiServerSuite.SetUpTest(c)

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerModelConfigService := controllerDomainServices.Config()
	err := controllerModelConfigService.UpdateModelConfig(c.Context(),
		map[string]any{
			"image-stream": "daily",
		},
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Reset previous machines (if any) and create 3 machines
	// for the tests, plus an optional controller machine.
	s.machines = nil
	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	st := s.ControllerModel(c).State()

	if withController {
		controllerConfigService := controllerDomainServices.ControllerConfig()
		controllerConfig, err := controllerConfigService.ControllerConfig(c.Context())
		c.Assert(err, tc.ErrorIsNil)

		s.machines = append(s.machines, testing.AddControllerMachine(c, st, controllerConfig))
	}

	s.domainServices = s.ControllerDomainServices(c)

	for i := 0; i < 5; i++ {
		m, err := st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Check(err, tc.ErrorIsNil)
		_, err = s.domainServices.Machine().CreateMachine(c.Context(), coremachine.Name(m.Id()))
		c.Assert(err, tc.ErrorIsNil)
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
	provisionerAPI, err := provisioner.NewProvisionerAPIV11(c.Context(), facadetest.ModelContext{
		Auth_:           s.authorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.domainServices,
		Logger_:         loggertesting.WrapCheckLog(c),
		ControllerUUID_: coretesting.ControllerTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	s.provisioner = provisionerAPI
}

type withoutControllerSuite struct {
	provisionerSuite
}

func TestWithoutControllerSuite(t *stdtesting.T) { tc.Run(t, &withoutControllerSuite{}) }
func (s *withoutControllerSuite) SetUpTest(c *tc.C) {
	s.setUpTest(c, false)
}

func (s *withoutControllerSuite) TestProvisionerFailsWithNonMachineAgentNonManagerUser(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = true
	// Works with a controller, which is not a machine agent.
	st := s.ControllerModel(c).State()
	aProvisioner, err := provisioner.MakeProvisionerAPI(c.Context(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(aProvisioner, tc.NotNil)

	// But fails with neither a machine agent or a controller.
	anAuthorizer.Controller = false
	aProvisioner, err = provisioner.MakeProvisionerAPI(c.Context(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.NotNil)
	c.Assert(aProvisioner, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *withoutControllerSuite) TestSetPasswords(c *tc.C) {
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
	results, err := s.provisioner.SetPasswords(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
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
		c.Assert(err, tc.ErrorIsNil)
		changed := machine.PasswordValid(fmt.Sprintf("xxx%d-1234567890123457890", i))
		c.Assert(changed, tc.IsTrue)
	}
}

func (s *withoutControllerSuite) TestShortSetPasswords(c *tc.C) {
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: s.machines[1].Tag().String(), Password: "xxx1"},
		},
	}
	results, err := s.provisioner.SetPasswords(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches,
		"password is only 4 bytes long, and is not a valid Agent password")
}

func (s *withoutControllerSuite) TestLifeAsMachineAgent(c *tc.C) {
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
	aProvisioner, err := provisioner.MakeProvisionerAPI(c.Context(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(aProvisioner, tc.NotNil)

	// Make the machine dead before trying to add containers.
	err = s.machines[0].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	// Create some containers to work on.
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	var containers []*state.Machine
	for i := 0; i < 3; i++ {
		container, err := st.AddMachineInsideMachine(template, s.machines[0].Id(), instance.LXD)
		c.Check(err, tc.ErrorIsNil)
		containers = append(containers, container)
	}
	// Make one container dead.
	err = containers[1].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

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
	result, err := aProvisioner.Life(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
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

func (s *withoutControllerSuite) TestLifeAsController(c *tc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machines[0].Life(), tc.Equals, state.Alive)
	c.Assert(s.machines[1].Life(), tc.Equals, state.Dead)
	c.Assert(s.machines[2].Life(), tc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Life(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
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
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	result, err = s.provisioner.Life(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: s.machines[1].Tag().String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.NotFoundError("machine 1")},
		},
	})
}

func (s *withoutControllerSuite) TestRemove(c *tc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
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
	result, err := s.provisioner.Remove(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
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
	c.Assert(err, tc.ErrorIsNil)
	s.assertLife(c, 2, state.Alive)
}

func (s *withoutControllerSuite) TestSetStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Stopped,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	}
	err = s.machines[2].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

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
	result, err := s.provisioner.SetStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
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

func (s *withoutControllerSuite) TestSetInstanceStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Provisioning,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Running,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

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
	result, err := s.provisioner.SetInstanceStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
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

func (s *withoutControllerSuite) TestSetModificationStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetModificationStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Applied,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetModificationStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	}
	err = s.machines[2].SetModificationStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

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
	result, err := s.provisioner.SetModificationStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
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

func (s *withoutControllerSuite) TestMachinesWithTransientErrors(c *tc.C) {
	svc := s.ControllerDomainServices(c)
	machineService := svc.Machine()

	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Provisioning,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "transient error",
		Data:    map[string]any{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Data:    map[string]any{"transient": false},
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "error",
		Since:   &now,
	}
	err = s.machines[3].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	// Machine 4 is provisioned but error not reset yet.
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "transient error",
		Data:    map[string]any{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[4].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	hwChars := instance.MustParseHardware("arch=arm64", "mem=4G")
	machine4UUID, err := machineService.GetMachineUUID(c.Context(), coremachine.Name(s.machines[4].Id()))
	c.Assert(err, tc.ErrorIsNil)
	err = machineService.SetMachineCloudInstance(c.Context(), machine4UUID, "i-am", "", &hwChars)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.provisioner.MachinesWithTransientErrors(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Id: "1", Life: "alive", Status: "provisioning error", Info: "transient error",
				Data: map[string]any{"transient": true, "foo": "bar"}},
		},
	})
}

func (s *withoutControllerSuite) TestMachinesWithTransientErrorsPermission(c *tc.C) {
	// Machines where there's permission issues are omitted.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = names.NewMachineTag("1")
	aProvisioner, err := provisioner.MakeProvisionerAPI(c.Context(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	},
	)
	c.Assert(err, tc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Running,
		Message: "blah",
		Since:   &now,
	}
	err = s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "transient error",
		Data:    map[string]any{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Data:    map[string]any{"transient": false},
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Since:   &now,
	}
	err = s.machines[3].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	result, err := aProvisioner.MachinesWithTransientErrors(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{{
			Id: "1", Life: "alive", Status: "provisioning error",
			Info: "transient error",
			Data: map[string]any{"transient": true, "foo": "bar"},
		},
		},
	})
}

func (s *withoutControllerSuite) TestEnsureDead(c *tc.C) {
	machineName0 := coremachine.Name(s.machines[0].Id())
	machineName1 := coremachine.Name(s.machines[1].Id())
	machineName2 := coremachine.Name(s.machines[2].Id())

	err := s.domainServices.Machine().SetMachineLife(c.Context(), machineName0, life.Alive)
	c.Assert(err, tc.ErrorIsNil)
	err = s.domainServices.Machine().SetMachineLife(c.Context(), machineName1, life.Dead)
	c.Assert(err, tc.ErrorIsNil)
	err = s.domainServices.Machine().SetMachineLife(c.Context(), machineName2, life.Alive)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.EnsureDead(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
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
	obtainedLife, err := s.domainServices.Machine().GetMachineLife(c.Context(), coremachine.Name(s.machines[0].Id()))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*obtainedLife, tc.Equals, life.Dead)
	obtainedLife, err = s.domainServices.Machine().GetMachineLife(c.Context(), coremachine.Name(s.machines[1].Id()))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*obtainedLife, tc.Equals, life.Dead)
	obtainedLife, err = s.domainServices.Machine().GetMachineLife(c.Context(), coremachine.Name(s.machines[2].Id()))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*obtainedLife, tc.Equals, life.Dead)
}

func (s *withoutControllerSuite) assertLife(c *tc.C, index int, expectLife state.Life) {
	err := s.machines[index].Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machines[index].Life(), tc.Equals, expectLife)
}

func (s *withoutControllerSuite) assertStatus(c *tc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]any) {

	statusInfo, err := s.machines[index].Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, expectStatus)
	c.Assert(statusInfo.Message, tc.Equals, expectInfo)
	c.Assert(statusInfo.Data, tc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) assertInstanceStatus(c *tc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]any) {

	statusInfo, err := s.machines[index].InstanceStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, expectStatus)
	c.Assert(statusInfo.Message, tc.Equals, expectInfo)
	c.Assert(statusInfo.Data, tc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) assertModificationStatus(c *tc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]any) {

	statusInfo, err := s.machines[index].ModificationStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, expectStatus)
	c.Assert(statusInfo.Message, tc.Equals, expectInfo)
	c.Assert(statusInfo.Data, tc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) TestWatchContainers(c *tc.C) {
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.WatchContainers{Params: []params.WatchContainer{
		{MachineTag: s.machines[0].Tag().String(), ContainerType: string(instance.LXD)},
		{MachineTag: s.machines[1].Tag().String(), ContainerType: string(instance.LXD)},
		{MachineTag: "machine-42", ContainerType: ""},
		{MachineTag: "unit-foo-0", ContainerType: ""},
		{MachineTag: "application-bar", ContainerType: ""},
	}}
	result, err := s.provisioner.WatchContainers(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{}},
			{StringsWatcherId: "2", Changes: []string{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), tc.Equals, 2)
	m0Watcher := s.resources.Get("1")
	defer workertest.CleanKill(c, m0Watcher)
	m1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, m1Watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc0 := watchertest.NewStringsWatcherC(c, m0Watcher.(state.StringsWatcher))
	wc0.AssertNoChange()
	wc1 := watchertest.NewStringsWatcherC(c, m1Watcher.(state.StringsWatcher))
	wc1.AssertNoChange()
}

func (s *withoutControllerSuite) TestWatchAllContainers(c *tc.C) {
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.WatchContainers{Params: []params.WatchContainer{
		{MachineTag: s.machines[0].Tag().String()},
		{MachineTag: s.machines[1].Tag().String()},
		{MachineTag: "machine-42"},
		{MachineTag: "unit-foo-0"},
		{MachineTag: "application-bar"},
	}}
	result, err := s.provisioner.WatchAllContainers(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{}},
			{StringsWatcherId: "2", Changes: []string{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), tc.Equals, 2)
	m0Watcher := s.resources.Get("1")
	defer workertest.CleanKill(c, m0Watcher)
	m1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, m1Watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc0 := watchertest.NewStringsWatcherC(c, m0Watcher.(state.StringsWatcher))
	wc0.AssertNoChange()
	wc1 := watchertest.NewStringsWatcherC(c, m1Watcher.(state.StringsWatcher))
	wc1.AssertNoChange()
}

func (s *withoutControllerSuite) TestStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Stopped,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	}
	err = s.machines[2].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Status(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	// Zero out the updated timestamps so we can easily check the results.
	for i, statusResult := range result.Results {
		r := statusResult
		if r.Status != "" {
			c.Assert(r.Since, tc.NotNil)
		}
		r.Since = nil
		result.Results[i] = r
	}
	c.Assert(result, tc.DeepEquals, params.StatusResults{
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

func (s *withoutControllerSuite) TestInstanceStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Provisioning,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Running,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "not really",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.InstanceStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	// Zero out the updated timestamps so we can easily check the results.
	for i, statusResult := range result.Results {
		r := statusResult
		if r.Status != "" {
			c.Assert(r.Since, tc.NotNil)
		}
		r.Since = nil
		result.Results[i] = r
	}
	c.Assert(result, tc.DeepEquals, params.StatusResults{
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

func (s *withoutControllerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Calling AvailabilityZones with machines that have populated, empty, and nil AZ
- Calling KeepInstance with variaety of results - true, false, not found, unauthorised.
`)
}

func (s *withoutControllerSuite) TestDistributionGroupControllerAuth(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.DistributionGroup(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.DistributionGroupResults{
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

func (s *withoutControllerSuite) TestDistributionGroupMachineAgentAuth(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	provisioner, err := provisioner.MakeProvisionerAPI(c.Context(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Check(err, tc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "machine-1-lxd-99"},
		{Tag: "machine-1-lxd-99-lxd-100"},
	}}
	result, err := provisioner.DistributionGroup(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.DistributionGroupResults{
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

func (s *withoutControllerSuite) TestDistributionGroupByMachineIdControllerAuth(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.DistributionGroupByMachineId(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResults{
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

func (s *withoutControllerSuite) TestDistributionGroupByMachineIdMachineAgentAuth(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	provisioner, err := provisioner.MakeProvisionerAPI(c.Context(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Check(err, tc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "machine-1-lxd-99"},
		{Tag: "machine-1-lxd-99-lxd-100"},
	}}
	result, err := provisioner.DistributionGroupByMachineId(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResults{
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

func (s *withoutControllerSuite) TestConstraints(c *tc.C) {
	// Add a machine with some constraints.
	cons := constraints.MustParse("cores=123", "mem=8G")
	template := state.MachineTemplate{
		Base:        state.UbuntuBase("12.10"),
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
	}
	consMachine, err := s.ControllerModel(c).State().AddOneMachine(template)
	c.Assert(err, tc.ErrorIsNil)

	machine0Constraints, err := s.machines[0].Constraints()
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: consMachine.Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Constraints(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ConstraintsResults{
		Results: []params.ConstraintsResult{
			{Constraints: machine0Constraints},
			{Constraints: template.Constraints},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestSetInstanceInfo(c *tc.C) {
	st := s.ControllerModel(c).State()
	svc := s.ControllerDomainServices(c)
	machineService := svc.Machine()
	storageService := svc.Storage()

	err := storageService.CreateStoragePool(c.Context(), "static-pool", "static", map[string]any{"foo": "bar"})
	c.Assert(err, tc.ErrorIsNil)
	err = s.ControllerDomainServices(c).Config().UpdateModelConfig(c.Context(), map[string]any{
		"storage-default-block-source": "static-pool",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Provision machine 0 first.
	hwChars := instance.MustParseHardware("arch=arm64", "mem=4G")
	machine0UUID, err := machineService.GetMachineUUID(c.Context(), coremachine.Name(s.machines[0].Id()))
	c.Assert(err, tc.ErrorIsNil)
	err = machineService.SetMachineCloudInstance(c.Context(), machine0UUID, instance.Id("i-am"), "", &hwChars)
	c.Assert(err, tc.ErrorIsNil)

	// We keep this SetInstanceInfo only for the nonce.
	err = s.machines[0].SetInstanceInfo("i-am", "", "fake_nonce", &hwChars, nil, nil, nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	volumesMachine, err := st.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{Size: 1000},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = machineService.CreateMachine(c.Context(), coremachine.Name(volumesMachine.Id()))
	c.Assert(err, tc.ErrorIsNil)

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
	result, err := s.provisioner.SetInstanceInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{
				Message: `cannot record provisioning info for "i-was": cannot set instance data for machine "0": already set`,
			}},
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{
				Message: `cannot record provisioning info for "i-am-also": cannot set info for volume "0": volume "0" not found`,
				Code:    params.CodeNotFound,
			}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify machine 1 and 2 were provisioned.
	c.Assert(s.machines[1].Refresh(), tc.IsNil)
	c.Assert(s.machines[2].Refresh(), tc.IsNil)

	machine1UUID, err := machineService.GetMachineUUID(c.Context(), coremachine.Name(s.machines[1].Id()))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.machines[1].CheckProvisioned("fake_nonce"), tc.IsTrue)
	c.Check(s.machines[2].CheckProvisioned("fake"), tc.IsTrue)
	gotHardware, err := machineService.HardwareCharacteristics(c.Context(), machine1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotHardware, tc.DeepEquals, &hwChars)

	// Verify the machine with requested volumes was provisioned, and the
	// volume information recorded in state.
	sb, err := state.NewStorageBackend(st)
	c.Assert(err, tc.ErrorIsNil)
	volumeAttachments, err := sb.MachineVolumeAttachments(volumesMachine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeAttachments, tc.HasLen, 1)

	// Note (stickupkid): This is all incorrect, because we are no longer
	// using model-config for fallback storage pools. This should be fixed
	// once that's implemented in the new storage code.
	// See: https://warthogs.atlassian.net/browse/JUJU-6933
	volumeAttachmentInfo, err := volumeAttachments[0].Info()
	c.Assert(err, tc.ErrorIs, errors.NotProvisioned)
	c.Assert(volumeAttachmentInfo, tc.Equals, state.VolumeAttachmentInfo{})
	volume, err := sb.Volume(volumeAttachments[0].Volume())
	c.Assert(err, tc.ErrorIsNil)
	volumeInfo, err := volume.Info()
	c.Assert(err, tc.ErrorIs, errors.NotProvisioned)
	c.Assert(volumeInfo, tc.Equals, state.VolumeInfo{})

	// Verify the machine without requested volumes still has no volume
	// attachments recorded in state.
	volumeAttachments, err = sb.MachineVolumeAttachments(s.machines[1].MachineTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeAttachments, tc.HasLen, 0)
}

func (s *withoutControllerSuite) TestInstanceId(c *tc.C) {
	svc := s.ControllerDomainServices(c)
	machineService := svc.Machine()

	// Provision 2 machines first.
	machine0UUID, err := machineService.GetMachineUUID(c.Context(), coremachine.Name(s.machines[0].Id()))
	c.Assert(err, tc.ErrorIsNil)
	err = machineService.SetMachineCloudInstance(c.Context(), machine0UUID, "i-am", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	machine1UUID, err := machineService.GetMachineUUID(c.Context(), coremachine.Name(s.machines[1].Id()))
	c.Assert(err, tc.ErrorIsNil)
	hwChars := instance.MustParseHardware("arch=arm64", "mem=4G")
	err = machineService.SetMachineCloudInstance(c.Context(), machine1UUID, "i-am-not", "", &hwChars)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.InstanceId(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
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

func (s *withoutControllerSuite) TestWatchModelMachines(c *tc.C) {
	c.Assert(s.resources.Count(), tc.Equals, 0)

	got, err := s.provisioner.WatchModelMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	want := params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0", "1", "2", "3", "4"},
	}
	c.Assert(got.StringsWatcherId, tc.Equals, want.StringsWatcherId)
	c.Assert(got.Changes, tc.SameContents, want.Changes)

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), tc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := watchertest.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Make sure WatchModelMachines fails with a machine agent login.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	aProvisioner, err := provisioner.MakeProvisionerAPI(c.Context(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err := aProvisioner.WatchModelMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(result, tc.DeepEquals, params.StringsWatchResult{})
}

func (s *withoutControllerSuite) TestWatchMachineErrorRetry(c *tc.C) {
	s.PatchValue(&provisioner.ErrorRetryWaitDelay, 2*coretesting.ShortWait)
	c.Assert(s.resources.Count(), tc.Equals, 0)

	_, err := s.provisioner.WatchMachineErrorRetry(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), tc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := watchertest.NewNotifyWatcherC(c, resource.(state.NotifyWatcher))
	wc.AssertNoChange()

	// We should now get a time triggered change.
	wc.AssertOneChange()

	// Make sure WatchMachineErrorRetry fails with a machine agent login.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	aProvisioner, err := provisioner.MakeProvisionerAPI(c.Context(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err := aProvisioner.WatchMachineErrorRetry(c.Context())
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{})
}

func (s *withoutControllerSuite) TestMarkMachinesForRemoval(c *tc.C) {
	err := s.machines[0].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines[2].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	res, err := s.provisioner.MarkMachinesForRemoval(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-2"},         // ok
			{Tag: "machine-100"},       // not found
			{Tag: "machine-0"},         // ok
			{Tag: "machine-1"},         // not dead
			{Tag: "machine-0-lxd-5"},   // unauthorised
			{Tag: "application-thing"}, // only machines allowed
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	results := res.Results
	c.Assert(results, tc.HasLen, 6)
	c.Check(results[0].Error, tc.IsNil)
	c.Check(*results[1].Error, tc.DeepEquals,
		*apiservererrors.ServerError(errors.NotFoundf("machine 100")))
	c.Check(*results[1].Error, tc.Satisfies, params.IsCodeNotFound)
	c.Check(results[2].Error, tc.IsNil)
	c.Check(*results[3].Error, tc.DeepEquals,
		*apiservererrors.ServerError(errors.New("cannot remove machine 1: machine is not dead")))
	c.Check(*results[4].Error, tc.DeepEquals, *apiservertesting.ErrUnauthorized)
	c.Check(*results[5].Error, tc.DeepEquals,
		*apiservererrors.ServerError(errors.New(`"application-thing" is not a valid machine tag`)))

	removals, err := s.ControllerModel(c).State().AllMachineRemovals()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removals, tc.SameContents, []string{"0", "2"})
}

func (s *withoutControllerSuite) TestSetSupportedContainers(c *tc.C) {
	args := params.MachineContainersParams{Params: []params.MachineContainers{{
		MachineTag:     "machine-0",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}, {
		MachineTag:     "machine-1",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}}}
	results, err := s.provisioner.SetSupportedContainers(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, tc.IsNil)
	}
	st := s.ControllerModel(c).State()
	m0, err := st.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, tc.IsTrue)
	c.Assert(containers, tc.DeepEquals, []instance.ContainerType{instance.LXD})
	m1, err := st.Machine("1")
	c.Assert(err, tc.ErrorIsNil)
	containers, ok = m1.SupportedContainers()
	c.Assert(ok, tc.IsTrue)
	c.Assert(containers, tc.DeepEquals, []instance.ContainerType{instance.LXD})
}

func (s *withoutControllerSuite) TestSetSupportedContainersPermissions(c *tc.C) {
	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.MakeProvisionerAPI(c.Context(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(aProvisioner, tc.NotNil)

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
	results, err := aProvisioner.SetSupportedContainers(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestSupportedContainers(c *tc.C) {
	setArgs := params.MachineContainersParams{Params: []params.MachineContainers{{
		MachineTag:     "machine-0",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}, {
		MachineTag:     "machine-1",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}}}
	_, err := s.provisioner.SetSupportedContainers(c.Context(), setArgs)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "machine-1",
	}}}
	results, err := s.provisioner.SupportedContainers(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, tc.IsNil)
	}
	st := s.ControllerModel(c).State()
	m0, err := st.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, tc.IsTrue)
	c.Assert(containers, tc.DeepEquals, results.Results[0].ContainerTypes)
	m1, err := st.Machine("1")
	c.Assert(err, tc.ErrorIsNil)
	containers, ok = m1.SupportedContainers()
	c.Assert(ok, tc.IsTrue)
	c.Assert(containers, tc.DeepEquals, results.Results[1].ContainerTypes)
}

func (s *withoutControllerSuite) TestSupportedContainersWithoutBeingSet(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "machine-1",
	}}}
	results, err := s.provisioner.SupportedContainers(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, tc.IsNil)
		c.Assert(result.ContainerTypes, tc.HasLen, 0)
	}
}

func (s *withoutControllerSuite) TestSupportedContainersWithInvalidTag(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: "user-0",
	}}}
	results, err := s.provisioner.SupportedContainers(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	for _, result := range results.Results {
		c.Assert(result.Error, tc.ErrorMatches, "permission denied")
	}
}

func (s *withoutControllerSuite) TestSupportsNoContainers(c *tc.C) {
	args := params.MachineContainersParams{
		Params: []params.MachineContainers{
			{
				MachineTag: "machine-0",
			},
		},
	}
	results, err := s.provisioner.SetSupportedContainers(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	m0, err := s.ControllerModel(c).State().Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, tc.IsTrue)
	c.Assert(containers, tc.DeepEquals, []instance.ContainerType{})
}
func TestWithControllerSuite(t *stdtesting.T) { tc.Run(t, &withControllerSuite{}) }

type withControllerSuite struct {
	provisionerSuite
}

func (s *withControllerSuite) SetUpTest(c *tc.C) {
	s.provisionerSuite.setUpTest(c, true)
}

func (s *withControllerSuite) TestAPIAddresses(c *tc.C) {
	hostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}
	st := s.ControllerModel(c).State()

	controllerCfg := coretesting.FakeControllerConfig()

	err := st.SetAPIHostPorts(controllerCfg, hostPorts, hostPorts)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.provisioner.APIAddresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *withControllerSuite) TestCACert(c *tc.C) {
	result, err := s.provisioner.CACert(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.BytesResult{
		Result: []byte(coretesting.CACert),
	})
}
