// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type watcherSuite struct {
	testing.BaseSuite
	st         *state.State
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.st = nil // none of the watcher facades use the State object
	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{}
}

func (s *watcherSuite) getFacade(c *gc.C, name string, version int, id string) interface{} {
	factory, err := common.Facades.GetFactory(name, version)
	c.Assert(err, jc.ErrorIsNil)
	facade, err := factory(s.st, s.resources, s.authorizer, id)
	c.Assert(err, jc.ErrorIsNil)
	return facade
}

func (s *watcherSuite) TestVolumeAttachmentsWatcher(c *gc.C) {
	ch := make(chan []string, 1)
	id := s.resources.Register(&fakeStringsWatcher{ch: ch})
	s.authorizer.Tag = names.NewMachineTag("123")

	ch <- []string{"0:1", "1:2"}
	facade := s.getFacade(c, "VolumeAttachmentsWatcher", 2, id).(machineStorageIdsWatcher)
	result, err := facade.Next()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, params.MachineStorageIdsWatchResult{
		Changes: []params.MachineStorageId{
			{MachineTag: "machine-0", AttachmentTag: "volume-1"},
			{MachineTag: "machine-1", AttachmentTag: "volume-2"},
		},
	})
}

func (s *watcherSuite) TestFilesystemAttachmentsWatcher(c *gc.C) {
	ch := make(chan []string, 1)
	id := s.resources.Register(&fakeStringsWatcher{ch: ch})
	s.authorizer.Tag = names.NewMachineTag("123")

	ch <- []string{"0:1", "1:2"}
	facade := s.getFacade(c, "FilesystemAttachmentsWatcher", 2, id).(machineStorageIdsWatcher)
	result, err := facade.Next()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, params.MachineStorageIdsWatchResult{
		Changes: []params.MachineStorageId{
			{MachineTag: "machine-0", AttachmentTag: "filesystem-1"},
			{MachineTag: "machine-1", AttachmentTag: "filesystem-2"},
		},
	})
}

func (s *watcherSuite) TestMigrationMasterWatcher(c *gc.C) {
	w := apiservertesting.NewFakeNotifyWatcher()
	id := s.resources.Register(w)
	s.authorizer.EnvironManager = true
	apiserver.PatchGetMigrationBackend(s, new(fakeMigrationBackend))

	w.C <- struct{}{}
	facade := s.getFacade(c, "MigrationMasterWatcher", 1, id).(migrationMasterWatcher)
	defer c.Check(facade.Stop(), jc.ErrorIsNil)
	result, err := facade.Next()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, params.ModelMigrationTargetInfo{
		ControllerTag: "model-uuid",
		Addrs:         []string{"1.2.3.4:5555"},
		CACert:        "trust me",
		AuthTag:       "user-admin",
		Password:      "sekret",
	})
}

func (s *watcherSuite) TestMigrationMasterNotModelManager(c *gc.C) {
	id := s.resources.Register(apiservertesting.NewFakeNotifyWatcher())
	s.authorizer.EnvironManager = false

	factory, err := common.Facades.GetFactory("MigrationMasterWatcher", 1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = factory(s.st, s.resources, s.authorizer, id)
	c.Assert(err, gc.Equals, common.ErrPerm)
}

func (s *watcherSuite) TestMigrationAgentWatcher(c *gc.C) {
	w := apiservertesting.NewFakeNotifyWatcher()
	id := s.resources.Register(w)
	s.authorizer.Tag = names.NewMachineTag("12")
	apiserver.PatchGetMigrationBackend(s, new(fakeMigrationBackend))

	w.C <- struct{}{}
	facade := s.getFacade(c, "MigrationAgentWatcher", 1, id).(migrationAgentWatcher)
	defer c.Check(facade.Stop(), jc.ErrorIsNil)
	result, err := facade.Next()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.MigrationStatus{
		Attempt:        2,
		Phase:          migration.READONLY,
		SourceAPIAddrs: []string{"1.2.3.4:5", "2.3.4.5:6", "3.4.5.6:7"},
		TargetAPIAddrs: []string{"1.2.3.4:5555"},
	})
}

func (s *watcherSuite) TestMigrationAgentNotAgent(c *gc.C) {
	id := s.resources.Register(apiservertesting.NewFakeNotifyWatcher())
	s.authorizer.Tag = names.NewUserTag("frogdog")

	factory, err := common.Facades.GetFactory("MigrationAgentWatcher", 1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = factory(s.st, s.resources, s.authorizer, id)
	c.Assert(err, gc.Equals, common.ErrPerm)
}

type machineStorageIdsWatcher interface {
	Next() (params.MachineStorageIdsWatchResult, error)
}

type fakeStringsWatcher struct {
	state.StringsWatcher
	ch chan []string
}

func (w *fakeStringsWatcher) Changes() <-chan []string {
	return w.ch
}

type fakeMigrationBackend struct{}

func (b *fakeMigrationBackend) GetModelMigration() (state.ModelMigration, error) {
	return new(fakeModelMigration), nil
}

func (b *fakeMigrationBackend) APIHostPorts() ([][]network.HostPort, error) {
	return [][]network.HostPort{
		MustParseHostPorts("1.2.3.4:5", "2.3.4.5:6"),
		MustParseHostPorts("3.4.5.6:7"),
	}, nil
}

func MustParseHostPorts(hostports ...string) []network.HostPort {
	out, err := network.ParseHostPorts(hostports...)
	if err != nil {
		panic(err)
	}
	return out
}

type fakeModelMigration struct {
	state.ModelMigration
}

func (m *fakeModelMigration) Attempt() (int, error) {
	return 2, nil
}

func (m *fakeModelMigration) Phase() (migration.Phase, error) {
	return migration.READONLY, nil
}

func (m *fakeModelMigration) TargetInfo() (*migration.TargetInfo, error) {
	return &migration.TargetInfo{
		ControllerTag: names.NewModelTag("uuid"),
		Addrs:         []string{"1.2.3.4:5555"},
		CACert:        "trust me",
		AuthTag:       names.NewUserTag("admin"),
		Password:      "sekret",
	}, nil
}

type migrationMasterWatcher interface {
	Next() (params.ModelMigrationTargetInfo, error)
	Stop() error
}

type migrationAgentWatcher interface {
	Next() (params.MigrationStatus, error)
	Stop() error
}
