// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"context"
	"sort"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/core/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type caasProvisionerSuite struct {
	provisionerSuite
}

var _ = gc.Suite(&caasProvisionerSuite{})

func (s *caasProvisionerSuite) SetUpTest(c *gc.C) {
	s.provisionerSuite.SetUpTest(c)
	s.provisionerSuite.SeedCAASCloud(c)
	s.provisionerSuite.storageSetUp = s

	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	s.st = f.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { s.st.Close() })
	var err error
	m, err := s.st.Model()
	c.Assert(err, jc.ErrorIsNil)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	serviceFactory := s.ControllerServiceFactory(c)
	modelInfo, err := serviceFactory.ModelInfo().GetModelInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(m, serviceFactory.Cloud(), serviceFactory.Credential())
	c.Assert(err, jc.ErrorIsNil)
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	serviceFactoryGetter := s.ServiceFactoryGetter(c)
	storageService := serviceFactoryGetter.FactoryForModel(s.st.ModelUUID()).Storage(registry)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	backend, storageBackend, err := storageprovisioner.NewStateBackends(s.st)
	c.Assert(err, jc.ErrorIsNil)
	s.storageBackend = storageBackend
	s.api, err = storageprovisioner.NewStorageProvisionerAPIv4(
		context.Background(),
		nil, // tests which need a watcher factory need to create an api with a non nil value.
		backend,
		storageBackend,
		s.DefaultModelServiceFactory(c).BlockDevice(),
		s.ControllerServiceFactory(c).ControllerConfig(),
		s.ControllerServiceFactory(c).Config(),
		s.resources,
		s.authorizer,
		registry,
		storageService,
		loggertesting.WrapCheckLog(c),
		modelInfo.UUID,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *caasProvisionerSuite) setupFilesystems(c *gc.C) {
	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

	ch := f.MakeCharm(c, &factory.CharmParams{
		Name:   "storage-filesystem",
		Series: "focal",
	})
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: ch,
		Name:  "mariadb",
		Storage: map[string]state.StorageConstraints{
			"data":  {Count: 1, Size: 1024},
			"cache": {Count: 2, Size: 1024},
		},
	})
	f.MakeUnit(c, &factory.UnitParams{Application: app})

	// Only provision the first and third backing volumes.
	err := s.storageBackend.SetVolumeInfo(names.NewVolumeTag("0"), state.VolumeInfo{
		HardwareId: "123",
		VolumeId:   "abc",
		Size:       1024,
		Persistent: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeAttachmentInfo(
		names.NewUnitTag("mariadb/0"),
		names.NewVolumeTag("0"),
		state.VolumeAttachmentInfo{ReadOnly: false},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.SetVolumeInfo(names.NewVolumeTag("2"), state.VolumeInfo{
		HardwareId: "456",
		VolumeId:   "def",
		Size:       4096,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeAttachmentInfo(
		names.NewUnitTag("mariadb/0"),
		names.NewVolumeTag("2"),
		state.VolumeAttachmentInfo{ReadOnly: false},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Only provision the first and third filesystems.
	err = s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("0"), state.FilesystemInfo{
		FilesystemId: "abc",
		Size:         1024,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("2"), state.FilesystemInfo{
		FilesystemId: "def",
		Size:         4096,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *caasProvisionerSuite) TestWatchApplications(c *gc.C) {
	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

	ch := f.MakeCharm(c, &factory.CharmParams{
		Name:   "storage-filesystem",
		Series: "focal",
	})
	f.MakeApplication(c, &factory.ApplicationParams{
		Charm: ch,
		Name:  "mariadb",
		Storage: map[string]state.StorageConstraints{
			"data": {Count: 1, Size: 1024},
		},
	})

	result, err := s.api.WatchApplications(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, []string{"mariadb"})

	w := s.resources.Get("1").(state.StringsWatcher)
	f.MakeApplication(c, &factory.ApplicationParams{
		Charm: ch,
		Name:  "mysql",
		Storage: map[string]state.StorageConstraints{
			"data": {Count: 1, Size: 1024},
		},
	})
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange("mysql")
}

func (s *caasProvisionerSuite) TestWatchFilesystemAttachments(c *gc.C) {
	s.setupFilesystems(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	modelInfo, err := s.ControllerServiceFactory(c).ModelInfo().GetModelInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "application-mariadb"},
		{Tag: names.NewModelTag(modelInfo.UUID.String()).String()},
		{Tag: "environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{Tag: "unit-mysql-0"}},
	}
	result, err := s.api.WatchFilesystemAttachments(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	sort.Sort(byMachineAndEntity(result.Results[0].Changes))
	sort.Sort(byMachineAndEntity(result.Results[1].Changes))
	c.Assert(result, jc.DeepEquals, params.MachineStorageIdsWatchResults{
		Results: []params.MachineStorageIdsWatchResult{
			{
				MachineStorageIdsWatcherId: "1",
				Changes: []params.MachineStorageId{{
					MachineTag:    "unit-mariadb-0",
					AttachmentTag: "filesystem-0",
				}, {
					MachineTag:    "unit-mariadb-0",
					AttachmentTag: "filesystem-1",
				}, {
					MachineTag:    "unit-mariadb-0",
					AttachmentTag: "filesystem-2",
				}},
			}, {
				MachineStorageIdsWatcherId: "2",
				Changes:                    []params.MachineStorageId{},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 2)
	v0Watcher := s.resources.Get("1")
	defer workertest.CleanKill(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewStringsWatcherC(c, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *caasProvisionerSuite) TestRemoveFilesystemAttachments(c *gc.C) {
	s.setupFilesystems(c)

	err := s.storageBackend.DetachFilesystem(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.RemoveAttachment(context.Background(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-0",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-1",
		}, {
			MachineTag:    "unit-mysql-2",
			AttachmentTag: "filesystem-4",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "removing attachment of filesystem 0 from unit mariadb/0: filesystem attachment is not dying"}},
			{Error: nil},
			{Error: &params.Error{Message: `removing attachment of filesystem 4 from unit mysql/2: filesystem "4" on "unit mysql/2" not found`, Code: "not found"}},
			{Error: &params.Error{Message: `removing attachment of filesystem 42 from unit mariadb/0: filesystem "42" on "unit mariadb/0" not found`, Code: "not found"}},
		},
	})
}

func (s *caasProvisionerSuite) TestRemoveFilesystemsApplicationAgent(c *gc.C) {
	s.setupFilesystems(c)
	s.authorizer.Controller = false
	args := params.Entities{Entities: []params.Entity{
		{Tag: "filesystem-42"},
		{Tag: "filesystem-invalid"}, {Tag: "machine-0"},
	}}

	err := s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("0"), false)
	c.Assert(err, gc.ErrorMatches, "destroying filesystem 0: filesystem is assigned to storage cache/0")
	err = s.storageBackend.RemoveFilesystemAttachment(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("0"), false)
	c.Assert(err, gc.ErrorMatches, "removing attachment of filesystem 0 from unit mariadb/0: filesystem attachment is not dying")

	result, err := s.api.Remove(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `"filesystem-invalid" is not a valid filesystem tag`}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *caasProvisionerSuite) TestFilesystemLife(c *gc.C) {
	s.setupFilesystems(c)
	args := params.Entities{Entities: []params.Entity{{Tag: "filesystem-0"}, {Tag: "filesystem-1"}, {Tag: "filesystem-42"}}}
	result, err := s.api.Life(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Alive},
			{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: `filesystem "42" not found`,
			}},
		},
	})
}

func (s *caasProvisionerSuite) TestFilesystemAttachmentLife(c *gc.C) {
	s.setupFilesystems(c)

	results, err := s.api.AttachmentLife(context.Background(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-0",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-1",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Alive},
			{Error: &params.Error{Message: `filesystem "42" on "unit mariadb/0" not found`, Code: "not found"}},
		},
	})
}
