// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/application/charm"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type metadataSuite struct {
	schematesting.ModelSuite
}

func TestMetadataSuite(t *testing.T) {
	tc.Run(t, &metadataSuite{})
}

var metadataDecodeTestCases = [...]struct {
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
			MinJujuVersion: semversion.MustParse("2.0.0"),
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
					Name:      "db1",
					Role:      "provider",
					Interface: "mysql",
					Optional:  true,
					Capacity:  1,
					Scope:     "global",
				},
				{
					Name:      "db2",
					Role:      "provider",
					Interface: "postgres",
					Optional:  true,
					Capacity:  1,
					Scope:     "global",
				},
				{
					Name:      "blog",
					Role:      "requirer",
					Interface: "wordpress",
					Optional:  true,
					Capacity:  2,
					Scope:     "container",
				},
				{
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
				"db1": {
					Name:      "db1",
					Role:      charm.RoleProvider,
					Interface: "mysql",
					Optional:  true,
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
				"db2": {
					Name:      "db2",
					Role:      charm.RoleProvider,
					Interface: "postgres",
					Optional:  true,
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"blog": {
					Name:      "blog",
					Role:      charm.RoleRequirer,
					Interface: "wordpress",
					Optional:  true,
					Limit:     2,
					Scope:     charm.ScopeContainer,
				},
			},
			Peers: map[string]charm.Relation{
				"enclave": {
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
				{Name: "foo"},
				{Name: "baz"},
			},
		},
		output: charm.Metadata{
			RunAs: charm.RunAsDefault,
			ExtraBindings: map[string]charm.ExtraBinding{
				"foo": {Name: "foo"},
				"baz": {Name: "baz"},
			},
		},
	},
	{
		name:  "storage",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			storage: []charmStorage{
				{
					Name:        "foo",
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
					Name:        "foo",
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
					Name:        "bar",
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
					Name:        "foo",
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
					Name:        "bar",
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
		name:  "resources",
		input: charmMetadata{},
		inputArgs: decodeMetadataArgs{
			resources: []charmResource{
				{
					Name:        "foo",
					Kind:        "file",
					Path:        "path1",
					Description: "description1",
				},
				{
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
				"foo": {
					Name:        "foo",
					Type:        charm.ResourceTypeFile,
					Path:        "path1",
					Description: "description1",
				},
				"baz": {
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

func (s *metadataSuite) TestDecodeMetadata(c *tc.C) {
	for _, testCase := range metadataDecodeTestCases {
		c.Logf("Running test case %q", testCase.name)

		result, err := decodeMetadata(testCase.input, testCase.inputArgs)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, testCase.output)
	}
}

func (s *metadataSuite) TestEncodeMetadata(c *tc.C) {
	id := corecharm.GenCharmID(c)

	result, err := encodeMetadata(id, charm.Metadata{
		Name:           "foo",
		Summary:        "summary",
		Description:    "description",
		MinJujuVersion: semversion.MustParse("2.0.0"),
		RunAs:          "root",
		Subordinate:    true,
		Assumes:        []byte("null"),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, setCharmMetadata{
		CharmUUID:      id.String(),
		Name:           "foo",
		Summary:        "summary",
		Description:    "description",
		MinJujuVersion: "2.0.0",
		RunAsID:        1,
		Subordinate:    true,
		Assumes:        []byte("null"),
	})

}

type metadataStateSuite struct {
	schematesting.ModelSuite
}

func TestMetadataStateSuite(t *testing.T) {
	tc.Run(t, &metadataStateSuite{})
}

// Bake the charm.RunAs values into the database.
func (s *metadataStateSuite) TestMetadataRunAs(c *tc.C) {
	type charmRunAs struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	stmt := sqlair.MustPrepare(`
SELECT charm_run_as_kind.* AS &charmRunAs.* FROM charm_run_as_kind ORDER BY id;
`, charmRunAs{})

	var results []charmRunAs
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).GetAll(&results)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 4)

	m := []charm.RunAs{
		charm.RunAsDefault,
		charm.RunAsRoot,
		charm.RunAsSudoer,
		charm.RunAsNonRoot,
	}

	for i, value := range m {
		c.Logf("result %d: %#v", i, value)
		result, err := encodeRunAs(value)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, results[i].ID)
	}
}

func (s *metadataStateSuite) TestMetadataRunAsWithError(c *tc.C) {
	_, err := encodeRunAs(charm.RunAs("invalid"))
	c.Assert(err, tc.ErrorMatches, `unknown run as value "invalid"`)
}

func (s *metadataStateSuite) TestMetadataRelationRole(c *tc.C) {
	type charmRelationRole struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	stmt := sqlair.MustPrepare(`
SELECT charm_relation_role.* AS &charmRelationRole.* FROM charm_relation_role ORDER BY id;
`, charmRelationRole{})

	var results []charmRelationRole
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).GetAll(&results)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 3)

	m := []charm.RelationRole{
		charm.RoleProvider,
		charm.RoleRequirer,
		charm.RolePeer,
	}

	for i, value := range m {
		c.Logf("result %d: %#v", i, value)
		result, err := encodeRelationRole(value)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, results[i].ID)
	}
}

func (s *metadataStateSuite) TestMetadataRelationRoleWithError(c *tc.C) {
	_, err := encodeRelationRole(charm.RelationRole("invalid"))
	c.Assert(err, tc.ErrorMatches, `unknown relation role "invalid"`)
}

func (s *metadataStateSuite) TestMetadataRelationScope(c *tc.C) {
	type charmRelationScope struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	stmt := sqlair.MustPrepare(`
SELECT charm_relation_scope.* AS &charmRelationScope.* FROM charm_relation_scope ORDER BY id;
`, charmRelationScope{})

	var results []charmRelationScope
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).GetAll(&results)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)

	m := []charm.RelationScope{
		charm.ScopeGlobal,
		charm.ScopeContainer,
	}

	for i, value := range m {
		c.Logf("result %d: %#v", i, value)
		result, err := encodeRelationScope(value)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, results[i].ID)
	}
}

func (s *metadataStateSuite) TestMetadataRelationScopeWithError(c *tc.C) {
	_, err := encodeRelationScope(charm.RelationScope("invalid"))
	c.Assert(err, tc.ErrorMatches, `unknown relation scope "invalid"`)
}

func (s *metadataStateSuite) TestMetadataStorageKind(c *tc.C) {
	type storageKind struct {
		ID   int    `db:"id"`
		Kind string `db:"kind"`
	}

	stmt := sqlair.MustPrepare(`
SELECT charm_storage_kind.* AS &storageKind.* FROM charm_storage_kind ORDER BY id;
`, storageKind{})

	var results []storageKind
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).GetAll(&results)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)

	m := []charm.StorageType{
		charm.StorageBlock,
		charm.StorageFilesystem,
	}

	for i, value := range m {
		c.Logf("result %d: %#v", i, value)
		result, err := encodeStorageType(value)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, results[i].ID)
	}
}

func (s *metadataStateSuite) TestMetadataStorageKindWithError(c *tc.C) {
	_, err := encodeStorageType(charm.StorageType("invalid"))
	c.Assert(err, tc.ErrorMatches, `unknown storage kind "invalid"`)
}

func (s *metadataStateSuite) TestMetadataResourceKind(c *tc.C) {
	type charmResourceKind struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	stmt := sqlair.MustPrepare(`
SELECT charm_resource_kind.* AS &charmResourceKind.* FROM charm_resource_kind ORDER BY id;
`, charmResourceKind{})

	var results []charmResourceKind
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).GetAll(&results)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)

	m := []charm.ResourceType{
		charm.ResourceTypeFile,
		charm.ResourceTypeContainerImage,
	}

	for i, value := range m {
		c.Logf("result %d: %#v", i, value)
		result, err := encodeResourceType(value)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, results[i].ID)
	}
}

func (s *metadataStateSuite) TestMetadataResourceKindWithError(c *tc.C) {
	_, err := encodeResourceType(charm.ResourceType("invalid"))
	c.Assert(err, tc.ErrorMatches, `unknown resource kind "invalid"`)
}
