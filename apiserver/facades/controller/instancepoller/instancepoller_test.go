// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/instancepoller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
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

	clock clock.Clock
}

var _ = gc.Suite(&InstancePollerSuite{})

func (s *InstancePollerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.authoriser = apiservertesting.FakeAuthorizer{
		Controller: true,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.st = NewMockState()
	instancepoller.PatchState(s, s.st)

	var err error
	s.clock = testclock.NewClock(time.Now())
	s.api, err = instancepoller.NewInstancePollerAPI(nil, nil, s.resources, s.authoriser, s.clock)
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
			{Tag: "application-unknown"},
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
			{Error: apiservertesting.ServerError(`"application-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}}
}

func (s *InstancePollerSuite) TestNewInstancePollerAPIRequiresController(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Controller = false
	api, err := instancepoller.NewInstancePollerAPI(nil, nil, s.resources, anAuthoriser, s.clock)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *InstancePollerSuite) TestModelConfigFailure(c *gc.C) {
	s.st.SetErrors(errors.New("boom"))

	result, err := s.api.ModelConfig()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, jc.DeepEquals, params.ModelConfigResult{})

	s.st.CheckCallNames(c, "ModelConfig")
}

func (s *InstancePollerSuite) TestModelConfigSuccess(c *gc.C) {
	modelConfig := coretesting.ModelConfig(c)
	s.st.SetConfig(c, modelConfig)

	result, err := s.api.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ModelConfigResult{
		Config: modelConfig.AllAttrs(),
	})

	s.st.CheckCallNames(c, "ModelConfig")
}

func (s *InstancePollerSuite) TestWatchForModelConfigChangesFailure(c *gc.C) {
	// Force the Changes() method of the mock watcher to return a
	// closed channel by setting an error.
	s.st.SetErrors(errors.New("boom"))

	result, err := s.api.WatchForModelConfigChanges()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{})

	c.Assert(s.resources.Count(), gc.Equals, 0) // no watcher registered
	s.st.CheckCallNames(c, "WatchForModelConfigChanges")
}

func (s *InstancePollerSuite) TestWatchForModelConfigChangesSuccess(c *gc.C) {
	result, err := s.api.WatchForModelConfigChanges()
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

	s.st.CheckCallNames(c, "WatchForModelConfigChanges")

	// Try changing the config to verify an event is reported.
	modelConfig := coretesting.ModelConfig(c)
	s.st.SetConfig(c, modelConfig)
	wc.AssertOneChange()
}

func (s *InstancePollerSuite) TestWatchModelMachinesFailure(c *gc.C) {
	s.assertMachineWatcherFails(c, "WatchModelMachines", s.api.WatchModelMachines)
}

func (s *InstancePollerSuite) TestWatchModelMachinesSuccess(c *gc.C) {
	s.assertMachineWatcherSucceeds(c, "WatchModelMachines", s.api.WatchModelMachines)
}

func (s *InstancePollerSuite) TestWatchModelMachineStartTimesFailure(c *gc.C) {
	s.assertMachineWatcherFails(c, "WatchModelMachineStartTimes", s.api.WatchModelMachineStartTimes)
}

func (s *InstancePollerSuite) TestWatchModelMachineStartTimesSuccess(c *gc.C) {
	s.assertMachineWatcherFails(c, "WatchModelMachineStartTimes", s.api.WatchModelMachineStartTimes)
}

func (s *InstancePollerSuite) assertMachineWatcherFails(c *gc.C, watchFacadeName string, getWatcherFn func() (params.StringsWatchResult, error)) {
	// Force the Changes() method of the mock watcher to return a
	// closed channel by setting an error.
	s.st.SetErrors(errors.Errorf("boom"))

	result, err := getWatcherFn()
	c.Assert(err, gc.ErrorMatches, "cannot obtain initial model machines: boom")
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{})

	c.Assert(s.resources.Count(), gc.Equals, 0) // no watcher registered
	s.st.CheckCallNames(c, watchFacadeName)
}

func (s *InstancePollerSuite) assertMachineWatcherSucceeds(c *gc.C, watchFacadeName string, getWatcherFn func() (params.StringsWatchResult, error)) {
	// Add a couple of machines.
	s.st.SetMachineInfo(c, machineInfo{id: "2"})
	s.st.SetMachineInfo(c, machineInfo{id: "1"})

	expectedResult := params.StringsWatchResult{
		Error:            nil,
		StringsWatcherId: "1",
		Changes:          []string{"1", "2"}, // initial event (sorted ids)
	}
	result, err := getWatcherFn()
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

	s.st.CheckCallNames(c, watchFacadeName)

	// Add another watcher to verify events coalescence.
	result, err = getWatcherFn()
	c.Assert(err, jc.ErrorIsNil)
	expectedResult.StringsWatcherId = "2"
	c.Assert(result, jc.DeepEquals, expectedResult)
	s.st.CheckCallNames(c, watchFacadeName, watchFacadeName)
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
			{Life: life.Alive},
			{Life: life.Dying},
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
			{Life: life.Dead},
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
	s1 := status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Data: map[string]interface{}{
			"price": 4.2,
			"bool":  false,
			"bar":   []string{"a", "b"},
		},
		Since: &now,
	}
	s2 := status.StatusInfo{}
	s.st.SetMachineInfo(c, machineInfo{id: "1", status: s1})
	s.st.SetMachineInfo(c, machineInfo{id: "2", status: s2})

	result, err := s.api.Status(s.mixedEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{
				Status: status.Error.String(),
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
	addrs := network.NewSpaceAddresses("0.1.2.3", "127.0.0.1", "8.8.8.8")
	s.st.SetMachineInfo(c, machineInfo{id: "1", providerAddresses: addrs})
	s.st.SetMachineInfo(c, machineInfo{id: "2", providerAddresses: nil})

	result, err := s.api.ProviderAddresses(s.mixedEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.MachineAddressesResults{
		Results: []params.MachineAddressesResult{
			{Addresses: toParamAddresses(addrs)},
			{Addresses: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"application-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}},
	)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckCall(c, 1, "ProviderAddresses")
	s.st.CheckCall(c, 2, "AllSpaceInfos")
	s.st.CheckMachineCall(c, 3, "2")
	s.st.CheckCall(c, 4, "ProviderAddresses")
	s.st.CheckMachineCall(c, 5, "42")
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

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckMachineCall(c, 1, "2")
	s.st.CheckCall(c, 2, "ProviderAddresses")
	s.st.CheckMachineCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestSetProviderAddressesSuccess(c *gc.C) {
	oldAddrs := network.NewSpaceAddresses("0.1.2.3", "127.0.0.1", "8.8.8.8")
	newAddrs := network.NewSpaceAddresses("1.2.3.4", "8.4.4.8", "2001:db8::")
	s.st.SetMachineInfo(c, machineInfo{id: "1", providerAddresses: oldAddrs})
	s.st.SetMachineInfo(c, machineInfo{id: "2", providerAddresses: nil})

	result, err := s.api.SetProviderAddresses(params.SetMachinesAddresses{
		MachineAddresses: []params.MachineAddresses{
			{Tag: "machine-1", Addresses: nil},
			{Tag: "machine-2", Addresses: toParamAddresses(newAddrs)},
			{Tag: "machine-42"},
			{Tag: "application-unknown"},
			{Tag: "invalid-tag"},
			{Tag: "unit-missing-1"},
			{Tag: ""},
			{Tag: "42"},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.mixedErrorResults)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckSetProviderAddressesCall(c, 1, []network.SpaceAddress{})
	s.st.CheckMachineCall(c, 2, "2")
	s.st.CheckCall(c, 3, "AllSpaceInfos")
	s.st.CheckSetProviderAddressesCall(c, 4, newAddrs)
	s.st.CheckMachineCall(c, 5, "42")

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
	oldAddrs := network.NewSpaceAddresses("0.1.2.3", "127.0.0.1", "8.8.8.8")
	newAddrs := network.NewSpaceAddresses("1.2.3.4", "8.4.4.8", "2001:db8::")
	s.st.SetMachineInfo(c, machineInfo{id: "1", providerAddresses: oldAddrs})
	s.st.SetMachineInfo(c, machineInfo{id: "2", providerAddresses: nil})

	result, err := s.api.SetProviderAddresses(params.SetMachinesAddresses{
		MachineAddresses: []params.MachineAddresses{
			{Tag: "machine-1"},
			{Tag: "machine-2", Addresses: toParamAddresses(newAddrs)},
			{Tag: "machine-3"},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, s.machineErrorResults)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckMachineCall(c, 1, "2")
	s.st.CheckCall(c, 2, "AllSpaceInfos")
	s.st.CheckSetProviderAddressesCall(c, 3, newAddrs)
	s.st.CheckMachineCall(c, 4, "3")

	// Ensure machine 2 wasn't updated.
	machine, err := s.st.Machine("2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.ProviderAddresses(), gc.HasLen, 0)
}

func toParamAddresses(addrs network.SpaceAddresses) []params.Address {
	paramAddrs := make([]params.Address, len(addrs))
	for i, addr := range addrs {
		paramAddrs[i] = params.Address{
			Value: addr.Value,
			Type:  string(addr.Type),
			Scope: string(addr.Scope),
		}
	}
	return paramAddrs
}

func (s *InstancePollerSuite) TestInstanceStatusSuccess(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: statusInfo("")})

	result, err := s.api.InstanceStatus(s.mixedEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Status: "foo"},
			{Status: ""},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"application-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		},
	},
	)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckCall(c, 1, "InstanceStatus")
	s.st.CheckMachineCall(c, 2, "2")
	s.st.CheckCall(c, 3, "InstanceStatus")
	s.st.CheckMachineCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestInstanceStatusFailure(c *gc.C) {
	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1")
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.InstanceStatus()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: statusInfo("")})

	result, err := s.api.InstanceStatus(s.machineEntities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckMachineCall(c, 1, "2")
	s.st.CheckCall(c, 2, "InstanceStatus")
	s.st.CheckMachineCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestSetInstanceStatusSuccess(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: statusInfo("")})

	result, err := s.api.SetInstanceStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-1", Status: ""},
			{Tag: "machine-2", Status: "new status"},
			{Tag: "machine-42", Status: ""},
			{Tag: "application-unknown", Status: ""},
			{Tag: "invalid-tag", Status: ""},
			{Tag: "unit-missing-1", Status: ""},
			{Tag: "", Status: ""},
			{Tag: "42", Status: ""},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.mixedErrorResults)

	now := s.clock.Now()
	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckCall(c, 1, "SetInstanceStatus", status.StatusInfo{Status: status.Status(""), Since: &now})
	s.st.CheckMachineCall(c, 2, "2")
	s.st.CheckCall(c, 3, "SetInstanceStatus", status.StatusInfo{Status: status.Status("new status"), Since: &now})
	s.st.CheckMachineCall(c, 4, "42")

	// Ensure machines were updated.
	machine, err := s.st.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	// TODO (perrito666) there should not be an empty StatusInfo here,
	// this is certainly a smell.
	setStatus, err := machine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	setStatus.Since = nil
	c.Assert(setStatus, gc.DeepEquals, status.StatusInfo{})

	machine, err = s.st.Machine("2")
	c.Assert(err, jc.ErrorIsNil)
	setStatus, err = machine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	setStatus.Since = nil
	c.Assert(setStatus, gc.DeepEquals, status.StatusInfo{Status: "new status"})
}

func (s *InstancePollerSuite) TestSetInstanceStatusFailure(c *gc.C) {
	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1")
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.SetInstanceStatus()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: statusInfo("")})

	result, err := s.api.SetInstanceStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-1", Status: "new"},
			{Tag: "machine-2", Status: "invalid"},
			{Tag: "machine-3", Status: ""},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.machineErrorResults)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckMachineCall(c, 1, "2")
	now := s.clock.Now()
	s.st.CheckCall(c, 2, "SetInstanceStatus", status.StatusInfo{Status: "invalid", Since: &now})
	s.st.CheckMachineCall(c, 3, "3")
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
			{Error: apiservertesting.ServerError(`"application-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}},
	)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckCall(c, 1, "IsManual")
	s.st.CheckMachineCall(c, 2, "2")
	s.st.CheckCall(c, 3, "IsManual")
	s.st.CheckMachineCall(c, 4, "42")
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

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckMachineCall(c, 1, "2")
	s.st.CheckCall(c, 2, "IsManual")
	s.st.CheckMachineCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestSetProviderNetworkConfigSuccess(c *gc.C) {
	s.st.SetSpaceInfo(network.SpaceInfos{
		{ID: network.AlphaSpaceId, Name: network.AlphaSpaceName},
		{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{CIDR: "10.0.0.0/24"}}},
		{ID: "2", Name: "my-space-on-maas", Subnets: []network.SubnetInfo{{CIDR: "10.73.37.0/24", ProviderId: "my-space-on-maas"}}},
	})
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})

	results, err := s.api.SetProviderNetworkConfig(params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						Addresses: []params.Address{
							{
								Value: "10.0.0.42",
								Scope: "local-cloud",
							},
							{
								Value:           "10.73.37.111",
								Scope:           "local-cloud",
								ProviderSpaceID: "my-space-on-maas",
							},
							// This address does not match any of the CIDRs in
							// the known spaces; we expect this to end up in
							// alpha space.
							{
								Value: "192.168.0.1",
								Scope: "local-cloud",
							},
						},
						ShadowAddresses: []params.Address{
							{
								Value: "1.1.1.42",
								Scope: "public",
							},
						},
					},
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Modified, jc.IsTrue)
	c.Assert(result.Addresses, gc.HasLen, 4)
	c.Assert(result.Addresses[0], gc.DeepEquals, params.Address{
		Value:     "10.0.0.42",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "space1",
	})
	c.Assert(result.Addresses[1], gc.DeepEquals, params.Address{
		Value:     "10.73.37.111",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "my-space-on-maas",
	})
	c.Assert(result.Addresses[2], gc.DeepEquals, params.Address{
		Value:     "192.168.0.1",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "alpha",
	})
	c.Assert(result.Addresses[3], gc.DeepEquals, params.Address{
		Value:     "1.1.1.42",
		Type:      "ipv4",
		Scope:     "public",
		SpaceName: "alpha",
	})

	machine, err := s.st.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	providerAddrs := machine.ProviderAddresses()

	c.Assert(providerAddrs, gc.DeepEquals, network.SpaceAddresses{
		makeSpaceAddress("10.0.0.42", network.ScopeCloudLocal, "1"),
		makeSpaceAddress("10.73.37.111", network.ScopeCloudLocal, "2"),
		makeSpaceAddress("192.168.0.1", network.ScopeCloudLocal, network.AlphaSpaceId),
		makeSpaceAddress("1.1.1.42", network.ScopePublic, network.AlphaSpaceId),
	})
}

func (s *InstancePollerSuite) TestSetProviderNetworkConfigNoChange(c *gc.C) {
	s.st.SetSpaceInfo(network.SpaceInfos{
		{ID: network.AlphaSpaceId, Name: network.AlphaSpaceName},
		{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{CIDR: "10.0.0.0/24"}}},
		{ID: "2", Name: "my-space-on-maas", Subnets: []network.SubnetInfo{{CIDR: "10.73.37.0/24", ProviderId: "my-space-on-maas"}}},
	})
	s.st.SetMachineInfo(c, machineInfo{
		id:             "1",
		instanceStatus: statusInfo("foo"),
		providerAddresses: network.SpaceAddresses{
			makeSpaceAddress("10.0.0.42", network.ScopeCloudLocal, "1"),
			makeSpaceAddress("10.73.37.111", network.ScopeCloudLocal, "2"),
			makeSpaceAddress("192.168.0.1", network.ScopeCloudLocal, network.AlphaSpaceId),
			makeSpaceAddress("1.1.1.42", network.ScopePublic, network.AlphaSpaceId),
		},
	})

	results, err := s.api.SetProviderNetworkConfig(params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						Addresses: []params.Address{
							{
								Value: "10.0.0.42",
								Scope: "local-cloud",
							},
							{
								Value:           "10.73.37.111",
								Scope:           "local-cloud",
								ProviderSpaceID: "my-space-on-maas",
							},
							// This address does not match any of the CIDRs in
							// the known spaces; we expect this to end up in
							// alpha space.
							{
								Value: "192.168.0.1",
								Scope: "local-cloud",
							},
						},
						ShadowAddresses: []params.Address{
							{
								Value: "1.1.1.42",
								Scope: "public",
							},
						},
					},
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Modified, jc.IsFalse)
	c.Assert(result.Addresses, gc.HasLen, 4)
	c.Assert(result.Addresses[0], gc.DeepEquals, params.Address{
		Value:     "10.0.0.42",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "space1",
	})
	c.Assert(result.Addresses[1], gc.DeepEquals, params.Address{
		Value:     "10.73.37.111",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "my-space-on-maas",
	})
	c.Assert(result.Addresses[2], gc.DeepEquals, params.Address{
		Value:     "192.168.0.1",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "alpha",
	})
	c.Assert(result.Addresses[3], gc.DeepEquals, params.Address{
		Value:     "1.1.1.42",
		Type:      "ipv4",
		Scope:     "public",
		SpaceName: "alpha",
	})
}

func (s *InstancePollerSuite) TestSetProviderNetworkConfigBackfillsProviderIDs(c *gc.C) {
	s.st.SetSpaceInfo(network.SpaceInfos{
		{ID: network.AlphaSpaceId, Name: network.AlphaSpaceName},
		{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{CIDR: "10.0.0.0/24"}}},
		{ID: "2", Name: "my-space-on-maas", Subnets: []network.SubnetInfo{{CIDR: "10.73.37.0/24", ProviderId: "my-space-on-maas"}}},
	})
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})
	s.st.SetMachineLinkLayerDevices(c, "1", []instancepoller.StateLinkLayerDevice{
		// This device will be matched and its provider IDs backfilled
		mockLinkLayerDevice{
			name:        "foo0",
			mtu:         1234,
			devType:     network.EthernetDevice,
			macAddress:  "aa:bb:cc:dd:ee:ff",
			isAutoStart: true,
			isUp:        true,
			parentName:  "bond0",
			addresses: []instancepoller.StateLinkLayerDeviceAddress{
				&mockLinkLayerDeviceAddress{
					configMethod:     network.StaticAddress,
					subnetCIDR:       "172.31.32.0/24",
					dnsServers:       []string{"1.1.1.1"},
					dnsSearchDomains: []string{"cloud"},
					gatewayAddress:   "172.31.32.1",
					isDefaultGateway: true,
					value:            "172.31.32.33",
				},
			},
		},
		// This device will not be matched
		mockLinkLayerDevice{
			name:        "lo",
			mtu:         1234,
			devType:     network.EthernetDevice,
			macAddress:  "aa:bb:cc:dd:ee:00",
			isAutoStart: true,
			addresses: []instancepoller.StateLinkLayerDeviceAddress{
				&mockLinkLayerDeviceAddress{
					configMethod:     network.LoopbackAddress,
					subnetCIDR:       "127.0.0.0/24",
					dnsServers:       []string{"1.1.1.1"},
					dnsSearchDomains: []string{"cloud"},
					gatewayAddress:   "172.31.32.1",
					isDefaultGateway: false,
					value:            "127.0.0.1",
				},
			},
		},
	})

	results, err := s.api.SetProviderNetworkConfig(params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						ProviderId:        "eni-1234",
						ProviderAddressId: "addr-404",
						ProviderSubnetId:  "subnet-f00f",
						ProviderNetworkId: "net-cafe",
						Addresses: []params.Address{
							{
								Value: "172.31.32.33",
								Scope: "local-cloud",
							},
						},
					},
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Modified, jc.IsTrue)

	// Verify that the provider IDs were indeed backfilled for the matched
	// ethernet device.
	s.st.CheckCall(c, 6, "SetParentLinkLayerDevicesBeforeTheirChildren", state.LinkLayerDeviceArgs{
		Name:        "foo0",
		MTU:         1234,
		ProviderID:  "eni-1234", // backfilled
		Type:        network.EthernetDevice,
		MACAddress:  "aa:bb:cc:dd:ee:ff",
		IsAutoStart: true,
		IsUp:        true,
		ParentName:  "bond0",
	})
	s.st.CheckCall(c, 8, "SetDevicesAddressesIdempotently", state.LinkLayerDeviceAddress{
		DeviceName:        "foo0",
		ConfigMethod:      network.StaticAddress,
		ProviderID:        "addr-404",    // backfilled
		ProviderNetworkID: "net-cafe",    // backfilled
		ProviderSubnetID:  "subnet-f00f", //backfilled
		CIDRAddress:       "172.31.32.33/24",
		DNSServers:        []string{"1.1.1.1"},
		DNSSearchDomains:  []string{"cloud"},
		GatewayAddress:    "172.31.32.1",
		IsDefaultGateway:  true,
	})
}

func makeSpaceAddress(ip string, scope network.Scope, spaceID string) network.SpaceAddress {
	addr := network.NewScopedSpaceAddress(ip, scope)
	addr.SpaceID = spaceID
	return addr
}

func statusInfo(st string) status.StatusInfo {
	return status.StatusInfo{Status: status.Status(st)}
}
