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

const (
	uuid1 = "12345678-1234-1234-1234-123456789abc"
	tag1  = "model-12345678-1234-1234-1234-123456789abc"
	uuid2 = "12345678-1234-1234-1234-123456789abd"
	tag2  = "model-12345678-1234-1234-1234-123456789abd"
)

func (*undertakerSuite) TestRequiresModelManager(c *gc.C) {
	backend := &mockBackend{}
	_, err := machineundertaker.NewMachineUndertakerAPI(
		backend,
		nil,
		apiservertesting.FakeAuthorizer{EnvironManager: false},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	_, err = machineundertaker.NewMachineUndertakerAPI(
		backend,
		nil,
		apiservertesting.FakeAuthorizer{EnvironManager: true},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (*undertakerSuite) TestAllMachineRemovalsNoResults(c *gc.C) {
	_, _, api := makeApi(c, uuid1)
	result, err := api.AllMachineRemovals(makeEntities(tag1))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.Entities{})
}

func (*undertakerSuite) TestAllMachineRemovalsError(c *gc.C) {
	backend, _, api := makeApi(c, uuid1)
	backend.SetErrors(errors.New("I don't want to set the world on fire"))
	result, err := api.AllMachineRemovals(makeEntities(tag1))
	c.Assert(err, gc.ErrorMatches, "I don't want to set the world on fire")
	c.Assert(result, gc.DeepEquals, params.Entities{})
}

func (*undertakerSuite) TestAllMachineRemovalsRequiresOneTag(c *gc.C) {
	_, _, api := makeApi(c, "")
	_, err := api.AllMachineRemovals(params.Entities{})
	c.Assert(err, gc.ErrorMatches, "one model tag is required")
}

func (*undertakerSuite) TestAllMachineRemovalsRequiresModelTags(c *gc.C) {
	_, _, api := makeApi(c, uuid1)
	_, err := api.AllMachineRemovals(makeEntities(tag1, "machine-0"))
	c.Assert(err, gc.ErrorMatches, `"machine-0" is not a valid model tag`)
}

func (*undertakerSuite) TestAllMachineRemovalsChecksModelTag(c *gc.C) {
	_, _, api := makeApi(c, uuid1)
	_, err := api.AllMachineRemovals(makeEntities(tag2))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (*undertakerSuite) TestAllMachineRemovals(c *gc.C) {
	backend, _, api := makeApi(c, uuid1)
	backend.removals = []string{"0", "2"}

	result, err := api.AllMachineRemovals(makeEntities(tag1))

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, makeEntities("machine-0", "machine-2"))
}

func (*undertakerSuite) TestGetMachineProviderInterfaceInfo(c *gc.C) {
	backend, _, api := makeApi(c, "")
	backend.machines = map[string]*mockMachine{
		"0": &mockMachine{
			Stub: &testing.Stub{},
			interfaceInfos: []network.ProviderInterfaceInfo{{
				InterfaceName: "billy",
				MACAddress:    "hexadecimal!",
				ProviderId:    "a number",
			}, {
				InterfaceName: "lily",
				MACAddress:    "octal?",
				ProviderId:    "different number",
			}}},
		"2": &mockMachine{
			Stub: &testing.Stub{},
			interfaceInfos: []network.ProviderInterfaceInfo{{
				InterfaceName: "gilly",
				MACAddress:    "sexagesimal?!",
				ProviderId:    "some number",
			}},
		},
	}
	backend.SetErrors(nil, errors.NotFoundf("no machine 100 fool!"))

	args := makeEntities("machine-2", "machine-100", "machine-0", "machine-inv")
	result := api.GetMachineProviderInterfaceInfo(args)

	c.Assert(result, gc.DeepEquals, params.ProviderInterfaceInfoResults{
		Results: []params.ProviderInterfaceInfoResult{{
			MachineTag: "machine-2",
			Interfaces: []params.ProviderInterfaceInfo{{
				InterfaceName: "gilly",
				MACAddress:    "sexagesimal?!",
				ProviderId:    "some number",
			}},
		}, {
			MachineTag: "machine-100",
			Error: common.ServerError(
				errors.NotFoundf("no machine 100 fool!"),
			),
		}, {
			MachineTag: "machine-0",
			Interfaces: []params.ProviderInterfaceInfo{{
				InterfaceName: "billy",
				MACAddress:    "hexadecimal!",
				ProviderId:    "a number",
			}, {
				InterfaceName: "lily",
				MACAddress:    "octal?",
				ProviderId:    "different number",
			}},
		}, {
			MachineTag: "machine-inv",
			Error: common.ServerError(
				errors.New(`"machine-inv" is not a valid machine tag`),
			),
		}},
	})
}

func (*undertakerSuite) TestGetMachineProviderInterfaceInfoHandlesError(c *gc.C) {
	backend, _, api := makeApi(c, "")
	backend.machines = map[string]*mockMachine{
		"0": &mockMachine{Stub: backend.Stub},
	}
	backend.SetErrors(nil, errors.New("oops - problem getting interface infos"))
	result := api.GetMachineProviderInterfaceInfo(makeEntities("machine-0"))

	c.Assert(result.Results, gc.DeepEquals, []params.ProviderInterfaceInfoResult{{
		MachineTag: "machine-0",
		Error:      common.ServerError(errors.New("oops - problem getting interface infos")),
	}})
}

func (*undertakerSuite) TestCompleteMachineRemovalsWithNonMachineTags(c *gc.C) {
	_, _, api := makeApi(c, "")
	err := api.CompleteMachineRemovals(makeEntities("machine-2", "application-a1"))
	c.Assert(err, gc.ErrorMatches, `"application-a1" is not a valid machine tag`)
}

func (*undertakerSuite) TestCompleteMachineRemovalsWithOtherError(c *gc.C) {
	backend, _, api := makeApi(c, "")
	backend.SetErrors(errors.New("boom"))
	err := api.CompleteMachineRemovals(makeEntities("machine-2"))
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (*undertakerSuite) TestCompleteMachineRemovals(c *gc.C) {
	backend, _, api := makeApi(c, "")
	err := api.CompleteMachineRemovals(makeEntities("machine-2", "machine-52"))
	c.Assert(err, jc.ErrorIsNil)
	backend.CheckCallNames(c, "CompleteMachineRemovals")
	callArgs := backend.Calls()[0].Args
	c.Assert(len(callArgs), gc.Equals, 1)
	values, ok := callArgs[0].([]string)
	c.Assert(ok, jc.IsTrue)
	c.Assert(values, gc.DeepEquals, []string{"2", "52"})
}

func (*undertakerSuite) TestWatchMachineRemovals(c *gc.C) {
	backend, res, api := makeApi(c, uuid1)

	result := api.WatchMachineRemovals(makeEntities(tag1))
	c.Assert(res.Get(result.NotifyWatcherId), gc.NotNil)
	backend.CheckCallNames(c, "WatchMachineRemovals")
}

func (*undertakerSuite) TestWatchMachineRemovalsPermissionError(c *gc.C) {
	_, _, api := makeApi(c, uuid1)
	result := api.WatchMachineRemovals(makeEntities(tag2))
	c.Assert(result.Error, gc.ErrorMatches, "permission denied")
}

func (*undertakerSuite) TestWatchMachineRemovalsError(c *gc.C) {
	backend, _, api := makeApi(c, uuid1)
	backend.watcherBlowsUp = true
	backend.SetErrors(errors.New("oh no!"))

	result := api.WatchMachineRemovals(makeEntities(tag1))
	c.Assert(result.Error, gc.ErrorMatches, "oh no!")
	backend.CheckCallNames(c, "WatchMachineRemovals")
}

func makeApi(c *gc.C, modelUUID string) (*mockBackend, *common.Resources, *machineundertaker.MachineUndertakerAPI) {
	backend := &mockBackend{Stub: &testing.Stub{}}
	res := common.NewResources()
	api, err := machineundertaker.NewMachineUndertakerAPI(
		backend,
		res,
		apiservertesting.FakeAuthorizer{
			EnvironManager: true,
			ModelUUID:      modelUUID,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return backend, res, api
}

func makeEntities(tags ...string) params.Entities {
	entities := make([]params.Entity, len(tags))
	for i := range tags {
		entities[i] = params.Entity{Tag: tags[i]}
	}
	return params.Entities{Entities: entities}
}

type mockBackend struct {
	*testing.Stub

	removals       []string
	machines       map[string]*mockMachine
	watcherBlowsUp bool
}

func (b *mockBackend) AllMachineRemovals() ([]string, error) {
	b.AddCall("AllMachineRemovals")
	return b.removals, b.NextErr()
}

func (b *mockBackend) CompleteMachineRemovals(machineIDs ...string) error {
	b.AddCall("CompleteMachineRemovals", machineIDs)
	return b.NextErr()
}

func (b *mockBackend) WatchMachineRemovals() state.NotifyWatcher {
	b.AddCall("WatchMachineRemovals")
	watcher := &mockWatcher{backend: b, out: make(chan struct{}, 1)}
	if b.watcherBlowsUp {
		close(watcher.out)
	} else {
		watcher.out <- struct{}{}
	}
	return watcher
}

func (b *mockBackend) Machine(id string) (machineundertaker.Machine, error) {
	b.AddCall("Machine", id)
	return b.machines[id], b.NextErr()
}

type mockMachine struct {
	*testing.Stub
	interfaceInfos []network.ProviderInterfaceInfo
}

func (m *mockMachine) AllProviderInterfaceInfos() ([]network.ProviderInterfaceInfo, error) {
	m.AddCall("AllProviderInterfaceInfos")
	err := m.NextErr()
	if err != nil {
		return nil, err
	}
	return m.interfaceInfos, err
}

type mockWatcher struct {
	state.NotifyWatcher

	backend *mockBackend
	out     chan struct{}
}

func (w *mockWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *mockWatcher) Err() error {
	return w.backend.NextErr()
}
