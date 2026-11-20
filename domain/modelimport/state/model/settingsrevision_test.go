// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"encoding/json"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/export"
	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	importstate "github.com/juju/juju/domain/modelimport/state/model"
	relationstate "github.com/juju/juju/domain/relation/state"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *roundTripSuite) TestRelationSettingsRevisionsRoundTrip(c *tc.C) {
	s.bootstrapModel(c)
	payload := relationSettingsRevisionPayload()
	data, err := yaml.Marshal(payload)
	c.Assert(err, tc.ErrorIsNil)
	decoded, err := export.DecodePayload(semversion.MustParse("4.1.0"), data)
	c.Assert(err, tc.ErrorIsNil)
	decodedPayload, ok := decoded.(v4_1_0.ModelExport)
	c.Assert(ok, tc.IsTrue)
	err = importstate.NewState(s.TxnRunnerFactory()).Import(c.Context(), &decodedPayload)
	c.Assert(err, tc.ErrorIsNil)
	got, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.RelationApplicationSettingsHash, tc.DeepEquals, payload.RelationApplicationSettingsHash)
	c.Check(got.RelationUnitSettingsHash, tc.DeepEquals, payload.RelationUnitSettingsHash)
	c.Check(got.Sequence, tc.SameContents, payload.Sequence)

	for _, marshal := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal} {
		data, err = marshal(got)
		c.Assert(err, tc.ErrorIsNil)
		decoded, err = export.DecodePayload(semversion.MustParse("4.1.0"), data)
		c.Assert(err, tc.ErrorIsNil)
		reexported, ok := decoded.(v4_1_0.ModelExport)
		c.Assert(ok, tc.IsTrue)
		c.Check(reexported.RelationApplicationSettingsHash, tc.DeepEquals, payload.RelationApplicationSettingsHash)
		c.Check(reexported.RelationUnitSettingsHash, tc.DeepEquals, payload.RelationUnitSettingsHash)
		c.Check(reexported.Sequence, tc.SameContents, payload.Sequence)
	}

	// Deleted bags may have used revisions higher than any surviving bag.
	// The source's high-water mark, rather than MAX(current bags), must win.
	s.changeImportedUnitHash(c)
	got, err = exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.RelationUnitSettingsHash[0].Revision, tc.Equals, int64(100))
}

func (s *roundTripSuite) TestInitialiseLegacySettingsRevisions(c *tc.C) {
	s.bootstrapModel(c)
	payload := relationSettingsRevisionPayload()
	payload.RelationApplicationSettingsHash[0].Revision = 0
	payload.RelationUnitSettingsHash[0].Revision = 0
	payload.Sequence = nil
	st := importstate.NewState(s.TxnRunnerFactory())
	err := st.Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InitialiseRelationSettingsRevisions(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	got, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.RelationApplicationSettingsHash[0].Revision > 0, tc.IsTrue)
	c.Check(got.RelationUnitSettingsHash[0].Revision > 0, tc.IsTrue)
	c.Check(got.RelationApplicationSettingsHash[0].Revision != got.RelationUnitSettingsHash[0].Revision, tc.IsTrue)
	c.Check(got.RelationApplicationSettingsHash[0].Sha256, tc.Equals, payload.RelationApplicationSettingsHash[0].Sha256)
	c.Check(got.RelationUnitSettingsHash[0].Sha256, tc.Equals, payload.RelationUnitSettingsHash[0].Sha256)
	c.Check(got.Sequence, tc.DeepEquals, []v4_1_0.Sequence{{Namespace: "relation-settings_relation-uuid", Value: 2}})

	err = st.InitialiseRelationSettingsRevisions(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	retried, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retried.RelationApplicationSettingsHash, tc.DeepEquals, got.RelationApplicationSettingsHash)
	c.Check(retried.RelationUnitSettingsHash, tc.DeepEquals, got.RelationUnitSettingsHash)
	c.Check(retried.Sequence, tc.DeepEquals, got.Sequence)
}

func (s *roundTripSuite) TestImportedRevisionsNeedNotBeOrdered(c *tc.C) {
	s.bootstrapModel(c)
	payload := relationSettingsRevisionPayload()
	payload.RelationUnitSettingsHash[0].Revision = 5
	err := importstate.NewState(s.TxnRunnerFactory()).Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIsNil)
	got, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.RelationUnitSettingsHash, tc.DeepEquals, payload.RelationUnitSettingsHash)
	c.Check(got.RelationApplicationSettingsHash, tc.DeepEquals, payload.RelationApplicationSettingsHash)
	c.Check(got.Sequence, tc.SameContents, payload.Sequence)
	s.changeImportedUnitHash(c)
	got, err = exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.RelationUnitSettingsHash[0].Revision, tc.Equals, int64(100))
}

func (s *roundTripSuite) TestInitialiseLegacySettingsUsesPerRelationSequences(c *tc.C) {
	s.bootstrapModel(c)
	payload := relationSettingsRevisionPayload()
	payload.Sequence = nil
	payload.RelationApplicationSettingsHash[0].Revision = 0
	payload.RelationUnitSettingsHash[0].Revision = 0
	payload.Relation = append(payload.Relation, v4_1_0.Relation{
		UUID: "other-relation-uuid", RelationID: 1,
	})
	payload.RelationEndpoint = append(payload.RelationEndpoint, v4_1_0.RelationEndpoint{
		UUID: "other-endpoint-uuid", RelationUUID: "other-relation-uuid", EndpointUUID: "application-endpoint-uuid",
	})
	payload.RelationUnit = append(payload.RelationUnit, v4_1_0.RelationUnit{
		UUID: "other-relation-unit-uuid", RelationEndpointUUID: "other-endpoint-uuid", UnitUUID: "unit-uuid",
	})
	payload.RelationApplicationSettingsHash = append(payload.RelationApplicationSettingsHash, v4_1_0.RelationApplicationSettingsHash{
		RelationEndpointUUID: "other-endpoint-uuid", Sha256: "other-application-hash",
	})
	payload.RelationUnitSettingsHash = append(payload.RelationUnitSettingsHash, v4_1_0.RelationUnitSettingsHash{
		RelationUnitUUID: "other-relation-unit-uuid", Sha256: "other-unit-hash",
	})
	st := importstate.NewState(s.TxnRunnerFactory())
	err := st.Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InitialiseRelationSettingsRevisions(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	got, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.RelationApplicationSettingsHash, tc.HasLen, 2)
	c.Check(got.RelationUnitSettingsHash, tc.HasLen, 2)
	for _, bag := range got.RelationApplicationSettingsHash {
		c.Check(bag.Revision, tc.Equals, int64(1))
	}
	for _, bag := range got.RelationUnitSettingsHash {
		c.Check(bag.Revision, tc.Equals, int64(2))
	}
	c.Check(got.Sequence, tc.SameContents, []v4_1_0.Sequence{
		{Namespace: "relation-settings_relation-uuid", Value: 2},
		{Namespace: "relation-settings_other-relation-uuid", Value: 2},
	})
}

func (s *roundTripSuite) TestInitialiseLegacySettingsRollsBackAndRetries(c *tc.C) {
	s.bootstrapModel(c)
	payload := relationSettingsRevisionPayload()
	payload.Sequence = nil
	payload.RelationApplicationSettingsHash[0].Revision = 0
	payload.RelationUnitSettingsHash[0].Revision = 0
	st := importstate.NewState(s.TxnRunnerFactory())
	err := st.Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
CREATE TRIGGER fail_unit_settings_revision
BEFORE UPDATE OF revision ON relation_unit_settings_hash
BEGIN
    SELECT RAISE(ABORT, 'revision update failed');
END;
`)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InitialiseRelationSettingsRevisions(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*revision update failed.*")
	got, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.RelationApplicationSettingsHash, tc.DeepEquals, payload.RelationApplicationSettingsHash)
	c.Check(got.RelationUnitSettingsHash, tc.DeepEquals, payload.RelationUnitSettingsHash)
	c.Check(got.Sequence, tc.HasLen, 0)

	_, err = s.DB().ExecContext(c.Context(), "DROP TRIGGER fail_unit_settings_revision")
	c.Assert(err, tc.ErrorIsNil)
	err = st.InitialiseRelationSettingsRevisions(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	got, err = exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.RelationApplicationSettingsHash[0].Revision, tc.Equals, int64(1))
	c.Check(got.RelationUnitSettingsHash[0].Revision, tc.Equals, int64(2))
	c.Check(got.Sequence, tc.DeepEquals, []v4_1_0.Sequence{{Namespace: "relation-settings_relation-uuid", Value: 2}})
}

func (s *roundTripSuite) changeImportedUnitHash(c *tc.C) {
	st := relationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c), nil)
	err := st.SetRelationUnitSettings(c.Context(), corerelation.UnitUUID("relation-unit-uuid"), map[string]string{"token": "B"}, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func relationSettingsRevisionPayload() *v4_1_0.ModelExport {
	return &v4_1_0.ModelExport{
		Charm: []v4_1_0.Charm{{UUID: "charm-uuid", ReferenceName: "source"}},
		CharmRelation: []v4_1_0.CharmRelation{{
			UUID: "charm-relation-uuid", CharmUUID: "charm-uuid", Name: "endpoint",
		}},
		Application: []v4_1_0.Application{{
			UUID: "app-uuid", Name: "source", CharmUUID: "charm-uuid", SpaceUUID: network.AlphaSpaceId.String(),
		}},
		ApplicationEndpoint: []v4_1_0.ApplicationEndpoint{{
			UUID: "application-endpoint-uuid", ApplicationUUID: "app-uuid", CharmRelationUUID: "charm-relation-uuid",
		}},
		NetNode: []v4_1_0.NetNode{{UUID: "net-node-uuid"}},
		Unit: []v4_1_0.Unit{{
			UUID: "unit-uuid", Name: "source/0", ApplicationUUID: "app-uuid", CharmUUID: "charm-uuid", NetNodeUUID: "net-node-uuid",
		}},
		Relation: []v4_1_0.Relation{{UUID: "relation-uuid"}},
		RelationEndpoint: []v4_1_0.RelationEndpoint{{
			UUID: "relation-endpoint-uuid", RelationUUID: "relation-uuid", EndpointUUID: "application-endpoint-uuid",
		}},
		RelationUnit: []v4_1_0.RelationUnit{{
			UUID: "relation-unit-uuid", RelationEndpointUUID: "relation-endpoint-uuid", UnitUUID: "unit-uuid",
		}},
		RelationApplicationSettingsHash: []v4_1_0.RelationApplicationSettingsHash{{
			RelationEndpointUUID: "relation-endpoint-uuid", Sha256: "application-A", Revision: 7,
		}},
		RelationUnitSettingsHash: []v4_1_0.RelationUnitSettingsHash{{
			RelationUnitUUID: "relation-unit-uuid", Sha256: "unit-A", Revision: 11,
		}},
		Sequence: []v4_1_0.Sequence{{Namespace: "relation-settings_relation-uuid", Value: 99}},
	}
}
