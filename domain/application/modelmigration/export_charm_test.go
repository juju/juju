// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/semversion"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
)

type exportCharmSuite struct {
	exportSuite
}

var _ = gc.Suite(&exportCharmSuite{})

func (s *exportCharmSuite) TestApplicationExportMinimalCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectApplication(c)
	s.expectMinimalCharm()
	s.expectApplicationConfig()
	s.expectApplicationConstraints(constraints.Value{})

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	model := description.NewModel(description.ModelArgs{})

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model.Applications(), gc.HasLen, 1)

	app := model.Applications()[0]
	c.Check(app.Name(), gc.Equals, "prometheus")
	c.Check(app.CharmURL(), gc.Equals, "ch:amd64/prometheus-42")

	metadata := app.CharmMetadata()
	c.Assert(metadata, gc.NotNil)
	c.Check(metadata.Name(), gc.Equals, "prometheus")
}

func (s *exportCharmSuite) TestExportCharmMetadata(c *gc.C) {
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

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	args, err := exportOp.exportCharmMetadata(meta, "{}")
	c.Assert(err, jc.ErrorIsNil)

	// As the description package exposes interfaces, it becomes difficult to
	// test it nicely. To work around this, we'll check the individual fields
	// of the CharmMetadataArgs struct. Once they've been checked, we nil
	// out the fields so that we can compare the rest of the struct.

	provides := args.Provides
	c.Assert(provides, gc.HasLen, 1)
	provider := provides["prometheus"]
	c.Check(provider.Name(), gc.Equals, "prometheus")
	c.Check(provider.Role(), gc.Equals, "provider")
	c.Check(provider.Interface(), gc.Equals, "monitoring")
	c.Check(provider.Optional(), gc.Equals, true)
	c.Check(provider.Limit(), gc.Equals, 1)
	c.Check(provider.Scope(), gc.Equals, "global")
	args.Provides = nil

	requires := args.Requires
	c.Assert(requires, gc.HasLen, 1)
	require := requires["foo"]
	c.Check(require.Name(), gc.Equals, "bar")
	c.Check(require.Role(), gc.Equals, "requirer")
	c.Check(require.Interface(), gc.Equals, "baz")
	c.Check(require.Optional(), gc.Equals, true)
	c.Check(require.Limit(), gc.Equals, 2)
	c.Check(require.Scope(), gc.Equals, "container")
	args.Requires = nil

	peers := args.Peers
	c.Assert(peers, gc.HasLen, 1)
	peer := peers["alpha"]
	c.Check(peer.Name(), gc.Equals, "omega")
	c.Check(peer.Role(), gc.Equals, "peer")
	c.Check(peer.Interface(), gc.Equals, "monitoring")
	c.Check(peer.Optional(), gc.Equals, true)
	c.Check(peer.Limit(), gc.Equals, 3)
	c.Check(peer.Scope(), gc.Equals, "global")
	args.Peers = nil

	storage := args.Storage
	c.Assert(storage, gc.HasLen, 1)
	stor := storage["foo"]
	c.Check(stor.Name(), gc.Equals, "bar")
	c.Check(stor.Description(), gc.Equals, "baz")
	c.Check(stor.Type(), gc.Equals, "block")
	c.Check(stor.Shared(), gc.Equals, true)
	c.Check(stor.Readonly(), gc.Equals, true)
	c.Check(stor.CountMin(), gc.Equals, 1)
	c.Check(stor.CountMax(), gc.Equals, 2)
	c.Check(stor.MinimumSize(), gc.Equals, 1024)
	c.Check(stor.Location(), gc.Equals, "/var/lib/foo")
	c.Check(stor.Properties(), jc.DeepEquals, []string{"foo", "bar"})
	args.Storage = nil

	devices := args.Devices
	c.Assert(devices, gc.HasLen, 1)
	device := devices["foo"]
	c.Check(device.Name(), gc.Equals, "bar")
	c.Check(device.Description(), gc.Equals, "baz")
	c.Check(device.Type(), gc.Equals, "gpu")
	c.Check(device.CountMin(), gc.Equals, 1)
	c.Check(device.CountMax(), gc.Equals, 2)
	args.Devices = nil

	containers := args.Containers
	c.Assert(containers, gc.HasLen, 1)
	container := containers["foo"]
	c.Check(container.Resource(), gc.Equals, "resource")
	mounts := container.Mounts()
	c.Assert(mounts, gc.HasLen, 1)
	mount := mounts[0]
	c.Check(mount.Location(), gc.Equals, "/var/lib/foo")
	c.Check(mount.Storage(), gc.Equals, "bar")
	args.Containers = nil

	resources := args.Resources
	c.Assert(resources, gc.HasLen, 1)
	resource := resources["foo"]
	c.Check(resource.Name(), gc.Equals, "bar")
	c.Check(resource.Description(), gc.Equals, "baz")
	c.Check(resource.Type(), gc.Equals, "file")
	c.Check(resource.Path(), gc.Equals, "/var/lib/foo")
	args.Resources = nil

	c.Check(args, gc.DeepEquals, description.CharmMetadataArgs{
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

func (s *exportCharmSuite) TestExportCharmManifest(c *gc.C) {
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

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	args, err := exportOp.exportCharmManifest(manifest)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(args.Bases, gc.HasLen, 1)
	base := args.Bases[0]
	c.Check(base.Name(), gc.Equals, "ubuntu")
	c.Check(base.Channel(), gc.Equals, "devel/edge/foo")
	c.Check(base.Architectures(), jc.DeepEquals, []string{"amd64"})
}

func (s *exportCharmSuite) TestExportCharmConfig(c *gc.C) {
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

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	args, err := exportOp.exportCharmConfig(config)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(args.Configs, gc.HasLen, 1)
	option := args.Configs["foo"]
	c.Check(option.Type(), gc.Equals, "string")
	c.Check(option.Description(), gc.Equals, "foo option")
	c.Check(option.Default(), gc.Equals, "bar")
}

func (s *exportCharmSuite) TestExportCharmActions(c *gc.C) {
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

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	args, err := exportOp.exportCharmActions(actions)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(args.Actions, gc.HasLen, 1)
	action := args.Actions["foo"]
	c.Check(action.Description(), gc.Equals, "foo action")
	c.Check(action.Parallel(), gc.Equals, true)
	c.Check(action.ExecutionGroup(), gc.Equals, "group")
	c.Check(action.Parameters(), jc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
}
