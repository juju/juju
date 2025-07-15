// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
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

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

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

func (b *fakeMigrationBackend) GetAllAPIAddressesForClients(ctx context.Context) ([]string, error) {
	return []string{"1.2.3.4:5", "2.3.4.5:6", "3.4.5.6:7"}, nil
}

func (b *fakeMigrationBackend) ControllerModel() (*state.Model, error) {
	return nil, nil
}

func (b *fakeMigrationBackend) ControllerConfig() (controller.Config, error) {
	return nil, nil
}

type migrationStatusWatcher interface {
	Next(context.Context) (params.MigrationStatus, error)
	Stop() error
}

func nopDispose() {}
