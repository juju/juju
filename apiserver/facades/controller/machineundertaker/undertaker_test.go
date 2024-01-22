// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/machineundertaker"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type undertakerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&undertakerSuite{})

const (
	uuid1 = "12345678-1234-1234-1234-123456789abc"
	tag1  = "model-12345678-1234-1234-1234-123456789abc"
	tag2  = "model-12345678-1234-1234-1234-123456789abd"
)

func (*undertakerSuite) TestRequiresModelManager(c *gc.C) {
	backend := &mockBackend{}
	_, err := machineundertaker.NewAPI(
		backend,
		nil,
		apiservertesting.FakeAuthorizer{Controller: false},
		&mockMachineRemover{},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	_, err = machineundertaker.NewAPI(
		backend,
		nil,
		apiservertesting.FakeAuthorizer{Controller: true},
		&mockMachineRemover{},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (*undertakerSuite) TestAllMachineRemovalsNoResults(c *gc.C) {
	_, _, _, api := makeAPI(c, uuid1)
	result := api.AllMachineRemovals(context.Background(), makeEntities(tag1))
	c.Assert(result, gc.DeepEquals, params.EntitiesResults{
		Results: []params.EntitiesResult{{}}, // So, one empty set of entities.
	})
}

func (*undertakerSuite) TestAllMachineRemovalsError(c *gc.C) {
	backend, _, _, api := makeAPI(c, uuid1)
	backend.SetErrors(errors.New("I don't want to set the world on fire"))
	result := api.AllMachineRemovals(context.Background(), makeEntities(tag1))
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "I don't want to set the world on fire")
	c.Assert(result.Results[0].Entities, jc.DeepEquals, []params.Entity{})
}

func (*undertakerSuite) TestAllMachineRemovalsRequiresModelTags(c *gc.C) {
	_, _, _, api := makeAPI(c, uuid1)
	results := api.AllMachineRemovals(context.Background(), makeEntities(tag1, "machine-0"))
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Entities, jc.DeepEquals, []params.Entity{})
	c.Assert(results.Results[1].Error, gc.ErrorMatches, `"machine-0" is not a valid model tag`)
	c.Assert(results.Results[1].Entities, jc.DeepEquals, []params.Entity{})
}

func (*undertakerSuite) TestAllMachineRemovalsChecksModelTag(c *gc.C) {
	_, _, _, api := makeAPI(c, uuid1)
	results := api.AllMachineRemovals(context.Background(), makeEntities(tag2))
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Assert(results.Results[0].Entities, gc.IsNil)
}

func (*undertakerSuite) TestAllMachineRemovals(c *gc.C) {
	backend, _, _, api := makeAPI(c, uuid1)
	backend.removals = []string{"0", "2"}

	result := api.AllMachineRemovals(context.Background(), makeEntities(tag1))
	c.Assert(result, gc.DeepEquals, makeEntitiesResults("machine-0", "machine-2"))
}

func (*undertakerSuite) TestGetMachineProviderInterfaceInfo(c *gc.C) {
	backend, _, _, api := makeAPI(c, "")
	backend.machines = map[string]*mockMachine{
		"0": {
			Stub: &testing.Stub{},
			interfaceInfos: []network.ProviderInterfaceInfo{{
				InterfaceName:   "billy",
				HardwareAddress: "hexadecimal!",
				ProviderId:      "a number",
			}, {
				InterfaceName:   "lily",
				HardwareAddress: "octal?",
				ProviderId:      "different number",
			}}},
		"2": {
			Stub: &testing.Stub{},
			interfaceInfos: []network.ProviderInterfaceInfo{{
				InterfaceName:   "gilly",
				HardwareAddress: "sexagesimal?!",
				ProviderId:      "some number",
			}},
		},
	}
	backend.SetErrors(nil, errors.NotFoundf("no machine 100 fool!"))

	args := makeEntities("machine-2", "machine-100", "machine-0", "machine-inv")
	result := api.GetMachineProviderInterfaceInfo(context.Background(), args)

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
			Error: apiservererrors.ServerError(
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
			Error: apiservererrors.ServerError(
				errors.New(`"machine-inv" is not a valid machine tag`),
			),
		}},
	})
}

func (*undertakerSuite) TestGetMachineProviderInterfaceInfoHandlesError(c *gc.C) {
	backend, _, _, api := makeAPI(c, "")
	backend.machines = map[string]*mockMachine{
		"0": {Stub: backend.Stub},
	}
	backend.SetErrors(nil, errors.New("oops - problem getting interface infos"))
	result := api.GetMachineProviderInterfaceInfo(context.Background(), makeEntities("machine-0"))

	c.Assert(result.Results, gc.DeepEquals, []params.ProviderInterfaceInfoResult{{
		MachineTag: "machine-0",
		Error:      apiservererrors.ServerError(errors.New("oops - problem getting interface infos")),
	}})
}

func (*undertakerSuite) TestCompleteMachineRemovalsWithNonMachineTags(c *gc.C) {
	_, _, _, api := makeAPI(c, "")
	err := api.CompleteMachineRemovals(context.Background(), makeEntities("machine-2", "application-a1"))
	c.Assert(err, gc.ErrorMatches, `"application-a1" is not a valid machine tag`)
}

func (*undertakerSuite) TestCompleteMachineRemovalsWithOtherError(c *gc.C) {
	backend, _, _, api := makeAPI(c, "")
	backend.SetErrors(errors.New("boom"))
	err := api.CompleteMachineRemovals(context.Background(), makeEntities("machine-2"))
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (*undertakerSuite) TestCompleteMachineRemovals(c *gc.C) {
	backend, remover, _, api := makeAPI(c, "")
	err := api.CompleteMachineRemovals(context.Background(), makeEntities("machine-2", "machine-52"))
	c.Assert(err, jc.ErrorIsNil)
	backend.CheckCallNames(c, "CompleteMachineRemovals")
	callArgs := backend.Calls()[0].Args
	c.Assert(len(callArgs), gc.Equals, 1)
	values, ok := callArgs[0].([]string)
	c.Assert(ok, jc.IsTrue)
	c.Assert(values, gc.DeepEquals, []string{"2", "52"})
	remover.stub.CheckCalls(c, []testing.StubCall{
		{"Delete", []any{"2"}},
		{"Delete", []any{"52"}},
	})
}

func (*undertakerSuite) TestWatchMachineRemovals(c *gc.C) {
	backend, _, res, api := makeAPI(c, uuid1)

	result := api.WatchMachineRemovals(context.Background(), makeEntities(tag1))
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(res.Get(result.Results[0].NotifyWatcherId), gc.NotNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	backend.CheckCallNames(c, "WatchMachineRemovals")
}

func (*undertakerSuite) TestWatchMachineRemovalsPermissionError(c *gc.C) {
	_, _, _, api := makeAPI(c, uuid1)
	result := api.WatchMachineRemovals(context.Background(), makeEntities(tag2))
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
}

func (*undertakerSuite) TestWatchMachineRemovalsError(c *gc.C) {
	backend, _, _, api := makeAPI(c, uuid1)
	backend.watcherBlowsUp = true
	backend.SetErrors(errors.New("oh no!"))

	result := api.WatchMachineRemovals(context.Background(), makeEntities(tag1))
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "oh no!")
	c.Assert(result.Results[0].NotifyWatcherId, gc.Equals, "")
	backend.CheckCallNames(c, "WatchMachineRemovals")
}

func makeAPI(c *gc.C, modelUUID string) (*mockBackend, *mockMachineRemover, *common.Resources, *machineundertaker.API) {
	backend := &mockBackend{Stub: &testing.Stub{}}
	machineRemover := &mockMachineRemover{stub: &testing.Stub{}}
	res := common.NewResources()
	api, err := machineundertaker.NewAPI(
		backend,
		res,
		apiservertesting.FakeAuthorizer{
			Controller: true,
			ModelUUID:  modelUUID,
		},
		machineRemover,
	)
	c.Assert(err, jc.ErrorIsNil)
	return backend, machineRemover, res, api
}

func makeEntities(tags ...string) params.Entities {
	return params.Entities{Entities: makeEntitySlice(tags...)}
}

func makeEntitySlice(tags ...string) []params.Entity {
	entities := make([]params.Entity, len(tags))
	for i := range tags {
		entities[i] = params.Entity{Tag: tags[i]}
	}
	return entities
}

func makeEntitiesResults(tags ...string) params.EntitiesResults {
	return params.EntitiesResults{
		Results: []params.EntitiesResult{{
			Entities: makeEntitySlice(tags...),
		}},
	}
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

type mockMachineRemover struct {
	stub *testing.Stub
}

func (m *mockMachineRemover) Delete(_ context.Context, machineId string) error {
	m.stub.AddCall("Delete", machineId)
	return nil
}
