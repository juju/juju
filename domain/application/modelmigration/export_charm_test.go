// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/description/v9"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/semversion"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
)

type exportCharmSuite struct {
	exportSuite
}

var _ = tc.Suite(&exportCharmSuite{})

func (s *exportCharmSuite) TestApplicationExportMinimalCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectApplication(c)
	s.expectMinimalCharm()
	s.expectApplicationConfig()
	s.expectApplicationConstraints(constraints.Value{})
	s.expectApplicationUnits()
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(false, nil)

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{
		Type: "iaas",
	})

	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(model.Applications(), tc.HasLen, 1)

	app := model.Applications()[0]
	c.Check(app.Name(), tc.Equals, "prometheus")
	c.Check(app.CharmURL(), tc.Equals, "ch:amd64/prometheus-42")

	metadata := app.CharmMetadata()
	c.Assert(metadata, tc.NotNil)
	c.Check(metadata.Name(), tc.Equals, "prometheus")
}

func (s *exportCharmSuite) TestExportCharmMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test that all the properties are correctly exported to the description
	// package. This is a bit of a beast just because of the number of fields
	// that need to be checked.

	meta := &internalcharm.Meta{
		Name:        "prometheus",
		Summary:     "Prometheus monitoring",
		Description: "Prometheus is a monitoring system and time series database.",
		Subordinate: true,
		Categories:  []string{"monitoring"},
		Tags:        []string{"monitoring", "time-series"},
		Terms:       []string{"monitoring", "time-series", "database"},
		CharmUser:   "root",
		Assumes: &assumes.ExpressionTree{
			Expression: assumes.CompositeExpression{
				ExprType:       assumes.AllOfExpression,
				SubExpressions: []assumes.Expression{},
			},
		},
		MinJujuVersion: semversion.MustParse("4.0.0"),
		Provides: map[string]internalcharm.Relation{
			"prometheus": {
				Name:      "prometheus",
				Role:      internalcharm.RoleProvider,
				Interface: "monitoring",
				Optional:  true,
				Limit:     1,
				Scope:     internalcharm.ScopeGlobal,
			},
		},
		Requires: map[string]internalcharm.Relation{
			"foo": {
				Name:      "bar",
				Role:      internalcharm.RoleRequirer,
				Interface: "baz",
				Optional:  true,
				Limit:     2,
				Scope:     internalcharm.ScopeContainer,
			},
		},
		Peers: map[string]internalcharm.Relation{
			"alpha": {
				Name:      "omega",
				Role:      internalcharm.RolePeer,
				Interface: "monitoring",
				Optional:  true,
				Limit:     3,
				Scope:     internalcharm.ScopeGlobal,
			},
		},
		ExtraBindings: map[string]internalcharm.ExtraBinding{
			"foo": {
				Name: "bar",
			},
		},
		Storage: map[string]internalcharm.Storage{
			"foo": {
				Name:        "bar",
				Description: "baz",
				Type:        internalcharm.StorageBlock,
				Shared:      true,
				ReadOnly:    true,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 1024,
				Location:    "/var/lib/foo",
				Properties:  []string{"foo", "bar"},
			},
		},
		Devices: map[string]internalcharm.Device{
			"foo": {
				Name:        "bar",
				Description: "baz",
				Type:        internalcharm.DeviceType("gpu"),
				CountMin:    1,
				CountMax:    2,
			},
		},
		Containers: map[string]internalcharm.Container{
			"foo": {
				Resource: "resource",
				Mounts: []internalcharm.Mount{
					{
						Location: "/var/lib/foo",
						Storage:  "bar",
					},
				},
			},
		},
		Resources: map[string]resource.Meta{
			"foo": {
				Name:        "bar",
				Description: "baz",
				Type:        resource.TypeFile,
				Path:        "/var/lib/foo",
			},
		},
	}

	exportOp := s.newExportOperation()

	args, err := exportOp.exportCharmMetadata(meta, "{}")
	c.Assert(err, tc.ErrorIsNil)

	// As the description package exposes interfaces, it becomes difficult to
	// test it nicely. To work around this, we'll check the individual fields
	// of the CharmMetadataArgs struct. Once they've been checked, we nil
	// out the fields so that we can compare the rest of the struct.

	provides := args.Provides
	c.Assert(provides, tc.HasLen, 1)
	provider := provides["prometheus"]
	c.Check(provider.Name(), tc.Equals, "prometheus")
	c.Check(provider.Role(), tc.Equals, "provider")
	c.Check(provider.Interface(), tc.Equals, "monitoring")
	c.Check(provider.Optional(), tc.Equals, true)
	c.Check(provider.Limit(), tc.Equals, 1)
	c.Check(provider.Scope(), tc.Equals, "global")
	args.Provides = nil

	requires := args.Requires
	c.Assert(requires, tc.HasLen, 1)
	require := requires["foo"]
	c.Check(require.Name(), tc.Equals, "bar")
	c.Check(require.Role(), tc.Equals, "requirer")
	c.Check(require.Interface(), tc.Equals, "baz")
	c.Check(require.Optional(), tc.Equals, true)
	c.Check(require.Limit(), tc.Equals, 2)
	c.Check(require.Scope(), tc.Equals, "container")
	args.Requires = nil

	peers := args.Peers
	c.Assert(peers, tc.HasLen, 1)
	peer := peers["alpha"]
	c.Check(peer.Name(), tc.Equals, "omega")
	c.Check(peer.Role(), tc.Equals, "peer")
	c.Check(peer.Interface(), tc.Equals, "monitoring")
	c.Check(peer.Optional(), tc.Equals, true)
	c.Check(peer.Limit(), tc.Equals, 3)
	c.Check(peer.Scope(), tc.Equals, "global")
	args.Peers = nil

	storage := args.Storage
	c.Assert(storage, tc.HasLen, 1)
	stor := storage["foo"]
	c.Check(stor.Name(), tc.Equals, "bar")
	c.Check(stor.Description(), tc.Equals, "baz")
	c.Check(stor.Type(), tc.Equals, "block")
	c.Check(stor.Shared(), tc.Equals, true)
	c.Check(stor.Readonly(), tc.Equals, true)
	c.Check(stor.CountMin(), tc.Equals, 1)
	c.Check(stor.CountMax(), tc.Equals, 2)
	c.Check(stor.MinimumSize(), tc.Equals, 1024)
	c.Check(stor.Location(), tc.Equals, "/var/lib/foo")
	c.Check(stor.Properties(), tc.DeepEquals, []string{"foo", "bar"})
	args.Storage = nil

	devices := args.Devices
	c.Assert(devices, tc.HasLen, 1)
	device := devices["foo"]
	c.Check(device.Name(), tc.Equals, "bar")
	c.Check(device.Description(), tc.Equals, "baz")
	c.Check(device.Type(), tc.Equals, "gpu")
	c.Check(device.CountMin(), tc.Equals, 1)
	c.Check(device.CountMax(), tc.Equals, 2)
	args.Devices = nil

	containers := args.Containers
	c.Assert(containers, tc.HasLen, 1)
	container := containers["foo"]
	c.Check(container.Resource(), tc.Equals, "resource")
	mounts := container.Mounts()
	c.Assert(mounts, tc.HasLen, 1)
	mount := mounts[0]
	c.Check(mount.Location(), tc.Equals, "/var/lib/foo")
	c.Check(mount.Storage(), tc.Equals, "bar")
	args.Containers = nil

	resources := args.Resources
	c.Assert(resources, tc.HasLen, 1)
	resource := resources["foo"]
	c.Check(resource.Name(), tc.Equals, "bar")
	c.Check(resource.Description(), tc.Equals, "baz")
	c.Check(resource.Type(), tc.Equals, "file")
	c.Check(resource.Path(), tc.Equals, "/var/lib/foo")
	args.Resources = nil

	c.Check(args, tc.DeepEquals, description.CharmMetadataArgs{
		Name:           "prometheus",
		Summary:        "Prometheus monitoring",
		Description:    "Prometheus is a monitoring system and time series database.",
		Subordinate:    true,
		Categories:     []string{"monitoring"},
		Tags:           []string{"monitoring", "time-series"},
		Terms:          []string{"monitoring", "time-series", "database"},
		RunAs:          "root",
		Assumes:        "[]",
		MinJujuVersion: "4.0.0",
		ExtraBindings: map[string]string{
			"foo": "bar",
		},
		LXDProfile: "{}",
	})
}

func (s *exportCharmSuite) TestExportCharmManifest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	manifest := &internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name: "ubuntu",
			Channel: internalcharm.Channel{
				Track:  "devel",
				Risk:   "edge",
				Branch: "foo",
			},
			Architectures: []string{"amd64"},
		}},
	}

	exportOp := s.newExportOperation()

	args, err := exportOp.exportCharmManifest(manifest)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(args.Bases, tc.HasLen, 1)
	base := args.Bases[0]
	c.Check(base.Name(), tc.Equals, "ubuntu")
	c.Check(base.Channel(), tc.Equals, "devel/edge/foo")
	c.Check(base.Architectures(), tc.DeepEquals, []string{"amd64"})
}

func (s *exportCharmSuite) TestExportCharmConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := &internalcharm.Config{
		Options: map[string]internalcharm.Option{
			"foo": {
				Type:        "string",
				Description: "foo option",
				Default:     "bar",
			},
		},
	}

	exportOp := s.newExportOperation()

	args, err := exportOp.exportCharmConfig(config)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(args.Configs, tc.HasLen, 1)
	option := args.Configs["foo"]
	c.Check(option.Type(), tc.Equals, "string")
	c.Check(option.Description(), tc.Equals, "foo option")
	c.Check(option.Default(), tc.Equals, "bar")
}

func (s *exportCharmSuite) TestExportCharmActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	actions := &internalcharm.Actions{
		ActionSpecs: map[string]internalcharm.ActionSpec{
			"foo": {
				Description:    "foo action",
				Parallel:       true,
				ExecutionGroup: "group",
				Params: map[string]interface{}{
					"foo": "bar",
				},
			},
		},
	}

	exportOp := s.newExportOperation()

	args, err := exportOp.exportCharmActions(actions)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(args.Actions, tc.HasLen, 1)
	action := args.Actions["foo"]
	c.Check(action.Description(), tc.Equals, "foo action")
	c.Check(action.Parallel(), tc.Equals, true)
	c.Check(action.ExecutionGroup(), tc.Equals, "group")
	c.Check(action.Parameters(), tc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
}
