// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/registry"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type watcherSuite struct {
	jujutesting.ApiServerSuite

	resources       *common.Resources
	watcherRegistry facade.WatcherRegistry
	authorizer      apiservertesting.FakeAuthorizer
}

var _ = tc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, s.watcherRegistry) })

	s.resources = common.NewResources()
	s.AddCleanup(func(*tc.C) {
		s.resources.StopAll()
	})
	s.authorizer = apiservertesting.FakeAuthorizer{}
}

func (s *watcherSuite) getFacade(
	c *tc.C,
	name string,
	version int,
	id string,
	dispose func(),
) interface{} {
	factory := getFacadeFactory(c, name, version)
	facade, err := factory(c.Context(), s.facadeContext(c, id, dispose))
	c.Assert(err, tc.ErrorIsNil)
	return facade
}

func (s *watcherSuite) facadeContext(c *tc.C, id string, dispose func()) facadetest.MultiModelContext {
	return facadetest.MultiModelContext{
		ModelContext: facadetest.ModelContext{
			Resources_:       s.resources,
			WatcherRegistry_: s.watcherRegistry,
			Auth_:            s.authorizer,
			DomainServices_:  s.ControllerDomainServices(c),
			ID_:              id,
			Dispose_:         dispose,
		},
	}
}

func getFacadeFactory(c *tc.C, name string, version int) facade.MultiModelFactory {
	factory, err := apiserver.AllFacades().GetFactory(name, version)
	c.Assert(err, tc.ErrorIsNil)
	return factory
}

func (s *watcherSuite) TestVolumeAttachmentsWatcher(c *tc.C) {
	ch := make(chan []string, 1)
	id := s.resources.Register(&fakeStringsWatcher{ch: ch})
	s.authorizer.Tag = names.NewMachineTag("123")

	ch <- []string{"0:1", "1:2"}
	facade := s.getFacade(c, "VolumeAttachmentsWatcher", 2, id, nopDispose).(machineStorageIdsWatcher)
	result, err := facade.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result, tc.DeepEquals, params.MachineStorageIdsWatchResult{
		Changes: []params.MachineStorageId{
			{MachineTag: "machine-0", AttachmentTag: "volume-1"},
			{MachineTag: "machine-1", AttachmentTag: "volume-2"},
		},
	})
}

func (s *watcherSuite) TestFilesystemAttachmentsWatcher(c *tc.C) {
	ch := make(chan []string, 1)
	id := s.resources.Register(&fakeStringsWatcher{ch: ch})
	s.authorizer.Tag = names.NewMachineTag("123")

	ch <- []string{"0:1", "1:2"}
	facade := s.getFacade(c, "FilesystemAttachmentsWatcher", 2, id, nopDispose).(machineStorageIdsWatcher)
	result, err := facade.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result, tc.DeepEquals, params.MachineStorageIdsWatchResult{
		Changes: []params.MachineStorageId{
			{MachineTag: "machine-0", AttachmentTag: "filesystem-1"},
			{MachineTag: "machine-1", AttachmentTag: "filesystem-2"},
		},
	})
}

func (s *watcherSuite) TestMigrationStatusWatcher(c *tc.C) {
	w := apiservertesting.NewFakeNotifyWatcher()
	id := s.resources.Register(w)
	s.authorizer.Tag = names.NewMachineTag("12")
	apiserver.PatchGetMigrationBackend(s, new(fakeMigrationBackend), new(fakeMigrationBackend))
	apiserver.PatchGetControllerCACert(s, "no worries")

	facade := s.getFacade(c, "MigrationStatusWatcher", 1, id, nopDispose).(migrationStatusWatcher)
	defer c.Check(facade.Stop(), tc.ErrorIsNil)
	result, err := facade.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.MigrationStatus{
		MigrationId:    "id",
		Attempt:        2,
		Phase:          "IMPORT",
		SourceAPIAddrs: []string{"1.2.3.4:5", "2.3.4.5:6", "3.4.5.6:7"},
		SourceCACert:   "no worries",
		TargetAPIAddrs: []string{"1.2.3.4:5555"},
		TargetCACert:   "trust me",
	})
}

func (s *watcherSuite) TestMigrationStatusWatcherNoMigration(c *tc.C) {
	w := apiservertesting.NewFakeNotifyWatcher()
	id := s.resources.Register(w)
	s.authorizer.Tag = names.NewMachineTag("12")
	backend := &fakeMigrationBackend{noMigration: true}
	apiserver.PatchGetMigrationBackend(s, backend, backend)

	facade := s.getFacade(c, "MigrationStatusWatcher", 1, id, nopDispose).(migrationStatusWatcher)
	defer c.Check(facade.Stop(), tc.ErrorIsNil)
	result, err := facade.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.MigrationStatus{
		Phase: "NONE",
	})
}

func (s *watcherSuite) TestMigrationStatusWatcherNotAgent(c *tc.C) {
	id := s.resources.Register(apiservertesting.NewFakeNotifyWatcher())
	s.authorizer.Tag = names.NewUserTag("frogdog")

	factory, err := apiserver.AllFacades().GetFactory("MigrationStatusWatcher", 1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = factory(c.Context(), facadetest.MultiModelContext{
		ModelContext: facadetest.ModelContext{
			Resources_:      s.resources,
			Auth_:           s.authorizer,
			ID_:             id,
			DomainServices_: s.ControllerDomainServices(c),
		},
	})
	c.Assert(err, tc.Equals, apiservererrors.ErrPerm)
}

type machineStorageIdsWatcher interface {
	Next(context.Context) (params.MachineStorageIdsWatchResult, error)
}

type fakeStringsWatcher struct {
	state.StringsWatcher
	ch chan []string
}

func (w *fakeStringsWatcher) Changes() <-chan []string {
	return w.ch
}

func (w *fakeStringsWatcher) Kill() {}

func (w *fakeStringsWatcher) Wait() error {
	return nil
}

type fakeMigrationBackend struct {
	noMigration bool
}

func (b *fakeMigrationBackend) LatestMigration() (state.ModelMigration, error) {
	if b.noMigration {
		return nil, errors.NotFoundf("migration")
	}
	return new(fakeModelMigration), nil
}

func (b *fakeMigrationBackend) APIHostPortsForClients(controller.Config) ([]network.SpaceHostPorts, error) {
	return []network.SpaceHostPorts{
		{
			network.SpaceHostPort{SpaceAddress: network.NewSpaceAddress("1.2.3.4"), NetPort: 5},
			network.SpaceHostPort{SpaceAddress: network.NewSpaceAddress("2.3.4.5"), NetPort: 6},
		}, {
			network.SpaceHostPort{SpaceAddress: network.NewSpaceAddress("3.4.5.6"), NetPort: 7},
		},
	}, nil
}

func (b *fakeMigrationBackend) ControllerModel() (*state.Model, error) {
	return nil, nil
}

func (b *fakeMigrationBackend) ControllerConfig() (controller.Config, error) {
	return nil, nil
}

type fakeModelMigration struct {
	state.ModelMigration
}

func (m *fakeModelMigration) Id() string {
	return "id"
}

func (m *fakeModelMigration) Attempt() int {
	return 2
}

func (m *fakeModelMigration) Phase() (migration.Phase, error) {
	return migration.IMPORT, nil
}

func (m *fakeModelMigration) TargetInfo() (*migration.TargetInfo, error) {
	return &migration.TargetInfo{
		ControllerTag: names.NewControllerTag("uuid"),
		Addrs:         []string{"1.2.3.4:5555"},
		CACert:        "trust me",
		AuthTag:       names.NewUserTag("admin"),
		Password:      "sekret",
	}, nil
}

type migrationStatusWatcher interface {
	Next(context.Context) (params.MigrationStatus, error)
	Stop() error
}

func nopDispose() {}
