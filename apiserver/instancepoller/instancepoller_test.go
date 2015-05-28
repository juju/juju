// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/instancepoller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type InstancePollerSuite struct {
	coretesting.BaseSuite

	st         *mockState
	api        *instancepoller.InstancePollerAPI
	authoriser apiservertesting.FakeAuthorizer
	resources  *common.Resources

	machineEntities     params.Entities
	machineErrorResults params.ErrorResults

	mixedEntities     params.Entities
	mixedErrorResults params.ErrorResults
}

var _ = gc.Suite(&InstancePollerSuite{})

func (s *InstancePollerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.authoriser = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.st = NewMockState()
	instancepoller.PatchState(s, s.st)

	var err error
	s.api, err = instancepoller.NewInstancePollerAPI(nil, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)

	s.machineEntities = params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-2"},
			{Tag: "machine-3"},
		}}
	s.machineErrorResults = params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("42")},
		}}

	s.mixedEntities = params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-2"},
			{Tag: "machine-42"},
			{Tag: "service-unknown"},
			{Tag: "invalid-tag"},
			{Tag: "unit-missing-1"},
			{Tag: ""},
			{Tag: "42"},
		}}
	s.mixedErrorResults = params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"service-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}}
}

func (s *InstancePollerSuite) TestNewInstancePollerAPIRequiresEnvironManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.EnvironManager = false
	api, err := instancepoller.NewInstancePollerAPI(nil, s.resources, anAuthoriser)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *InstancePollerSuite) TestEnvironConfigFailure(c *gc.C) {
	s.st.SetErrors(errors.New("boom"))

	result, err := s.api.EnvironConfig()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, jc.DeepEquals, params.EnvironConfigResult{})

	s.st.CheckCallNames(c, "EnvironConfig")
}

func (s *InstancePollerSuite) TestEnvironConfigSuccess(c *gc.C) {
	envConfig := coretesting.EnvironConfig(c)
	s.st.SetConfig(c, envConfig)

	result, err := s.api.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.EnvironConfigResult{
		Config: envConfig.AllAttrs(),
	})

	s.st.CheckCallNames(c, "EnvironConfig")
}

func (s *InstancePollerSuite) TestWatchForEnvironConfigChangesFailure(c *gc.C) {
	// Force the Changes() method of the mock watcher to return a
	// closed channel by setting an error.
	s.st.SetErrors(errors.New("boom"))

	result, err := s.api.WatchForEnvironConfigChanges()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{})

	c.Assert(s.resources.Count(), gc.Equals, 0) // no watcher registered
	s.st.CheckCallNames(c, "WatchForEnvironConfigChanges")
}

func (s *InstancePollerSuite) TestWatchForEnvironConfigChangesSuccess(c *gc.C) {
	result, err := s.api.WatchForEnvironConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{
		Error: nil, NotifyWatcherId: "1",
	})

	// Verify the watcher resource was registered.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the watcher has consumed the initial event
	wc := statetesting.NewNotifyWatcherC(c, s.st, resource.(state.NotifyWatcher))
	wc.AssertNoChange()

	s.st.CheckCallNames(c, "WatchForEnvironConfigChanges")

	// Try changing the config to verify an event is reported.
	envConfig := coretesting.EnvironConfig(c)
	s.st.SetConfig(c, envConfig)
	wc.AssertOneChange()
}

func (s *InstancePollerSuite) TestWatchEnvironMachinesFailure(c *gc.C) {
	// Force the Changes() method of the mock watcher to return a
	// closed channel by setting an error.
	s.st.SetErrors(errors.Errorf("boom"))

	result, err := s.api.WatchEnvironMachines()
	c.Assert(err, gc.ErrorMatches, "cannot obtain initial environment machines: boom")
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{})

	c.Assert(s.resources.Count(), gc.Equals, 0) // no watcher registered
	s.st.CheckCallNames(c, "WatchEnvironMachines")
}

func (s *InstancePollerSuite) TestWatchEnvironMachinesSuccess(c *gc.C) {
	// Add a couple of machines.
	s.st.SetMachineInfo(c, machineInfo{id: "2"})
	s.st.SetMachineInfo(c, machineInfo{id: "1"})

	expectedResult := params.StringsWatchResult{
		Error:            nil,
		StringsWatcherId: "1",
		Changes:          []string{"1", "2"}, // initial event (sorted ids)
	}
	result, err := s.api.WatchEnvironMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expectedResult)

	// Verify the watcher resource was registered.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource1 := s.resources.Get("1")
	defer func() {
		if resource1 != nil {
			statetesting.AssertStop(c, resource1)
		}
	}()

	// Check that the watcher has consumed the initial event
	wc1 := statetesting.NewStringsWatcherC(c, s.st, resource1.(state.StringsWatcher))
	wc1.AssertNoChange()

	s.st.CheckCallNames(c, "WatchEnvironMachines")

	// Add another watcher to verify events coalescence.
	result, err = s.api.WatchEnvironMachines()
	c.Assert(err, jc.ErrorIsNil)
	expectedResult.StringsWatcherId = "2"
	c.Assert(result, jc.DeepEquals, expectedResult)
	s.st.CheckCallNames(c, "WatchEnvironMachines", "WatchEnvironMachines")
	c.Assert(s.resources.Count(), gc.Equals, 2)
	resource2 := s.resources.Get("2")
	defer statetesting.AssertStop(c, resource2)
	wc2 := statetesting.NewStringsWatcherC(c, s.st, resource2.(state.StringsWatcher))
	wc2.AssertNoChange()

	// Remove machine 1, check it's reported.
	s.st.RemoveMachine(c, "1")
	wc1.AssertChangeInSingleEvent("1")

	// Make separate changes, check they're combined.
	s.st.SetMachineInfo(c, machineInfo{id: "2", life: state.Dying})
	s.st.SetMachineInfo(c, machineInfo{id: "3"})
	s.st.RemoveMachine(c, "42") // ignored
	wc1.AssertChangeInSingleEvent("2", "3")
	wc2.AssertChangeInSingleEvent("1", "2", "3")

	// Stop the first watcher and assert its changes chan is closed.
	c.Assert(resource1.Stop(), jc.ErrorIsNil)
	wc1.AssertClosed()
	resource1 = nil
}

func (s *InstancePollerSuite) TestLifeSuccess(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", life: state.Alive})
	s.st.SetMachineInfo(c, machineInfo{id: "2", life: state.Dying})

	result, err := s.api.Life(s.mixedEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: params.Alive},
			{Life: params.Dying},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckCall(c, 1, "Life")
	s.st.CheckFindEntityCall(c, 2, "2")
	s.st.CheckCall(c, 3, "Life")
	s.st.CheckFindEntityCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestLifeFailure(c *gc.C) {
	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1"); Life not called
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.Life() - unused
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", life: state.Alive})
	s.st.SetMachineInfo(c, machineInfo{id: "2", life: state.Dead})
	s.st.SetMachineInfo(c, machineInfo{id: "3", life: state.Dying})

	result, err := s.api.Life(s.machineEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Life: params.Dead},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckFindEntityCall(c, 1, "2")
	s.st.CheckCall(c, 2, "Life")
	s.st.CheckFindEntityCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestInstanceIdSuccess(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceId: "i-foo"})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceId: ""})

	result, err := s.api.InstanceId(s.mixedEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "i-foo"},
			{Result: ""},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckCall(c, 1, "InstanceId")
	s.st.CheckFindEntityCall(c, 2, "2")
	s.st.CheckCall(c, 3, "InstanceId")
	s.st.CheckFindEntityCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestInstanceIdFailure(c *gc.C) {
	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1"); InstanceId not called
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.InstanceId()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceId: ""})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceId: "i-bar"})

	result, err := s.api.InstanceId(s.machineEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckFindEntityCall(c, 1, "2")
	s.st.CheckCall(c, 2, "InstanceId")
	s.st.CheckFindEntityCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestStatusSuccess(c *gc.C) {
	now := time.Now()
	s1 := state.StatusInfo{
		Status:  state.StatusError,
		Message: "not really",
		Data: map[string]interface{}{
			"price": 4.2,
			"bool":  false,
			"bar":   []string{"a", "b"},
		},
		Since: &now,
	}
	s2 := state.StatusInfo{}
	s.st.SetMachineInfo(c, machineInfo{id: "1", status: s1})
	s.st.SetMachineInfo(c, machineInfo{id: "2", status: s2})

	result, err := s.api.Status(s.mixedEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{
				Status: params.StatusError,
				Info:   s1.Message,
				Data:   s1.Data,
				Since:  s1.Since,
			},
			{Status: "", Info: "", Data: nil, Since: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckCall(c, 1, "Status")
	s.st.CheckFindEntityCall(c, 2, "2")
	s.st.CheckCall(c, 3, "Status")
	s.st.CheckFindEntityCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestStatusFailure(c *gc.C) {
	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1"); Status not called
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.Status()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1"})
	s.st.SetMachineInfo(c, machineInfo{id: "2"})

	result, err := s.api.Status(s.machineEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckFindEntityCall(c, 1, "2")
	s.st.CheckCall(c, 2, "Status")
	s.st.CheckFindEntityCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestProviderAddressesSuccess(c *gc.C) {
	addrs := network.NewAddresses("0.1.2.3", "127.0.0.1", "8.8.8.8")
	expectedAddresses := params.FromNetworkAddresses(addrs)
	s.st.SetMachineInfo(c, machineInfo{id: "1", providerAddresses: addrs})
	s.st.SetMachineInfo(c, machineInfo{id: "2", providerAddresses: nil})

	result, err := s.api.ProviderAddresses(s.mixedEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.MachineAddressesResults{
		Results: []params.MachineAddressesResult{
			{Addresses: expectedAddresses},
			{Addresses: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"service-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckCall(c, 1, "ProviderAddresses")
	s.st.CheckFindEntityCall(c, 2, "2")
	s.st.CheckCall(c, 3, "ProviderAddresses")
	s.st.CheckFindEntityCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestProviderAddressesFailure(c *gc.C) {
	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1")
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.ProviderAddresses()- unused
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1"})
	s.st.SetMachineInfo(c, machineInfo{id: "2"})

	result, err := s.api.ProviderAddresses(s.machineEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.MachineAddressesResults{
		Results: []params.MachineAddressesResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Addresses: nil},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckFindEntityCall(c, 1, "2")
	s.st.CheckCall(c, 2, "ProviderAddresses")
	s.st.CheckFindEntityCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestSetProviderAddressesSuccess(c *gc.C) {
	oldAddrs := network.NewAddresses("0.1.2.3", "127.0.0.1", "8.8.8.8")
	newAddrs := network.NewAddresses("1.2.3.4", "8.4.4.8", "2001:db8::")
	s.st.SetMachineInfo(c, machineInfo{id: "1", providerAddresses: oldAddrs})
	s.st.SetMachineInfo(c, machineInfo{id: "2", providerAddresses: nil})

	result, err := s.api.SetProviderAddresses(params.SetMachinesAddresses{
		MachineAddresses: []params.MachineAddresses{
			{Tag: "machine-1", Addresses: nil},
			{Tag: "machine-2", Addresses: params.FromNetworkAddresses(newAddrs)},
			{Tag: "machine-42"},
			{Tag: "service-unknown"},
			{Tag: "invalid-tag"},
			{Tag: "unit-missing-1"},
			{Tag: ""},
			{Tag: "42"},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.mixedErrorResults)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckSetProviderAddressesCall(c, 1, []network.Address{})
	s.st.CheckFindEntityCall(c, 2, "2")
	s.st.CheckSetProviderAddressesCall(c, 3, newAddrs)
	s.st.CheckFindEntityCall(c, 4, "42")

	// Ensure machines were updated.
	machine, err := s.st.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.ProviderAddresses(), gc.HasLen, 0)

	machine, err = s.st.Machine("2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.ProviderAddresses(), jc.DeepEquals, newAddrs)
}

func (s *InstancePollerSuite) TestSetProviderAddressesFailure(c *gc.C) {
	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1")
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.SetProviderAddresses()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	oldAddrs := network.NewAddresses("0.1.2.3", "127.0.0.1", "8.8.8.8")
	newAddrs := network.NewAddresses("1.2.3.4", "8.4.4.8", "2001:db8::")
	s.st.SetMachineInfo(c, machineInfo{id: "1", providerAddresses: oldAddrs})
	s.st.SetMachineInfo(c, machineInfo{id: "2", providerAddresses: nil})

	result, err := s.api.SetProviderAddresses(params.SetMachinesAddresses{
		MachineAddresses: []params.MachineAddresses{
			{Tag: "machine-1", Addresses: nil},
			{Tag: "machine-2", Addresses: params.FromNetworkAddresses(newAddrs)},
			{Tag: "machine-3"},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.machineErrorResults)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckFindEntityCall(c, 1, "2")
	s.st.CheckSetProviderAddressesCall(c, 2, newAddrs)
	s.st.CheckFindEntityCall(c, 3, "3")

	// Ensure machine 2 wasn't updated.
	machine, err := s.st.Machine("2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.ProviderAddresses(), gc.HasLen, 0)
}

func (s *InstancePollerSuite) TestInstanceStatusSuccess(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: "foo"})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: ""})

	result, err := s.api.InstanceStatus(s.mixedEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "foo"},
			{Result: ""},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"service-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckCall(c, 1, "InstanceStatus")
	s.st.CheckFindEntityCall(c, 2, "2")
	s.st.CheckCall(c, 3, "InstanceStatus")
	s.st.CheckFindEntityCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestInstanceStatusFailure(c *gc.C) {
	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1")
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.InstanceStatus()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: "foo"})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: ""})

	result, err := s.api.InstanceStatus(s.machineEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckFindEntityCall(c, 1, "2")
	s.st.CheckCall(c, 2, "InstanceStatus")
	s.st.CheckFindEntityCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestSetInstanceStatusSuccess(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: "foo"})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: ""})

	result, err := s.api.SetInstanceStatus(params.SetInstancesStatus{
		Entities: []params.InstanceStatus{
			{Tag: "machine-1", Status: ""},
			{Tag: "machine-2", Status: "new status"},
			{Tag: "machine-42"},
			{Tag: "service-unknown"},
			{Tag: "invalid-tag"},
			{Tag: "unit-missing-1"},
			{Tag: ""},
			{Tag: "42"},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.mixedErrorResults)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckCall(c, 1, "SetInstanceStatus", "")
	s.st.CheckFindEntityCall(c, 2, "2")
	s.st.CheckCall(c, 3, "SetInstanceStatus", "new status")
	s.st.CheckFindEntityCall(c, 4, "42")

	// Ensure machines were updated.
	machine, err := s.st.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	setStatus, err := machine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(setStatus, gc.Equals, "")

	machine, err = s.st.Machine("2")
	c.Assert(err, jc.ErrorIsNil)
	setStatus, err = machine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(setStatus, gc.Equals, "new status")
}

func (s *InstancePollerSuite) TestSetInstanceStatusFailure(c *gc.C) {
	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1")
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.SetInstanceStatus()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: "foo"})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: ""})

	result, err := s.api.SetInstanceStatus(params.SetInstancesStatus{
		Entities: []params.InstanceStatus{
			{Tag: "machine-1", Status: "new"},
			{Tag: "machine-2", Status: "invalid"},
			{Tag: "machine-3", Status: ""},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.machineErrorResults)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckFindEntityCall(c, 1, "2")
	s.st.CheckCall(c, 2, "SetInstanceStatus", "invalid")
	s.st.CheckFindEntityCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestAreManuallyProvisionedSuccess(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", isManual: true})
	s.st.SetMachineInfo(c, machineInfo{id: "2", isManual: false})

	result, err := s.api.AreManuallyProvisioned(s.mixedEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: true},
			{Result: false},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"service-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckCall(c, 1, "IsManual")
	s.st.CheckFindEntityCall(c, 2, "2")
	s.st.CheckCall(c, 3, "IsManual")
	s.st.CheckFindEntityCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestAreManuallyProvisionedFailure(c *gc.C) {
	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1")
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.IsManual()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", isManual: true})
	s.st.SetMachineInfo(c, machineInfo{id: "2", isManual: false})

	result, err := s.api.AreManuallyProvisioned(s.machineEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckFindEntityCall(c, 1, "2")
	s.st.CheckCall(c, 2, "IsManual")
	s.st.CheckFindEntityCall(c, 3, "3")
}
