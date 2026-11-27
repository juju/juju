// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	domainrelation "github.com/juju/juju/domain/relation"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
)

func (s *settingsRevisionSuite) TestSetSettingsSequences(c *tc.C) {
	s.query(c, `INSERT INTO sequence (namespace, value)
VALUES ('relation', 43), ('relation-settings_unrelated', 99)`)
	namespace := domainsequence.MakePrefixNamespace(domainrelation.SettingsSequenceNamespace, s.relationUUID.String()).String()
	for _, test := range []struct {
		name                string
		unitRevision        *int64
		applicationRevision *int64
		counter             *int64
		want                uint64
	}{
		{name: "no settings"},
		{name: "zero revisions", unitRevision: new(int64(0)), applicationRevision: new(int64(0))},
		{name: "unit settings only", unitRevision: new(int64(11)), want: 11},
		{name: "application settings only", applicationRevision: new(int64(12)), want: 12},
		{name: "unit maximum", unitRevision: new(int64(20)), applicationRevision: new(int64(10)), want: 20},
		{name: "application maximum", unitRevision: new(int64(10)), applicationRevision: new(int64(20)), want: 20},
		{name: "counter behind", unitRevision: new(int64(10)), applicationRevision: new(int64(20)), counter: new(int64(3)), want: 20},
		{name: "counter ahead", unitRevision: new(int64(10)), applicationRevision: new(int64(20)), counter: new(int64(99)), want: 99},
		{name: "counter without settings", counter: new(int64(30)), want: 30},
	} {
		c.Logf("%s", test.name)
		s.query(c, "DELETE FROM relation_unit_settings_hash WHERE relation_unit_uuid = ?", s.relationUnitUUID)
		s.query(c, "DELETE FROM relation_application_settings_hash WHERE relation_endpoint_uuid = ?", s.endpointUUID)
		s.query(c, "DELETE FROM sequence WHERE namespace = ?", namespace)
		if test.unitRevision != nil {
			s.query(c, `INSERT INTO relation_unit_settings_hash (relation_unit_uuid, sha256, revision)
VALUES (?, 'unit-hash', ?)`, s.relationUnitUUID, *test.unitRevision)
		}
		if test.applicationRevision != nil {
			s.query(c, `INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256, revision)
VALUES (?, 'application-hash', ?)`, s.endpointUUID, *test.applicationRevision)
		}
		if test.counter != nil {
			s.query(c, "INSERT INTO sequence (namespace, value) VALUES (?, ?)", namespace, *test.counter)
		}
		before := s.changes(c)
		for range 2 {
			err := s.state.SetSettingsSequences(c.Context())
			c.Assert(err, tc.ErrorIsNil)
		}
		want := map[string]uint64{"relation": 43, "relation-settings_unrelated": 99}
		if test.unitRevision != nil || test.applicationRevision != nil || test.counter != nil {
			want[namespace] = test.want
		}
		c.Check(s.settingsSequences(c), tc.DeepEquals, want)
		c.Check(s.changes(c), tc.DeepEquals, before)
	}
}

func (s *settingsRevisionSuite) TestSetSettingsSequencesAllRelationLives(c *tc.C) {
	want := make(map[string]uint64)
	for _, relationLife := range []life.Life{life.Alive, life.Dying, life.Dead} {
		relationUUID := s.addRelation(c)
		s.query(c, "UPDATE relation SET life_id = ? WHERE uuid = ?", relationLife, relationUUID)
		endpointUUID := s.addRelationEndpoint(c, relationUUID, s.applicationEndpointUUID)
		revision := int64(20 + relationLife)
		s.query(c, `INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256, revision)
VALUES (?, 'application-hash', ?)`, endpointUUID, revision)
		namespace := domainsequence.MakePrefixNamespace(domainrelation.SettingsSequenceNamespace, relationUUID.String()).String()
		want[namespace] = uint64(revision)
	}
	err := s.state.SetSettingsSequences(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.settingsSequences(c), tc.DeepEquals, want)
}

func (s *settingsRevisionSuite) TestSetSettingsSequencesAllEndpoints(c *tc.C) {
	charmUUID := s.addCharm(c)
	charmRelationUUID := s.addCharmRelationWithDefaults(c, charmUUID)
	applicationUUID := s.addApplication(c, charmUUID, "other")
	applicationEndpointUUID := s.addApplicationEndpoint(c, applicationUUID, charmRelationUUID)
	otherEndpointUUID := s.addRelationEndpoint(c, s.relationUUID, applicationEndpointUUID)
	s.query(c, `INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256, revision)
VALUES (?, 'application-hash', 10), (?, 'other-application-hash', 40)`, s.endpointUUID, otherEndpointUUID)
	s.query(c, `INSERT INTO relation_unit_settings_hash (relation_unit_uuid, sha256, revision)
VALUES (?, 'unit-hash', 30)`, s.relationUnitUUID)

	err := s.state.SetSettingsSequences(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	namespace := domainsequence.MakePrefixNamespace(domainrelation.SettingsSequenceNamespace, s.relationUUID.String()).String()
	c.Check(s.settingsSequences(c), tc.DeepEquals, map[string]uint64{namespace: 40})
}

func (s *settingsRevisionSuite) TestSetSettingsSequencesRollsBackPartialWrites(c *tc.C) {
	otherRelationUUID := s.addRelation(c)
	otherEndpointUUID := s.addRelationEndpoint(c, otherRelationUUID, s.applicationEndpointUUID)
	s.query(c, `INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256, revision)
VALUES (?, 'application-hash', 20), (?, 'other-application-hash', 30)`, s.endpointUUID, otherEndpointUUID)

	firstUUID := min(s.relationUUID.String(), otherRelationUUID.String())
	firstNamespace := domainsequence.MakePrefixNamespace(domainrelation.SettingsSequenceNamespace, firstUUID).String()
	s.query(c, "INSERT INTO sequence (namespace, value) VALUES (?, 3)", firstNamespace)
	s.query(c, `CREATE TRIGGER reject_settings_sequence BEFORE INSERT ON sequence
WHEN EXISTS (SELECT 1 FROM sequence AS s WHERE s.value > 3)
BEGIN SELECT RAISE(ABORT, 'settings sequence write rejected'); END`)

	err := s.state.SetSettingsSequences(c.Context())
	c.Check(err, tc.ErrorMatches, `.*settings sequence write rejected.*`)
	c.Check(s.settingsSequences(c), tc.DeepEquals, map[string]uint64{firstNamespace: 3})
}

func (s *settingsRevisionSuite) settingsSequences(c *tc.C) map[string]uint64 {
	sequences, err := sequencestate.NewState(s.TxnRunnerFactory()).GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	return sequences
}
