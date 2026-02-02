// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/description/v11"
	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationmodelmigration "github.com/juju/juju/domain/application/modelmigration"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/deployment/charm/assumes"
	"github.com/juju/juju/domain/deployment/charm/resource"
	machinemodelmigration "github.com/juju/juju/domain/machine/modelmigration"
	migrationtesting "github.com/juju/juju/domain/modelmigration/testing"
	networkmodelmigration "github.com/juju/juju/domain/network/modelmigration"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domaintesting "github.com/juju/juju/domain/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	schematesting.ModelSuite

	coordinator *modelmigration.Coordinator
	scope       modelmigration.Scope
	svc         *service.Service
}

// applicationProviderStorageID is used to represent an application's
// storage id, used for testing its retrieval.
type applicationProviderStorageID struct {
	storage         string
	storageUniqueID string
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := model.UUID(s.ModelUUID())

	s.coordinator = modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	s.scope = modelmigration.NewScope(nil, s.TxnRunnerFactory(), nil, modelUUID)

	modelDB := func(context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	s.svc = service.NewService(
		state.NewState(modelDB, modelUUID, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domaintesting.NoopLeaderEnsurer(),
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	c.Cleanup(func() {
		s.coordinator = nil
		s.svc = nil
		s.scope = modelmigration.Scope{}
	})
}

func (s *importSuite) TestImportMaximalCharmMetadata(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	// Create a charm description and write it into the model, then import
	// it using the model migration framework. Verify that the charm has been
	// imported correctly into the database.

	// This skips both Payloads and LXD profiles, as it's not longer used, so
	// can be skipped.

	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name:           "foo",
		Summary:        "summary foo",
		Description:    "description foo",
		Subordinate:    false,
		MinJujuVersion: "4.0.0",
		RunAs:          "root",
		Assumes:        "[]",
		Categories:     []string{"bar", "baz"},
		Tags:           []string{"alpha", "beta"},
		Terms:          []string{"terms", "and", "conditions"},
		Provides: map[string]description.CharmMetadataRelation{
			"db": migrationtesting.Relation{
				Name_:          "db",
				Role_:          "provider",
				InterfaceName_: "db",
				Optional_:      true,
				Limit_:         1,
				Scope_:         "global",
			},
		},
		Peers: map[string]description.CharmMetadataRelation{
			"restart": migrationtesting.Relation{
				Name_:          "restart",
				Role_:          "peer",
				InterfaceName_: "restarter",
				Optional_:      true,
				Limit_:         2,
				Scope_:         "global",
			},
		},
		Requires: map[string]description.CharmMetadataRelation{
			"cache": migrationtesting.Relation{
				Name_:          "cache",
				Role_:          "requirer",
				InterfaceName_: "cache",
				Optional_:      true,
				Limit_:         3,
				Scope_:         "container",
			},
		},
		Storage: map[string]description.CharmMetadataStorage{
			"ebs": migrationtesting.Storage{
				Name_:        "ebs",
				Description_: "ebs storage",
				Shared_:      false,
				Readonly_:    true,
				CountMin_:    1,
				CountMax_:    1,
				MinimumSize_: 10,
				Location_:    "/ebs",
				Properties_:  []string{"fast", "encrypted"},
				Stype_:       "filesystem",
			},
		},
		ExtraBindings: map[string]string{
			"db-admin": "db-admin",
		},
		Devices: map[string]description.CharmMetadataDevice{
			"gpu": migrationtesting.Device{
				Name_:        "gpu",
				Description_: "A GPU device",
				Dtype_:       "gpu",
				CountMin_:    1,
				CountMax_:    2,
			},
		},
		Containers: map[string]description.CharmMetadataContainer{
			"deadbeef": migrationtesting.Container{
				Resource_: "deadbeef",
				Mounts_: []description.CharmMetadataContainerMount{
					migrationtesting.ContainerMount{
						Storage_:  "tmpfs",
						Location_: "/tmp",
					},
				},
				Uid_: ptr(1000),
				Gid_: ptr(1000),
			},
		},
		Resources: map[string]description.CharmMetadataResource{
			"file1": migrationtesting.ResourceMeta{
				Name_:        "file1",
				Rtype_:       "file",
				Description_: "A resource file",
				Path_:        "resources/deadbeef1",
			},
			"oci2": migrationtesting.ResourceMeta{
				Name_:        "oci2",
				Rtype_:       "oci-image",
				Description_: "A resource oci image",
				Path_:        "resources/deadbeef2",
			},
		},
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	metadata, err := s.svc.GetCharmMetadata(c.Context(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.CharmHubSource,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(metadata, tc.DeepEquals, internalcharm.Meta{
		Name:           "foo",
		Summary:        "summary foo",
		Description:    "description foo",
		Subordinate:    false,
		MinJujuVersion: semversion.MustParse("4.0.0"),
		CharmUser:      internalcharm.RunAsRoot,
		Assumes: &assumes.ExpressionTree{
			Expression: assumes.CompositeExpression{
				ExprType:       assumes.AllOfExpression,
				SubExpressions: []assumes.Expression{},
			},
		},
		Categories: []string{"bar", "baz"},
		Tags:       []string{"alpha", "beta"},
		Terms:      []string{"terms", "and", "conditions"},
		Provides: map[string]internalcharm.Relation{
			"db": {
				Name:      "db",
				Role:      internalcharm.RoleProvider,
				Interface: "db",
				Optional:  true,
				Limit:     1,
				Scope:     "global",
			},
			"juju-info": {
				Name:      "juju-info",
				Role:      internalcharm.RoleProvider,
				Interface: "juju-info",
				Scope:     "global",
			},
		},
		Peers: map[string]internalcharm.Relation{
			"restart": {
				Name:      "restart",
				Role:      internalcharm.RolePeer,
				Interface: "restarter",
				Optional:  true,
				Limit:     2,
				Scope:     "global",
			},
		},
		Requires: map[string]internalcharm.Relation{
			"cache": {
				Name:      "cache",
				Role:      internalcharm.RoleRequirer,
				Interface: "cache",
				Optional:  true,
				Limit:     3,
				Scope:     "container",
			},
		},
		Storage: map[string]internalcharm.Storage{
			"ebs": {
				Name:        "ebs",
				Type:        internalcharm.StorageFilesystem,
				Description: "ebs storage",
				Shared:      false,
				ReadOnly:    true,
				CountMin:    1,
				CountMax:    1,
				MinimumSize: 10,
				Location:    "/ebs",
				Properties:  []string{"fast", "encrypted"},
			},
		},
		ExtraBindings: map[string]internalcharm.ExtraBinding{
			"db-admin": {Name: "db-admin"},
		},
		Devices: map[string]internalcharm.Device{
			"gpu": {
				Name:        "gpu",
				Description: "A GPU device",
				Type:        "gpu",
				CountMin:    1,
				CountMax:    2,
			},
		},
		Containers: map[string]internalcharm.Container{
			"deadbeef": {
				Resource: "deadbeef",
				Mounts: []internalcharm.Mount{
					{
						Storage:  "tmpfs",
						Location: "/tmp",
					},
				},
				Uid: ptr(1000),
				Gid: ptr(1000),
			},
		},
		Resources: map[string]resource.Meta{
			"file1": {
				Name:        "file1",
				Type:        resource.TypeFile,
				Description: "A resource file",
				Path:        "resources/deadbeef1",
			},
			"oci2": {
				Name:        "oci2",
				Type:        resource.TypeContainerImage,
				Description: "A resource oci image",
				Path:        "resources/deadbeef2",
			},
		},
	})
}

func (s *importSuite) TestImportMinimalCharmMetadata(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	metadata, err := s.svc.GetCharmMetadata(c.Context(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.CharmHubSource,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(metadata, tc.DeepEquals, internalcharm.Meta{
		Name: "foo",
		Provides: map[string]internalcharm.Relation{
			"juju-info": {
				Name:      "juju-info",
				Role:      internalcharm.RoleProvider,
				Interface: "juju-info",
				Scope:     "global",
			},
		},
	})
}

func (s *importSuite) TestImportMaximalCharmManifest(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:    "ubuntu",
				Channel_: "stable",
			},
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "4.0/stable/foo",
				Architectures_: []string{"arm64"},
			},
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "latest/stable",
				Architectures_: []string{"amd64", "s390x", "ppc64el"},
			},
		},
	})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	manifest, err := s.svc.GetCharmManifest(c.Context(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.CharmHubSource,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(manifest, tc.DeepEquals, internalcharm.Manifest{
		Bases: []internalcharm.Base{
			{
				Name:          "ubuntu",
				Channel:       internalcharm.Channel{Risk: "stable"},
				Architectures: []string{"amd64"},
			},
			{
				Name:          "ubuntu",
				Channel:       internalcharm.Channel{Track: "4.0", Risk: "stable", Branch: "foo"},
				Architectures: []string{"arm64"},
			},
			{
				Name:          "ubuntu",
				Channel:       internalcharm.Channel{Track: "latest", Risk: "stable"},
				Architectures: []string{"amd64", "s390x", "ppc64el"},
			},
		},
	})
}

func (s *importSuite) TestImportMinimalCharmManifest(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:    "ubuntu",
				Channel_: "stable",
			},
		},
	})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	manifest, err := s.svc.GetCharmManifest(c.Context(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.CharmHubSource,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(manifest, tc.DeepEquals, internalcharm.Manifest{
		Bases: []internalcharm.Base{
			{
				Name:          "ubuntu",
				Channel:       internalcharm.Channel{Risk: "stable"},
				Architectures: []string{"amd64"},
			},
		},
	})
}

func (s *importSuite) TestImportMinimalCharmConfig(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})
	app.SetCharmConfigs(description.CharmConfigsArgs{
		Configs: map[string]description.CharmConfig{
			"foo": migrationtesting.Config{
				ConfigType_:   "string",
				DefaultValue_: "bar",
				Description_:  "foo description",
			},
		},
	})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	config, err := s.svc.GetCharmConfig(c.Context(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.CharmHubSource,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(config, tc.DeepEquals, internalcharm.ConfigSpec{
		Options: map[string]internalcharm.Option{
			"foo": {
				Type:        "string",
				Default:     "bar",
				Description: "foo description",
			},
		},
	})
}

func (s *importSuite) TestImportMaximalCharmConfig(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})
	app.SetCharmConfigs(description.CharmConfigsArgs{
		Configs: map[string]description.CharmConfig{
			"foo": migrationtesting.Config{
				ConfigType_:   "string",
				DefaultValue_: "bar",
				Description_:  "foo description",
			},
			"baz": migrationtesting.Config{
				ConfigType_:   "int",
				DefaultValue_: 42,
				Description_:  "baz description",
			},
			"qux": migrationtesting.Config{
				ConfigType_:   "boolean",
				DefaultValue_: true,
				Description_:  "qux description",
			},
			"norf": migrationtesting.Config{
				ConfigType_:   "secret",
				DefaultValue_: "foo-bar-baz",
				Description_:  "norf description",
			},
		},
	})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	config, err := s.svc.GetCharmConfig(c.Context(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.CharmHubSource,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(config, tc.DeepEquals, internalcharm.ConfigSpec{
		Options: map[string]internalcharm.Option{
			"foo": {
				Type:        "string",
				Default:     "bar",
				Description: "foo description",
			},
			"baz": {
				Type:        "int",
				Default:     42,
				Description: "baz description",
			},
			"qux": {
				Type:        "boolean",
				Default:     true,
				Description: "qux description",
			},
			"norf": {
				Type:        "secret",
				Default:     "foo-bar-baz",
				Description: "norf description",
			},
		},
	})
}

func (s *importSuite) TestImportMinimalCharmActions(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})
	app.SetCharmActions(description.CharmActionsArgs{
		Actions: map[string]description.CharmAction{
			"foo": migrationtesting.Action{
				Description_:    "foo description",
				Parallel_:       true,
				ExecutionGroup_: "bar",
				Params_: map[string]any{
					"a": int(1),
				},
			},
		},
	})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	actions, err := s.svc.GetCharmActions(c.Context(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.CharmHubSource,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(actions, tc.DeepEquals, internalcharm.Actions{
		ActionSpecs: map[string]internalcharm.ActionSpec{
			"foo": {
				Description:    "foo description",
				Parallel:       true,
				ExecutionGroup: "bar",
				Params: map[string]any{
					// All params are marshalled to JSON to try and keep all
					// types consistent when there are complex types. But, the
					// downside is that numbers become float64.
					"a": float64(1),
				},
			},
		},
	})
}

func (s *importSuite) TestImportMaximalCharmActions(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})
	app.SetCharmActions(description.CharmActionsArgs{
		Actions: map[string]description.CharmAction{
			"foo": migrationtesting.Action{
				Description_:    "foo description",
				Parallel_:       true,
				ExecutionGroup_: "bar",
				Params_: map[string]any{
					"a": int(1),
					"b": "string param",
					"c": true,
					"d": 3.14,
					"e": []any{1, 2.0, "x"},
					"f": map[string]any{
						"nested": "value",
					},
				},
			},
			"baz": migrationtesting.Action{
				Description_: "baz description",
			},
		},
	})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	actions, err := s.svc.GetCharmActions(c.Context(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.CharmHubSource,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(actions, tc.DeepEquals, internalcharm.Actions{
		ActionSpecs: map[string]internalcharm.ActionSpec{
			"foo": {
				Description:    "foo description",
				Parallel:       true,
				ExecutionGroup: "bar",
				Params: map[string]any{
					// All params are marshalled to JSON to try and keep all
					// types consistent when there are complex types. But, the
					// downside is that numbers become float64.
					"a": float64(1),
					"b": "string param",
					"c": true,
					"d": 3.14,
					"e": []any{float64(1), 2.0, "x"},
					"f": map[string]any{
						"nested": "value",
					},
				},
			},
			"baz": {
				Description: "baz description",
			},
		},
	})
}

func (s *importSuite) TestIAASApplication(c *tc.C) {
	// Arrange
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	setupMinimalApplication(desc)
	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	obtainedLocator, err := s.svc.GetCharmLocatorByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedLocator, tc.DeepEquals, charm.CharmLocator{
		Name:         "foo",
		Revision:     7,
		Source:       charm.CharmHubSource,
		Architecture: architecture.ARM64,
	})
}

func (s *importSuite) TestCAASApplication(c *tc.C) {
	// Arrange
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.CAAS),
	})
	app := setupMinimalApplication(desc)
	app.SetDesiredScale(42)

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	obtainedLocator, err := s.svc.GetCharmLocatorByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedLocator, tc.DeepEquals, charm.CharmLocator{
		Name:         "foo",
		Revision:     7,
		Source:       charm.CharmHubSource,
		Architecture: architecture.ARM64,
	})

	obtainedScale, err := s.svc.GetApplicationScale(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedScale, tc.DeepEquals, 42)
}

func (s *importSuite) TestImportCAASUnit(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.CAAS),
	})

	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/20.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})

	// Add unit with comprehensive field coverage.
	unit1 := app.AddUnit(description.UnitArgs{
		Name:            "foo/0",
		Type:            string(model.CAAS),
		PasswordHash:    "passwordhash-0",
		WorkloadVersion: "1.2.3",
		CharmState: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		RelationState: map[int]string{
			1: "relation-state-1",
			2: "relation-state-2",
		},
		UniterState:  `{"some": "uniter-state"}`,
		StorageState: `{"some": "storage-state"}`,
	})
	unit1.SetAgentStatus(description.StatusArgs{
		Value:   "idle",
		Message: "agent idle",
		Data: map[string]any{
			"agent-key": "agent-value",
		},
	})
	unit1.SetWorkloadStatus(description.StatusArgs{
		Value:   "active",
		Message: "workload active",
		Data: map[string]any{
			"workload-key": "workload-value",
		},
	})
	unit1.SetTools(description.AgentToolsArgs{
		Version: "4.0.0-ubuntu-amd64",
	})

	// Add second unit.
	unit2 := app.AddUnit(description.UnitArgs{
		Name:            "foo/1",
		Type:            string(model.CAAS),
		PasswordHash:    "passwordhash-1",
		WorkloadVersion: "2.0.0",
	})
	unit2.SetAgentStatus(description.StatusArgs{
		Value: "idle",
	})
	unit2.SetWorkloadStatus(description.StatusArgs{
		Value: "active",
	})
	unit2.SetTools(description.AgentToolsArgs{
		Version: "4.0.0-ubuntu-amd64",
	})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	// Verify unit fields imported by the application domain.
	version0, err := s.svc.GetUnitWorkloadVersion(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(version0, tc.Equals, "1.2.3")

	version1, err := s.svc.GetUnitWorkloadVersion(c.Context(), "foo/1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(version1, tc.Equals, "2.0.0")
}

func (s *importSuite) TestImportIAASUnit(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	// Add machines for the units to be placed on.
	desc.AddMachine(description.MachineArgs{
		Id:   "0",
		Base: "ubuntu@22.04",
	})
	desc.AddMachine(description.MachineArgs{
		Id:   "1",
		Base: "ubuntu@22.04",
	})

	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "bar",
		CharmURL: "ch:bar-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/22.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "bar",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "22.04/stable",
				Architectures_: []string{"amd64"},
			},
		},
	})

	// Add first unit with comprehensive field coverage.
	unit1 := app.AddUnit(description.UnitArgs{
		Name:            "bar/0",
		Type:            string(model.IAAS),
		Machine:         "0",
		PasswordHash:    "passwordhash-0",
		WorkloadVersion: "2.0.0",
		CharmState: map[string]string{
			"state-key": "state-value",
		},
		RelationState: map[int]string{
			5: "relation-state-5",
		},
		UniterState:  `{"uniter": "state"}`,
		StorageState: `{"storage": "state"}`,
	})
	unit1.SetAgentStatus(description.StatusArgs{
		Value:   "idle",
		Message: "agent is idle",
	})
	unit1.SetWorkloadStatus(description.StatusArgs{
		Value:   "active",
		Message: "workload is active",
	})
	unit1.SetTools(description.AgentToolsArgs{
		Version: "4.0.0-ubuntu-amd64",
	})

	// Add second unit on a different machine.
	unit2 := app.AddUnit(description.UnitArgs{
		Name:            "bar/1",
		Type:            string(model.IAAS),
		Machine:         "1",
		PasswordHash:    "passwordhash-1",
		WorkloadVersion: "2.1.0",
	})
	unit2.SetAgentStatus(description.StatusArgs{
		Value: "executing",
	})
	unit2.SetWorkloadStatus(description.StatusArgs{
		Value: "maintenance",
	})
	unit2.SetTools(description.AgentToolsArgs{
		Version: "4.0.0-ubuntu-amd64",
	})

	machinemodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	// Verify unit fields imported by the application domain.
	version0, err := s.svc.GetUnitWorkloadVersion(c.Context(), "bar/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(version0, tc.Equals, "2.0.0")

	version1, err := s.svc.GetUnitWorkloadVersion(c.Context(), "bar/1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(version1, tc.Equals, "2.1.0")
}

func (s *importSuite) TestApplicationConfig(c *tc.C) {
	// Arrange
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	app := setupMinimalApplication(desc)
	app.SetCharmConfigs(description.CharmConfigsArgs{
		Configs: map[string]description.CharmConfig{
			"foo": migrationtesting.Config{
				ConfigType_:   "string",
				DefaultValue_: "bar",
				Description_:  "foo description",
			},
		},
	})
	app.SetCharmConfig(map[string]interface{}{"foo": "test-value"})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtainedDetails, err := s.svc.GetApplicationDetailsByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	obtainedConfig, err := s.svc.GetApplicationConfigWithDefaults(c.Context(), obtainedDetails.UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedConfig, tc.DeepEquals, internalcharm.Config{
		"foo": "test-value",
	})
}

func (s *importSuite) TestApplicationEndpointBindings(c *tc.C) {
	// Arrange
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})
	desc.AddSpace(description.SpaceArgs{
		Id:         "3",
		UUID:       "space-uuid",
		Name:       "test",
		ProviderID: "space-provider-id",
	})
	desc.AddSubnet(description.SubnetArgs{
		ID:                "43",
		UUID:              "subnet-uuid",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		ProviderSpaceId:   "space-provider-id",
		CIDR:              "cidr",
	})
	app := desc.AddApplication(description.ApplicationArgs{
		Name:             "foo",
		CharmURL:         "ch:foo-1",
		EndpointBindings: map[string]string{"db": "0", "db-admin": "3", "": "3"},
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
		Provides: map[string]description.CharmMetadataRelation{
			"db": migrationtesting.Relation{
				Name_:          "db",
				Role_:          "provider",
				InterfaceName_: "db",
				Optional_:      true,
				Scope_:         "global",
			},
		},
		ExtraBindings: map[string]string{
			"db-admin": "db-admin",
		},
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})

	s.setModel(c, "lxd", model.IAAS.String())

	networkmodelmigration.RegisterImportSubnets(s.coordinator, loggertesting.WrapCheckLog(c))
	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtainedBindings, err := s.svc.GetApplicationEndpointBindings(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedBindings, tc.DeepEquals, map[string]network.SpaceUUID{
		"":          "space-uuid",
		"db":        network.AlphaSpaceId,
		"db-admin":  "space-uuid",
		"juju-info": "space-uuid",
	})
}

func (s *importSuite) TestApplicationExposedEndpoints(c *tc.C) {
	// Arrange
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})
	desc.AddSpace(description.SpaceArgs{
		Id:         "3",
		UUID:       "space-uuid",
		Name:       "test",
		ProviderID: "space-provider-id",
	})
	desc.AddSubnet(description.SubnetArgs{
		ID:                "43",
		UUID:              "subnet-uuid",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		ProviderSpaceId:   "space-provider-id",
		CIDR:              "198.51.100.0/24",
	})
	app := desc.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-7",
		Exposed:  true,
		ExposedEndpoints: map[string]description.ExposedEndpointArgs{
			"db": {
				ExposeToSpaceIDs: []string{"3"},
				ExposeToCIDRs:    []string{"198.51.100.42/24"},
			},
		},
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 7,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
		Provides: map[string]description.CharmMetadataRelation{
			"db": migrationtesting.Relation{
				Name_:          "db",
				Role_:          "provider",
				InterfaceName_: "db",
				Optional_:      true,
				Scope_:         "global",
			},
		},
		ExtraBindings: map[string]string{
			"db-admin": "db-admin",
		},
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})

	s.setModel(c, "ec2", model.IAAS.String())

	networkmodelmigration.RegisterImportSubnets(s.coordinator, loggertesting.WrapCheckLog(c))
	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained, err := s.svc.GetExposedEndpoints(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, map[string]application.ExposedEndpoint{
		"db": {ExposeToSpaceIDs: set.Strings{"space-uuid": true},
			ExposeToCIDRs: set.Strings{"198.51.100.42/24": true}}})
}

func (s *importSuite) TestApplicationConstraints(c *tc.C) {
	// Arrange
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	app := setupMinimalApplication(desc)
	app.SetConstraints(description.ConstraintsArgs{
		AllocatePublicIP: true,
		Architecture:     "arm64",
		Memory:           uint64(1024),
		Zones:            []string{"z1", "z2", "z3"},
	})

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtainedDetails, err := s.svc.GetApplicationDetailsByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	obtainedConstraints, err := s.svc.GetApplicationConstraints(c.Context(), obtainedDetails.UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedConstraints, tc.DeepEquals, constraints.Value{
		AllocatePublicIP: ptr(true),
		Arch:             ptr("arm64"),
		Mem:              ptr(uint64(1024)),
		Zones:            ptr([]string{"z1", "z2", "z3"}),
	})
}

// TestImportApplicationStorageUniqueID tests that the sotrage unique id is
// inserted correctly in the DB.
func (s *importSuite) TestImportApplicationStorageUniqueID(c *tc.C) {
	// Arrange
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.CAAS),
	})
	storageDirectives := make(map[string]description.StorageDirectiveArgs)
	storageDirectives["cert"] = description.StorageDirectiveArgs{
		Pool:  "custompool",
		Size:  100,
		Count: 1,
	}
	storageDirectives["cache"] = description.StorageDirectiveArgs{
		Pool:  "custompool",
		Size:  150,
		Count: 1,
	}
	app := setupMinimalApplicationWithStorageDirectives(desc, storageDirectives)
	app.SetDesiredScale(3)
	app.SetStorageUniqueID("uniqid")

	applicationmodelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	id := s.getApplicationStorageUniqueID(c, app.Name())
	c.Assert(id, tc.Equals, "uniqid")
}

func setupMinimalApplication(model description.Model) description.Application {
	return setupMinimalApplicationWithStorageDirectives(model, nil)
}

func setupMinimalApplicationWithStorageDirectives(model description.Model,
	storageDirectives map[string]description.StorageDirectiveArgs) description.Application {
	app := model.AddApplication(description.ApplicationArgs{
		Name:              "foo",
		CharmURL:          "ch:foo-7",
		StorageDirectives: storageDirectives,
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 7,
		Channel:  "latest/stable",
		Platform: "arm64/ubuntu/24.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "foo",
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64", "arm64"},
			},
		},
	})
	return app
}

func (s *importSuite) setModel(c *tc.C, cloudType, modelType string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod",  ?, "test-model", ?)
		`, s.ModelUUID(), "controller-uuid", modelType, cloudType)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// getApplicationStorageUniqueIDs is a test helper that retrieves an app's storage unique id.
func (s *importSuite) getApplicationStorageUniqueID(c *tc.C, appName string) string {
	var val string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT storage_unique_id 
			FROM   application_storage_suffix
			WHERE  application_uuid = (
			 SELECT uuid FROM application
			 WHERE  name = ?
			)`, appName)
		if row.Err() != nil {
			return row.Err()
		}
		return row.Scan(&val)
	})
	c.Assert(err, tc.ErrorIsNil)
	return val
}

func ptr[T any](i T) *T {
	return &i
}
