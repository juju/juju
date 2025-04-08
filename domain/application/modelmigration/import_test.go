// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/description/v9"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/config"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/service"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	testing.IsolationSuite

	importService *MockImportService

	charmMetadata  *MockCharmMetadata
	charmRequires  *MockCharmMetadataRelation
	charmProvides  *MockCharmMetadataRelation
	charmPeers     *MockCharmMetadataRelation
	storage        *MockCharmMetadataStorage
	device         *MockCharmMetadataDevice
	container      *MockCharmMetadataContainer
	containerMount *MockCharmMetadataContainerMount
	resources      *MockCharmMetadataResource

	charmManifest *MockCharmManifest
	charmBase     *MockCharmManifestBase

	charmConfigs *MockCharmConfigs
	charmConfig  *MockCharmConfig

	charmActions *MockCharmActions
	charmAction  *MockCharmAction
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) TestRollback(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	model.AddApplication(appArgs)

	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}

	s.importService.EXPECT().RemoveImportedApplication(gomock.Any(), "prometheus").Return(nil)

	err := importOp.Rollback(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestRollbackForMultipleApplicationsRollbacksAll(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	appArgs0 := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	model.AddApplication(appArgs0)

	appArgs1 := description.ApplicationArgs{
		Name:     "grafana",
		CharmURL: "ch:grafana-1",
	}
	model.AddApplication(appArgs1)

	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}

	gomock.InOrder(
		s.importService.EXPECT().RemoveImportedApplication(gomock.Any(), "prometheus").Return(errors.Errorf("boom")),
		s.importService.EXPECT().RemoveImportedApplication(gomock.Any(), "grafana").Return(nil),
	)

	err := importOp.Rollback(context.Background(), model)
	c.Assert(err, gc.ErrorMatches, "rollback failed: boom")
}

func (s *importSuite) TestApplicationImportWithMinimalCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name:         "prometheus/0",
		PasswordHash: "passwordhash",
		CloudContainer: &description.CloudContainerArgs{
			ProviderId: "provider-id",
			Address: description.AddressArgs{
				Value:   "10.6.6.6",
				Type:    "ipv4",
				Scope:   "local-machine",
				Origin:  "provider",
				SpaceID: "666",
			},
			Ports: []string{"6666"},
		},
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "prometheus",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{baseType{
			name:          "ubuntu",
			channel:       "24.04",
			architectures: []string{"amd64"},
		}},
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "1234",
		Hash:     "deadbeef",
		Revision: 1,
		Channel:  "666/stable",
		Platform: "arm64/ubuntu/24.04",
	})

	var importArgs service.ImportApplicationArgs
	s.importService.EXPECT().ImportApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, args service.ImportApplicationArgs) error {
		importArgs = args
		return nil
	})

	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(importArgs.Charm.Meta().Name, gc.Equals, "prometheus")
	c.Check(importArgs.Units, gc.DeepEquals, []service.ImportUnitArg{{
		UnitName:     "prometheus/0",
		PasswordHash: ptr("passwordhash"),
		CloudContainer: ptr(application.CloudContainerParams{
			ProviderID: "provider-id",
			Address: ptr(network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.6.6.6",
					Type:  "ipv4",
					Scope: "local-machine",
				},
				SpaceID: "666",
			}),
			AddressOrigin: ptr(network.OriginProvider),
			Ports:         ptr([]string{"6666"}),
		}),
	}})
}

func (s *importSuite) TestApplicationImportWithApplicationConfigAndSettings(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		CharmConfig: map[string]interface{}{
			"foo": "bar",
		},
		ApplicationConfig: map[string]interface{}{
			"trust": true,
		},
	}
	app := model.AddApplication(appArgs)
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "prometheus",
	})
	app.SetCharmConfigs(description.CharmConfigsArgs{
		Configs: map[string]description.CharmConfig{
			"foo": charmConfig{ConfigType: "string", DefaultValue: "baz"},
		},
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{baseType{
			name:          "ubuntu",
			channel:       "24.04",
			architectures: []string{"amd64"},
		}},
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "1234",
		Hash:     "deadbeef",
		Revision: 1,
		Channel:  "666/stable",
		Platform: "arm64/ubuntu/24.04",
	})

	var importArgs service.ImportApplicationArgs
	s.importService.EXPECT().ImportApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, args service.ImportApplicationArgs) error {
		importArgs = args
		return nil
	})

	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(importArgs.Charm.Meta().Name, gc.Equals, "prometheus")
	c.Check(importArgs.ApplicationConfig, jc.DeepEquals, config.ConfigAttributes{
		"foo": "bar",
	})
	c.Check(importArgs.ApplicationSettings, jc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})
}

func (s *importSuite) TestApplicationImportWithConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "prometheus",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{baseType{
			name:          "ubuntu",
			channel:       "24.04",
			architectures: []string{"amd64"},
		}},
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "1234",
		Hash:     "deadbeef",
		Revision: 1,
		Channel:  "666/stable",
		Platform: "arm64/ubuntu/24.04",
	})

	app.SetConstraints(description.ConstraintsArgs{
		AllocatePublicIP: true,
		Architecture:     "amd64",
		Container:        "lxd",
		CpuCores:         uint64(2),
		CpuPower:         uint64(1000),
		ImageID:          "foo",
		InstanceType:     "baz",
		VirtType:         "vm",
		Memory:           uint64(1024),
		RootDisk:         uint64(1024),
		RootDiskSource:   "qux",
		Spaces:           []string{"space0", "space1"},
		Tags:             []string{"tag0", "tag1"},
		Zones:            []string{"zone0", "zone1"},
	})

	s.importService.EXPECT().ImportApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, args service.ImportApplicationArgs) error {
		c.Assert(args.Charm.Meta().Name, gc.Equals, "prometheus")
		c.Check(args.ApplicationConstraints.AllocatePublicIP, gc.DeepEquals, ptr(true))
		c.Check(args.ApplicationConstraints.Arch, gc.DeepEquals, ptr("amd64"))
		c.Check(args.ApplicationConstraints.Container, gc.DeepEquals, ptr(instance.ContainerType("lxd")))
		c.Check(args.ApplicationConstraints.CpuCores, gc.DeepEquals, ptr(uint64(2)))
		c.Check(args.ApplicationConstraints.CpuPower, gc.DeepEquals, ptr(uint64(1000)))
		c.Check(args.ApplicationConstraints.ImageID, gc.DeepEquals, ptr("foo"))
		c.Check(args.ApplicationConstraints.InstanceType, gc.DeepEquals, ptr("baz"))
		c.Check(args.ApplicationConstraints.VirtType, gc.DeepEquals, ptr("vm"))
		c.Check(args.ApplicationConstraints.Mem, gc.DeepEquals, ptr(uint64(1024)))
		c.Check(args.ApplicationConstraints.RootDisk, gc.DeepEquals, ptr(uint64(1024)))
		c.Check(args.ApplicationConstraints.RootDiskSource, gc.DeepEquals, ptr("qux"))
		c.Check(args.ApplicationConstraints.Spaces, gc.DeepEquals, ptr([]string{"space0", "space1"}))
		c.Check(args.ApplicationConstraints.Tags, gc.DeepEquals, ptr([]string{"tag0", "tag1"}))
		c.Check(args.ApplicationConstraints.Zones, gc.DeepEquals, ptr([]string{"zone0", "zone1"}))
		return nil
	})

	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportCharmMetadataEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := importCharmMetadata(nil)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("foo").Times(2)

	_, err := importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidAssumes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("!![]")

	_, err := importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidMinJujuVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("[]")
	metaExp.MinJujuVersion().Return("foo")

	_, err := importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidRelationRole(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.charmProvides.EXPECT()
	exp.Role().Return("blah")

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("[]")
	metaExp.MinJujuVersion().Return("4.0.0")

	metaExp.Provides().Return(map[string]description.CharmMetadataRelation{
		"provides": s.charmProvides,
	})

	_, err := importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidRelationScope(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.charmProvides.EXPECT()
	exp.Role().Return("provider")
	exp.Scope().Return("blah")

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("[]")
	metaExp.MinJujuVersion().Return("4.0.0")

	metaExp.Provides().Return(map[string]description.CharmMetadataRelation{
		"provides": s.charmProvides,
	})

	_, err := importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidStorage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectRequiresRelation()
	s.expectProvidesRelation()
	s.expectPeersRelation()

	exp := s.storage.EXPECT()
	exp.Type().Return("fred")

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("[]")
	metaExp.MinJujuVersion().Return("4.0.0")

	metaExp.Provides().Return(map[string]description.CharmMetadataRelation{
		"provides": s.charmProvides,
	})
	metaExp.Requires().Return(map[string]description.CharmMetadataRelation{
		"requires": s.charmRequires,
	})
	metaExp.Peers().Return(map[string]description.CharmMetadataRelation{
		"peers": s.charmPeers,
	})
	metaExp.Storage().Return(map[string]description.CharmMetadataStorage{
		"storage": s.storage,
	})

	_, err := importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectRequiresRelation()
	s.expectProvidesRelation()
	s.expectPeersRelation()
	s.expectStorage()
	s.expectDevice()
	s.expectContainer()

	exp := s.resources.EXPECT()
	exp.Type().Return("fred")

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("[]")
	metaExp.MinJujuVersion().Return("4.0.0")

	metaExp.Provides().Return(map[string]description.CharmMetadataRelation{
		"provides": s.charmProvides,
	})
	metaExp.Requires().Return(map[string]description.CharmMetadataRelation{
		"requires": s.charmRequires,
	})
	metaExp.Peers().Return(map[string]description.CharmMetadataRelation{
		"peers": s.charmPeers,
	})
	metaExp.Storage().Return(map[string]description.CharmMetadataStorage{
		"storage": s.storage,
	})
	metaExp.Devices().Return(map[string]description.CharmMetadataDevice{
		"device": s.device,
	})
	metaExp.Containers().Return(map[string]description.CharmMetadataContainer{
		"container": s.container,
	})
	metaExp.Resources().Return(map[string]description.CharmMetadataResource{
		"resource": s.resources,
	})

	_, err := importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectRequiresRelation()
	s.expectProvidesRelation()
	s.expectPeersRelation()
	s.expectStorage()
	s.expectDevice()
	s.expectContainer()
	s.expectResource()

	metaExp := s.charmMetadata.EXPECT()
	metaExp.Name().Return("foo")
	metaExp.Summary().Return("bar")
	metaExp.Description().Return("baz")
	metaExp.Subordinate().Return(true)
	metaExp.Categories().Return([]string{"foo", "bar"})
	metaExp.Tags().Return([]string{"baz", "qux"})
	metaExp.Terms().Return([]string{"alpha"})
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("[]")
	metaExp.MinJujuVersion().Return("4.0.0")
	metaExp.Provides().Return(map[string]description.CharmMetadataRelation{
		"provides": s.charmProvides,
	})
	metaExp.Requires().Return(map[string]description.CharmMetadataRelation{
		"requires": s.charmRequires,
	})
	metaExp.Peers().Return(map[string]description.CharmMetadataRelation{
		"peers": s.charmPeers,
	})
	metaExp.ExtraBindings().Return(map[string]string{
		"foo": "bar",
	})
	metaExp.Storage().Return(map[string]description.CharmMetadataStorage{
		"storage": s.storage,
	})
	metaExp.Devices().Return(map[string]description.CharmMetadataDevice{
		"device": s.device,
	})
	metaExp.Containers().Return(map[string]description.CharmMetadataContainer{
		"container": s.container,
	})
	metaExp.Resources().Return(map[string]description.CharmMetadataResource{
		"resource": s.resources,
	})

	meta, err := importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(meta, gc.DeepEquals, &internalcharm.Meta{
		Name:           "foo",
		Summary:        "bar",
		Description:    "baz",
		Subordinate:    true,
		Categories:     []string{"foo", "bar"},
		Tags:           []string{"baz", "qux"},
		Terms:          []string{"alpha"},
		CharmUser:      "root",
		MinJujuVersion: semversion.MustParse("4.0.0"),
		Assumes: &assumes.ExpressionTree{
			Expression: assumes.CompositeExpression{
				ExprType:       assumes.AllOfExpression,
				SubExpressions: []assumes.Expression{},
			},
		},
		Provides: map[string]internalcharm.Relation{
			"provides": {
				Name:      "bar",
				Role:      "provider",
				Interface: "db",
				Optional:  true,
				Limit:     1,
				Scope:     "global",
			},
		},
		Requires: map[string]internalcharm.Relation{
			"requires": {
				Name:      "foo",
				Role:      "requirer",
				Interface: "db",
				Optional:  false,
				Limit:     0,
				Scope:     "global",
			},
		},
		Peers: map[string]internalcharm.Relation{
			"peers": {
				Name:      "baz",
				Role:      "peer",
				Interface: "db",
				Optional:  true,
				Limit:     1,
				Scope:     "container",
			},
		},
		ExtraBindings: map[string]internalcharm.ExtraBinding{
			"foo": {
				Name: "bar",
			},
		},
		Storage: map[string]internalcharm.Storage{
			"storage": {
				Name:        "baz",
				Type:        "filesystem",
				Description: "baz storage",
				Shared:      true,
				ReadOnly:    true,
				MinimumSize: 1024,
				Location:    "baz location",
				CountMin:    1,
				CountMax:    2,
				Properties:  []string{"baz"},
			},
		},
		Devices: map[string]internalcharm.Device{
			"device": {
				Name:        "baz",
				Type:        "gpu",
				Description: "baz device",
				CountMin:    1,
				CountMax:    2,
			},
		},
		Containers: map[string]internalcharm.Container{
			"container": {
				Resource: "baz",
				Gid:      ptr(1000),
				Uid:      nil,
				Mounts: []internalcharm.Mount{
					{
						Location: "baz",
						Storage:  "bar",
					},
				},
			},
		},
		Resources: map[string]resource.Meta{
			"resource": {
				Name:        "baz",
				Description: "baz resource",
				Path:        "baz",
				Type:        resource.TypeFile,
			},
		},
	})
}

func (s *importSuite) TestImportEmptyCharmManifest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectEmptyManifestBases()

	_, err := importCharmManifest(s.charmManifest)
	c.Assert(err, gc.NotNil)
}

func (s *importSuite) TestImportCharmManifest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectManifestBases()

	meta, err := importCharmManifest(s.charmManifest)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(meta, gc.DeepEquals, &internalcharm.Manifest{
		Bases: []internalcharm.Base{
			{
				Name: "ubuntu",
				Channel: internalcharm.Channel{
					Track: "4.0",
					Risk:  "stable",
				},
				Architectures: []string{"amd64"},
			},
		},
	})
}

func (s *importSuite) TestImportCharmManifestWithInvalidBase(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Notice that we do allow centos here for now. We probably want to
	// consider preventing a migration with anything other than ubuntu.

	baseExp := s.charmBase.EXPECT()
	baseExp.Name().Return("centos")
	baseExp.Channel().Return("4.0/blah")

	exp := s.charmManifest.EXPECT()
	exp.Bases().Return([]description.CharmManifestBase{
		s.charmBase,
	})

	_, err := importCharmManifest(s.charmManifest)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportEmptyCharmLXDProfile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectEmptyLXDProfile()

	meta, err := importCharmLXDProfile(s.charmMetadata)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(meta, gc.IsNil)
}

func (s *importSuite) TestImportCharmLXDProfile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectLXDProfile()

	meta, err := importCharmLXDProfile(s.charmMetadata)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(meta, gc.DeepEquals, &internalcharm.LXDProfile{
		Config: map[string]string{
			"foo": "bar",
		},
	})
}

func (s *importSuite) TestImportEmptyCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectEmptyCharmConfigs()

	meta, err := importCharmConfig(s.charmConfigs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta, gc.NotNil)
	c.Check(meta.Options, gc.HasLen, 0)
}

func (s *importSuite) TestImportCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCharmConfigs()

	meta, err := importCharmConfig(s.charmConfigs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta, gc.NotNil)
	c.Check(meta.Options, gc.DeepEquals, map[string]internalcharm.Option{
		"foo": {
			Type:        "string",
			Default:     "bar",
			Description: "baz",
		},
	})
}

func (s *importSuite) TestImportEmptyCharmActions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectEmptyCharmActions()

	meta, err := importCharmActions(s.charmActions)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta, gc.NotNil)
	c.Check(meta.ActionSpecs, gc.HasLen, 0)
}

func (s *importSuite) TestImportCharmActions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCharmActions()

	meta, err := importCharmActions(s.charmActions)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta, gc.NotNil)
	c.Check(meta.ActionSpecs, gc.DeepEquals, map[string]internalcharm.ActionSpec{
		"foo": {
			Description:    "baz",
			Parallel:       true,
			ExecutionGroup: "group",
			Params: map[string]interface{}{
				"foo": "bar",
			},
		},
	})
}

func (s *importSuite) TestImportCharmActionsNestedMaps(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCharmActionsNested()

	meta, err := importCharmActions(s.charmActions)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta, gc.NotNil)
	c.Check(meta.ActionSpecs, gc.DeepEquals, map[string]internalcharm.ActionSpec{
		"foo": {
			Description:    "baz",
			Parallel:       true,
			ExecutionGroup: "group",
			Params: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "baz",
					"foo": map[string]interface{}{
						"1":    2,
						"true": false,
						"0.1":  "0.2",
						"2":    int64(2),
					},
				},
			},
		},
	})
}

func (s *importSuite) TestImportExposedEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		Exposed:  true,
		ExposedEndpoints: map[string]description.ExposedEndpointArgs{
			"": {
				ExposeToSpaceIDs: []string{"alpha"},
			},
			"endpoint0": {
				ExposeToCIDRs:    []string{"10.0.0.0/24", "10.0.1.0/24"},
				ExposeToSpaceIDs: []string{"space0", "space1"},
			},
		},
	}
	app := model.AddApplication(appArgs)
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "prometheus",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{baseType{
			name:          "ubuntu",
			channel:       "24.04",
			architectures: []string{"amd64"},
		}},
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "1234",
		Hash:     "deadbeef",
		Revision: 1,
		Channel:  "666/stable",
		Platform: "arm64/ubuntu/24.04",
	})

	s.importService.EXPECT().ImportApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, args service.ImportApplicationArgs) error {
		c.Assert(args.Charm.Meta().Name, gc.Equals, "prometheus")
		c.Check(args.ExposedEndpoints, gc.HasLen, 2)
		c.Check(args.ExposedEndpoints[""].ExposeToSpaceIDs, gc.DeepEquals, set.NewStrings("alpha"))
		c.Check(args.ExposedEndpoints["endpoint0"].ExposeToCIDRs, gc.DeepEquals, set.NewStrings("10.0.0.0/24", "10.0.1.0/24"))
		c.Check(args.ExposedEndpoints["endpoint0"].ExposeToSpaceIDs, gc.DeepEquals, set.NewStrings("space0", "space1"))
		return nil
	})

	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)

	s.charmMetadata = NewMockCharmMetadata(ctrl)
	s.charmProvides = NewMockCharmMetadataRelation(ctrl)
	s.charmRequires = NewMockCharmMetadataRelation(ctrl)
	s.charmPeers = NewMockCharmMetadataRelation(ctrl)
	s.storage = NewMockCharmMetadataStorage(ctrl)
	s.device = NewMockCharmMetadataDevice(ctrl)
	s.container = NewMockCharmMetadataContainer(ctrl)
	s.containerMount = NewMockCharmMetadataContainerMount(ctrl)
	s.resources = NewMockCharmMetadataResource(ctrl)

	s.charmManifest = NewMockCharmManifest(ctrl)
	s.charmBase = NewMockCharmManifestBase(ctrl)

	s.charmConfigs = NewMockCharmConfigs(ctrl)
	s.charmConfig = NewMockCharmConfig(ctrl)

	s.charmActions = NewMockCharmActions(ctrl)
	s.charmAction = NewMockCharmAction(ctrl)

	return ctrl
}

func (s *importSuite) expectRequiresRelation() {
	exp := s.charmRequires.EXPECT()
	exp.Name().Return("foo")
	exp.Role().Return("requirer")
	exp.Interface().Return("db")
	exp.Optional().Return(false)
	exp.Limit().Return(0)
	exp.Scope().Return("global")
}

func (s *importSuite) expectProvidesRelation() {
	exp := s.charmProvides.EXPECT()
	exp.Name().Return("bar")
	exp.Role().Return("provider")
	exp.Interface().Return("db")
	exp.Optional().Return(true)
	exp.Limit().Return(1)
	exp.Scope().Return("global")
}

func (s *importSuite) expectPeersRelation() {
	exp := s.charmPeers.EXPECT()
	exp.Name().Return("baz")
	exp.Role().Return("peer")
	exp.Interface().Return("db")
	exp.Optional().Return(true)
	exp.Limit().Return(1)
	exp.Scope().Return("container")
}

func (s *importSuite) expectStorage() {
	exp := s.storage.EXPECT()
	exp.Name().Return("baz")
	exp.Type().Return("filesystem")
	exp.Description().Return("baz storage")
	exp.Shared().Return(true)
	exp.Readonly().Return(true)
	exp.MinimumSize().Return(1024)
	exp.Location().Return("baz location")
	exp.CountMin().Return(1)
	exp.CountMax().Return(2)
	exp.Properties().Return([]string{"baz"})
}

func (s *importSuite) expectDevice() {
	exp := s.device.EXPECT()
	exp.Name().Return("baz")
	exp.Type().Return("gpu")
	exp.Description().Return("baz device")
	exp.CountMin().Return(1)
	exp.CountMax().Return(2)
}

func (s *importSuite) expectContainer() {
	exp := s.container.EXPECT()
	exp.Resource().Return("baz")
	exp.Gid().Return(ptr(1000))
	exp.Uid().Return(nil)

	expMount := s.containerMount.EXPECT()
	expMount.Location().Return("baz")
	expMount.Storage().Return("bar")

	exp.Mounts().Return([]description.CharmMetadataContainerMount{s.containerMount}).AnyTimes()
}

func (s *importSuite) expectResource() {
	exp := s.resources.EXPECT()
	exp.Name().Return("baz")
	exp.Description().Return("baz resource")
	exp.Type().Return("file")
	exp.Path().Return("baz")
}

func (s *importSuite) expectEmptyManifestBases() {
	exp := s.charmManifest.EXPECT()
	exp.Bases().Return([]description.CharmManifestBase{})
}

func (s *importSuite) expectManifestBases() {
	baseExp := s.charmBase.EXPECT()
	baseExp.Name().Return("ubuntu")
	baseExp.Channel().Return("4.0/stable")
	baseExp.Architectures().Return([]string{"amd64"})

	exp := s.charmManifest.EXPECT()
	exp.Bases().Return([]description.CharmManifestBase{
		s.charmBase,
	})
}

func (s *importSuite) expectEmptyLXDProfile() {
	exp := s.charmMetadata.EXPECT()
	exp.LXDProfile().Return("")
}

func (s *importSuite) expectLXDProfile() {
	exp := s.charmMetadata.EXPECT()
	exp.LXDProfile().Return(`{"config": {"foo": "bar"}}`)
}

func (s *importSuite) expectEmptyCharmConfigs() {
	exp := s.charmConfigs.EXPECT()
	exp.Configs().Return(map[string]description.CharmConfig{})
}

func (s *importSuite) expectCharmConfigs() {
	configExp := s.charmConfig.EXPECT()
	configExp.Type().Return("string")
	configExp.Default().Return("bar")
	configExp.Description().Return("baz")

	exp := s.charmConfigs.EXPECT()
	exp.Configs().Return(map[string]description.CharmConfig{
		"foo": s.charmConfig,
	})
}

func (s *importSuite) expectEmptyCharmActions() {
	exp := s.charmActions.EXPECT()
	exp.Actions().Return(map[string]description.CharmAction{})
}

func (s *importSuite) expectCharmActions() {
	actionExp := s.charmAction.EXPECT()
	actionExp.Description().Return("baz")
	actionExp.Parallel().Return(true)
	actionExp.ExecutionGroup().Return("group")
	actionExp.Parameters().Return(map[string]interface{}{
		"foo": "bar",
	})

	exp := s.charmActions.EXPECT()
	exp.Actions().Return(map[string]description.CharmAction{
		"foo": s.charmAction,
	})
}

func (s *importSuite) expectCharmActionsNested() {
	actionExp := s.charmAction.EXPECT()
	actionExp.Description().Return("baz")
	actionExp.Parallel().Return(true)
	actionExp.ExecutionGroup().Return("group")
	actionExp.Parameters().Return(map[string]interface{}{
		"foo": map[interface{}]interface{}{
			"bar": "baz",
			"foo": map[interface{}]interface{}{
				1:        2,
				true:     false,
				0.1:      "0.2",
				int64(2): int64(2),
			},
		},
	})

	exp := s.charmActions.EXPECT()
	exp.Actions().Return(map[string]description.CharmAction{
		"foo": s.charmAction,
	})
}

type charmConfig struct {
	ConfigType       string
	DefaultValue     any
	CharmDescription string
}

func (c charmConfig) Type() string {
	return c.ConfigType
}

func (c charmConfig) Default() any {
	return c.DefaultValue
}

func (c charmConfig) Description() string {
	return c.CharmDescription
}
