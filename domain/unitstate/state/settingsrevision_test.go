// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/tc"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/unitstate/internal"
)

type settingsRevisionSnapshot struct {
	unitHash, applicationHash         string
	unitRevision, applicationRevision int64
}

func (s *commitHookSuite) TestCommitHookSettingsRevisionsABA(c *tc.C) {
	applicationEndpointUUID := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, s.fakeCharmRelationProvidesUUID)
	relationUUID := s.addRelation(c)
	endpointUUID := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID)
	unitUUID := s.addUnitAndNetNode(c, "source/0", s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, endpointUUID)

	write := func(value string) {
		err := s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
			UnitUUID: unitUUID.String(),
			RelationSettings: []internal.RelationSettings{{
				RelationUUID:   relationUUID,
				UnitSet:        map[string]string{"token": value},
				ApplicationSet: map[string]string{"token": value},
			}},
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	write("A")
	a := s.settingsRevisionSnapshot(c, relationUnitUUID, endpointUUID)
	write("B")
	b := s.settingsRevisionSnapshot(c, relationUnitUUID, endpointUUID)
	write("A")
	current := s.settingsRevisionSnapshot(c, relationUnitUUID, endpointUUID)
	c.Check(a.unitRevision > 0, tc.IsTrue)
	c.Check(a.applicationRevision > 0, tc.IsTrue)
	c.Check(a.unitRevision != a.applicationRevision, tc.IsTrue)
	c.Check(b.unitRevision > a.unitRevision, tc.IsTrue)
	c.Check(b.applicationRevision > a.applicationRevision, tc.IsTrue)
	c.Check(current.unitRevision > b.unitRevision, tc.IsTrue)
	c.Check(current.applicationRevision > b.applicationRevision, tc.IsTrue)
	c.Check(current.unitHash, tc.Equals, a.unitHash)
	c.Check(current.applicationHash, tc.Equals, a.applicationHash)

	write("A")
	c.Check(s.settingsRevisionSnapshot(c, relationUnitUUID, endpointUUID), tc.DeepEquals, current)
}

func (s *commitHookSuite) settingsRevisionSnapshot(
	c *tc.C, relationUnitUUID corerelation.UnitUUID, endpointUUID string,
) settingsRevisionSnapshot {
	var snapshot settingsRevisionSnapshot
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT rush.sha256, rush.revision, rash.sha256, rash.revision
FROM relation_unit_settings_hash AS rush
CROSS JOIN relation_application_settings_hash AS rash
WHERE rush.relation_unit_uuid = ? AND rash.relation_endpoint_uuid = ?
`, relationUnitUUID, endpointUUID).Scan(
			&snapshot.unitHash, &snapshot.unitRevision,
			&snapshot.applicationHash, &snapshot.applicationRevision,
		)
	})
	c.Assert(err, tc.ErrorIsNil)
	return snapshot
}
