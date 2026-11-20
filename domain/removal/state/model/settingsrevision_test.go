// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/tc"

	corerelation "github.com/juju/juju/core/relation"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/life"
	domainrelation "github.com/juju/juju/domain/relation"
	relationstate "github.com/juju/juju/domain/relation/state"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	domainsequence "github.com/juju/juju/domain/sequence"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *relationSuite) TestDeleteRelationRemovesSettingsSequence(c *tc.C) {
	relationUUID, _, _ := s.addAppUnitRelationScope(c, domaincharm.CharmHubSource)
	initial, err := s.relationSettingsSequence(c, relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(initial, tc.Equals, int64(2))
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
VALUES ('other-relation-uuid', 0, 43, 0);
INSERT INTO sequence (namespace, value) VALUES ('relation-settings_other-relation-uuid', 99);
INSERT INTO sequence (namespace, value) VALUES ('relation', 43);
`)
	c.Assert(err, tc.ErrorIsNil)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteRelationUnits(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	current, err := s.relationSettingsSequence(c, relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(current, tc.Equals, initial)
	err = st.DeleteRelation(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.relationSettingsSequence(c, relationUUID)
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
	other, err := s.relationSettingsSequence(c, "other-relation-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(other, tc.Equals, int64(99))
	var relationIDSequence int64
	err = s.DB().QueryRowContext(c.Context(), "SELECT value FROM sequence WHERE namespace = 'relation'").Scan(&relationIDSequence)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relationIDSequence, tc.Equals, int64(43))

	// Removal remains idempotent after the counter has been deleted.
	err = st.DeleteRelation(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestIncompleteRelationRemovalKeepsSettingsSequence(c *tc.C) {
	relationUUID, _, _ := s.addAppUnitRelationScope(c, domaincharm.CharmHubSource)
	initial, err := s.relationSettingsSequence(c, relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteRelation(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIs, removalerrors.UnitsStillInScope)
	current, err := s.relationSettingsSequence(c, relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(current, tc.Equals, initial)
}

func (s *relationSuite) TestLeaveScopeKeepsSettingsSequence(c *tc.C) {
	relationUUID, unitUUID, relationUnitUUID := s.addAppUnitRelationScope(c, domaincharm.CharmHubSource)
	initial, err := s.relationSettingsSequence(c, relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.LeaveScope(c.Context(), relationUnitUUID)
	c.Assert(err, tc.ErrorIsNil)
	current, err := s.relationSettingsSequence(c, relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(current, tc.Equals, initial)

	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid)
SELECT 'rejoined-relation-unit', re.uuid, ?
FROM relation_endpoint AS re WHERE re.relation_uuid = ?;
`, unitUUID, relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	relationState := relationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c), nil)
	err = relationState.SetRelationUnitSettings(c.Context(), corerelation.UnitUUID("rejoined-relation-unit"), map[string]string{"key": "value"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	current, err = s.relationSettingsSequence(c, relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(current, tc.Equals, initial+1)
}

func (s *relationWithRemoteOfferer) TestDeleteRelationWithRemoteOffererRemovesSettingsSequence(c *tc.C) {
	relationUUID, _ := s.createRelationWithRemoteOfferer(c)
	_, err := s.relationSettingsSequence(c, relationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	s.advanceRelationLife(c, relationUUID, life.Dying)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteRelationUnits(c.Context(), relationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	err = st.DeleteRelationWithRemoteOfferer(c.Context(), relationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.relationSettingsSequence(c, relationUUID.String())
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
}

func (s *relationWithRemoteConsumer) TestDeleteRelationWithRemoteConsumerRemovesSettingsSequence(c *tc.C) {
	relationUUID, _, _ := s.createRelationWithRemoteConsumer(c)
	_, err := s.relationSettingsSequence(c, relationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	s.advanceRelationLife(c, relationUUID, life.Dying)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteRelationUnits(c.Context(), relationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	err = st.DeleteRelationWithRemoteConsumer(c.Context(), relationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.relationSettingsSequence(c, relationUUID.String())
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
}

func (s *baseSuite) relationSettingsSequence(c *tc.C, relationUUID string) (int64, error) {
	var value int64
	namespace := domainsequence.MakePrefixNamespace(domainrelation.SettingsSequenceNamespace, relationUUID)
	err := s.DB().QueryRowContext(c.Context(), "SELECT value FROM sequence WHERE namespace = ?", namespace.String()).Scan(&value)
	return value, err
}
