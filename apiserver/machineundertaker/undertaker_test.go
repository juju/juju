// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/machineundertaker"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type undertakerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&undertakerSuite{})

func (s *undertakerSuite) TestRequiresModelManager(c *gc.C) {
	st := &mockState{}
	_, err := machineundertaker.NewMachineUndertaker(
		st,
		nil,
		apiservertesting.FakeAuthorizer{EnvironManager: false},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	_, err = machineundertaker.NewMachineUndertaker(
		st,
		nil,
		apiservertesting.FakeAuthorizer{EnvironManager: true},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *undertakerSuite) TestAllMachineRemovalsNoResults(c *gc.C) {
	_, _, api := makeApi(c)
	result, err := api.AllMachineRemovals()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.MachineRemovalsResults{})
}

func (s *undertakerSuite) TestAllMachineRemovalsError(c *gc.C) {
	st, _, api := makeApi(c)
	st.SetErrors(errors.New("I don't want to set the world on fire"))
	result, err := api.AllMachineRemovals()
	c.Assert(err, gc.ErrorMatches, "I don't want to set the world on fire")
	c.Assert(result, gc.DeepEquals, params.MachineRemovalsResults{})
}

func (s *undertakerSuite) TestAllMachineRemovals(c *gc.C) {
	st, _, api := makeApi(c)
	st.removals = []*mockRemoval{{machineID: "0", linkLayerDevices: []*mockLinkLayerDevice{{
		name:       "jasper",
		macAddress: "12:34:56:78:9a:bc",
		type_:      "ethernet",
		mtu:        1234,
		providerID: "205",
	}}}, {machineID: "2", linkLayerDevices: []*mockLinkLayerDevice{{
		name:       "lapis",
		macAddress: "12:34:56:78:9a:bd",
		type_:      "ethernet",
		mtu:        1234,
		providerID: "206",
	}}}}

	result, err := api.AllMachineRemovals()

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.MachineRemovalsResults{
		Results: []params.MachineRemoval{{
			MachineTag: "machine-0",
			LinkLayerDevices: []params.NetworkConfig{{
				InterfaceName: "jasper",
				MACAddress:    "12:34:56:78:9a:bc",
				InterfaceType: "ethernet",
				MTU:           1234,
				ProviderId:    "205",
			}}}, {
			MachineTag: "machine-2",
			LinkLayerDevices: []params.NetworkConfig{{
				InterfaceName: "lapis",
				MACAddress:    "12:34:56:78:9a:bd",
				InterfaceType: "ethernet",
				MTU:           1234,
				ProviderId:    "206",
			}}}},
	})
}

func (s *undertakerSuite) TestClearMachineRemovalsWithNonMachineTags(c *gc.C) {
	_, _, api := makeApi(c)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-2"},
		{Tag: "application-a1"},
	}}
	err := api.ClearMachineRemovals(args)
	c.Assert(err, gc.ErrorMatches, `"application-a1" is not a valid machine tag`)
}

func (s *undertakerSuite) TestClearMachineRemovalsWithOtherError(c *gc.C) {
	st, _, api := makeApi(c)
	st.SetErrors(errors.New("boom"))
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-2"},
	}}
	err := api.ClearMachineRemovals(args)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *undertakerSuite) TestClearMachineRemovals(c *gc.C) {
	st, _, api := makeApi(c)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-2"},
		{Tag: "machine-52"},
	}}
	err := api.ClearMachineRemovals(args)
	c.Assert(err, jc.ErrorIsNil)
	st.CheckCallNames(c, "ClearMachineRemovals")
	callArgs := st.Calls()[0].Args
	c.Assert(len(callArgs), gc.Equals, 1)
	values, ok := callArgs[0].([]string)
	c.Assert(ok, jc.IsTrue)
	c.Assert(values, gc.DeepEquals, []string{"2", "52"})
}

func (s *undertakerSuite) TestWatchMachineRemovals(c *gc.C) {
	st, res, api := makeApi(c)

	result := api.WatchMachineRemovals()
	c.Assert(res.Get(result.NotifyWatcherId), gc.NotNil)
	st.CheckCallNames(c, "WatchMachineRemovals")
}

func (s *undertakerSuite) TestWatchMachineRemovalsError(c *gc.C) {
	st, _, api := makeApi(c)
	st.watcherBlowsUp = true
	st.SetErrors(errors.New("oh no!"))

	result := api.WatchMachineRemovals()
	c.Assert(result.Error, gc.ErrorMatches, "oh no!")
	st.CheckCallNames(c, "WatchMachineRemovals")
}

func makeApi(c *gc.C) (*mockState, *common.Resources, *machineundertaker.MachineUndertakerAPI) {
	st := &mockState{Stub: &testing.Stub{}}
	res := common.NewResources()
	api, err := machineundertaker.NewMachineUndertaker(
		st,
		res,
		apiservertesting.FakeAuthorizer{EnvironManager: true},
	)
	c.Assert(err, jc.ErrorIsNil)
	return st, res, api
}

type mockState struct {
	machineundertaker.State
	*testing.Stub

	removals       []*mockRemoval
	watcherBlowsUp bool
}

func (st *mockState) AllMachineRemovals() ([]machineundertaker.MachineRemoval, error) {
	st.AddCall("AllMachineRemovals")
	err := st.NextErr()
	if err != nil {
		return nil, err
	}
	var result []machineundertaker.MachineRemoval
	for _, removal := range st.removals {
		result = append(result, removal)
	}
	return result, nil
}

func (st *mockState) ClearMachineRemovals(machineIDs []string) error {
	st.AddCall("ClearMachineRemovals", machineIDs)
	return st.NextErr()
}

func (st *mockState) WatchMachineRemovals() state.NotifyWatcher {
	st.AddCall("WatchMachineRemovals")
	watcher := &mockWatcher{st: st, out: make(chan struct{}, 1)}
	if st.watcherBlowsUp {
		close(watcher.out)
	} else {
		watcher.out <- struct{}{}
	}
	return watcher
}

type mockWatcher struct {
	state.NotifyWatcher

	st  *mockState
	out chan struct{}
}

func (w *mockWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *mockWatcher) Err() error {
	return w.st.NextErr()
}

type mockRemoval struct {
	machineID        string
	linkLayerDevices []*mockLinkLayerDevice
}

func (r *mockRemoval) MachineID() string {
	return r.machineID
}

func (r *mockRemoval) LinkLayerDevices() []machineundertaker.LinkLayerDevice {
	var result []machineundertaker.LinkLayerDevice
	for _, device := range r.linkLayerDevices {
		result = append(result, device)
	}
	return result
}

type mockLinkLayerDevice struct {
	name       string
	macAddress string
	type_      state.LinkLayerDeviceType
	mtu        uint
	providerID network.Id
}

func (d *mockLinkLayerDevice) Name() string {
	return d.name
}

func (d *mockLinkLayerDevice) MACAddress() string {
	return d.macAddress
}

func (d *mockLinkLayerDevice) Type() state.LinkLayerDeviceType {
	return d.type_
}

func (d *mockLinkLayerDevice) MTU() uint {
	return d.mtu
}

func (d *mockLinkLayerDevice) ProviderID() network.Id {
	return d.providerID
}
