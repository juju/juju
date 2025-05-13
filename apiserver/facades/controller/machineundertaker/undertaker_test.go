// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/machineundertaker"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type undertakerSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&undertakerSuite{})

const (
	uuid1 = "12345678-1234-1234-1234-123456789abc"
	tag1  = "model-12345678-1234-1234-1234-123456789abc"
	tag2  = "model-12345678-1234-1234-1234-123456789abd"
)

func (*undertakerSuite) TestRequiresModelManager(c *tc.C) {
	backend := &mockBackend{}
	_, err := machineundertaker.NewAPI(
		model.UUID(internaltesting.ModelTag.Id()),
		backend,
		nil,
		apiservertesting.FakeAuthorizer{Controller: false},
		&mockMachineRemover{},
	)
	c.Assert(err, tc.ErrorMatches, "permission denied")
	_, err = machineundertaker.NewAPI(
		model.UUID(internaltesting.ModelTag.Id()),
		backend,
		nil,
		apiservertesting.FakeAuthorizer{Controller: true},
		&mockMachineRemover{},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (*undertakerSuite) TestAllMachineRemovalsNoResults(c *tc.C) {
	_, _, _, api := makeAPI(c, uuid1)
	result := api.AllMachineRemovals(context.Background(), makeEntities(tag1))
	c.Assert(result, tc.DeepEquals, params.EntitiesResults{
		Results: []params.EntitiesResult{{}}, // So, one empty set of entities.
	})
}

func (*undertakerSuite) TestAllMachineRemovalsError(c *tc.C) {
	backend, _, _, api := makeAPI(c, uuid1)
	backend.SetErrors(errors.New("I don't want to set the world on fire"))
	result := api.AllMachineRemovals(context.Background(), makeEntities(tag1))
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, "I don't want to set the world on fire")
	c.Assert(result.Results[0].Entities, tc.DeepEquals, []params.Entity{})
}

func (*undertakerSuite) TestAllMachineRemovalsRequiresModelTags(c *tc.C) {
	_, _, _, api := makeAPI(c, uuid1)
	results := api.AllMachineRemovals(context.Background(), makeEntities(tag1, "machine-0"))
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].Entities, tc.DeepEquals, []params.Entity{})
	c.Assert(results.Results[1].Error, tc.ErrorMatches, `"machine-0" is not a valid model tag`)
	c.Assert(results.Results[1].Entities, tc.DeepEquals, []params.Entity{})
}

func (*undertakerSuite) TestAllMachineRemovalsChecksModelTag(c *tc.C) {
	_, _, _, api := makeAPI(c, uuid1)
	results := api.AllMachineRemovals(context.Background(), makeEntities(tag2))
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "permission denied")
	c.Assert(results.Results[0].Entities, tc.IsNil)
}

func (*undertakerSuite) TestAllMachineRemovals(c *tc.C) {
	backend, _, _, api := makeAPI(c, uuid1)
	backend.removals = []string{"0", "2"}

	result := api.AllMachineRemovals(context.Background(), makeEntities(tag1))
	c.Assert(result, tc.DeepEquals, makeEntitiesResults("machine-0", "machine-2"))
}

func (*undertakerSuite) TestGetMachineProviderInterfaceInfo(c *tc.C) {
	backend, _, _, api := makeAPI(c, "")
	backend.machines = map[string]*mockMachine{
		"0": {
			Stub: &testhelpers.Stub{},
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
			Stub: &testhelpers.Stub{},
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

	c.Assert(result, tc.DeepEquals, params.ProviderInterfaceInfoResults{
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

func (*undertakerSuite) TestGetMachineProviderInterfaceInfoHandlesError(c *tc.C) {
	backend, _, _, api := makeAPI(c, "")
	backend.machines = map[string]*mockMachine{
		"0": {Stub: backend.Stub},
	}
	backend.SetErrors(nil, errors.New("oops - problem getting interface infos"))
	result := api.GetMachineProviderInterfaceInfo(context.Background(), makeEntities("machine-0"))

	c.Assert(result.Results, tc.DeepEquals, []params.ProviderInterfaceInfoResult{{
		MachineTag: "machine-0",
		Error:      apiservererrors.ServerError(errors.New("oops - problem getting interface infos")),
	}})
}

func (*undertakerSuite) TestCompleteMachineRemovalsWithNonMachineTags(c *tc.C) {
	_, _, _, api := makeAPI(c, "")
	err := api.CompleteMachineRemovals(context.Background(), makeEntities("machine-2", "application-a1"))
	c.Assert(err, tc.ErrorMatches, `"application-a1" is not a valid machine tag`)
}

func (*undertakerSuite) TestCompleteMachineRemovalsWithOtherError(c *tc.C) {
	backend, _, _, api := makeAPI(c, "")
	backend.SetErrors(errors.New("boom"))
	err := api.CompleteMachineRemovals(context.Background(), makeEntities("machine-2"))
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (*undertakerSuite) TestCompleteMachineRemovals(c *tc.C) {
	backend, remover, _, api := makeAPI(c, "")
	err := api.CompleteMachineRemovals(context.Background(), makeEntities("machine-2", "machine-52"))
	c.Assert(err, tc.ErrorIsNil)
	backend.CheckCallNames(c, "CompleteMachineRemovals")
	callArgs := backend.Calls()[0].Args
	c.Assert(len(callArgs), tc.Equals, 1)
	values, ok := callArgs[0].([]string)
	c.Assert(ok, tc.IsTrue)
	c.Assert(values, tc.DeepEquals, []string{"2", "52"})
	remover.stub.CheckCalls(c, []testhelpers.StubCall{
		{"Delete", []any{machine.Name("2")}},
		{"Delete", []any{machine.Name("52")}},
	})
}

func (*undertakerSuite) TestWatchMachineRemovals(c *tc.C) {
	backend, _, res, api := makeAPI(c, uuid1)

	result := api.WatchMachineRemovals(context.Background(), makeEntities(tag1))
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(res.Get(result.Results[0].NotifyWatcherId), tc.NotNil)
	c.Assert(result.Results[0].Error, tc.IsNil)
	backend.CheckCallNames(c, "WatchMachineRemovals")
}

func (*undertakerSuite) TestWatchMachineRemovalsPermissionError(c *tc.C) {
	_, _, _, api := makeAPI(c, uuid1)
	result := api.WatchMachineRemovals(context.Background(), makeEntities(tag2))
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, "permission denied")
}

func (*undertakerSuite) TestWatchMachineRemovalsError(c *tc.C) {
	backend, _, _, api := makeAPI(c, uuid1)
	backend.watcherBlowsUp = true
	backend.SetErrors(errors.New("oh no!"))

	result := api.WatchMachineRemovals(context.Background(), makeEntities(tag1))
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, "oh no!")
	c.Assert(result.Results[0].NotifyWatcherId, tc.Equals, "")
	backend.CheckCallNames(c, "WatchMachineRemovals")
}

func makeAPI(c *tc.C, modelUUID string) (*mockBackend, *mockMachineRemover, *common.Resources, *machineundertaker.API) {
	backend := &mockBackend{Stub: &testhelpers.Stub{}}
	machineRemover := &mockMachineRemover{stub: &testhelpers.Stub{}}
	res := common.NewResources()
	api, err := machineundertaker.NewAPI(
		model.UUID(modelUUID),
		backend,
		res,
		apiservertesting.FakeAuthorizer{
			Controller: true,
		},
		machineRemover,
	)
	c.Assert(err, tc.ErrorIsNil)
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
	*testhelpers.Stub

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
	*testhelpers.Stub
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
	stub *testhelpers.Stub
}

func (m *mockMachineRemover) DeleteMachine(_ context.Context, machineId machine.Name) error {
	m.stub.AddCall("Delete", machineId)
	return nil
}
