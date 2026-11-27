// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	coreapplication "github.com/juju/juju/core/application"
	coremodel "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/export"
	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_0_12"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/domain/modelimport"
	migrationdomain "github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	relationstate "github.com/juju/juju/domain/relation/state"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *controllerImportSuite) TestActivateModelReconcilesLegacySettingsSequences(c *tc.C) {
	const (
		unitHash                 = "8b8e46eb11d5b6f7ef30b1d361f43e995d055d9a8330dc188f6de7a59d844f60"
		applicationHash          = "d1372b6ad7fd7319aeb75df4ef4a36355fb0ec308efd1e5582344c6b6b9c463a"
		unitVersion        int64 = 7090132787029107256
		applicationVersion int64 = 7004894238235373924
	)
	modelUUID := s.importForActivation(c, "1.0.0")
	modelFactory := s.modelFactory(c, modelUUID.String())
	legacy := v4_0_12.ModelExport{
		ModelAgent: []v4_0_12.ModelAgent{{ModelUUID: modelUUID.String()}},
		Charm: []v4_0_12.Charm{{
			UUID: "charm", SourceID: 1, Revision: 1, ReferenceName: "test-charm",
			CreateTime: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		}},
		CharmRelation: []v4_0_12.CharmRelation{{
			UUID: "charm-relation", CharmUUID: "charm", Name: "peer", RoleID: 2,
		}},
		Application: []v4_0_12.Application{{
			UUID: "app", Name: "test-app", CharmUUID: "charm",
			SpaceUUID: "656b4a82-e28c-53d6-a014-f0dd53417eb6",
		}},
		ApplicationEndpoint: []v4_0_12.ApplicationEndpoint{{
			UUID: "app-endpoint", ApplicationUUID: "app", CharmRelationUUID: "charm-relation",
		}},
		NetNode: []v4_0_12.NetNode{{UUID: "node"}},
		Unit: []v4_0_12.Unit{{
			UUID: "unit", Name: "test-app/0", ApplicationUUID: "app", NetNodeUUID: "node", CharmUUID: "charm",
		}},
		Relation: []v4_0_12.Relation{{UUID: "relation", RelationID: 1}},
		RelationEndpoint: []v4_0_12.RelationEndpoint{{
			UUID: "endpoint", RelationUUID: "relation", EndpointUUID: "app-endpoint",
		}},
		RelationUnit: []v4_0_12.RelationUnit{{
			UUID: "relation-unit", RelationEndpointUUID: "endpoint", UnitUUID: "unit",
		}},
		RelationApplicationSetting: []v4_0_12.RelationApplicationSetting{{
			RelationEndpointUUID: "endpoint", Key: "token", Value: new("B"),
		}},
		RelationApplicationSettingsHash: []v4_0_12.RelationApplicationSettingsHash{{
			RelationEndpointUUID: "endpoint", Sha256: applicationHash,
		}},
		RelationUnitSetting: []v4_0_12.RelationUnitSetting{{
			RelationUnitUUID: "relation-unit", Key: "token", Value: new("A"),
		}},
		RelationUnitSettingsHash: []v4_0_12.RelationUnitSettingsHash{{
			RelationUnitUUID: "relation-unit", Sha256: unitHash,
		}},
	}
	data, err := yaml.Marshal(legacy)
	c.Assert(err, tc.ErrorIsNil)
	payload, err := export.DecodePayload(semversion.MustParse("4.0.12"), data)
	c.Assert(err, tc.ErrorIsNil)
	transformer, err := modelimport.NewTransformer()
	c.Assert(err, tc.ErrorIsNil)
	transformed, err := transformer.Transform(c.Context(), semversion.MustParse("4.0.12"), payload)
	c.Assert(err, tc.ErrorIsNil)
	latestPayload, ok := transformed.(latest.ModelExport)
	c.Assert(ok, tc.IsTrue)
	err = modelimport.NewImporter(modelFactory).Import(c.Context(), &latestPayload)
	c.Assert(err, tc.ErrorIsNil)

	exportSt := exportstate.NewState(modelFactory)
	before, err := exportSt.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(before.Sequence, tc.HasLen, 0)

	for range 2 {
		err = s.activateModel(c, modelUUID, ActivateModelArgs{ModelUUID: modelUUID})
		c.Assert(err, tc.ErrorIsNil)
	}
	prepared, err := exportSt.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(prepared.Sequence, tc.DeepEquals, []v4_1_0.Sequence{{
		Namespace: "relation-settings_relation", Value: unitVersion,
	}})
	c.Check(prepared.RelationUnitSettingsHash, tc.DeepEquals, before.RelationUnitSettingsHash)
	c.Check(prepared.RelationApplicationSettingsHash, tc.DeepEquals, before.RelationApplicationSettingsHash)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsTrue)
	claim, err := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock).GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseImporting)
	commitErr, _ := s.commitActivation(c, modelUUID)
	c.Assert(commitErr, tc.ErrorIsNil)

	relationSt := relationstate.NewState(modelFactory, clock.WallClock, loggertesting.WrapCheckLog(c), nil)
	checkVersions := func(unitRevision, applicationRevision int64) {
		changes, err := relationSt.GetRelationUnitChanges(c.Context(), "relation",
			[]coreunit.UUID{"unit"}, []coreapplication.UUID{"app"})
		c.Assert(err, tc.ErrorIsNil)
		c.Check(changes.Changed, tc.DeepEquals, map[coreunit.Name]int64{"test-app/0": unitRevision})
		c.Check(changes.AppChanged, tc.DeepEquals, map[string]int64{"test-app": applicationRevision})
		consumer, err := relationSt.GetConsumerRelationUnitsChange(c.Context(), "relation", "app")
		c.Assert(err, tc.ErrorIsNil)
		c.Check(consumer.UnitsSettingsVersions, tc.DeepEquals, map[string]int64{"test-app/0": unitRevision})
		c.Check(consumer.AppSettingsVersion, tc.DeepEquals, map[string]int64{"test-app": applicationRevision})
	}
	checkVersions(unitVersion, applicationVersion)

	// Writing unchanged imported content must retain the legacy versions.
	err = relationSt.SetRelationUnitSettings(c.Context(), corerelation.UnitUUID("relation-unit"), map[string]string{"token": "A"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	err = relationSt.SetRelationApplicationSettings(c.Context(), corerelation.UUID("relation"), coreapplication.UUID("app"), map[string]string{"token": "B"})
	c.Assert(err, tc.ErrorIsNil)
	checkVersions(unitVersion, applicationVersion)

	err = relationSt.SetRelationUnitSettings(c.Context(), corerelation.UnitUUID("relation-unit"), map[string]string{"token": "B"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	checkVersions(unitVersion+1, applicationVersion)
	err = relationSt.SetRelationApplicationSettings(c.Context(), corerelation.UUID("relation"), coreapplication.UUID("app"), map[string]string{"token": "A"})
	c.Assert(err, tc.ErrorIsNil)
	checkVersions(unitVersion+1, unitVersion+2)
	err = relationSt.SetRelationUnitSettings(c.Context(), corerelation.UnitUUID("relation-unit"), map[string]string{"token": "A"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	checkVersions(unitVersion+3, unitVersion+2)

	exported, err := exportSt.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exported.RelationUnitSettingsHash, tc.DeepEquals, []v4_1_0.RelationUnitSettingsHash{{
		RelationUnitUUID: "relation-unit", Sha256: unitHash, Revision: unitVersion + 3,
	}})
	c.Check(exported.Sequence, tc.DeepEquals, []v4_1_0.Sequence{{
		Namespace: "relation-settings_relation", Value: unitVersion + 3,
	}})
}

func (s *controllerImportSuite) TestActivateModelSettingsSequenceFailureLeavesModelAbortable(c *tc.C) {
	modelUUID := s.importForActivation(c, "1.0.0")
	s.addActivationSettings(c, modelUUID)
	runner := s.ModelTxnRunner(c, modelUUID.String())
	err := runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
CREATE TRIGGER reject_settings_sequence BEFORE INSERT ON sequence
WHEN NEW.namespace = 'relation-settings_relation'
BEGIN SELECT RAISE(ABORT, 'sequence seeding rejected'); END;
`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.activateModel(c, modelUUID, ActivateModelArgs{ModelUUID: modelUUID})
	c.Check(err, tc.ErrorMatches, `.*reconciling relation settings sequences.*sequence seeding rejected.*`)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsTrue)
	c.Check(s.modelActivated(c, modelUUID), tc.IsFalse)
	claim, err := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock).GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseImporting)

	// Preparation can be replayed after the failed write is resolved.
	failed, err := exportstate.NewState(s.modelFactory(c, modelUUID.String())).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(failed.Sequence, tc.HasLen, 0)
	err = runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DROP TRIGGER reject_settings_sequence")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.activateModel(c, modelUUID, ActivateModelArgs{ModelUUID: modelUUID})
	c.Assert(err, tc.ErrorIsNil)
	got, err := exportstate.NewState(s.modelFactory(c, modelUUID.String())).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Sequence, tc.DeepEquals, []v4_1_0.Sequence{{
		Namespace: "relation-settings_relation", Value: 10000,
	}})
}

func (s *controllerImportSuite) TestActivateModelValidatesBeforeSettingsSequenceReconciliation(c *tc.C) {
	modelUUID := s.importForActivation(c, "1.0.0")
	s.addActivationSettings(c, modelUUID)
	err := s.ModelTxnRunner(c, modelUUID.String()).StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO net_node (uuid) VALUES ('node');
INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid)
VALUES ('unit', 'test-app/0', 0, 'app', 'node', 'charm');
`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.activateModel(c, modelUUID, ActivateModelArgs{ModelUUID: modelUUID})
	c.Check(err, tc.ErrorMatches, `.*validating imported model.*unit test-app/0 hasn't joined relation.*`)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsTrue)
	c.Check(s.modelAgentVersion(c, modelUUID), tc.Equals, "1.0.0")
	got, err := exportstate.NewState(s.modelFactory(c, modelUUID.String())).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Sequence, tc.HasLen, 0)
}

func (s *controllerImportSuite) TestActivateModelRetriesAfterSettingsSequencesCommitted(c *tc.C) {
	modelUUID := s.importForActivation(c, "invalid-version")
	s.addActivationSettings(c, modelUUID)

	err := s.activateModel(c, modelUUID, ActivateModelArgs{ModelUUID: modelUUID})
	c.Check(err, tc.ErrorMatches, `.*parsing model target agent version.*`)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsTrue)
	claim, err := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock).GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseImporting)
	exportSt := exportstate.NewState(s.modelFactory(c, modelUUID.String()))
	before, err := exportSt.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(before.Sequence, tc.DeepEquals, []v4_1_0.Sequence{{
		Namespace: "relation-settings_relation", Value: 10000,
	}})

	err = s.ModelTxnRunner(c, modelUUID.String()).StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE agent_version SET target_version = '1.0.0', latest_version = '1.0.0'")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.activateModel(c, modelUUID, ActivateModelArgs{ModelUUID: modelUUID})
	c.Assert(err, tc.ErrorIsNil)
	after, err := exportSt.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(after.Sequence, tc.DeepEquals, before.Sequence)

	// Seeded counters do not take away the source's ability to abort.
	domainServices, err := s.activationGetter(c, modelUUID, nil).ServicesForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	err = domainServices.ModelMigration().SetImportPhaseAborting(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	err = s.activateModel(c, modelUUID, ActivateModelArgs{ModelUUID: modelUUID})
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrActivationAborting)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsTrue)
}

func (s *controllerImportSuite) addActivationSettings(c *tc.C, modelUUID coremodel.UUID) {
	err := s.ModelTxnRunner(c, modelUUID.String()).StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name) VALUES ('charm', 'test-charm');
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id)
VALUES ('charm-relation', 'charm', 'peer', 2, 0);
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
VALUES ('app', 'test-app', 0, 'charm', '656b4a82-e28c-53d6-a014-f0dd53417eb6');
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid)
VALUES ('app-endpoint', 'app', 'charm-relation');
INSERT INTO relation (uuid, relation_id, life_id, scope_id)
VALUES ('relation', 1, 0, 0);
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES ('endpoint', 'relation', 'app-endpoint');
INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256, revision)
VALUES ('endpoint', 'application-hash', 10000);
`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}
