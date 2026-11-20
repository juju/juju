// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_0_12"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/domain/modelimport"
)

func (s *importSuite) TestImporterInitialisesLegacyRelationSettings(c *tc.C) {
	s.bootstrapModel(c)
	legacy := v4_0_12.ModelExport{
		ModelAgent: []v4_0_12.ModelAgent{{ModelUUID: s.ModelUUID()}},
		Charm:      []v4_0_12.Charm{{UUID: "charm-uuid", ReferenceName: "source"}},
		CharmRelation: []v4_0_12.CharmRelation{{
			UUID: "charm-relation-uuid", CharmUUID: "charm-uuid", Name: "endpoint",
		}},
		Application: []v4_0_12.Application{{
			UUID: "app-uuid", Name: "source", CharmUUID: "charm-uuid", SpaceUUID: network.AlphaSpaceId.String(),
		}},
		ApplicationEndpoint: []v4_0_12.ApplicationEndpoint{{
			UUID: "application-endpoint-uuid", ApplicationUUID: "app-uuid", CharmRelationUUID: "charm-relation-uuid",
		}},
		NetNode: []v4_0_12.NetNode{{UUID: "net-node-uuid"}},
		Unit: []v4_0_12.Unit{{
			UUID: "unit-uuid", Name: "source/0", ApplicationUUID: "app-uuid", CharmUUID: "charm-uuid", NetNodeUUID: "net-node-uuid",
		}},
		Relation: []v4_0_12.Relation{{UUID: "relation-uuid"}},
		RelationEndpoint: []v4_0_12.RelationEndpoint{{
			UUID: "relation-endpoint-uuid", RelationUUID: "relation-uuid", EndpointUUID: "application-endpoint-uuid",
		}},
		RelationUnit: []v4_0_12.RelationUnit{{
			UUID: "relation-unit-uuid", RelationEndpointUUID: "relation-endpoint-uuid", UnitUUID: "unit-uuid",
		}},
		RelationApplicationSettingsHash: []v4_0_12.RelationApplicationSettingsHash{{
			RelationEndpointUUID: "relation-endpoint-uuid", Sha256: "application-hash",
		}},
		RelationUnitSettingsHash: []v4_0_12.RelationUnitSettingsHash{{
			RelationUnitUUID: "relation-unit-uuid", Sha256: "unit-hash",
		}},
	}
	transformer, err := modelimport.NewTransformer()
	c.Assert(err, tc.ErrorIsNil)
	transformed, err := transformer.Transform(c.Context(), semversion.MustParse("4.0.12"), legacy)
	c.Assert(err, tc.ErrorIsNil)
	payload, ok := transformed.(latest.ModelExport)
	c.Assert(ok, tc.IsTrue)
	err = modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), &payload)
	c.Assert(err, tc.ErrorIsNil)
	got, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.RelationApplicationSettingsHash, tc.DeepEquals, []v4_1_0.RelationApplicationSettingsHash{{
		RelationEndpointUUID: "relation-endpoint-uuid", Sha256: "application-hash", Revision: 1,
	}})
	c.Check(got.RelationUnitSettingsHash, tc.DeepEquals, []v4_1_0.RelationUnitSettingsHash{{
		RelationUnitUUID: "relation-unit-uuid", Sha256: "unit-hash", Revision: 2,
	}})
	c.Check(got.Sequence, tc.DeepEquals, []v4_1_0.Sequence{{Namespace: "relation-settings_relation-uuid", Value: 2}})
}
