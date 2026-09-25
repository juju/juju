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
	"github.com/juju/juju/domain/crossmodelrelation"
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

// TestImportLegacyConsumerWithMultipleOfferConnectionsDifferentEndpoints
// checks that the synthetic charm of every connection is built from the
// relation's own consumer endpoint. A 3.6 proxy only holds the endpoint of
// its first relation, so an additional connection referencing a different
// consuming endpoint must still import with a matching synthetic endpoint.
func (s *importSuite) TestImportLegacyConsumerWithMultipleOfferConnectionsDifferentEndpoints(c *tc.C) {
	defer s.setupMocks(c).Finish()
	m := description.NewModel(description.ModelArgs{})
	const app = "remote-13ea27915e7840d888c5e9451444b45d"
	const modelUUID = "4ddd6454-931d-4278-8779-b0b7208994d9"
	a := m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: app, SourceModelUUID: modelUUID, IsConsumerProxy: true,
	})
	// The proxy only holds the endpoint of its first relation.
	a.AddEndpoint(description.RemoteEndpointArgs{Name: "db", Role: "requirer", Interface: "db"})
	m.AddRemoteEntity(description.RemoteEntityArgs{
		ID: "application-" + app, Token: "13ea2791-5e78-40d8-88c5-e9451444b45d",
	})
	var expected []service.RemoteApplicationConsumerImport
	for n, local := range []string{"mysql", "postgres"} {
		// The second connection uses a consuming endpoint that the proxy
		// does not hold.
		endpointName := "db"
		if n == 1 {
			endpointName = "metrics"
		}
		key := app + ":" + endpointName + " " + local + ":db"
		relationKey, err := relation.NewKeyFromString(key)
		c.Assert(err, tc.ErrorIsNil)
		expected = append(expected, service.RemoteApplicationConsumerImport{
			RemoteApplicationImport: service.RemoteApplicationImport{
				Name: app, Units: []string{},
				OfferUUID: fmt.Sprintf("cfa46843-ebf2-4fff-8519-c1fb5a9816f%d", n),
				Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{{
					Name:      endpointName,
					Role:      charm.RoleRequirer,
					Interface: "db",
				}},
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
			ID:    fmt.Sprintf("relation-%s.%s#%s.db", app, endpointName, local),
			Token: fmt.Sprintf("6049aa01-76c9-462d-8440-964a6e26aac%d", n),
		})
		r := m.AddRelation(description.RelationArgs{Id: n + 41, Key: key})
		r.AddEndpoint(description.EndpointArgs{ApplicationName: local, Name: "db", Role: "provider", Interface: "db"})
		r.AddEndpoint(description.EndpointArgs{ApplicationName: app, Name: endpointName, Role: "requirer", Interface: "db"})
	}
	var got []service.RemoteApplicationConsumerImport
	s.importService.EXPECT().ImportRemoteApplicationConsumers(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, imports []service.RemoteApplicationConsumerImport) error {
			got = imports
			return nil
		})
	err := s.newImportOperation(c).Execute(c.Context(), m)
	c.Assert(err, tc.ErrorIsNil)
	// The consumer endpoint of every connection comes from the relation
	// itself, not from the proxy's stored endpoints.
	c.Assert(got, tc.DeepEquals, expected)
}

// TestImportLegacyConsumerWithMultipleOfferConnectionsRelationIDMismatch
// checks that the consistency of every offer connection is validated, not
// just the first one: the second connection of the proxy records a
// relation ID that does not match its relation, so the import fails
// instead of importing the mismatched identity.
func (s *importSuite) TestImportLegacyConsumerWithMultipleOfferConnectionsRelationIDMismatch(c *tc.C) {
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
	for n, local := range []string{"mysql", "postgres"} {
		key := app + ":db " + local + ":db"
		// The second offer connection records relation ID 99, while the
		// relation it points at has ID 42.
		connRelationID := n + 41
		if n == 1 {
			connRelationID = 99
		}
		m.AddOfferConnection(description.OfferConnectionArgs{
			OfferUUID:  fmt.Sprintf("cfa46843-ebf2-4fff-8519-c1fb5a9816f%d", n),
			RelationID: connRelationID, RelationKey: key, SourceModelUUID: modelUUID, UserName: "admin",
		})
		m.AddRemoteEntity(description.RemoteEntityArgs{
			ID:    "relation-" + app + ".db#" + local + ".db",
			Token: fmt.Sprintf("6049aa01-76c9-462d-8440-964a6e26aac%d", n),
		})
		r := m.AddRelation(description.RelationArgs{Id: n + 41, Key: key})
		r.AddEndpoint(description.EndpointArgs{ApplicationName: local, Name: "db", Role: "provider", Interface: "db"})
		r.AddEndpoint(description.EndpointArgs{ApplicationName: app, Name: "db", Role: "requirer", Interface: "db"})
	}
	err := s.newImportOperation(c).Execute(c.Context(), m)
	c.Assert(err, tc.ErrorMatches,
		`.*offer connection relation ID 99 does not match relation ID 42 for relation "remote-13ea27915e7840d888c5e9451444b45d:db postgres:db".*`)
}
