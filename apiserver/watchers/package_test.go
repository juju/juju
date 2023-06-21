// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers_test

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.BaseSuite
	watcherRegistry facade.WatcherRegistry
	resources       *common.Resources
	authorizer      apiservertesting.FakeAuthorizer
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)

	s.resources = common.NewResources()

	s.AddCleanup(func(*gc.C) {
		s.watcherRegistry.Kill()
		err := s.watcherRegistry.Wait()
		c.Assert(err, jc.ErrorIsNil)

		s.resources.StopAll()
	})
	s.authorizer = apiservertesting.FakeAuthorizer{}
}

func (s *baseSuite) getFacade(
	c *gc.C,
	name string,
	version int,
	id string,
	dispose func(),
) interface{} {
	factory := getFacadeFactory(c, name, version)
	facade, err := factory(s.facadeContext(id, dispose))
	c.Assert(err, jc.ErrorIsNil)
	return facade
}

func (s *baseSuite) facadeContext(id string, dispose func()) facadetest.Context {
	return facadetest.Context{
		ID_:              id,
		Auth_:            s.authorizer,
		WatcherRegistry_: s.watcherRegistry,
		Resources_:       s.resources,
		Dispose_:         dispose,
	}
}

func getFacadeFactory(c *gc.C, name string, version int) facade.Factory {
	factory, err := apiserver.AllFacades().GetFactory(name, version)
	c.Assert(err, jc.ErrorIsNil)
	return factory
}

type machineStorageIdsWatcher interface {
	Next() (params.MachineStorageIdsWatchResult, error)
}

type fakeStringsWatcher struct {
	state.StringsWatcher
	ch   chan []string
	done chan struct{}
}

func (w *fakeStringsWatcher) Changes() <-chan []string {
	return w.ch
}

func (w *fakeStringsWatcher) Kill() {}

func (w *fakeStringsWatcher) Wait() error {
	select {
	case <-w.done:
	case <-time.After(jujutesting.LongWait):
		return errors.Errorf("timed out waiting for watcher to stop")
	}
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

func (b *fakeMigrationBackend) APIHostPortsForClients() ([]network.SpaceHostPorts, error) {
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
	Next() (params.MigrationStatus, error)
	Stop() error
}

func nopDispose() {}
