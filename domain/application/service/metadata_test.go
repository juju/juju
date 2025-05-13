// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

type metadataSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&metadataSuite{})

var metadataTestCases = [...]struct {
	name   string
	input  charm.Metadata
	output internalcharm.Meta
}{
	{
		name: "name",
		input: charm.Metadata{
			Name: "foo",
			// RunAs is optional and defaults to "default", this means we're
			// storing a valid value in the persistence layer.
			RunAs: charm.RunAsDefault,
		},
		output: internalcharm.Meta{
			Name: "foo",
		},
	},
	{
		name: "common",
		input: charm.Metadata{
			Name:        "foo",
			RunAs:       charm.RunAsDefault,
			Summary:     "summary",
			Description: "description",
			Categories:  []string{"cat1", "cat2"},
			Subordinate: true,
			Terms:       []string{"term1", "term2"},
		},
		output: internalcharm.Meta{
			Name:        "foo",
			Summary:     "summary",
			Description: "description",
			Categories:  []string{"cat1", "cat2"},
			Subordinate: true,
			Terms:       []string{"term1", "term2"},
		},
	},
	{
		name: "min version",
		input: charm.Metadata{
			Name:           "foo",
			RunAs:          charm.RunAsDefault,
			MinJujuVersion: semversion.MustParse("2.0.0"),
		},
		output: internalcharm.Meta{
			Name:           "foo",
			MinJujuVersion: semversion.MustParse("2.0.0"),
		},
	},
	{
		name: "charm user",
		input: charm.Metadata{
			Name:  "foo",
			RunAs: charm.RunAsNonRoot,
		},
		output: internalcharm.Meta{
			Name:      "foo",
			CharmUser: internalcharm.RunAsNonRoot,
		},
	},
	{
		name: "provides",
		input: charm.Metadata{
			Name:  "foo",
			RunAs: charm.RunAsDefault,
			Provides: map[string]charm.Relation{
				"baz": {
					Name:      "baz",
					Role:      charm.RoleProvider,
					Interface: "mysql",
					Optional:  true,
					Limit:     2,
					Scope:     charm.ScopeGlobal,
				},
			},
		},
		output: internalcharm.Meta{
			Name: "foo",
			Provides: map[string]internalcharm.Relation{
				"baz": {
					Name:      "baz",
					Role:      internalcharm.RoleProvider,
					Interface: "mysql",
					Optional:  true,
					Limit:     2,
					Scope:     internalcharm.ScopeGlobal,
				},
			},
		},
	},
	{
		name: "provides juju-info",
		input: charm.Metadata{
			Name:  "foo",
			RunAs: charm.RunAsDefault,
			Provides: map[string]charm.Relation{
				relation.JujuInfo: {
					Name:      relation.JujuInfo,
					Role:      charm.RoleProvider,
					Interface: relation.JujuInfo,
					Scope:     charm.ScopeGlobal,
				},
			},
		},
		output: internalcharm.Meta{
			Name: "foo",
			Provides: map[string]internalcharm.Relation{
				relation.JujuInfo: {
					Name:      relation.JujuInfo,
					Role:      internalcharm.RoleProvider,
					Interface: relation.JujuInfo,
					Scope:     internalcharm.ScopeGlobal,
				},
			},
		},
	},
	{
		name: "requires",
		input: charm.Metadata{
			Name:  "foo",
			RunAs: charm.RunAsDefault,
			Requires: map[string]charm.Relation{
				"baz": {
					Name:      "baz",
					Role:      charm.RoleRequirer,
					Interface: "mysql",
					Optional:  true,
					Limit:     2,
					Scope:     charm.ScopeGlobal,
				},
			},
		},
		output: internalcharm.Meta{
			Name: "foo",
			Requires: map[string]internalcharm.Relation{
				"baz": {
					Name:      "baz",
					Role:      internalcharm.RoleRequirer,
					Interface: "mysql",
					Optional:  true,
					Limit:     2,
					Scope:     internalcharm.ScopeGlobal,
				},
			},
		},
	},
	{
		name: "peers",
		input: charm.Metadata{
			Name:  "foo",
			RunAs: charm.RunAsDefault,
			Peers: map[string]charm.Relation{
				"baz": {
					Name:      "baz",
					Role:      charm.RolePeer,
					Interface: "mysql",
					Optional:  true,
					Limit:     2,
					Scope:     charm.ScopeGlobal,
				},
			},
		},
		output: internalcharm.Meta{
			Name: "foo",
			Peers: map[string]internalcharm.Relation{
				"baz": {
					Name:      "baz",
					Role:      internalcharm.RolePeer,
					Interface: "mysql",
					Optional:  true,
					Limit:     2,
					Scope:     internalcharm.ScopeGlobal,
				},
			},
		},
	},
	{
		name: "storage",
		input: charm.Metadata{
			Name:  "foo",
			RunAs: charm.RunAsDefault,
			Storage: map[string]charm.Storage{
				"sda": {
					Name:        "sda",
					Description: "system disk",
					Type:        charm.StorageBlock,
					Shared:      true,
					ReadOnly:    true,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 1024,
					Location:    "/mnt/sda",
					Properties:  []string{"foo", "bar"},
				},
			},
		},
		output: internalcharm.Meta{
			Name: "foo",
			Storage: map[string]internalcharm.Storage{
				"sda": {
					Name:        "sda",
					Description: "system disk",
					Type:        internalcharm.StorageBlock,
					Shared:      true,
					ReadOnly:    true,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 1024,
					Location:    "/mnt/sda",
					Properties:  []string{"foo", "bar"},
				},
			},
		},
	},
	{
		name: "devices",
		input: charm.Metadata{
			Name:  "foo",
			RunAs: charm.RunAsDefault,
			Devices: map[string]charm.Device{
				"gpu": {
					Name:        "gpu",
					Description: "AMD GPU",
					Type:        "gpu",
					CountMin:    1,
					CountMax:    2,
				},
			},
		},
		output: internalcharm.Meta{
			Name: "foo",
			Devices: map[string]internalcharm.Device{
				"gpu": {
					Name:        "gpu",
					Description: "AMD GPU",
					Type:        "gpu",
					CountMin:    1,
					CountMax:    2,
				},
			},
		},
	},
	{
		name: "resources",
		input: charm.Metadata{
			Name:  "foo",
			RunAs: charm.RunAsDefault,
			Resources: map[string]charm.Resource{
				"foo": {
					Name:        "foo",
					Description: "bar",
					Path:        "/home/baz/foo",
					Type:        charm.ResourceTypeFile,
				},
			},
		},
		output: internalcharm.Meta{
			Name: "foo",
			Resources: map[string]resource.Meta{
				"foo": {
					Name:        "foo",
					Description: "bar",
					Path:        "/home/baz/foo",
					Type:        resource.TypeFile,
				},
			},
		},
	},
	{
		name: "containers",
		input: charm.Metadata{
			Name:  "foo",
			RunAs: charm.RunAsDefault,
			Containers: map[string]charm.Container{
				"foo": {
					Resource: "bar",
					Mounts: []charm.Mount{
						{
							Location: "blah",
							Storage:  "foo",
						},
					},
				},
			},
		},
		output: internalcharm.Meta{
			Name: "foo",
			Containers: map[string]internalcharm.Container{
				"foo": {
					Resource: "bar",
					Mounts: []internalcharm.Mount{
						{
							Location: "blah",
							Storage:  "foo",
						},
					},
				},
			},
		},
	},
	{
		name: "assumes",
		input: charm.Metadata{
			Name:    "foo",
			RunAs:   charm.RunAsDefault,
			Assumes: []byte(`{"assumes":["chips",{"any-of":["guacamole","salsa",{"any-of":["good-weather","great-music"]}]},{"all-of":["table","lazy-suzan"]}]}`),
		},
		output: internalcharm.Meta{
			Name: "foo",
			Assumes: &assumes.ExpressionTree{
				Expression: assumes.CompositeExpression{
					ExprType: assumes.AllOfExpression,
					SubExpressions: []assumes.Expression{
						assumes.FeatureExpression{Name: "chips"},
						assumes.CompositeExpression{
							ExprType: assumes.AnyOfExpression,
							SubExpressions: []assumes.Expression{
								assumes.FeatureExpression{Name: "guacamole"},
								assumes.FeatureExpression{Name: "salsa"},
								assumes.CompositeExpression{
									ExprType: assumes.AnyOfExpression,
									SubExpressions: []assumes.Expression{
										assumes.FeatureExpression{Name: "good-weather"},
										assumes.FeatureExpression{Name: "great-music"},
									},
								},
							},
						},
						assumes.CompositeExpression{
							ExprType: assumes.AllOfExpression,
							SubExpressions: []assumes.Expression{
								assumes.FeatureExpression{Name: "table"},
								assumes.FeatureExpression{Name: "lazy-suzan"},
							},
						},
					},
				},
			},
		},
	},
}

func (s *metadataSuite) TestConvertMetadata(c *tc.C) {
	for _, testCase := range metadataTestCases {
		c.Logf("Running test case %q", testCase.name)

		result, err := decodeMetadata(testCase.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, testCase.output)

		// Ensure that the conversion is idempotent.
		converted, err := encodeMetadata(&result)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(converted, tc.DeepEquals, testCase.input)
	}
}
