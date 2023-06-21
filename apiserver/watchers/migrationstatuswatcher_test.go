// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/watchers"
	"github.com/juju/juju/rpc/params"
)

type migrationStatusWatcherSuite struct {
	baseSuite
}

var _ = gc.Suite(&migrationStatusWatcherSuite{})

func (s *migrationStatusWatcherSuite) TestMigrationStatusWatcher(c *gc.C) {
	w := apiservertesting.NewFakeNotifyWatcher()
	id, err := s.watcherRegistry.Register(w)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer.Tag = names.NewMachineTag("12")
	watchers.PatchGetMigrationBackend(s, new(fakeMigrationBackend), new(fakeMigrationBackend))
	watchers.PatchGetControllerCACert(s, "no worries")

	facade := s.getFacade(c, "MigrationStatusWatcher", 1, id, nopDispose).(migrationStatusWatcher)
	defer c.Check(facade.Stop(), jc.ErrorIsNil)
	result, err := facade.Next()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.MigrationStatus{
		MigrationId:    "id",
		Attempt:        2,
		Phase:          "IMPORT",
		SourceAPIAddrs: []string{"1.2.3.4:5", "2.3.4.5:6", "3.4.5.6:7"},
		SourceCACert:   "no worries",
		TargetAPIAddrs: []string{"1.2.3.4:5555"},
		TargetCACert:   "trust me",
	})
}

func (s *migrationStatusWatcherSuite) TestMigrationStatusWatcherNoMigration(c *gc.C) {
	w := apiservertesting.NewFakeNotifyWatcher()
	id, err := s.watcherRegistry.Register(w)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer.Tag = names.NewMachineTag("12")
	backend := &fakeMigrationBackend{noMigration: true}
	watchers.PatchGetMigrationBackend(s, backend, backend)

	facade := s.getFacade(c, "MigrationStatusWatcher", 1, id, nopDispose).(migrationStatusWatcher)
	defer c.Check(facade.Stop(), jc.ErrorIsNil)
	result, err := facade.Next()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.MigrationStatus{
		Phase: "NONE",
	})
}

func (s *migrationStatusWatcherSuite) TestMigrationStatusWatcherNotAgent(c *gc.C) {
	id, err := s.watcherRegistry.Register(apiservertesting.NewFakeNotifyWatcher())
	c.Assert(err, jc.ErrorIsNil)
	s.authorizer.Tag = names.NewUserTag("frogdog")

	factory, err := apiserver.AllFacades().GetFactory("MigrationStatusWatcher", 1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = factory(facadetest.Context{
		WatcherRegistry_: s.watcherRegistry,
		Auth_:            s.authorizer,
		ID_:              id,
	})
	c.Assert(err, gc.Equals, apiservererrors.ErrPerm)
}
