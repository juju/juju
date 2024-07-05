// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/charm"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type metadataSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&metadataSuite{})

var metadataTestCases = [...]struct {
	name      string
	input     charmMetadata
	inputArgs decodeMetadataArgs
	output    charm.Metadata
}{
	{
		name:      "empty",
		input:     charmMetadata{},
		inputArgs: decodeMetadataArgs{},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
		},
	},
	{
		name: "basic",
		input: charmMetadata{
			Name:           "foo",
			Summary:        "summary",
			Description:    "description",
			MinJujuVersion: "2.0.0",
			RunAs:          "root",
			Subordinate:    true,
			Assumes:        []byte("null"),
		},
		inputArgs: decodeMetadataArgs{},
		output: charm.Metadata{
			Name:           "foo",
			Summary:        "summary",
			Description:    "description",
			MinJujuVersion: version.MustParse("2.0.0"),
			RunAs:          charm.RunAsRoot,
			Subordinate:    true,
			Assumes:        []byte("null"),
		},
	},
	{
		name:  "tags",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			tags: []charmTag{
				{Tag: "tag1"},
				{Tag: "tag2"},
			},
		},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
			Tags:  []string{"tag1", "tag2"},
		},
	},
	{
		name:  "categories",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			categories: []charmCategory{
				{Category: "category1"},
				{Category: "category2"},
			},
		},
		output: charm.Metadata{
			RunAs:      charm.RunAsDefault,
			Categories: []string{"category1", "category2"},
		},
	},
	{
		name:  "terms",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			terms: []charmTerm{
				{Term: "term1"},
				{Term: "term2"},
			},
		},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
			Terms: []string{"term1", "term2"},
		},
	},
	{
		name:  "relations",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			relations: []charmRelation{
				{
					Key:       "mysql",
					Kind:      "provides",
					Name:      "db1",
					Role:      "provider",
					Interface: "mysql",
					Optional:  true,
					Capacity:  1,
					Scope:     "global",
				},
				{
					Key:       "postgres",
					Kind:      "provides",
					Name:      "db2",
					Role:      "provider",
					Interface: "postgres",
					Optional:  true,
					Capacity:  1,
					Scope:     "global",
				},
				{
					Key:       "wordpress",
					Kind:      "requires",
					Name:      "blog",
					Role:      "requirer",
					Interface: "wordpress",
					Optional:  true,
					Capacity:  2,
					Scope:     "container",
				},
				{
					Key:       "vault",
					Kind:      "peers",
					Name:      "enclave",
					Role:      "peer",
					Interface: "vault",
					Optional:  true,
					Capacity:  3,
					Scope:     "global",
				},
			},
		},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
			Provides: map[string]charm.Relation{
				"mysql": {
					Key:       "mysql",
					Name:      "db1",
					Role:      charm.RoleProvider,
					Interface: "mysql",
					Optional:  true,
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
				"postgres": {
					Key:       "postgres",
					Name:      "db2",
					Role:      charm.RoleProvider,
					Interface: "postgres",
					Optional:  true,
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"wordpress": {
					Key:       "wordpress",
					Name:      "blog",
					Role:      charm.RoleRequirer,
					Interface: "wordpress",
					Optional:  true,
					Limit:     2,
					Scope:     charm.ScopeContainer,
				},
			},
			Peers: map[string]charm.Relation{
				"vault": {
					Key:       "vault",
					Name:      "enclave",
					Role:      charm.RolePeer,
					Interface: "vault",
					Optional:  true,
					Limit:     3,
					Scope:     charm.ScopeGlobal,
				},
			},
		},
	},
	{
		name:  "extra bindings",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			extraBindings: []charmExtraBinding{
				{Key: "alpha", Name: "foo"},
				{Key: "beta", Name: "baz"},
			},
		},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
			ExtraBindings: map[string]charm.ExtraBinding{
				"alpha": {Name: "foo"},
				"beta":  {Name: "baz"},
			},
		},
	},
	{
		name:  "storage",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			storage: []charmStorage{
				{
					Key:         "foo",
					Name:        "name1",
					Description: "description1",
					Kind:        "block",
					Shared:      true,
					ReadOnly:    true,
					MinimumSize: 1,
					CountMin:    2,
					CountMax:    3,
					Location:    "location1",
					Property:    "property1",
				},
				{
					Key:         "foo",
					Name:        "name1",
					Description: "description1",
					Kind:        "block",
					Shared:      true,
					ReadOnly:    true,
					MinimumSize: 1,
					CountMin:    2,
					CountMax:    3,
					Location:    "location1",
					Property:    "property2",
				},
				{
					Key:         "bar",
					Name:        "name2",
					Description: "description2",
					Kind:        "block",
					Shared:      true,
					ReadOnly:    true,
					MinimumSize: 4,
					CountMin:    5,
					CountMax:    6,
					Location:    "location2",
					Property:    "property3",
				},
			},
		},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
			Storage: map[string]charm.Storage{
				"foo": {
					Name:        "name1",
					Description: "description1",
					Type:        charm.StorageBlock,
					Shared:      true,
					ReadOnly:    true,
					MinimumSize: 1,
					CountMin:    2,
					CountMax:    3,
					Location:    "location1",
					Properties:  []string{"property1", "property2"},
				},
				"bar": {
					Name:        "name2",
					Description: "description2",
					Type:        charm.StorageBlock,
					Shared:      true,
					ReadOnly:    true,
					MinimumSize: 4,
					CountMin:    5,
					CountMax:    6,
					Location:    "location2",
					Properties:  []string{"property3"},
				},
			},
		},
	},
	{
		name:  "devices",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			devices: []charmDevice{
				{
					Key:         "alpha",
					Name:        "foo",
					Description: "description1",
					DeviceType:  "block",
					CountMin:    2,
					CountMax:    3,
				},
				{
					Key:         "beta",
					Name:        "baz",
					Description: "description2",
					DeviceType:  "filesystem",
					CountMin:    4,
					CountMax:    5,
				},
			},
		},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
			Devices: map[string]charm.Device{
				"alpha": {
					Name:        "foo",
					Description: "description1",
					Type:        charm.DeviceType("block"),
					CountMin:    2,
					CountMax:    3,
				},
				"beta": {
					Name:        "baz",
					Description: "description2",
					Type:        charm.DeviceType("filesystem"),
					CountMin:    4,
					CountMax:    5,
				},
			},
		},
	},
	{
		name:  "payloads",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			payloads: []charmPayload{
				{
					Key:  "alpha",
					Name: "foo",
					Type: "type1",
				},
				{
					Key:  "beta",
					Name: "baz",
					Type: "type2",
				},
			},
		},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
			PayloadClasses: map[string]charm.PayloadClass{
				"alpha": {
					Name: "foo",
					Type: "type1",
				},
				"beta": {
					Name: "baz",
					Type: "type2",
				},
			},
		},
	},
	{
		name:  "resources",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			resources: []charmResource{
				{
					Key:         "alpha",
					Name:        "foo",
					Kind:        "file",
					Path:        "path1",
					Description: "description1",
				},
				{
					Key:         "beta",
					Name:        "baz",
					Kind:        "oci-image",
					Path:        "path2",
					Description: "description2",
				},
			},
		},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
			Resources: map[string]charm.Resource{
				"alpha": {
					Name:        "foo",
					Type:        charm.ResourceTypeFile,
					Path:        "path1",
					Description: "description1",
				},
				"beta": {
					Name:        "baz",
					Type:        charm.ResourceTypeContainerImage,
					Path:        "path2",
					Description: "description2",
				},
			},
		},
	},
	{
		name:  "containers",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			containers: []charmContainer{
				{
					Key:      "alpha",
					Resource: "foo",
					Uid:      -1,
					Gid:      -1,
					Storage:  "storage1",
					Location: "location1",
				},
				{
					Key:      "alpha",
					Resource: "foo",
					Uid:      -1,
					Gid:      -1,
					Storage:  "storage2",
					Location: "location2",
				},
				{
					Key:      "beta",
					Resource: "baz",
					Uid:      1000,
					Gid:      1001,
					Storage:  "storage3",
					Location: "location3",
				},
			},
		},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
			Containers: map[string]charm.Container{
				"alpha": {
					Resource: "foo",
					Mounts: []charm.Mount{
						{Storage: "storage1", Location: "location1"},
						{Storage: "storage2", Location: "location2"},
					},
				},
				"beta": {
					Resource: "baz",
					Uid:      ptr(1000),
					Gid:      ptr(1001),
					Mounts: []charm.Mount{
						{Storage: "storage3", Location: "location3"},
					},
				},
			},
		},
	},
}

func (s *metadataSuite) TestConvertMetadata(c *gc.C) {
	for _, tc := range metadataTestCases {
		c.Logf("Running test case %q", tc.name)

		result, err := decodeMetadata(tc.input, tc.inputArgs)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, gc.DeepEquals, tc.output)
	}
}
