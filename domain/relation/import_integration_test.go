// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v11"
	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	corerelation "github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	applicationmodelmigration "github.com/juju/juju/domain/application/modelmigration"
	applicationstate "github.com/juju/juju/domain/application/state"
	migrationtesting "github.com/juju/juju/domain/modelmigration/testing"
	relationmodelmigration "github.com/juju/juju/domain/relation/modelmigration"
	"github.com/juju/juju/domain/relation/service"
	"github.com/juju/juju/domain/relation/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	schematesting.ModelSuite
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportRelation(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	// Add applications first as they are needed for relations.
	s.addApplicationWithRelation(c, desc, "app1", charmMetadataRelations{
		Requires: map[string]description.CharmMetadataRelation{
			"endpoint1": migrationtesting.Relation{
				Name_:          "endpoint1",
				Role_:          "requirer",
				InterfaceName_: "interface1",
				Scope_:         "global",
			},
		},
	})
	s.addApplicationWithRelation(c, desc, "app2", charmMetadataRelations{
		Provides: map[string]description.CharmMetadataRelation{
			"endpoint2": migrationtesting.Relation{
				Name_:          "endpoint2",
				Role_:          "provider",
				InterfaceName_: "interface1",
				Scope_:         "global",
			},
		},
	})

	relKey := relationtesting.GenNewKey(c, "app1:endpoint1 app2:endpoint2")
	rel := desc.AddRelation(description.RelationArgs{
		Id:  1,
		Key: relKey.String(),
	})

	for _, ep := range relKey.EndpointIdentifiers() {
		rel.AddEndpoint(description.EndpointArgs{
			ApplicationName: ep.ApplicationName,
			Name:            ep.EndpointName,
			Role:            string(ep.Role),
		})
	}

	s.doImport(c, desc)

	svc := s.setupService(c)
	relUUID, err := svc.GetRelationUUIDByKey(c.Context(), relKey)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relUUID, tc.Not(tc.DeepEquals), corerelation.UUID(""))

	details, err := svc.GetRelationDetails(c.Context(), relUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(details.Key, tc.DeepEquals, relKey)
}

func (s *importSuite) TestImportPeerRelation(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	s.addApplicationWithRelation(c, desc, "app1", charmMetadataRelations{
		Peers: map[string]description.CharmMetadataRelation{
			"peer": migrationtesting.Relation{
				Name_:          "peer",
				Role_:          "peer",
				InterfaceName_: "interface1",
				Scope_:         "global",
			},
		},
	})

	relKey := relationtesting.GenNewKey(c, "app1:peer")
	rel := desc.AddRelation(description.RelationArgs{
		Id:  1,
		Key: relKey.String(),
	})

	for _, ep := range relKey.EndpointIdentifiers() {
		rel.AddEndpoint(description.EndpointArgs{
			ApplicationName: ep.ApplicationName,
			Name:            ep.EndpointName,
			Role:            string(ep.Role),
		})
	}

	s.doImport(c, desc)

	svc := s.setupService(c)
	relUUID, err := svc.GetRelationUUIDByKey(c.Context(), relKey)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relUUID, tc.Not(tc.DeepEquals), corerelation.UUID(""))
	details, err := svc.GetRelationDetails(c.Context(), relUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(details.Key, tc.DeepEquals, relKey)
}

func (s *importSuite) TestImportRelationWithContainerScope(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	s.addApplicationWithRelation(c, desc, "app1", charmMetadataRelations{
		Provides: map[string]description.CharmMetadataRelation{
			"endpoint1": migrationtesting.Relation{
				Name_:          "endpoint1",
				Role_:          "provider",
				InterfaceName_: "interface1",
				Scope_:         "container",
			},
		},
	})
	s.addApplicationWithRelation(c, desc, "app2", charmMetadataRelations{
		Requires: map[string]description.CharmMetadataRelation{
			"endpoint2": migrationtesting.Relation{
				Name_:          "endpoint2",
				Role_:          "requirer",
				InterfaceName_: "interface1",
				Scope_:         "global",
			},
		},
	})

	relKey := relationtesting.GenNewKey(c, "app1:endpoint1 app2:endpoint2")
	rel := desc.AddRelation(description.RelationArgs{
		Id:  1,
		Key: relKey.String(),
	})

	for _, ep := range relKey.EndpointIdentifiers() {
		rel.AddEndpoint(description.EndpointArgs{
			ApplicationName: ep.ApplicationName,
			Name:            ep.EndpointName,
			Role:            string(ep.Role),
			Scope:           string(charm.ScopeContainer),
		})
	}

	s.doImport(c, desc)

	svc := s.setupService(c)
	relUUID, err := svc.GetRelationUUIDByKey(c.Context(), relKey)
	c.Assert(err, tc.ErrorIsNil)

	// Check the scope using the state layer directly
	st := s.setupState(c)
	app1UUID, err := st.GetApplicationUUIDByName(c.Context(), "app1")
	c.Assert(err, tc.ErrorIsNil)

	obtainedScope, err := st.GetRelationEndpointScope(c.Context(), relUUID, app1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedScope, tc.Equals, charm.ScopeContainer)
}

func (s *importSuite) TestImportRelationWithSettings(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	s.addApplicationWithRelation(c, desc, "app1", charmMetadataRelations{
		Requires: map[string]description.CharmMetadataRelation{
			"endpoint1": migrationtesting.Relation{
				Name_:          "endpoint1",
				Role_:          "requirer",
				InterfaceName_: "interface1",
				Scope_:         "global",
			},
		},
	})
	s.addApplicationWithRelation(c, desc, "app2", charmMetadataRelations{
		Provides: map[string]description.CharmMetadataRelation{
			"endpoint2": migrationtesting.Relation{
				Name_:          "endpoint2",
				Role_:          "provider",
				InterfaceName_: "interface1",
				Scope_:         "global",
			},
		},
	})

	relKey := relationtesting.GenNewKey(c, "app1:endpoint1 app2:endpoint2")
	rel := desc.AddRelation(description.RelationArgs{
		Id:  1,
		Key: relKey.String(),
	})

	for _, ep := range relKey.EndpointIdentifiers() {
		dep := rel.AddEndpoint(description.EndpointArgs{
			ApplicationName: ep.ApplicationName,
			Name:            ep.EndpointName,
			Role:            string(ep.Role),
		})
		dep.SetApplicationSettings(map[string]any{
			"app-key": "app-value-" + ep.ApplicationName,
		})
	}

	s.doImport(c, desc)

	svc := s.setupService(c)
	relUUID, err := svc.GetRelationUUIDByKey(c.Context(), relKey)
	c.Assert(err, tc.ErrorIsNil)

	st := s.setupState(c)
	app1UUID, err := st.GetApplicationUUIDByName(c.Context(), "app1")
	c.Assert(err, tc.ErrorIsNil)
	app2UUID, err := st.GetApplicationUUIDByName(c.Context(), "app2")
	c.Assert(err, tc.ErrorIsNil)

	settings1, err := svc.GetRelationApplicationSettings(c.Context(), relUUID, app1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(settings1, tc.DeepEquals, map[string]string{"app-key": "app-value-app1"})

	settings2, err := svc.GetRelationApplicationSettings(c.Context(), relUUID, app2UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(settings2, tc.DeepEquals, map[string]string{"app-key": "app-value-app2"})
}

func (s *importSuite) TestImportNoRelations(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	s.doImport(c, desc)

	svc := s.setupService(c)
	details, err := svc.GetAllRelationDetails(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(details), tc.Equals, 0)
}

func (s *importSuite) setupService(c *tc.C) *service.Service {
	st := s.setupState(c)
	return service.NewService(st, nil, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) setupState(c *tc.C) *state.State {
	modelDB := func(context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	unitState := applicationstate.NewInsertIAASUnitState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))
	return state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c), unitState)
}

// charmMetadataRelations represents relations defined in charm metadata.
type charmMetadataRelations struct {
	Requires map[string]description.CharmMetadataRelation
	Provides map[string]description.CharmMetadataRelation
	Peers    map[string]description.CharmMetadataRelation
}

// addApplicationWithRelation is a helper to add an application with the given
// relations to the model.
func (s *importSuite) addApplicationWithRelation(
	c *tc.C,
	desc description.Model,
	appName string,
	relations charmMetadataRelations,
) description.Application {
	app := desc.AddApplication(description.ApplicationArgs{
		Name:     appName,
		CharmURL: "ch:" + appName + "-1",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name:     appName,
		Requires: relations.Requires,
		Provides: relations.Provides,
		Peers:    relations.Peers,
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:    "ubuntu",
				Channel_: "stable",
			},
		},
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "id-" + appName,
		Hash:     "hash-" + appName,
		Revision: 1,
		Platform: "amd64/ubuntu/20.04",
	})
	return app
}

func (s *importSuite) doImport(c *tc.C, desc description.Model) {
	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	// We need to register both application and relation imports.
	applicationmodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	relationmodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	err := coordinator.Perform(c.Context(), modelmigration.NewScope(nil, s.TxnRunnerFactory(),
		nil, "deadbeef", model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)
}
