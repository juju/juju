// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/machineundertaker"
)

type undertakerSuite struct {
	testhelpers.IsolationSuite
}

func TestUndertakerSuite(t *testing.T) {
	tc.Run(t, &undertakerSuite{})
}

// Some tests to check that the handler is wired up to the
// NotifyWorker first.

func (s *undertakerSuite) TestErrorWatching(c *tc.C) {
	api := s.makeAPIWithWatcher()
	api.SetErrors(errors.New("blam"))
	w, err := machineundertaker.NewWorker(
		api, &fakeEnviron{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "blam")
	api.CheckCallNames(c, "WatchMachineRemovals")
}

func (s *undertakerSuite) TestErrorGettingRemovals(c *tc.C) {
	api := s.makeAPIWithWatcher()
	api.SetErrors(nil, errors.New("explodo"))
	w, err := machineundertaker.NewWorker(
		api, &fakeEnviron{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "explodo")
	api.CheckCallNames(c, "WatchMachineRemovals", "AllMachineRemovals")
}

// It's really fiddly trying to test the code behind the worker, so
// the rest of the tests use the Undertaker directly to test the
// Handle and MaybeReleaseAddresses methods. This is much simpler
// because everything happens in the same goroutine (and it's safe
// since all of the clever/tricky lifecycle management is taken care
// of in NotifyWorker instead).

func (*undertakerSuite) TestMaybeReleaseAddresses_NoNetworking(c *tc.C) {
	api := fakeAPI{Stub: &testhelpers.Stub{}}
	u := machineundertaker.Undertaker{API: &api, Logger: loggertesting.WrapCheckLog(c)}
	err := u.MaybeReleaseAddresses(c.Context(), names.NewMachineTag("3"))
	c.Assert(err, tc.ErrorIsNil)
	api.CheckCallNames(c)
}

func (*undertakerSuite) TestMaybeReleaseAddresses_NotContainer(c *tc.C) {
	api := fakeAPI{Stub: &testhelpers.Stub{}}
	releaser := fakeReleaser{}
	u := machineundertaker.Undertaker{
		API:      &api,
		Releaser: &releaser,
		Logger:   loggertesting.WrapCheckLog(c),
	}
	err := u.MaybeReleaseAddresses(c.Context(), names.NewMachineTag("4"))
	c.Assert(err, tc.ErrorIsNil)
	api.CheckCallNames(c)
}

func (*undertakerSuite) TestMaybeReleaseAddresses_ErrorGettingInfo(c *tc.C) {
	api := fakeAPI{Stub: &testhelpers.Stub{}}
	api.SetErrors(errors.New("a funny thing happened on the way"))
	releaser := fakeReleaser{}
	u := machineundertaker.Undertaker{
		API:      &api,
		Releaser: &releaser,
		Logger:   loggertesting.WrapCheckLog(c),
	}
	err := u.MaybeReleaseAddresses(c.Context(), names.NewMachineTag("4/lxd/2"))
	c.Assert(err, tc.ErrorMatches, "a funny thing happened on the way")
}

func (*undertakerSuite) TestMaybeReleaseAddresses_NoAddresses(c *tc.C) {
	api := fakeAPI{Stub: &testhelpers.Stub{}}
	releaser := fakeReleaser{Stub: &testhelpers.Stub{}}
	u := machineundertaker.Undertaker{
		API:      &api,
		Releaser: &releaser,
		Logger:   loggertesting.WrapCheckLog(c),
	}
	err := u.MaybeReleaseAddresses(c.Context(), names.NewMachineTag("4/lxd/4"))
	c.Assert(err, tc.ErrorIsNil)
	releaser.CheckCallNames(c)
}

func (*undertakerSuite) TestMaybeReleaseAddresses_NotSupported(c *tc.C) {
	api := fakeAPI{
		Stub: &testhelpers.Stub{},
		interfaces: map[string][]network.ProviderInterfaceInfo{
			"4/lxd/4": {
				{InterfaceName: "chloe"},
			},
		},
	}
	releaser := fakeReleaser{Stub: &testhelpers.Stub{}}
	releaser.SetErrors(errors.NotSupportedf("this sort of thing"))
	u := machineundertaker.Undertaker{
		API:      &api,
		Releaser: &releaser,
		Logger:   loggertesting.WrapCheckLog(c),
	}
	err := u.MaybeReleaseAddresses(c.Context(), names.NewMachineTag("4/lxd/4"))
	c.Assert(err, tc.ErrorIsNil)
	releaser.CheckCall(c, 0, "ReleaseContainerAddresses",
		[]network.ProviderInterfaceInfo{{InterfaceName: "chloe"}},
	)
}

func (*undertakerSuite) TestMaybeReleaseAddresses_ErrorReleasing(c *tc.C) {
	api := fakeAPI{
		Stub: &testhelpers.Stub{},
		interfaces: map[string][]network.ProviderInterfaceInfo{
			"4/lxd/4": {
				{InterfaceName: "chloe"},
			},
		},
	}
	releaser := fakeReleaser{Stub: &testhelpers.Stub{}}
	releaser.SetErrors(errors.New("something unexpected"))
	u := machineundertaker.Undertaker{
		API:      &api,
		Releaser: &releaser,
		Logger:   loggertesting.WrapCheckLog(c),
	}
	err := u.MaybeReleaseAddresses(c.Context(), names.NewMachineTag("4/lxd/4"))
	c.Assert(err, tc.ErrorMatches, "something unexpected")
	releaser.CheckCall(c, 0, "ReleaseContainerAddresses",
		[]network.ProviderInterfaceInfo{{InterfaceName: "chloe"}},
	)
}

func (*undertakerSuite) TestMaybeReleaseAddresses_Success(c *tc.C) {
	api := fakeAPI{
		Stub: &testhelpers.Stub{},
		interfaces: map[string][]network.ProviderInterfaceInfo{
			"4/lxd/4": {
				{InterfaceName: "chloe"},
			},
		},
	}
	releaser := fakeReleaser{Stub: &testhelpers.Stub{}}
	u := machineundertaker.Undertaker{
		API:      &api,
		Releaser: &releaser,
		Logger:   loggertesting.WrapCheckLog(c),
	}
	err := u.MaybeReleaseAddresses(c.Context(), names.NewMachineTag("4/lxd/4"))
	c.Assert(err, tc.ErrorIsNil)
	releaser.CheckCall(c, 0, "ReleaseContainerAddresses",
		[]network.ProviderInterfaceInfo{{InterfaceName: "chloe"}},
	)
}

func (*undertakerSuite) TestHandle_CompletesRemoval(c *tc.C) {
	api := fakeAPI{
		Stub:     &testhelpers.Stub{},
		removals: []string{"3", "4/lxd/4"},
		interfaces: map[string][]network.ProviderInterfaceInfo{
			"4/lxd/4": {
				{InterfaceName: "chloe"},
			},
		},
	}
	releaser := fakeReleaser{Stub: &testhelpers.Stub{}}
	u := machineundertaker.Undertaker{
		API:      &api,
		Releaser: &releaser,
		Logger:   loggertesting.WrapCheckLog(c),
	}
	err := u.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(releaser.Calls(), tc.HasLen, 1)
	releaser.CheckCall(c, 0, "ReleaseContainerAddresses",
		[]network.ProviderInterfaceInfo{{InterfaceName: "chloe"}},
	)

	checkRemovalsMatch(c, api.Stub, "3", "4/lxd/4")
}

func (*undertakerSuite) TestHandle_NoRemovalOnErrorReleasing(c *tc.C) {
	api := fakeAPI{
		Stub:     &testhelpers.Stub{},
		removals: []string{"3", "4/lxd/4", "5"},
		interfaces: map[string][]network.ProviderInterfaceInfo{
			"4/lxd/4": {
				{InterfaceName: "chloe"},
			},
		},
	}
	releaser := fakeReleaser{Stub: &testhelpers.Stub{}}
	releaser.SetErrors(errors.New("couldn't release address"))
	u := machineundertaker.Undertaker{
		API:      &api,
		Releaser: &releaser,
		Logger:   loggertesting.WrapCheckLog(c),
	}
	err := u.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(releaser.Calls(), tc.HasLen, 1)
	releaser.CheckCall(c, 0, "ReleaseContainerAddresses",
		[]network.ProviderInterfaceInfo{{InterfaceName: "chloe"}},
	)

	checkRemovalsMatch(c, api.Stub, "3", "5")
}

func (*undertakerSuite) TestHandle_ErrorOnRemoval(c *tc.C) {
	api := fakeAPI{
		Stub:     &testhelpers.Stub{},
		removals: []string{"3", "4/lxd/4"},
	}
	api.SetErrors(nil, errors.New("couldn't remove machine 3"))
	u := machineundertaker.Undertaker{API: &api, Logger: loggertesting.WrapCheckLog(c)}
	err := u.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	checkRemovalsMatch(c, api.Stub, "3", "4/lxd/4")
}

func checkRemovalsMatch(c *tc.C, stub *testhelpers.Stub, expected ...string) {
	var completedRemovals []string
	for _, call := range stub.Calls() {
		if call.FuncName == "CompleteRemoval" {
			machineId := call.Args[0].(names.MachineTag).Id()
			completedRemovals = append(completedRemovals, machineId)
		}
	}
	c.Check(completedRemovals, tc.DeepEquals, expected)
}

func (s *undertakerSuite) makeAPIWithWatcher() *fakeAPI {
	return &fakeAPI{
		Stub:    &testhelpers.Stub{},
		watcher: s.newMockNotifyWatcher(),
	}
}

func (s *undertakerSuite) newMockNotifyWatcher() *mockNotifyWatcher {
	m := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	m.tomb.Go(func() error {
		<-m.tomb.Dying()
		return nil
	})
	s.AddCleanup(func(c *tc.C) {
		err := worker.Stop(m)
		c.Check(err, tc.ErrorIsNil)
	})
	m.Change()
	return m
}

type fakeEnviron struct {
	environs.NetworkingEnviron
}

type fakeReleaser struct {
	*testhelpers.Stub
}

func (r *fakeReleaser) ReleaseContainerAddresses(ctx context.Context, interfaces []network.ProviderInterfaceInfo) error {
	r.Stub.AddCall("ReleaseContainerAddresses", interfaces)
	return r.Stub.NextErr()
}

type fakeAPI struct {
	machineundertaker.Facade

	*testhelpers.Stub
	watcher    *mockNotifyWatcher
	removals   []string
	interfaces map[string][]network.ProviderInterfaceInfo
}

func (a *fakeAPI) WatchMachineRemovals(context.Context) (watcher.NotifyWatcher, error) {
	a.Stub.AddCall("WatchMachineRemovals")
	return a.watcher, a.Stub.NextErr()
}

func (a *fakeAPI) AllMachineRemovals(context.Context) ([]names.MachineTag, error) {
	a.Stub.AddCall("AllMachineRemovals")
	result := make([]names.MachineTag, len(a.removals))
	for i := range a.removals {
		result[i] = names.NewMachineTag(a.removals[i])
	}
	return result, a.Stub.NextErr()
}

func (a *fakeAPI) GetProviderInterfaceInfo(ctx context.Context, machine names.MachineTag) ([]network.ProviderInterfaceInfo, error) {
	a.Stub.AddCall("GetProviderInterfaceInfo", machine)
	return a.interfaces[machine.Id()], a.Stub.NextErr()
}

func (a *fakeAPI) CompleteRemoval(ctx context.Context, machine names.MachineTag) error {
	a.Stub.AddCall("CompleteRemoval", machine)
	return a.Stub.NextErr()
}

type mockNotifyWatcher struct {
	watcher.NotifyWatcher

	tomb    tomb.Tomb
	changes chan struct{}
}

func (m *mockNotifyWatcher) Kill() {
	m.tomb.Kill(nil)
}

func (m *mockNotifyWatcher) Wait() error {
	return m.tomb.Wait()
}

func (m *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return m.changes
}

func (m *mockNotifyWatcher) Change() {
	m.changes <- struct{}{}
}
