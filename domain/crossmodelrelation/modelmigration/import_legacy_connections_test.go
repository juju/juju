// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation/service"
)

// TestImportLegacyConsumerWithMultipleOfferConnections checks that the 3.6
// consumer proxy is not mistaken for a single connection. Its name identifies
// the consuming application, which can relate to several offered applications.
// Every exported relation must retain its own offer and remote relation token.
func (s *importSuite) TestImportLegacyConsumerWithMultipleOfferConnections(c *tc.C) {
	defer s.setupMocks(c).Finish()
	m := description.NewModel(description.ModelArgs{})
	const app = "remote-13ea27915e7840d888c5e9451444b45d"
	const modelUUID = "4ddd6454-931d-4278-8779-b0b7208994d9"
	a := m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: app, SourceModelUUID: modelUUID, IsConsumerProxy: true,
	})
	a.AddEndpoint(description.RemoteEndpointArgs{Name: "db", Role: "requirer", Interface: "db"})
	m.AddRemoteEntity(description.RemoteEntityArgs{
		ID: "application-" + app, Token: "13ea2791-5e78-40d8-88c5-e9451444b45d",
	})
	var expected []service.RemoteApplicationConsumerImport
	for n, local := range []string{"mysql", "postgres"} {
		key := app + ":db " + local + ":db"
		relationKey, err := relation.NewKeyFromString(key)
		c.Assert(err, tc.ErrorIsNil)
		expected = append(expected, service.RemoteApplicationConsumerImport{
			RemoteApplicationImport: service.RemoteApplicationImport{
				Name: app, Units: []string{}, OfferUUID: fmt.Sprintf("cfa46843-ebf2-4fff-8519-c1fb5a9816f%d", n),
			},
			RelationUUID: fmt.Sprintf("6049aa01-76c9-462d-8440-964a6e26aac%d", n),
			RelationKey:  relationKey, ConsumerModelUUID: modelUUID,
			RelationID:              n + 41,
			RelationScope:           charm.ScopeGlobal,
			ConsumerApplicationUUID: "13ea2791-5e78-40d8-88c5-e9451444b45d", UserName: "admin",
		})
		m.AddOfferConnection(description.OfferConnectionArgs{
			OfferUUID:  fmt.Sprintf("cfa46843-ebf2-4fff-8519-c1fb5a9816f%d", n),
			RelationID: n + 41, RelationKey: key, SourceModelUUID: modelUUID, UserName: "admin",
		})
		m.AddRemoteEntity(description.RemoteEntityArgs{
			ID:    "relation-" + app + ".db#" + local + ".db",
			Token: fmt.Sprintf("6049aa01-76c9-462d-8440-964a6e26aac%d", n),
		})
		r := m.AddRelation(description.RelationArgs{Id: n + 41, Key: key})
		r.AddEndpoint(description.EndpointArgs{ApplicationName: local, Name: "db", Role: "provider", Interface: "db"})
		r.AddEndpoint(description.EndpointArgs{ApplicationName: app, Name: "db", Role: "requirer", Interface: "db"})
	}
	s.importService.EXPECT().ImportRemoteApplicationConsumers(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, imports []service.RemoteApplicationConsumerImport) error {
			// Endpoint conversion is covered separately; compare the identities
			// and ownership of every connection without relying on order.
			for n := range imports {
				imports[n].Endpoints = nil
			}
			c.Check(imports, tc.SameContents, expected)
			return nil
		})
	err := s.newImportOperation(c).Execute(c.Context(), m)
	c.Assert(err, tc.ErrorIsNil)
}
