// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/description/v10"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/config"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/service"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	testhelpers.IsolationSuite

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

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestRollback(c *tc.C) {
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

	err := importOp.Rollback(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestRollbackForMultipleApplicationsRollbacksAll(c *tc.C) {
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

	err := importOp.Rollback(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, "rollback failed: boom")
}

func (s *importSuite) TestApplicationImportWithMinimalCharmForCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.CAAS.String(),
	})

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
	s.importService.EXPECT().ImportCAASApplication(
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

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(importArgs.Charm.Meta().Name, tc.Equals, "prometheus")
	c.Check(importArgs.Units, tc.DeepEquals, []service.ImportUnitArg{{
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

func (s *importSuite) TestApplicationImportWithMinimalCharmForIAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name:         "prometheus/0",
		PasswordHash: "passwordhash",
		Machine:      "0",
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
	s.importService.EXPECT().ImportIAASApplication(
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

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(importArgs.Charm.Meta().Name, tc.Equals, "prometheus")
	c.Check(importArgs.Units, tc.DeepEquals, []service.ImportUnitArg{{
		UnitName:     "prometheus/0",
		PasswordHash: ptr("passwordhash"),
		Machine:      machine.Name("0"),
	}})
}

func (s *importSuite) TestApplicationImportWithApplicationConfigAndSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

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
	s.importService.EXPECT().ImportIAASApplication(
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

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(importArgs.Charm.Meta().Name, tc.Equals, "prometheus")
	c.Check(importArgs.ApplicationConfig, tc.DeepEquals, config.ConfigAttributes{
		"foo": "bar",
	})
	c.Check(importArgs.ApplicationSettings, tc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})
}

func (s *importSuite) TestApplicationImportWithConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

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

	s.importService.EXPECT().ImportIAASApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, args service.ImportApplicationArgs) error {
		c.Assert(args.Charm.Meta().Name, tc.Equals, "prometheus")
		c.Check(args.ApplicationConstraints.AllocatePublicIP, tc.DeepEquals, ptr(true))
		c.Check(args.ApplicationConstraints.Arch, tc.DeepEquals, ptr("amd64"))
		c.Check(args.ApplicationConstraints.Container, tc.DeepEquals, ptr(instance.ContainerType("lxd")))
		c.Check(args.ApplicationConstraints.CpuCores, tc.DeepEquals, ptr(uint64(2)))
		c.Check(args.ApplicationConstraints.CpuPower, tc.DeepEquals, ptr(uint64(1000)))
		c.Check(args.ApplicationConstraints.ImageID, tc.DeepEquals, ptr("foo"))
		c.Check(args.ApplicationConstraints.InstanceType, tc.DeepEquals, ptr("baz"))
		c.Check(args.ApplicationConstraints.VirtType, tc.DeepEquals, ptr("vm"))
		c.Check(args.ApplicationConstraints.Mem, tc.DeepEquals, ptr(uint64(1024)))
		c.Check(args.ApplicationConstraints.RootDisk, tc.DeepEquals, ptr(uint64(1024)))
		c.Check(args.ApplicationConstraints.RootDiskSource, tc.DeepEquals, ptr("qux"))
		c.Check(args.ApplicationConstraints.Spaces, tc.DeepEquals, ptr([]string{"space0", "space1"}))
		c.Check(args.ApplicationConstraints.Tags, tc.DeepEquals, ptr([]string{"tag0", "tag1"}))
		c.Check(args.ApplicationConstraints.Zones, tc.DeepEquals, ptr([]string{"zone0", "zone1"}))
		return nil
	})

	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportCharmMetadataEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(nil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("foo").Times(2)

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidAssumes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("!![]")

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidMinJujuVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("[]")
	metaExp.MinJujuVersion().Return("foo")

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidRelationRole(c *tc.C) {
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

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidRelationScope(c *tc.C) {
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

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidStorage(c *tc.C) {
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

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidResource(c *tc.C) {
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

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportCharmMetadata(c *tc.C) {
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

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(meta, tc.DeepEquals, &internalcharm.Meta{
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

func (s *importSuite) TestImportEmptyCharmManifest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectEmptyManifestBases()

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmManifest(s.charmManifest)
	c.Assert(err, tc.NotNil)
}

func (s *importSuite) TestImportCharmManifest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectManifestBases()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmManifest(s.charmManifest)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(meta, tc.DeepEquals, &internalcharm.Manifest{
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

func (s *importSuite) TestImportCharmManifestWithInvalidBase(c *tc.C) {
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

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmManifest(s.charmManifest)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportEmptyCharmLXDProfile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectEmptyLXDProfile()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmLXDProfile(s.charmMetadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(meta, tc.IsNil)
}

func (s *importSuite) TestImportCharmLXDProfile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectLXDProfile()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmLXDProfile(s.charmMetadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(meta, tc.DeepEquals, &internalcharm.LXDProfile{
		Config: map[string]string{
			"foo": "bar",
		},
	})
}

func (s *importSuite) TestImportEmptyCharmConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectEmptyCharmConfigs()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmConfig(s.charmConfigs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(meta, tc.NotNil)
	c.Check(meta.Options, tc.HasLen, 0)
}

func (s *importSuite) TestImportCharmConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCharmConfigs()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmConfig(s.charmConfigs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(meta, tc.NotNil)
	c.Check(meta.Options, tc.DeepEquals, map[string]internalcharm.Option{
		"foo": {
			Type:        "string",
			Default:     "bar",
			Description: "baz",
		},
	})
}

func (s *importSuite) TestImportEmptyCharmActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectEmptyCharmActions()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmActions(s.charmActions)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(meta, tc.NotNil)
	c.Check(meta.ActionSpecs, tc.HasLen, 0)
}

func (s *importSuite) TestImportCharmActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCharmActions()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmActions(s.charmActions)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(meta, tc.NotNil)
	c.Check(meta.ActionSpecs, tc.DeepEquals, map[string]internalcharm.ActionSpec{
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

func (s *importSuite) TestImportCharmActionsNestedMaps(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCharmActionsNested()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmActions(s.charmActions)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(meta, tc.NotNil)
	c.Check(meta.ActionSpecs, tc.DeepEquals, map[string]internalcharm.ActionSpec{
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

// TestImportEndpointBindings36 checks that the endpoint bindings are correctly
// imported with integer spaces Ids, as found in 3.6.
func (s *importSuite) TestImportEndpointBindings36(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	// Arrange: Declare application args with endpoint bindings set.
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		Exposed:  true,
		EndpointBindings: map[string]string{
			"endpoint0": "0",
			"endpoint1": "1",
			"endpoint2": "2",
			// An empty endpoint name represents the applications default space.
			"": "2",
		},
	}
	app := model.AddApplication(appArgs)

	// Arrange: Set required fields.
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

	// Arrange: Add space id to name information. This is in the 3.6 format,
	// where the Id is an integer.
	model.AddSpace(description.SpaceArgs{
		Id:   "1",
		Name: "beta",
	})
	model.AddSpace(description.SpaceArgs{
		Id:   "2",
		Name: "gamma",
	})

	var importArgs service.ImportApplicationArgs
	// Arrange: Expect the import of the application.
	s.importService.EXPECT().ImportIAASApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, args service.ImportApplicationArgs) error {
		importArgs = args
		return nil
	})

	// Act: Import the application.
	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}
	err := importOp.Execute(c.Context(), model)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)

	// Assert: Check that the endpoints are mapped to the correct space names
	c.Assert(importArgs.Charm.Meta().Name, tc.Equals, "prometheus")
	c.Assert(importArgs.EndpointBindings, tc.HasLen, 3)
	c.Check(importArgs.EndpointBindings["endpoint0"], tc.DeepEquals, network.AlphaSpaceName)
	c.Check(importArgs.EndpointBindings["endpoint1"], tc.DeepEquals, network.SpaceName("beta"))
	c.Check(importArgs.EndpointBindings[""], tc.DeepEquals, network.SpaceName("gamma"))
}

// TestImportEndpointBindings40 checks that the endpoint bindings are correctly
// imported with UUIDs as spaces Ids, as found in 4.0.
func (s *importSuite) TestImportEndpointBindings40(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	space1UUID := uuid.MustNewUUID().String()
	space2UUID := uuid.MustNewUUID().String()
	// Arrange: Declare application args with endpoint bindings set.
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		Exposed:  true,
		EndpointBindings: map[string]string{
			"endpoint0": network.AlphaSpaceId.String(),
			"endpoint1": space1UUID,
			"endpoint2": "",
			"endpoint3": space2UUID,
			// An empty endpoint name represents the applications default space.
			"": space2UUID,
		},
	}
	app := model.AddApplication(appArgs)

	// Arrange: Set required fields.
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

	// Arrange: Add space id to name information. This is in the 4.0 format,
	// where the Id is a UUID.
	model.AddSpace(description.SpaceArgs{
		Id:   space1UUID,
		Name: "beta",
	})
	model.AddSpace(description.SpaceArgs{
		Id:   space2UUID,
		Name: "gamma",
	})

	// Arrange: Expect the import of the application.
	var importArgs service.ImportApplicationArgs
	s.importService.EXPECT().ImportIAASApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, args service.ImportApplicationArgs) error {
		importArgs = args
		return nil
	})

	// Act: Import the application.
	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}
	err := importOp.Execute(c.Context(), model)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)

	// Assert: Check that the endpoints are mapped to the correct space names
	c.Assert(importArgs.Charm.Meta().Name, tc.Equals, "prometheus")
	c.Assert(importArgs.EndpointBindings, tc.HasLen, 3)
	c.Check(importArgs.EndpointBindings["endpoint0"], tc.DeepEquals, network.AlphaSpaceName)
	c.Check(importArgs.EndpointBindings["endpoint1"], tc.DeepEquals, network.SpaceName("beta"))
	c.Check(importArgs.EndpointBindings[""], tc.DeepEquals, network.SpaceName("gamma"))
}

// TestImportEndpointBindingsDefaultSpace checks that the endpoint bindings are
// imported correctly when the application default is the alpha space.
func (s *importSuite) TestImportEndpointBindingsDefaultSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	space1UUID := uuid.MustNewUUID().String()
	space2UUID := uuid.MustNewUUID().String()
	// Arrange: Declare application args with endpoint bindings set.
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		Exposed:  true,
		EndpointBindings: map[string]string{
			"endpoint1": space1UUID,
			"endpoint2": "",
			// An empty endpoint name represents the applications default space.
			"": "",
		},
	}
	app := model.AddApplication(appArgs)

	// Arrange: Set required fields.
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

	// Arrange: Add space id to name information. This is in the 4.0 format,
	// where the Id is a UUID.
	model.AddSpace(description.SpaceArgs{
		Id:   space1UUID,
		Name: "beta",
	})
	model.AddSpace(description.SpaceArgs{
		Id:   space2UUID,
		Name: "gamma",
	})

	// Arrange: Expect the import of the application.
	var importArgs service.ImportApplicationArgs
	s.importService.EXPECT().ImportIAASApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, args service.ImportApplicationArgs) error {
		importArgs = args
		return nil
	})

	// Act: Import the application.
	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}
	err := importOp.Execute(c.Context(), model)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)

	// Assert: Check that the endpoints are mapped to the correct space names
	c.Assert(importArgs.Charm.Meta().Name, tc.Equals, "prometheus")
	c.Assert(importArgs.EndpointBindings, tc.HasLen, 1)
	c.Check(importArgs.EndpointBindings["endpoint1"], tc.DeepEquals, network.SpaceName("beta"))
}

func (s *importSuite) TestImportExposedEndpointsFrom36(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		Exposed:  true,
		ExposedEndpoints: map[string]description.ExposedEndpointArgs{
			"": {
				// The legacy alpha space ID ("0") should be mapped to the new
				// alpha space UUID.
				ExposeToSpaceIDs: []string{"0"},
			},
			"endpoint0": {
				ExposeToCIDRs:    []string{"10.0.0.0/24", "10.0.1.0/24"},
				ExposeToSpaceIDs: []string{"1"},
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

	// We add a pre-4.0 space, which has a Id and not an UUID in description.
	model.AddSpace(description.SpaceArgs{
		Id:   "1",
		Name: "beta",
	})

	spUUID := networktesting.GenSpaceUUID(c)
	s.importService.EXPECT().GetSpaceUUIDByName(gomock.Any(), "beta").Return(spUUID, nil)

	s.importService.EXPECT().ImportIAASApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, args service.ImportApplicationArgs) error {
		c.Assert(args.Charm.Meta().Name, tc.Equals, "prometheus")
		c.Check(args.ExposedEndpoints, tc.HasLen, 2)
		c.Check(args.ExposedEndpoints[""].ExposeToSpaceIDs, tc.DeepEquals, set.NewStrings(network.AlphaSpaceId.String()))
		c.Check(args.ExposedEndpoints["endpoint0"].ExposeToCIDRs, tc.DeepEquals, set.NewStrings("10.0.0.0/24", "10.0.1.0/24"))
		c.Check(args.ExposedEndpoints["endpoint0"].ExposeToSpaceIDs, tc.DeepEquals, set.NewStrings(spUUID.String()))
		return nil
	})

	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportExposedEndpointsFrom40(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	spaceUUID := networktesting.GenSpaceUUID(c)
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		Exposed:  true,
		ExposedEndpoints: map[string]description.ExposedEndpointArgs{
			"": {
				ExposeToSpaceIDs: []string{network.AlphaSpaceId.String()},
			},
			"endpoint0": {
				ExposeToCIDRs:    []string{"10.0.0.0/24", "10.0.1.0/24"},
				ExposeToSpaceIDs: []string{spaceUUID.String()},
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

	// We add a pre-4.0 space, which has a Id and not an UUID in description.
	model.AddSpace(description.SpaceArgs{
		UUID: spaceUUID.String(),
		Name: "beta",
	})

	s.importService.EXPECT().GetSpaceUUIDByName(gomock.Any(), "beta").Return(spaceUUID, nil)

	s.importService.EXPECT().ImportIAASApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, args service.ImportApplicationArgs) error {
		c.Assert(args.Charm.Meta().Name, tc.Equals, "prometheus")
		c.Check(args.ExposedEndpoints, tc.HasLen, 2)
		c.Check(args.ExposedEndpoints[""].ExposeToSpaceIDs, tc.DeepEquals, set.NewStrings(network.AlphaSpaceId.String()))
		c.Check(args.ExposedEndpoints["endpoint0"].ExposeToCIDRs, tc.DeepEquals, set.NewStrings("10.0.0.0/24", "10.0.1.0/24"))
		c.Check(args.ExposedEndpoints["endpoint0"].ExposeToSpaceIDs, tc.DeepEquals, set.NewStrings(spaceUUID.String()))
		return nil
	})

	importOp := importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestSpaceNameNotFoundFrom36(c *tc.C) {
	defer s.setupMocks(c).Finish()

	importOp := importOperation{
		service: s.importService,
	}

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		Exposed:  true,
		ExposedEndpoints: map[string]description.ExposedEndpointArgs{
			"": {
				ExposeToSpaceIDs: []string{"1"},
			},
		},
	}
	app := model.AddApplication(appArgs)
	// Space "1" is not in the model.
	model.AddSpace(description.SpaceArgs{
		Id:   "2",
		Name: "beta",
	})

	_, err := importOp.importExposedEndpoints(c.Context(), app, model.Spaces())
	c.Assert(err, tc.ErrorMatches, "endpoint exposed to space \"1\" does not exist")
}

func (s *importSuite) TestSpaceNameNotFoundFrom40(c *tc.C) {
	defer s.setupMocks(c).Finish()

	importOp := importOperation{
		service: s.importService,
	}

	model := description.NewModel(description.ModelArgs{})

	spaceUUID := uuid.MustNewUUID()
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		Exposed:  true,
		ExposedEndpoints: map[string]description.ExposedEndpointArgs{
			"": {
				ExposeToSpaceIDs: []string{spaceUUID.String()},
			},
		},
	}
	app := model.AddApplication(appArgs)
	// Space with UUID {spaceUUID} is not in the model.
	model.AddSpace(description.SpaceArgs{
		Id:   "other-space-uuid",
		Name: "beta",
	})

	_, err := importOp.importExposedEndpoints(c.Context(), app, model.Spaces())
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("endpoint exposed to space %q does not exist", spaceUUID.String()))
}

func (s *importSuite) TestSpaceNameNotFoundInDB(c *tc.C) {
	defer s.setupMocks(c).Finish()

	importOp := importOperation{
		service: s.importService,
	}

	model := description.NewModel(description.ModelArgs{})

	spaceUUID := uuid.MustNewUUID()
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		Exposed:  true,
		ExposedEndpoints: map[string]description.ExposedEndpointArgs{
			"": {
				ExposeToSpaceIDs: []string{spaceUUID.String()},
			},
		},
	}
	app := model.AddApplication(appArgs)
	// Space with UUID {spaceUUID} is not in the model.
	model.AddSpace(description.SpaceArgs{
		Id:   spaceUUID.String(),
		Name: "beta",
	})

	s.importService.EXPECT().GetSpaceUUIDByName(gomock.Any(), "beta").Return("", errors.Errorf("boom"))

	_, err := importOp.importExposedEndpoints(c.Context(), app, model.Spaces())
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("getting space UUID by name %q: boom", spaceUUID.String()))
}

func (s *importSuite) TestMultipleSpaceLookupExposedEndpoints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	importOp := importOperation{
		service: s.importService,
	}

	model := description.NewModel(description.ModelArgs{})

	spaceUUID0 := networktesting.GenSpaceUUID(c)
	spaceUUID1 := networktesting.GenSpaceUUID(c)
	spaceUUID2 := networktesting.GenSpaceUUID(c)
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
		Exposed:  true,
		ExposedEndpoints: map[string]description.ExposedEndpointArgs{
			"": {
				ExposeToSpaceIDs: []string{spaceUUID0.String(), spaceUUID1.String(), spaceUUID2.String()},
			},
		},
	}
	app := model.AddApplication(appArgs)
	// All spaces are in the model.
	model.AddSpace(description.SpaceArgs{
		Id:   spaceUUID0.String(),
		Name: "beta",
	})
	model.AddSpace(description.SpaceArgs{
		Id:   spaceUUID1.String(),
		Name: "gamma",
	})
	model.AddSpace(description.SpaceArgs{
		Id:   spaceUUID2.String(),
		Name: "delta",
	})

	s.importService.EXPECT().GetSpaceUUIDByName(gomock.Any(), "beta").Return(spaceUUID0, nil)
	s.importService.EXPECT().GetSpaceUUIDByName(gomock.Any(), "gamma").Return(spaceUUID1, nil)
	s.importService.EXPECT().GetSpaceUUIDByName(gomock.Any(), "delta").Return(spaceUUID2, nil)

	_, err := importOp.importExposedEndpoints(c.Context(), app, model.Spaces())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestApplicationImportSubordinate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name:         "prometheus/0",
		PasswordHash: "passwordhash",
		Machine:      "0",
		Principal:    "principal/0",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name:        "prometheus",
		Subordinate: true,
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
	s.importService.EXPECT().ImportIAASApplication(
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

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(importArgs.Charm.Meta().Name, tc.Equals, "prometheus")
	c.Check(importArgs.Units, tc.DeepEquals, []service.ImportUnitArg{{
		UnitName:     "prometheus/0",
		PasswordHash: ptr("passwordhash"),
		Machine:      machine.Name("0"),
		Principal:    "principal/0",
	}})
}

func (s *importSuite) TestImportPeerRelations(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})

	rel1 := model.AddRelation(description.RelationArgs{
		Id: 1,
	})
	rel1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "prometheus",
		Name:            "testtwo",
		Role:            "peer",
	})
	rel2 := model.AddRelation(description.RelationArgs{
		Id: 7,
	})
	rel2.AddEndpoint(description.EndpointArgs{
		ApplicationName: "prometheus",
		Name:            "testone",
		Role:            "peer",
	})
	// rel3 is a peer relation for a different application
	// should not be found.
	rel3 := model.AddRelation(description.RelationArgs{
		Id: 27,
	})
	rel3.AddEndpoint(description.EndpointArgs{
		ApplicationName: "failme",
		Name:            "testone",
		Role:            "peer",
	})
	rel4 := model.AddRelation(description.RelationArgs{
		Id: 29,
	})
	// rel4 is a non peer relation with the application
	// under test, should not be found.
	rel4.AddEndpoint(description.EndpointArgs{
		ApplicationName: "prometheus",
		Name:            "testone",
		Role:            "provider",
	})
	rel4.AddEndpoint(description.EndpointArgs{
		ApplicationName: "failme",
		Name:            "testone",
		Role:            "requirer",
	})
	expected := map[string]int{"testone": 7, "testtwo": 1}

	op := &importOperation{}
	obtained := op.importPeerRelations("prometheus", model.Relations())
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
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
