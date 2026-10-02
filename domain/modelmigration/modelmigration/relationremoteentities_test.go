// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/core/relation"
	deploymentcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/internal/testhelpers"
)

type relationRemoteEntitiesSuite struct {
	testhelpers.IsolationSuite
}

func TestRelationRemoteEntitiesSuite(t *testing.T) {
	tc.Run(t, &relationRemoteEntitiesSuite{})
}

func (s *relationRemoteEntitiesSuite) TestExtractRelationUUIDFromRemoteEntities(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-dummy-source.sink#remote-13ea27915e7840d888c5e9451444b45d.source",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})

	entities, err := ExtractRelationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(entities, tc.DeepEquals, []RelationRemoteEntity{{
		RelationKey: relation.Key{{
			ApplicationName: "dummy-source",
			EndpointName:    "sink",
			Role:            deploymentcharm.RoleRequirer,
		}, {
			ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d",
			EndpointName:    "source",
			Role:            deploymentcharm.RoleProvider,
		}},
		RelationUUID: "6049aa01-76c9-462d-8440-964a6e26aac2",
	}})
}

// Only the relation tokens are of interest, the model description also records
// tokens for applications and offers.
func (s *relationRemoteEntitiesSuite) TestExtractRelationUUIDIgnoresOtherEntities(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-dummy-source.sink#remote-13ea27915e7840d888c5e9451444b45d.source",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "application-remote-13ea2791-5e78-40d8-88c5-e9451444b45d",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "applicationoffer-cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
		Token: "ec7383b4-7ca1-49de-85d3-1ce8d9cf3d6e",
	})

	entities, err := ExtractRelationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(entities, tc.DeepEquals, []RelationRemoteEntity{{
		RelationKey: relation.Key{{
			ApplicationName: "dummy-source",
			EndpointName:    "sink",
			Role:            deploymentcharm.RoleRequirer,
		}, {
			ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d",
			EndpointName:    "source",
			Role:            deploymentcharm.RoleProvider,
		}},
		RelationUUID: "6049aa01-76c9-462d-8440-964a6e26aac2",
	}})
}

func (s *relationRemoteEntitiesSuite) TestExtractRelationUUIDNoEntities(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})

	entities, err := ExtractRelationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entities, tc.HasLen, 0)
}

func (s *relationRemoteEntitiesSuite) TestExtractRelationUUIDBadKey(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-not-a-relation-key",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})

	_, err := ExtractRelationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorMatches, ".*parsing relation key from remote entity id.*")
}

func (s *relationRemoteEntitiesSuite) TestFindRelationUUID(c *tc.C) {
	entities := []RelationRemoteEntity{{
		RelationKey: relation.Key{
			{ApplicationName: "dummy-source", EndpointName: "sink"},
			{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source"},
		},
		RelationUUID: "6049aa01-76c9-462d-8440-964a6e26aac2",
	}}

	token, found := FindRelationUUID(entities, relation.Key{
		{ApplicationName: "dummy-source", EndpointName: "sink"},
		{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source"},
	})
	c.Check(found, tc.IsTrue)
	c.Check(token, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac2")
}

// The endpoints of a relation key are not guaranteed to be in any particular
// order, so the lookup must not depend on it.
func (s *relationRemoteEntitiesSuite) TestFindRelationUUIDIgnoresOrder(c *tc.C) {
	entities := []RelationRemoteEntity{{
		RelationKey: relation.Key{
			{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source"},
			{ApplicationName: "dummy-source", EndpointName: "sink"},
		},
		RelationUUID: "6049aa01-76c9-462d-8440-964a6e26aac2",
	}}

	for _, key := range []relation.Key{{
		{ApplicationName: "dummy-source", EndpointName: "sink"},
		{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source"},
	}, {
		{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source"},
		{ApplicationName: "dummy-source", EndpointName: "sink"},
	}} {
		token, found := FindRelationUUID(entities, key)
		c.Check(found, tc.IsTrue)
		c.Check(token, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac2")
	}
}

// A key that was never exported has no token, and that is reported to the
// caller rather than guessed at.
func (s *relationRemoteEntitiesSuite) TestFindRelationUUIDNotFound(c *tc.C) {
	entities := []RelationRemoteEntity{{
		RelationKey: relation.Key{
			{ApplicationName: "dummy-source", EndpointName: "sink"},
			{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source"},
		},
		RelationUUID: "6049aa01-76c9-462d-8440-964a6e26aac2",
	}}

	token, found := FindRelationUUID(entities, relation.Key{
		{ApplicationName: "mysql", EndpointName: "db"},
		{ApplicationName: "wordpress", EndpointName: "db"},
	})
	c.Check(found, tc.IsFalse)
	c.Check(token, tc.Equals, "")
}

// A peer relation key has a single endpoint, and the token lookup is only
// defined for the two endpoints of a cross model relation, so peer keys are
// never equal, not even to themselves.
func (s *relationRemoteEntitiesSuite) TestRelationKeysEqualPeerKeys(c *tc.C) {
	peer := relation.Key{{
		ApplicationName: "mysql", EndpointName: "cluster", Role: deploymentcharm.RolePeer,
	}}
	regular := relation.Key{
		{ApplicationName: "mysql", EndpointName: "db"},
		{ApplicationName: "wordpress", EndpointName: "db"},
	}

	c.Check(RelationKeysEqual(peer, peer), tc.IsFalse)
	c.Check(RelationKeysEqual(peer, regular), tc.IsFalse)
}

// Keys with more than two endpoints are never equal, rather than silently
// comparing only the first two endpoints.
func (s *relationRemoteEntitiesSuite) TestRelationKeysEqualTooManyEndpoints(c *tc.C) {
	key := relation.Key{
		{ApplicationName: "mysql", EndpointName: "db"},
		{ApplicationName: "wordpress", EndpointName: "db"},
		{ApplicationName: "mediawiki", EndpointName: "db"},
	}

	c.Check(RelationKeysEqual(key, key), tc.IsFalse)
}

// The comparison ignores the roles of the endpoints, not only their order:
// legacy positional exports get their roles assigned when the key is
// parsed, so a description key and the key of the relation found in the
// state can legitimately disagree on roles while referring to the same
// relation.
func (s *relationRemoteEntitiesSuite) TestRelationKeysEqualIgnoresRole(c *tc.C) {
	key := relation.Key{
		{ApplicationName: "dummy-source", EndpointName: "sink", Role: deploymentcharm.RoleRequirer},
		{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source", Role: deploymentcharm.RoleProvider},
	}
	rolesSwapped := relation.Key{
		{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source", Role: deploymentcharm.RoleRequirer},
		{ApplicationName: "dummy-source", EndpointName: "sink", Role: deploymentcharm.RoleProvider},
	}
	rolesZeroed := relation.Key{
		{ApplicationName: "dummy-source", EndpointName: "sink"},
		{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source"},
	}

	c.Check(RelationKeysEqual(key, rolesSwapped), tc.IsTrue)
	c.Check(RelationKeysEqual(key, rolesZeroed), tc.IsTrue)
}

// Remote entities that record the same relation key with divergent tokens
// are resolved first-match-wins.
func (s *relationRemoteEntitiesSuite) TestFindRelationUUIDDuplicateKeys(c *tc.C) {
	key := relation.Key{
		{ApplicationName: "mysql", EndpointName: "db"},
		{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "db"},
	}
	entities := []RelationRemoteEntity{
		{RelationKey: key, RelationUUID: "6049aa01-76c9-462d-8440-964a6e26aac2"},
		{RelationKey: key, RelationUUID: "ec7383b4-7ca1-49de-85d3-1ce8d9cf3d6e"},
	}

	token, found := FindRelationUUID(entities, key)

	c.Check(found, tc.IsTrue)
	c.Check(token, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac2")
}
