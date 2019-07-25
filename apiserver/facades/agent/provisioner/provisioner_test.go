// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/proxy"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	"github.com/juju/juju/apiserver/facades/agent/provisioner/mocks"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/container"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	environtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/network/containerizer"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type provisionerSuite struct {
	testing.JujuConnSuite

	machines []*state.Machine

	authorizer  apiservertesting.FakeAuthorizer
	resources   *common.Resources
	provisioner *provisioner.ProvisionerAPIV9
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.setUpTest(c, false)
}

func (s *provisionerSuite) setUpTest(c *gc.C, withController bool) {
	if s.JujuConnSuite.ConfigAttrs == nil {
		s.JujuConnSuite.ConfigAttrs = make(map[string]interface{})
	}
	s.JujuConnSuite.ConfigAttrs["image-stream"] = "daily"
	s.JujuConnSuite.SetUpTest(c)

	// Reset previous machines (if any) and create 3 machines
	// for the tests, plus an optional controller machine.
	s.machines = nil
	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	if withController {
		s.machines = append(s.machines, testing.AddControllerMachine(c, s.State))
	}
	for i := 0; i < 5; i++ {
		machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Check(err, jc.ErrorIsNil)
		s.machines = append(s.machines, machine)
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
	provisionerAPI, err := provisioner.NewProvisionerAPIV9(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.provisioner = provisionerAPI
}

type withoutControllerSuite struct {
	provisionerSuite
	*commontesting.ModelWatcherTest
}

var _ = gc.Suite(&withoutControllerSuite{})

func (s *withoutControllerSuite) SetUpTest(c *gc.C) {
	s.setUpTest(c, false)
	s.ModelWatcherTest = commontesting.NewModelWatcherTest(s.provisioner, s.State, s.resources)
}

func (s *withoutControllerSuite) TestProvisionerFailsWithNonMachineAgentNonManagerUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = true
	// Works with a controller, which is not a machine agent.
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(aProvisioner, gc.NotNil)

	// But fails with neither a machine agent or a controller.
	anAuthorizer.Controller = false
	aProvisioner, err = provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
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
	results, err := s.provisioner.SetPasswords(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
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
	results, err := s.provisioner.SetPasswords(args)
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
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(aProvisioner, gc.NotNil)

	// Make the machine dead before trying to add containers.
	err = s.machines[0].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Create some containers to work on.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	var containers []*state.Machine
	for i := 0; i < 3; i++ {
		container, err := s.State.AddMachineInsideMachine(template, s.machines[0].Id(), instance.LXD)
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
	result, err := aProvisioner.Life(args)
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
	result, err := s.provisioner.Life(args)
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
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	result, err = s.provisioner.Life(params.Entities{
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
	result, err := s.provisioner.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: `cannot remove entity "machine-0": still alive`}},
			{nil},
			{&params.Error{Message: `cannot remove entity "machine-2": still alive`}},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
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
				Data: map[string]interface{}{"foo": "bar"}},
			{Tag: s.machines[1].Tag().String(), Status: status.Stopped.String(), Info: "foobar"},
			{Tag: s.machines[2].Tag().String(), Status: status.Started.String(), Info: "again"},
			{Tag: "machine-42", Status: status.Started.String(), Info: "blah"},
			{Tag: "unit-foo-0", Status: status.Stopped.String(), Info: "foobar"},
			{Tag: "application-bar", Status: status.Stopped.String(), Info: "foobar"},
		}}
	result, err := s.provisioner.SetStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertStatus(c, 0, status.Error, "not really", map[string]interface{}{"foo": "bar"})
	s.assertStatus(c, 1, status.Stopped, "foobar", map[string]interface{}{})
	s.assertStatus(c, 2, status.Started, "again", map[string]interface{}{})
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
				Data: map[string]interface{}{"foo": "bar"}},
			{Tag: s.machines[1].Tag().String(), Status: status.Running.String(), Info: "foobar"},
			{Tag: s.machines[2].Tag().String(), Status: status.ProvisioningError.String(), Info: "again"},
			{Tag: "machine-42", Status: status.Provisioning.String(), Info: "blah"},
			{Tag: "unit-foo-0", Status: status.Error.String(), Info: "foobar"},
			{Tag: "application-bar", Status: status.ProvisioningError.String(), Info: "foobar"},
		}}
	result, err := s.provisioner.SetInstanceStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertInstanceStatus(c, 0, status.Provisioning, "not really", map[string]interface{}{"foo": "bar"})
	s.assertInstanceStatus(c, 1, status.Running, "foobar", map[string]interface{}{})
	s.assertInstanceStatus(c, 2, status.ProvisioningError, "again", map[string]interface{}{})
	// ProvisioningError also has a special case which is to set the machine to Error
	s.assertStatus(c, 2, status.Error, "again", map[string]interface{}{})
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
				Data: map[string]interface{}{"foo": "bar"}},
			{Tag: s.machines[1].Tag().String(), Status: status.Applied.String(), Info: "foobar"},
			{Tag: s.machines[2].Tag().String(), Status: status.Error.String(), Info: "again"},
			{Tag: "machine-42", Status: status.Pending.String(), Info: "blah"},
			{Tag: "unit-foo-0", Status: status.Error.String(), Info: "foobar"},
			{Tag: "application-bar", Status: status.Error.String(), Info: "foobar"},
		}}
	result, err := s.provisioner.SetModificationStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertModificationStatus(c, 0, status.Pending, "not really", map[string]interface{}{"foo": "bar"})
	s.assertModificationStatus(c, 1, status.Applied, "foobar", map[string]interface{}{})
	s.assertModificationStatus(c, 2, status.Error, "again", map[string]interface{}{})
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
		Data:    map[string]interface{}{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Data:    map[string]interface{}{"transient": false},
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
		Data:    map[string]interface{}{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[4].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err = s.machines[4].SetProvisioned("i-am", "", "fake_nonce", &hwChars)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.provisioner.MachinesWithTransientErrors()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Id: "1", Life: "alive", Status: "provisioning error", Info: "transient error",
				Data: map[string]interface{}{"transient": true, "foo": "bar"}},
		},
	})
}

func (s *withoutControllerSuite) TestMachinesWithTransientErrorsPermission(c *gc.C) {
	// Machines where there's permission issues are omitted.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = names.NewMachineTag("1")
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources,
		anAuthorizer)
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
		Data:    map[string]interface{}{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Data:    map[string]interface{}{"transient": false},
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

	result, err := aProvisioner.MachinesWithTransientErrors()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{{
			Id: "1", Life: "alive", Status: "provisioning error",
			Info: "transient error",
			Data: map[string]interface{}{"transient": true, "foo": "bar"},
		},
		},
	})
}

func (s *withoutControllerSuite) TestEnsureDead(c *gc.C) {
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
	result, err := s.provisioner.EnsureDead(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertLife(c, 0, state.Dead)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Dead)
}

func (s *withoutControllerSuite) assertLife(c *gc.C, index int, expectLife state.Life) {
	err := s.machines[index].Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machines[index].Life(), gc.Equals, expectLife)
}

func (s *withoutControllerSuite) assertStatus(c *gc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]interface{}) {

	statusInfo, err := s.machines[index].Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, expectStatus)
	c.Assert(statusInfo.Message, gc.Equals, expectInfo)
	c.Assert(statusInfo.Data, gc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) assertInstanceStatus(c *gc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]interface{}) {

	statusInfo, err := s.machines[index].InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, expectStatus)
	c.Assert(statusInfo.Message, gc.Equals, expectInfo)
	c.Assert(statusInfo.Data, gc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) assertModificationStatus(c *gc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]interface{}) {

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
		{MachineTag: s.machines[1].Tag().String(), ContainerType: string(instance.KVM)},
		{MachineTag: "machine-42", ContainerType: ""},
		{MachineTag: "unit-foo-0", ContainerType: ""},
		{MachineTag: "application-bar", ContainerType: ""},
	}}
	result, err := s.provisioner.WatchContainers(args)
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
	defer statetesting.AssertStop(c, m0Watcher)
	m1Watcher := s.resources.Get("2")
	defer statetesting.AssertStop(c, m1Watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc0 := statetesting.NewStringsWatcherC(c, s.State, m0Watcher.(state.StringsWatcher))
	wc0.AssertNoChange()
	wc1 := statetesting.NewStringsWatcherC(c, s.State, m1Watcher.(state.StringsWatcher))
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
	result, err := s.provisioner.WatchAllContainers(args)
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
	defer statetesting.AssertStop(c, m0Watcher)
	m1Watcher := s.resources.Get("2")
	defer statetesting.AssertStop(c, m1Watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc0 := statetesting.NewStringsWatcherC(c, s.State, m0Watcher.(state.StringsWatcher))
	wc0.AssertNoChange()
	wc1 := statetesting.NewStringsWatcherC(c, s.State, m1Watcher.(state.StringsWatcher))
	wc1.AssertNoChange()
}

func (s *withoutControllerSuite) TestModelConfigNonManager(c *gc.C) {
	// Now test it with a non-controller and make sure
	// the secret attributes are masked.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources,
		anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertModelConfig(c, aProvisioner)
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
		Data:    map[string]interface{}{"foo": "bar"},
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
	result, err := s.provisioner.Status(args)
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
			{Status: status.Started.String(), Info: "blah", Data: map[string]interface{}{}},
			{Status: status.Stopped.String(), Info: "foo", Data: map[string]interface{}{}},
			{Status: status.Error.String(), Info: "not really", Data: map[string]interface{}{"foo": "bar"}},
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
		Data:    map[string]interface{}{"foo": "bar"},
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
	result, err := s.provisioner.InstanceStatus(args)
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
			{Status: status.Provisioning.String(), Info: "blah", Data: map[string]interface{}{}},
			{Status: status.Running.String(), Info: "foo", Data: map[string]interface{}{}},
			{Status: status.ProvisioningError.String(), Info: "not really", Data: map[string]interface{}{"foo": "bar"}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestSeries(c *gc.C) {
	// Add a machine with different series.
	foobarMachine := s.Factory.MakeMachine(c, &factory.MachineParams{Series: "foobar"})
	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: foobarMachine.Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Series(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: s.machines[0].Series()},
			{Result: foobarMachine.Series()},
			{Result: s.machines[2].Series()},
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

	// add machines with different availability zones: string, empty string, nil
	azMachine, _ := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Characteristics: &hcWithAZ,
	})

	emptyAzMachine, _ := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Characteristics: &hcWithEmptyAZ,
	})

	nilAzMachine, _ := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Characteristics: &hcWithNilAz,
	})
	args := params.Entities{Entities: []params.Entity{
		{Tag: azMachine.Tag().String()},
		{Tag: emptyAzMachine.Tag().String()},
		{Tag: nilAzMachine.Tag().String()},
	}}
	result, err := s.provisioner.AvailabilityZone(args)
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
	// Add a machine with keep-instance = true.
	foobarMachine := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: "1234"})
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
	result, err := s.provisioner.KeepInstance(args)
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
	addUnits := func(name string, machines ...*state.Machine) (units []*state.Unit) {
		app := s.AddTestingApplication(c, name, s.AddTestingCharm(c, name))
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
		m, err := s.State.Machine(id)
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
	_, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	setProvisioned("5")
	setProvisioned("7")

	// Create a logging service, subordinate to mysql.
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
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
	result, err := s.provisioner.DistributionGroup(args)
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
	result, err := s.provisioner.DistributionGroup(args)
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
	provisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "machine-1-lxd-99"},
		{Tag: "machine-1-lxd-99-lxd-100"},
	}}
	result, err := provisioner.DistributionGroup(args)
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
	addUnits := func(name string, machines ...*state.Machine) (units []*state.Unit) {
		app := s.AddTestingApplication(c, name, s.AddTestingCharm(c, name))
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
		m, err := s.State.Machine(id)
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
	_, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
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
	result, err := s.provisioner.DistributionGroupByMachineId(args)
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
	result, err := s.provisioner.DistributionGroupByMachineId(args)
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
	provisionerV5, err := provisioner.NewProvisionerAPIV5(s.State, s.resources, anAuthorizer)
	c.Check(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "machine-1-lxd-99"},
		{Tag: "machine-1-lxd-99-lxd-100"},
	}}
	result, err := provisionerV5.DistributionGroupByMachineId(args)
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

func (s *provisionerSuite) TestConstraints(c *gc.C) {
	// Add a machine with some constraints.
	cons := constraints.MustParse("cores=123", "mem=8G")
	template := state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
	}
	consMachine, err := s.State.AddOneMachine(template)
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
	result, err := s.provisioner.Constraints(args)
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
	pm := poolmanager.New(state.NewStateSettings(s.State), storage.ChainedProviderRegistry{
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	})
	_, err := pm.Create("static-pool", "static", map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.Model.UpdateModelConfig(map[string]interface{}{
		"storage-default-block-source": "static-pool",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Provision machine 0 first.
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err = s.machines[0].SetInstanceInfo("i-am", "", "fake_nonce", &hwChars, nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	volumesMachine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
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
	result, err := s.provisioner.SetInstanceInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{
				Message: `cannot record provisioning info for "i-was": cannot set instance data for machine "0": already set`,
			}},
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
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
	sb, err := state.NewStorageBackend(s.State)
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
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
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
	result, err := s.provisioner.InstanceId(args)
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

	got, err := s.provisioner.WatchModelMachines()
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
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Make sure WatchModelMachines fails with a machine agent login.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)

	result, err := aProvisioner.WatchModelMachines()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{})
}

func (s *provisionerSuite) getManagerConfig(c *gc.C, typ instance.ContainerType) map[string]string {
	args := params.ContainerManagerConfigParams{Type: typ}
	results, err := s.provisioner.ContainerManagerConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	return results.ManagerConfig
}

func (s *withoutControllerSuite) TestContainerManagerConfigDefaults(c *gc.C) {
	cfg := s.getManagerConfig(c, instance.KVM)
	c.Assert(cfg, jc.DeepEquals, map[string]string{
		container.ConfigModelUUID:      coretesting.ModelTag.Id(),
		config.ContainerImageStreamKey: "released",
	})
}

type withImageMetadataSuite struct {
	provisionerSuite
}

var _ = gc.Suite(&withImageMetadataSuite{})

func (s *withImageMetadataSuite) SetUpTest(c *gc.C) {
	s.ConfigAttrs = map[string]interface{}{
		config.ContainerImageStreamKey:      "daily",
		config.ContainerImageMetadataURLKey: "https://images.linuxcontainers.org/",
	}
	s.setUpTest(c, false)
}

func (s *withImageMetadataSuite) TestContainerManagerConfigImageMetadata(c *gc.C) {
	cfg := s.getManagerConfig(c, instance.LXD)
	c.Assert(cfg, jc.DeepEquals, map[string]string{
		container.ConfigModelUUID:           coretesting.ModelTag.Id(),
		config.ContainerImageStreamKey:      "daily",
		config.ContainerImageMetadataURLKey: "https://images.linuxcontainers.org/",
	})
}
func (s *withoutControllerSuite) TestContainerConfig(c *gc.C) {
	attrs := map[string]interface{}{
		"juju-http-proxy":              "http://proxy.example.com:9000",
		"apt-https-proxy":              "https://proxy.example.com:9000",
		"allow-lxd-loop-mounts":        true,
		"apt-mirror":                   "http://example.mirror.com",
		"cloudinit-userdata":           validCloudInitUserData,
		"container-inherit-properties": "ca-certs,apt-primary",
	}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	expectedAPTProxy := proxy.Settings{
		Http:    "http://proxy.example.com:9000",
		Https:   "https://proxy.example.com:9000",
		NoProxy: "127.0.0.1,localhost,::1",
	}

	expectedProxy := proxy.Settings{
		Http:    "http://proxy.example.com:9000",
		NoProxy: "127.0.0.1,localhost,::1",
	}

	results, err := s.provisioner.ContainerConfig()
	c.Check(err, jc.ErrorIsNil)
	c.Check(results.UpdateBehavior, gc.Not(gc.IsNil))
	c.Check(results.ProviderType, gc.Equals, "dummy")
	c.Check(results.AuthorizedKeys, gc.Equals, s.Environ.Config().AuthorizedKeys())
	c.Check(results.SSLHostnameVerification, jc.IsTrue)
	c.Check(results.LegacyProxy.HasProxySet(), jc.IsFalse)
	c.Check(results.JujuProxy, gc.DeepEquals, expectedProxy)
	c.Check(results.AptProxy, gc.DeepEquals, expectedAPTProxy)
	c.Check(results.AptMirror, gc.DeepEquals, "http://example.mirror.com")
	c.Check(results.CloudInitUserData, gc.DeepEquals, map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false})
	c.Check(results.ContainerInheritProperties, gc.DeepEquals, "ca-certs,apt-primary")
}

func (s *withoutControllerSuite) TestContainerConfigLegacy(c *gc.C) {
	attrs := map[string]interface{}{
		"http-proxy":                   "http://proxy.example.com:9000",
		"apt-https-proxy":              "https://proxy.example.com:9000",
		"allow-lxd-loop-mounts":        true,
		"apt-mirror":                   "http://example.mirror.com",
		"cloudinit-userdata":           validCloudInitUserData,
		"container-inherit-properties": "ca-certs,apt-primary",
	}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	expectedAPTProxy := proxy.Settings{
		Http:    "http://proxy.example.com:9000",
		Https:   "https://proxy.example.com:9000",
		NoProxy: "127.0.0.1,localhost,::1",
	}

	expectedProxy := proxy.Settings{
		Http:    "http://proxy.example.com:9000",
		NoProxy: "127.0.0.1,localhost,::1",
	}

	results, err := s.provisioner.ContainerConfig()
	c.Check(err, jc.ErrorIsNil)
	c.Check(results.UpdateBehavior, gc.Not(gc.IsNil))
	c.Check(results.ProviderType, gc.Equals, "dummy")
	c.Check(results.AuthorizedKeys, gc.Equals, s.Environ.Config().AuthorizedKeys())
	c.Check(results.SSLHostnameVerification, jc.IsTrue)
	c.Check(results.LegacyProxy, gc.DeepEquals, expectedProxy)
	c.Check(results.JujuProxy.HasProxySet(), jc.IsFalse)
	c.Check(results.AptProxy, gc.DeepEquals, expectedAPTProxy)
	c.Check(results.AptMirror, gc.DeepEquals, "http://example.mirror.com")
	c.Check(results.CloudInitUserData, gc.DeepEquals, map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false})
	c.Check(results.ContainerInheritProperties, gc.DeepEquals, "ca-certs,apt-primary")
}

func (s *withoutControllerSuite) TestContainerConfigV5(c *gc.C) {
	attrs := map[string]interface{}{
		"http-proxy":                   "http://proxy.example.com:9000",
		"apt-https-proxy":              "https://proxy.example.com:9000",
		"allow-lxd-loop-mounts":        true,
		"apt-mirror":                   "http://example.mirror.com",
		"cloudinit-userdata":           validCloudInitUserData,
		"container-inherit-properties": "ca-certs,apt-primary",
	}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	expectedAPTProxy := proxy.Settings{
		Http:    "http://proxy.example.com:9000",
		Https:   "https://proxy.example.com:9000",
		NoProxy: "127.0.0.1,localhost,::1",
	}

	expectedProxy := proxy.Settings{
		Http:    "http://proxy.example.com:9000",
		NoProxy: "127.0.0.1,localhost,::1",
	}

	provisionerV5, err := provisioner.NewProvisionerAPIV5(s.State, s.resources, s.authorizer)
	c.Check(err, jc.ErrorIsNil)

	var results params.ContainerConfigV5
	results, err = provisionerV5.ContainerConfig()
	c.Check(err, jc.ErrorIsNil)

	c.Check(results.UpdateBehavior, gc.Not(gc.IsNil))
	c.Check(results.ProviderType, gc.Equals, "dummy")
	c.Check(results.AuthorizedKeys, gc.Equals, s.Environ.Config().AuthorizedKeys())
	c.Check(results.SSLHostnameVerification, jc.IsTrue)
	c.Check(results.Proxy, gc.DeepEquals, expectedProxy)
	c.Check(results.AptProxy, gc.DeepEquals, expectedAPTProxy)
	c.Check(results.AptMirror, gc.DeepEquals, "http://example.mirror.com")
	c.Check(results.CloudInitUserData, gc.DeepEquals, map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false})
	c.Check(results.ContainerInheritProperties, gc.DeepEquals, "ca-certs,apt-primary")
}

func (s *withoutControllerSuite) TestSetSupportedContainers(c *gc.C) {
	args := params.MachineContainersParams{Params: []params.MachineContainers{{
		MachineTag:     "machine-0",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}, {
		MachineTag:     "machine-1",
		ContainerTypes: []instance.ContainerType{instance.LXD, instance.KVM},
	}}}
	results, err := s.provisioner.SetSupportedContainers(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, gc.IsNil)
	}
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{instance.LXD})
	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	containers, ok = m1.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{instance.LXD, instance.KVM})
}

func (s *withoutControllerSuite) TestSetSupportedContainersPermissions(c *gc.C) {
	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
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
	results, err := aProvisioner.SetSupportedContainers(args)
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
		ContainerTypes: []instance.ContainerType{instance.LXD, instance.KVM},
	}}}
	_, err := s.provisioner.SetSupportedContainers(setArgs)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "machine-1",
	}}}
	results, err := s.provisioner.SupportedContainers(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, gc.IsNil)
	}
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, results.Results[0].ContainerTypes)
	m1, err := s.State.Machine("1")
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
	results, err := s.provisioner.SupportedContainers(args)
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
	results, err := s.provisioner.SupportedContainers(args)
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
	results, err := s.provisioner.SetSupportedContainers(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	m0, err := s.State.Machine("0")
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
	hostPorts := [][]network.HostPort{
		network.NewHostPorts(1234, "0.1.2.3"),
	}
	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.provisioner.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *withControllerSuite) TestStateAddresses(c *gc.C) {
	addresses, err := s.State.Addresses()
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.provisioner.StateAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: addresses,
	})
}

func (s *withControllerSuite) TestCACert(c *gc.C) {
	result, err := s.provisioner.CACert()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.BytesResult{
		Result: []byte(coretesting.CACert),
	})
}

func (s *withoutControllerSuite) TestWatchMachineErrorRetry(c *gc.C) {
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	s.PatchValue(&provisioner.ErrorRetryWaitDelay, 2*coretesting.ShortWait)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	_, err := s.provisioner.WatchMachineErrorRetry()
	c.Assert(err, jc.ErrorIsNil)

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
	wc.AssertNoChange()

	// We should now get a time triggered change.
	wc.AssertOneChange()

	// Make sure WatchMachineErrorRetry fails with a machine agent login.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)

	result, err := aProvisioner.WatchMachineErrorRetry()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{})
}

func (s *withoutControllerSuite) TestFindTools(c *gc.C) {
	args := params.FindToolsParams{
		MajorVersion: -1,
		MinorVersion: -1,
	}
	result, err := s.provisioner.FindTools(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, gc.Not(gc.HasLen), 0)
	for _, tools := range result.List {
		url := fmt.Sprintf("https://%s/model/%s/tools/%s",
			s.APIState.Addr(), coretesting.ModelTag.Id(), tools.Version)
		c.Assert(tools.URL, gc.Equals, url)
	}
}

func (s *withoutControllerSuite) TestMarkMachinesForRemoval(c *gc.C) {
	err := s.machines[0].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines[2].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.provisioner.MarkMachinesForRemoval(params.Entities{
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
		*common.ServerError(errors.NotFoundf("machine 100")))
	c.Check(*results[1].Error, jc.Satisfies, params.IsCodeNotFound)
	c.Check(results[2].Error, gc.IsNil)
	c.Check(*results[3].Error, jc.DeepEquals,
		*common.ServerError(errors.New("cannot remove machine 1: machine is not dead")))
	c.Check(*results[4].Error, jc.DeepEquals, *apiservertesting.ErrUnauthorized)
	c.Check(*results[5].Error, jc.DeepEquals,
		*common.ServerError(errors.New(`"application-thing" is not a valid machine tag`)))

	removals, err := s.State.AllMachineRemovals()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(removals, jc.SameContents, []string{"0", "2"})
}

// TODO(jam): 2017-02-15 We seem to be lacking most of direct unit tests around ProcessOneContainer
// Some of the use cases we need to be testing are:
// 1) Provider can allocate addresses, should result in a container with
//    addresses requested from the provider, and 'static' configuration on those
//    devices.
// 2) Provider cannot allocate addresses, currently this should make us use
//    'lxdbr0' and DHCP allocated addresses.
// 3) Provider could allocate DHCP based addresses on the host device, which would let us
//    use a bridge on the device and DHCP. (Currently not supported, but desirable for
//    vSphere and Manual and probably LXD providers.)
// Addition (manadart 2018-10-09): To begin accommodating the deficiencies noted
// above, the new suite below uses mocks for tests ill-suited to the dummy
// provider. We could reasonably re-write the tests above over time to use the
// new suite.

type provisionerMockSuite struct {
	coretesting.BaseSuite

	environ      *environtesting.MockNetworkingEnviron
	host         *mocks.MockMachine
	container    *mocks.MockMachine
	device       *mocks.MockLinkLayerDevice
	parentDevice *mocks.MockLinkLayerDevice

	unit        *mocks.MockUnit
	application *mocks.MockApplication
	charm       *mocks.MockCharm
}

var _ = gc.Suite(&provisionerMockSuite{})

// Even when the provider supports container addresses, manually provisioned
// machines should fall back to DHCP.
func (s *provisionerMockSuite) TestManuallyProvisionedHostsUseDHCPForContainers(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectManuallyProvisionedHostsUseDHCPForContainers()

	res := params.MachineNetworkConfigResults{
		Results: []params.MachineNetworkConfigResult{{}},
	}
	ctx := provisioner.NewPrepareOrGetContext(res, false)

	// ProviderCallContext is not required by this logical path; we pass nil.
	err := ctx.ProcessOneContainer(s.environ, nil, 0, s.host, s.container)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Config, gc.HasLen, 1)

	cfg := res.Results[0].Config[0]
	c.Check(cfg.ConfigType, gc.Equals, "dhcp")
	c.Check(cfg.ProviderSubnetId, gc.Equals, "")
	c.Check(cfg.VLANTag, gc.Equals, 0)
}

func (s *provisionerMockSuite) expectManuallyProvisionedHostsUseDHCPForContainers() {
	s.expectNetworkingEnviron()
	s.expectLinkLayerDevices()

	emptySpace := ""

	cExp := s.container.EXPECT()
	cExp.InstanceId().Return(instance.UnknownId, errors.NotProvisionedf("idk-lol"))
	cExp.DesiredSpaces().Return(set.NewStrings(emptySpace), nil)
	cExp.Id().Return("lxd/0").AnyTimes()
	cExp.SetLinkLayerDevices(gomock.Any()).Return(nil)
	cExp.AllLinkLayerDevices().Return([]containerizer.LinkLayerDevice{s.device}, nil)

	hExp := s.host.EXPECT()
	hExp.Id().Return("0").AnyTimes()
	hExp.LinkLayerDevicesForSpaces(gomock.Any()).Return(
		map[string][]containerizer.LinkLayerDevice{emptySpace: {s.device}}, nil)
	// Crucial behavioural trait. Set false to test failure.
	hExp.IsManual().Return(true, nil)
	hExp.InstanceId().Return(instance.Id("manual:10.0.0.66"), nil)

}

// expectNetworkingEnviron stubs an environ that supports container networking.
func (s *provisionerMockSuite) expectNetworkingEnviron() {
	eExp := s.environ.EXPECT()
	eExp.Config().Return(&config.Config{}).AnyTimes()
	eExp.SupportsContainerAddresses(gomock.Any()).Return(true, nil).AnyTimes()
}

// expectLinkLayerDevices mocks a link-layer device and its parent,
// suitable for use as a bridge network for containers.
func (s *provisionerMockSuite) expectLinkLayerDevices() {
	devName := "eth0"
	mtu := uint(1500)
	mac := corenetwork.GenerateVirtualMACAddress()
	deviceArgs := state.LinkLayerDeviceArgs{
		Name:       devName,
		Type:       state.EthernetDevice,
		MACAddress: mac,
		MTU:        mtu,
	}

	dExp := s.device.EXPECT()
	dExp.Name().Return(devName).AnyTimes()
	dExp.Type().Return(state.BridgeDevice).AnyTimes()
	dExp.MTU().Return(mtu).AnyTimes()
	dExp.EthernetDeviceForBridge(devName).Return(deviceArgs, nil).MinTimes(1)
	dExp.ParentDevice().Return(s.parentDevice, nil)
	dExp.MACAddress().Return(mac)
	dExp.IsAutoStart().Return(true)
	dExp.IsUp().Return(true)

	pExp := s.parentDevice.EXPECT()
	// The address itself is unimportant, so we can use an empty one.
	// What is important is that there is one there to flex the path we are
	// testing.
	pExp.Addresses().Return([]*state.Address{{}}, nil)
	pExp.Name().Return(devName).MinTimes(1)
}

func (s *provisionerMockSuite) TestContainerAlreadyProvisionedError(c *gc.C) {
	defer s.setup(c).Finish()

	exp := s.container.EXPECT()
	exp.InstanceId().Return(instance.Id("juju-8ebd6c-0"), nil)
	exp.Id().Return("0/lxd/0")

	res := params.MachineNetworkConfigResults{
		Results: []params.MachineNetworkConfigResult{{}},
	}
	ctx := provisioner.NewPrepareOrGetContext(res, true)

	// ProviderCallContext is not required by this logical path; we pass nil.
	err := ctx.ProcessOneContainer(s.environ, nil, 0, s.host, s.container)
	c.Assert(err, gc.ErrorMatches, `container "0/lxd/0" already provisioned as "juju-8ebd6c-0"`)
}

func (s *provisionerMockSuite) TestGetContainerProfileInfo(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()
	s.expectCharmLXDProfiles(ctrl)

	s.application.EXPECT().Name().Return("application")
	s.charm.EXPECT().Revision().Return(3)
	s.charm.EXPECT().LXDProfile().Return(
		&charm.LXDProfile{
			Config: map[string]string{
				"security.nesting":    "true",
				"security.privileged": "true",
			},
		})

	res := params.ContainerProfileResults{
		Results: []params.ContainerProfileResult{{}},
	}
	ctx := provisioner.NewContainerProfileContext(res, "testme")

	// ProviderCallContext is not required by this logical path; we pass nil.
	err := ctx.ProcessOneContainer(s.environ, nil, 0, s.host, s.container)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, gc.IsNil)
	c.Assert(res.Results[0].LXDProfiles, gc.HasLen, 1)
	profile := res.Results[0].LXDProfiles[0]
	c.Check(profile.Name, gc.Equals, "juju-testme-application-3")
	c.Check(profile.Profile.Config, gc.DeepEquals,
		map[string]string{
			"security.nesting":    "true",
			"security.privileged": "true",
		},
	)
}

func (s *provisionerMockSuite) TestGetContainerProfileInfoNoProfile(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()
	s.expectCharmLXDProfiles(ctrl)

	s.charm.EXPECT().LXDProfile().Return(nil)
	s.unit.EXPECT().Name().Return("application/0")

	res := params.ContainerProfileResults{
		Results: []params.ContainerProfileResult{{}},
	}
	ctx := provisioner.NewContainerProfileContext(res, "testme")

	// ProviderCallContext is not required by this logical path; we pass nil.
	err := ctx.ProcessOneContainer(s.environ, nil, 0, s.host, s.container)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, gc.IsNil)
	c.Assert(res.Results[0].LXDProfiles, gc.HasLen, 0)
}

func (s *provisionerMockSuite) expectCharmLXDProfiles(ctrl *gomock.Controller) {
	s.unit = mocks.NewMockUnit(ctrl)
	s.application = mocks.NewMockApplication(ctrl)
	s.charm = mocks.NewMockCharm(ctrl)

	s.container.EXPECT().Units().Return([]containerizer.Unit{s.unit}, nil)
	s.unit.EXPECT().Application().Return(s.application, nil)
	s.application.EXPECT().Charm().Return(s.charm, false, nil)
}

func (s *provisionerMockSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.environ = environtesting.NewMockNetworkingEnviron(ctrl)
	s.host = mocks.NewMockMachine(ctrl)
	s.container = mocks.NewMockMachine(ctrl)
	s.device = mocks.NewMockLinkLayerDevice(ctrl)
	s.parentDevice = mocks.NewMockLinkLayerDevice(ctrl)

	return ctrl
}

type provisionerProfileMockSuite struct {
	coretesting.BaseSuite

	backend *mocks.MockProfileBackend
	charm   *mocks.MockProfileCharm
	machine *mocks.MockProfileMachine
}
