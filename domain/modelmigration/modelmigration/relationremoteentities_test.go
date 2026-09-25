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
