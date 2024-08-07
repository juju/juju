// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
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
	payload        *MockCharmMetadataPayload
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

func (s *importSuite) TestApplicationImportWithMinimalCharm(c *gc.C) {
	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Tag:      names.NewApplicationTag("prometheus"),
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Tag:          names.NewUnitTag("prometheus/0"),
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
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "1234",
		Hash:     "deadbeef",
		Revision: 1,
		Channel:  "666/stable",
		Platform: "arm64/ubuntu/24.04",
	})

	defer s.setupMocks(c).Finish()

	rev := 1
	s.importService.EXPECT().CreateApplication(
		gomock.Any(),
		"prometheus",
		&stubCharm{
			name:     "prometheus",
			revision: 1,
		},
		corecharm.Origin{
			Source:   "charm-hub",
			Type:     "charm",
			ID:       "1234",
			Hash:     "deadbeef",
			Revision: &rev,
			Channel: &charm.Channel{
				Track: "666",
				Risk:  "stable",
			},
			Platform: corecharm.Platform{
				Architecture: "arm64",
				OS:           "ubuntu",
				Channel:      "24.04",
			},
		},
		service.AddApplicationArgs{},
		[]service.AddUnitArg{{
			UnitName:     ptr("prometheus/0"),
			PasswordHash: ptr("passwordhash"),
			CloudContainer: ptr(service.CloudContainerParams{
				ProviderId: ptr("provider-id"),
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
		}},
	).Return("", nil)

	importOp := importOperation{
		service:      s.importService,
		logger:       loggertesting.WrapCheckLog(c),
		charmOrigins: make(map[string]*corecharm.Origin),
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportCharmMetadataEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(nil)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("foo").Times(2)

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidAssumes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("!![]")

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidMinJujuVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	metaExp := s.charmMetadata.EXPECT()
	metaExp.RunAs().Return("root")
	metaExp.Assumes().Return("[]")
	metaExp.MinJujuVersion().Return("foo")

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
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

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
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

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
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

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmMetadata(s.charmMetadata)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *importSuite) TestImportCharmMetadataInvalidResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectRequiresRelation()
	s.expectProvidesRelation()
	s.expectPeersRelation()
	s.expectStorage()
	s.expectDevice()
	s.expectPayload()
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
	metaExp.Payloads().Return(map[string]description.CharmMetadataPayload{
		"payload": s.payload,
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
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *importSuite) TestImportCharmMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectRequiresRelation()
	s.expectProvidesRelation()
	s.expectPeersRelation()
	s.expectStorage()
	s.expectDevice()
	s.expectPayload()
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
	metaExp.Payloads().Return(map[string]description.CharmMetadataPayload{
		"payload": s.payload,
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
		MinJujuVersion: version.MustParse("4.0.0"),
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
			"foo": internalcharm.ExtraBinding{
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
		PayloadClasses: map[string]internalcharm.PayloadClass{
			"payload": {
				Name: "baz",
				Type: "tar",
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

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmManifest(s.charmManifest)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(meta, gc.DeepEquals, &internalcharm.Manifest{})
}

func (s *importSuite) TestImportCharmManifest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectManifestBases()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmManifest(s.charmManifest)
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

	importOp := importOperation{
		service: s.importService,
	}

	_, err := importOp.importCharmManifest(s.charmManifest)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *importSuite) TestImportEmptyCharmLXDProfile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectEmptyLXDProfile()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmLXDProfile(s.charmMetadata)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(meta, gc.IsNil)
}

func (s *importSuite) TestImportCharmLXDProfile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectLXDProfile()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmLXDProfile(s.charmMetadata)
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

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmConfig(s.charmConfigs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta, gc.NotNil)
	c.Check(meta.Options, gc.HasLen, 0)
}

func (s *importSuite) TestImportCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCharmConfigs()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmConfig(s.charmConfigs)
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

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmActions(s.charmActions)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta, gc.NotNil)
	c.Check(meta.ActionSpecs, gc.HasLen, 0)
}

func (s *importSuite) TestImportCharmActions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCharmActions()

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmActions(s.charmActions)
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

	importOp := importOperation{
		service: s.importService,
	}

	meta, err := importOp.importCharmActions(s.charmActions)
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

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)

	s.charmMetadata = NewMockCharmMetadata(ctrl)
	s.charmProvides = NewMockCharmMetadataRelation(ctrl)
	s.charmRequires = NewMockCharmMetadataRelation(ctrl)
	s.charmPeers = NewMockCharmMetadataRelation(ctrl)
	s.storage = NewMockCharmMetadataStorage(ctrl)
	s.device = NewMockCharmMetadataDevice(ctrl)
	s.payload = NewMockCharmMetadataPayload(ctrl)
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

func (s *importSuite) expectPayload() {
	exp := s.payload.EXPECT()
	exp.Name().Return("baz")
	exp.Type().Return("tar")
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
