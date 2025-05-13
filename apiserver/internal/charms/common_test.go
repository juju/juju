// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type exportSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&exportSuite{})

func (s *exportSuite) TestExport(c *tc.C) {
	// Ensure that we can export a full charm.

	metadata := &internalcharm.Meta{
		Name:        "foo",
		Summary:     "summary",
		Description: "description",
		Subordinate: true,
		Provides: map[string]internalcharm.Relation{
			"foo": {Name: "foo", Role: internalcharm.RoleProvider, Interface: "bar", Optional: true, Limit: 1, Scope: internalcharm.ScopeContainer},
		},
		Requires: map[string]internalcharm.Relation{
			"bar": {Name: "bar", Role: internalcharm.RoleRequirer, Interface: "foo", Optional: true, Limit: 2, Scope: internalcharm.ScopeGlobal},
		},
		Peers: map[string]internalcharm.Relation{
			"baz": {Name: "baz", Role: internalcharm.RolePeer, Interface: "baz", Optional: true, Limit: 3, Scope: internalcharm.ScopeGlobal},
		},
		ExtraBindings: map[string]internalcharm.ExtraBinding{
			"qux": {Name: "mux"},
		},
		Categories: []string{"cat1", "cat2"},
		Tags:       []string{"tag1", "tag2"},
		Storage: map[string]internalcharm.Storage{
			"foo": {Name: "foo", Description: "bar", Shared: true, ReadOnly: true, CountMin: 1, CountMax: 2, MinimumSize: 1, Location: "/foo"},
		},
		Devices: map[string]internalcharm.Device{
			"bar": {Name: "bar", Description: "baz", Type: "gpu", CountMin: 1, CountMax: 2},
		},
		Resources: map[string]resource.Meta{
			"qux": {Name: "qux", Type: resource.TypeContainerImage, Description: "bar", Path: "/baz"},
		},
		Terms:          []string{"term1", "term2"},
		MinJujuVersion: semversion.MustParse("4.0.0"),
		Containers: map[string]internalcharm.Container{
			"foo": {
				Resource: "foo",
				Mounts:   []internalcharm.Mount{{Storage: "foo", Location: "/bar"}},
				Gid:      ptr(1000),
				Uid:      ptr(1000),
			},
		},
		Assumes: &assumes.ExpressionTree{
			Expression: assumes.CompositeExpression{
				ExprType:       assumes.AllOfExpression,
				SubExpressions: []assumes.Expression{},
			},
		},
		CharmUser: internalcharm.RunAsNonRoot,
	}
	manifest := &internalcharm.Manifest{
		Bases: []internalcharm.Base{
			{
				Name:          "ubuntu",
				Channel:       internalcharm.Channel{Track: "22.04", Risk: "stable"},
				Architectures: []string{"arm64", "amd64"},
			},
		},
	}
	config := &internalcharm.Config{
		Options: map[string]internalcharm.Option{"foo": {Type: "string", Description: "bar", Default: "baz"}},
	}
	actions := &internalcharm.Actions{
		ActionSpecs: map[string]internalcharm.ActionSpec{
			"bar": {
				Parallel:    true,
				Description: "baz",
				Params: map[string]interface{}{
					"foo": "bar",
					"blah": map[string]interface{}{
						"alpha": "omega",
					},
				},
				ExecutionGroup: "group",
			},
		},
	}
	lxdProfile := &internalcharm.LXDProfile{
		Config:      map[string]string{"foo": "bar"},
		Description: "description",
		Devices: map[string]map[string]string{
			"foo": {"bar": "baz"},
		},
	}

	charmBase := internalcharm.NewCharmBase(metadata, manifest, config, actions, lxdProfile)

	locator := applicationcharm.CharmLocator{
		Source:       applicationcharm.CharmHubSource,
		Revision:     42,
		Architecture: architecture.AMD64,
	}

	result, err := convertCharm("foo", charmBase, locator)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.Charm{
		Revision: 42,
		URL:      "ch:amd64/foo-42",
		Config: map[string]params.CharmOption{
			"foo": {Type: "string", Description: "bar", Default: "baz"},
		},
		Meta: &params.CharmMeta{
			Name:        "foo",
			Summary:     "summary",
			Description: "description",
			Subordinate: true,
			Provides: map[string]params.CharmRelation{
				"foo": {Name: "foo", Role: "provider", Interface: "bar", Optional: true, Limit: 1, Scope: "container"},
			},
			Requires: map[string]params.CharmRelation{
				"bar": {Name: "bar", Role: "requirer", Interface: "foo", Optional: true, Limit: 2, Scope: "global"},
			},
			Peers: map[string]params.CharmRelation{
				"baz": {Name: "baz", Role: "peer", Interface: "baz", Optional: true, Limit: 3, Scope: "global"},
			},
			ExtraBindings: map[string]string{
				"qux": "mux",
			},
			Categories:     []string{"cat1", "cat2"},
			Tags:           []string{"tag1", "tag2"},
			Storage:        map[string]params.CharmStorage{"foo": {Name: "foo", Description: "bar", Shared: true, ReadOnly: true, CountMin: 1, CountMax: 2, MinimumSize: 1, Location: "/foo"}},
			Devices:        map[string]params.CharmDevice{"bar": {Name: "bar", Description: "baz", Type: "gpu", CountMin: 1, CountMax: 2}},
			Resources:      map[string]params.CharmResourceMeta{"qux": {Name: "qux", Type: "oci-image", Description: "bar", Path: "/baz"}},
			Terms:          []string{"term1", "term2"},
			MinJujuVersion: "4.0.0",
			Containers: map[string]params.CharmContainer{
				"foo": {
					Resource: "foo",
					Mounts:   []params.CharmMount{{Storage: "foo", Location: "/bar"}},
					Gid:      ptr(1000),
					Uid:      ptr(1000),
				},
			},
			AssumesExpr: &assumes.ExpressionTree{
				Expression: assumes.CompositeExpression{
					ExprType:       assumes.AllOfExpression,
					SubExpressions: []assumes.Expression{},
				},
			},
			CharmUser: "non-root",
		},
		Actions: &params.CharmActions{
			ActionSpecs: map[string]params.CharmActionSpec{
				"bar": {
					Parallel:    true,
					Description: "baz",
					Params: map[string]interface{}{
						"foo": "bar",
						"blah": map[string]interface{}{
							"alpha": "omega",
						},
					},
					ExecutionGroup: "group",
				},
			},
		},
		Manifest: &params.CharmManifest{
			Bases: []params.CharmBase{
				{Name: "ubuntu", Channel: "22.04/stable", Architectures: []string{"arm64", "amd64"}},
			},
		},
		LXDProfile: &params.CharmLXDProfile{
			Config:      map[string]string{"foo": "bar"},
			Description: "description",
			Devices: map[string]map[string]string{
				"foo": {"bar": "baz"},
			},
		},
	})
}
