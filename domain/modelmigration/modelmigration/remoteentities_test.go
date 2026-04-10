// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type remoteEntities struct {
	testhelpers.IsolationSuite
}

func TestRemoteEntitiesSuite(t *testing.T) {
	tc.Run(t, &remoteEntities{})
}

func (s *remoteEntities) TestExtractApplicationUUIDFromRemoteEntities(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "application-remote-13ea2791-5e78-40d8-88c5-e9451444b45d",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})

	entities, err := ExtractApplicationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(entities, tc.DeepEquals, map[string]string{
		"remote-13ea2791-5e78-40d8-88c5-e9451444b45d": "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
}

func (s *remoteEntities) TestExtractApplicationUUIDFromRemoteEntitiesWithRelationEntities(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-dummy-source.sink#remote-13ea27915e7840d888c5e9451444b45d.source",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "application-remote-13ea2791-5e78-40d8-88c5-e9451444b45d",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})

	entities, err := ExtractApplicationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(entities, tc.DeepEquals, map[string]string{
		"remote-13ea2791-5e78-40d8-88c5-e9451444b45d": "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
}

func (s *remoteEntities) TestExtractApplicationUUIDFromRemoteEntitiesNoEntities(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})

	entities, err := ExtractApplicationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entities, tc.HasLen, 0)
}
