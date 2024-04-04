// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/common/networkingcommon/mocks"
	"github.com/juju/juju/apiserver/facades/controller/instancepoller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	jujutesting "github.com/juju/juju/testing"
)

type InstancePollerSuite struct {
	testing.IsolationSuite

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
	s.IsolationSuite.SetUpTest(c)

	s.authoriser = apiservertesting.FakeAuthorizer{
		Controller: true,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.st = NewMockState()
	instancepoller.PatchState(s, s.st)

	var err error
	s.clock = testclock.NewClock(time.Now())
	s.api, err = instancepoller.NewInstancePollerAPI(nil, nil, s.resources, s.authoriser, s.clock, loggo.GetLogger("juju.apiserver.instancepoller"), nil)
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
	api, err := instancepoller.NewInstancePollerAPI(nil, nil, s.resources, anAuthoriser, s.clock, loggo.GetLogger("juju.apiserver.instancepoller"), status.NoopStatusHistoryRecorder)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *InstancePollerSuite) TestModelConfigFailure(c *gc.C) {
	s.st.SetErrors(errors.New("boom"))

	result, err := s.api.ModelConfig(context.Background())
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, jc.DeepEquals, params.ModelConfigResult{})

	s.st.CheckCallNames(c, "ModelConfig")
}

func (s *InstancePollerSuite) TestModelConfigSuccess(c *gc.C) {
	modelConfig := jujutesting.ModelConfig(c)
	s.st.SetConfig(c, modelConfig)

	result, err := s.api.ModelConfig(context.Background())
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

	result, err := s.api.WatchForModelConfigChanges(context.Background())
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{})

	c.Assert(s.resources.Count(), gc.Equals, 0) // no watcher registered
	s.st.CheckCallNames(c, "WatchForModelConfigChanges")
}

func (s *InstancePollerSuite) TestWatchForModelConfigChangesSuccess(c *gc.C) {
	result, err := s.api.WatchForModelConfigChanges(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{
		Error: nil, NotifyWatcherId: "1",
	})

	// Verify the watcher resource was registered.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the watcher has consumed the initial event
	wc := statetesting.NewNotifyWatcherC(c, resource.(state.NotifyWatcher))
	wc.AssertNoChange()

	s.st.CheckCallNames(c, "WatchForModelConfigChanges")

	// Try changing the config to verify an event is reported.
	modelConfig := jujutesting.ModelConfig(c)
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

func (s *InstancePollerSuite) assertMachineWatcherFails(c *gc.C, watchFacadeName string, getWatcherFn func(context.Context) (params.StringsWatchResult, error)) {
	// Force the Changes() method of the mock watcher to return a
	// closed channel by setting an error.
	s.st.SetErrors(errors.Errorf("boom"))

	result, err := getWatcherFn(context.Background())
	c.Assert(err, gc.ErrorMatches, "cannot obtain initial model machines: boom")
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{})

	c.Assert(s.resources.Count(), gc.Equals, 0) // no watcher registered
	s.st.CheckCallNames(c, watchFacadeName)
}

func (s *InstancePollerSuite) assertMachineWatcherSucceeds(c *gc.C, watchFacadeName string, getWatcherFn func(context.Context) (params.StringsWatchResult, error)) {
	// Add a couple of machines.
	s.st.SetMachineInfo(c, machineInfo{id: "2"})
	s.st.SetMachineInfo(c, machineInfo{id: "1"})

	expectedResult := params.StringsWatchResult{
		Error:            nil,
		StringsWatcherId: "1",
		Changes:          []string{"1", "2"}, // initial event (sorted ids)
	}
	result, err := getWatcherFn(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expectedResult)

	// Verify the watcher resource was registered.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource1 := s.resources.Get("1")
	defer func() {
		if resource1 != nil {
			workertest.CleanKill(c, resource1)
		}
	}()

	// Check that the watcher has consumed the initial event
	wc1 := statetesting.NewStringsWatcherC(c, resource1.(state.StringsWatcher))
	wc1.AssertNoChange()

	s.st.CheckCallNames(c, watchFacadeName)

	// Add another watcher to verify events coalescence.
	result, err = getWatcherFn(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	expectedResult.StringsWatcherId = "2"
	c.Assert(result, jc.DeepEquals, expectedResult)
	s.st.CheckCallNames(c, watchFacadeName, watchFacadeName)
	c.Assert(s.resources.Count(), gc.Equals, 2)
	resource2 := s.resources.Get("2")
	defer workertest.CleanKill(c, resource2)
	wc2 := statetesting.NewStringsWatcherC(c, resource2.(state.StringsWatcher))
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
	resource1.Kill()
	c.Assert(resource1.Wait(), jc.ErrorIsNil)
	wc1.AssertClosed()
	resource1 = nil
}

func (s *InstancePollerSuite) TestLifeSuccess(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", life: state.Alive})
	s.st.SetMachineInfo(c, machineInfo{id: "2", life: state.Dying})

	result, err := s.api.Life(context.Background(), s.mixedEntities)
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

	result, err := s.api.Life(context.Background(), s.machineEntities)
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

	result, err := s.api.InstanceId(context.Background(), s.mixedEntities)
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

	result, err := s.api.InstanceId(context.Background(), s.machineEntities)
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

	result, err := s.api.Status(context.Background(), s.mixedEntities)
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

	result, err := s.api.Status(context.Background(), s.machineEntities)
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

	result, err := s.api.ProviderAddresses(context.Background(), s.mixedEntities)
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

	result, err := s.api.ProviderAddresses(context.Background(), s.machineEntities)
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
	newAddrs := network.SpaceAddresses{
		network.NewSpaceAddress("1.2.3.4", network.WithCIDR("1.2.3.0/24")),
		network.NewSpaceAddress("8.4.4.8", network.WithCIDR("8.4.4.0/24")),
		network.NewSpaceAddress("2001:db8::"),
	}

	s.st.SetMachineInfo(c, machineInfo{id: "1", providerAddresses: oldAddrs})
	s.st.SetMachineInfo(c, machineInfo{id: "2", providerAddresses: nil})

	result, err := s.api.SetProviderAddresses(context.Background(), params.SetMachinesAddresses{
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

	s.st.CheckMachineCall(c, 1, "1")
	s.st.CheckSetProviderAddressesCall(c, 2, []network.SpaceAddress{})
	s.st.CheckMachineCall(c, 3, "2")
	s.st.CheckCall(c, 4, "AllSpaceInfos")
	s.st.CheckSetProviderAddressesCall(c, 5, newAddrs)
	s.st.CheckMachineCall(c, 6, "42")

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

	result, err := s.api.SetProviderAddresses(context.Background(), params.SetMachinesAddresses{
		MachineAddresses: []params.MachineAddresses{
			{Tag: "machine-1"},
			{Tag: "machine-2", Addresses: toParamAddresses(newAddrs)},
			{Tag: "machine-3"},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, s.machineErrorResults)

	s.st.CheckMachineCall(c, 1, "1")
	s.st.CheckMachineCall(c, 2, "2")
	s.st.CheckCall(c, 3, "AllSpaceInfos")
	s.st.CheckSetProviderAddressesCall(c, 4, newAddrs)
	s.st.CheckMachineCall(c, 5, "3")

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
			CIDR:  addr.CIDR,
		}
	}
	return paramAddrs
}

func (s *InstancePollerSuite) TestInstanceStatusSuccess(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: statusInfo("")})

	result, err := s.api.InstanceStatus(context.Background(), s.mixedEntities)
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

	result, err := s.api.InstanceStatus(context.Background(), s.machineEntities)
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

	result, err := s.api.SetInstanceStatus(context.Background(), params.SetStatus{
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
	s.st.CheckCall(c, 1, "SetInstanceStatus", status.StatusInfo{Status: "", Since: &now}, (status.StatusHistoryRecorder)(nil))
	s.st.CheckMachineCall(c, 2, "2")
	s.st.CheckCall(c, 3, "SetInstanceStatus", status.StatusInfo{Status: "new status", Since: &now}, (status.StatusHistoryRecorder)(nil))
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

	result, err := s.api.SetInstanceStatus(context.Background(), params.SetStatus{
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
	s.st.CheckCall(c, 2, "SetInstanceStatus", status.StatusInfo{Status: "invalid", Since: &now}, (status.StatusHistoryRecorder)(nil))
	s.st.CheckMachineCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestAreManuallyProvisionedSuccess(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", isManual: true})
	s.st.SetMachineInfo(c, machineInfo{id: "2", isManual: false})

	result, err := s.api.AreManuallyProvisioned(context.Background(), s.mixedEntities)
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

	result, err := s.api.AreManuallyProvisioned(context.Background(), s.machineEntities)
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
	s.setDefaultSpaceInfo()

	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})

	results, err := s.api.SetProviderNetworkConfig(context.Background(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						// TODO (manadart 2021-05-31): This tests that we
						// consider the individual CIDRs and not the deprecated
						// CIDR for the device.
						// Remove for Juju 3/4.
						CIDR: "10.0.0.0/24",
						Addresses: []params.Address{
							{
								Value: "10.0.0.42",
								Scope: "local-cloud",
								CIDR:  "10.0.0.0/24",
							},
							{
								Value: "10.73.37.110",
								Scope: "local-cloud",
								CIDR:  "10.73.37.0/24",
							},
							// Resolved by provider's space ID.
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
	c.Assert(result.Addresses, gc.HasLen, 5)
	c.Assert(result.Addresses[0], gc.DeepEquals, params.Address{
		Value:     "10.0.0.42",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "space1",
	})
	c.Assert(result.Addresses[1], gc.DeepEquals, params.Address{
		Value:     "10.73.37.110",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "my-space-on-maas",
	})
	c.Assert(result.Addresses[2], gc.DeepEquals, params.Address{
		Value:     "10.73.37.111",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "my-space-on-maas",
	})
	c.Assert(result.Addresses[3], gc.DeepEquals, params.Address{
		Value:     "192.168.0.1",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "alpha",
	})
	c.Assert(result.Addresses[4], gc.DeepEquals, params.Address{
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
		makeSpaceAddress("10.73.37.110", network.ScopeCloudLocal, "2"),
		makeSpaceAddress("10.73.37.111", network.ScopeCloudLocal, "2"),
		makeSpaceAddress("192.168.0.1", network.ScopeCloudLocal, network.AlphaSpaceId),
		makeSpaceAddress("1.1.1.42", network.ScopePublic, network.AlphaSpaceId),
	})
}

func (s *InstancePollerSuite) TestSetProviderNetworkConfigNoChange(c *gc.C) {
	s.setDefaultSpaceInfo()

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

	results, err := s.api.SetProviderNetworkConfig(context.Background(), params.SetProviderNetworkConfig{
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

func (s *InstancePollerSuite) TestSetProviderNetworkConfigNotAlive(c *gc.C) {
	s.st.SetMachineInfo(c, machineInfo{id: "1", life: state.Dying})

	results, err := s.api.SetProviderNetworkConfig(context.Background(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{{
			Tag: "machine-1",
			Configs: []params.NetworkConfig{{
				Addresses: []params.Address{{Value: "10.0.0.42", Scope: "local-cloud"}},
			}},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, params.SetProviderNetworkConfigResults{
		Results: []params.SetProviderNetworkConfigResult{{}},
	})

	// We should just return after seeing that the machine is dying.
	s.st.Stub.CheckCallNames(c, "AllSpaceInfos", "Machine", "Life", "Id")
}

func (s *InstancePollerSuite) TestSetProviderNetworkConfigRelinquishUnseen(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.setDefaultSpaceInfo()

	// Hardware address not matched.
	dev := mocks.NewMockLinkLayerDevice(ctrl)
	dExp := dev.EXPECT()
	dExp.MACAddress().Return("01:01:01:01:01:01").MinTimes(1)
	dExp.Name().Return("eth0").MinTimes(1)
	dExp.SetProviderIDOps(network.Id("")).Return([]txn.Op{{C: "dev-provider-id"}}, nil)

	// Address should be set back to machine origin.
	addr := mocks.NewMockLinkLayerAddress(ctrl)
	addr.EXPECT().DeviceName().Return("eth0")
	addr.EXPECT().SetOriginOps(network.OriginMachine).Return([]txn.Op{{C: "address-origin-manual"}})

	s.st.SetMachineInfo(c, machineInfo{
		id:               "1",
		instanceStatus:   statusInfo("foo"),
		linkLayerDevices: []networkingcommon.LinkLayerDevice{dev},
		addresses:        []networkingcommon.LinkLayerAddress{addr},
	})

	result, err := s.api.SetProviderNetworkConfig(context.Background(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag:     "machine-1",
				Configs: []params.NetworkConfig{{MACAddress: "00:00:00:00:00:00"}},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	var buildCalled bool
	for _, call := range s.st.Calls() {
		if call.FuncName == "ApplyOperation.Build" {
			buildCalled = true
			c.Check(call.Args, gc.DeepEquals, []interface{}{[]txn.Op{
				{C: "machine-alive"},
				{C: "dev-provider-id"},
				{C: "address-origin-manual"},
			}})
		}
	}
	c.Assert(buildCalled, jc.IsTrue)
}

func (s *InstancePollerSuite) TestSetProviderNetworkClaimProviderOrigin(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.setDefaultSpaceInfo()

	// Hardware address will match; provider ID will be set.
	dev := mocks.NewMockLinkLayerDevice(ctrl)
	dExp := dev.EXPECT()
	dExp.MACAddress().Return("00:00:00:00:00:00").MinTimes(1)
	dExp.Name().Return("eth0").MinTimes(1)
	dExp.ProviderID().Return(network.Id(""))
	dExp.SetProviderIDOps(network.Id("p-dev")).Return([]txn.Op{{C: "dev-provider-id"}}, nil)

	// Address matched on device/value will have provider IDs set.
	addr := mocks.NewMockLinkLayerAddress(ctrl)
	aExp := addr.EXPECT()
	aExp.DeviceName().Return("eth0")
	aExp.Value().Return("10.0.0.42")
	aExp.SetProviderIDOps(network.Id("p-addr")).Return([]txn.Op{{C: "addr-provider-id"}}, nil)
	aExp.SetProviderNetIDsOps(network.Id("p-net"), network.Id("p-sub")).Return([]txn.Op{{C: "addr-provider-net-ids"}})

	s.st.SetMachineInfo(c, machineInfo{
		id:               "1",
		instanceStatus:   statusInfo("foo"),
		linkLayerDevices: []networkingcommon.LinkLayerDevice{dev},
		addresses:        []networkingcommon.LinkLayerAddress{addr},
	})

	result, err := s.api.SetProviderNetworkConfig(context.Background(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						// This should still be matched based on hardware address.
						InterfaceName:     "",
						MACAddress:        "00:00:00:00:00:00",
						ProviderId:        "p-dev",
						ProviderAddressId: "p-addr",
						ProviderNetworkId: "p-net",
						ProviderSubnetId:  "p-sub",
						CIDR:              "10.0.0.0/24",
						Addresses:         []params.Address{{Value: "10.0.0.42"}},
					},
					{
						// A duplicate (MAC and addresses) should make no difference.
						InterfaceName:     "",
						MACAddress:        "00:00:00:00:00:00",
						ProviderId:        "p-dev",
						ProviderAddressId: "p-addr",
						ProviderNetworkId: "p-net",
						ProviderSubnetId:  "p-sub",
						CIDR:              "10.0.0.0/24",
						Addresses:         []params.Address{{Value: "10.0.0.42"}},
					},
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	var buildCalled bool
	for _, call := range s.st.Calls() {
		if call.FuncName == "ApplyOperation.Build" {
			buildCalled = true
			c.Check(call.Args, gc.DeepEquals, []interface{}{[]txn.Op{
				{C: "machine-alive"},
				{C: "dev-provider-id"},
				{C: "addr-provider-id"},
				{C: "addr-provider-net-ids"},
			}})
		}
	}
	c.Assert(buildCalled, jc.IsTrue)
}

func (s *InstancePollerSuite) TestSetProviderNetworkProviderIDGoesToEthernetDev(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.setDefaultSpaceInfo()

	// Ethernet device will have the provider ID set.
	ethDev := mocks.NewMockLinkLayerDevice(ctrl)
	ethExp := ethDev.EXPECT()
	ethExp.MACAddress().Return("00:00:00:00:00:00").MinTimes(1)
	ethExp.Name().Return("eth0").MinTimes(1)
	ethExp.ProviderID().Return(network.Id("")).MinTimes(1)
	ethExp.SetProviderIDOps(network.Id("p-dev")).Return([]txn.Op{{C: "dev-provider-id"}}, nil)

	// Bridge has the same MAC, but will not get the provider ID.
	brDev := mocks.NewMockLinkLayerDevice(ctrl)
	brExp := brDev.EXPECT()
	brExp.MACAddress().Return("00:00:00:00:00:00").MinTimes(1)
	brExp.Name().Return("br-eth0").AnyTimes()
	brExp.Type().Return(network.BridgeDevice)

	s.st.SetMachineInfo(c, machineInfo{
		id:               "1",
		instanceStatus:   statusInfo("foo"),
		linkLayerDevices: []networkingcommon.LinkLayerDevice{ethDev, brDev},
		addresses:        []networkingcommon.LinkLayerAddress{},
	})

	result, err := s.api.SetProviderNetworkConfig(context.Background(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						// This should still be matched based on hardware address.
						InterfaceName: "",
						MACAddress:    "00:00:00:00:00:00",
						ProviderId:    "p-dev",
					},
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	var buildCalled bool
	for _, call := range s.st.Calls() {
		if call.FuncName == "ApplyOperation.Build" {
			buildCalled = true
		}
	}
	c.Assert(buildCalled, jc.IsTrue)
}

func (s *InstancePollerSuite) TestSetProviderNetworkProviderIDMultipleRefsError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.setDefaultSpaceInfo()

	ethDev := mocks.NewMockLinkLayerDevice(ctrl)
	ethExp := ethDev.EXPECT()
	ethExp.Name().Return("eth0").MinTimes(1)
	ethExp.ProviderID().Return(network.Id("")).MinTimes(1)
	ethExp.SetProviderIDOps(network.Id("p-dev")).Return([]txn.Op{{C: "dev-provider-id"}}, nil)

	brDev := mocks.NewMockLinkLayerDevice(ctrl)
	brExp := brDev.EXPECT()
	brExp.Name().Return("br-eth0").AnyTimes()
	brExp.ProviderID().Return(network.Id("")).MinTimes(1)
	// Note no calls to SetProviderIDOps.

	s.st.SetMachineInfo(c, machineInfo{
		id:               "1",
		instanceStatus:   statusInfo("foo"),
		linkLayerDevices: []networkingcommon.LinkLayerDevice{ethDev, brDev},
		addresses:        []networkingcommon.LinkLayerAddress{},
	})

	// Same provider ID for both.
	result, err := s.api.SetProviderNetworkConfig(context.Background(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						InterfaceName: "eth0",
						MACAddress:    "aa:00:00:00:00:00",
						ProviderId:    "p-dev",
					},
					{
						InterfaceName: "br-eth0",
						MACAddress:    "bb:00:00:00:00:00",
						ProviderId:    "p-dev",
					},
				},
			},
		},
	})

	// The error is logged but not returned.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	// But we should not have registered a successful call to Build.
	// This returns an error.
	var buildCalled bool
	for _, call := range s.st.Calls() {
		if call.FuncName == "ApplyOperation.Build" {
			buildCalled = true
		}
	}
	c.Assert(buildCalled, jc.IsFalse)
}

func (s *InstancePollerSuite) setDefaultSpaceInfo() {
	s.st.SetSpaceInfo(network.SpaceInfos{
		{ID: network.AlphaSpaceId, Name: network.AlphaSpaceName},
		{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{CIDR: "10.0.0.0/24"}}},
		{ID: "2", Name: "my-space-on-maas", ProviderId: "my-space-on-maas", Subnets: []network.SubnetInfo{{CIDR: "10.73.37.0/24"}}},
	})
}

func makeSpaceAddress(ip string, scope network.Scope, spaceID string) network.SpaceAddress {
	addr := network.NewSpaceAddress(ip, network.WithScope(scope))
	addr.SpaceID = spaceID
	return addr
}

func statusInfo(st string) status.StatusInfo {
	return status.StatusInfo{Status: status.Status(st)}
}
