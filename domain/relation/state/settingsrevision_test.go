// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/unit"
	domainrelation "github.com/juju/juju/domain/relation"
	domainsequence "github.com/juju/juju/domain/sequence"
	"github.com/juju/juju/internal/errors"
)

type settingsRevisionSuite struct {
	baseRelationSuite

	applicationUUID         application.UUID
	applicationEndpointUUID string
	relationUUID            corerelation.UUID
	endpointUUID            string
	unitUUID                unit.UUID
	relationUnitUUID        corerelation.UnitUUID
}

func TestSettingsRevisionSuite(t *testing.T) {
	tc.Run(t, &settingsRevisionSuite{})
}

func (s *settingsRevisionSuite) SetUpTest(c *tc.C) {
	s.baseRelationSuite.SetUpTest(c)
	charmUUID := s.addCharm(c)
	charmRelationUUID := s.addCharmRelationWithDefaults(c, charmUUID)
	s.applicationUUID = s.addApplication(c, charmUUID, "app")
	s.applicationEndpointUUID = s.addApplicationEndpoint(c, s.applicationUUID, charmRelationUUID)
	s.relationUUID = s.addRelation(c)
	s.endpointUUID = s.addRelationEndpoint(c, s.relationUUID, s.applicationEndpointUUID)
	s.unitUUID = s.addUnit(c, "app/0", s.applicationUUID, charmUUID)
	s.relationUnitUUID = s.addRelationUnit(c, s.unitUUID, s.endpointUUID)
}

func (s *settingsRevisionSuite) TestUnitSettingsABA(c *tc.C) {
	a := map[string]string{"token": "foobar"}
	err := s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, a, nil)
	c.Assert(err, tc.ErrorIsNil)
	initial := s.changes(c)
	initialHash := s.getRelationUnitSettingsHash(c, s.relationUnitUUID)

	err = s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, nil, []string{"token"})
	c.Assert(err, tc.ErrorIsNil)
	b := s.changes(c)
	settings, err := s.state.GetRelationUnitSettings(c.Context(), s.relationUnitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(settings, tc.HasLen, 0)

	err = s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, a, nil)
	c.Assert(err, tc.ErrorIsNil)
	current := s.changes(c)
	c.Check(initial.Changed["app/0"], tc.Equals, int64(1))
	c.Check(b.Changed["app/0"], tc.Equals, int64(2))
	c.Check(current.Changed["app/0"], tc.Equals, int64(3))
	c.Check(s.getRelationUnitSettingsHash(c, s.relationUnitUUID), tc.Equals, initialHash)

	consumer, err := s.state.GetConsumerRelationUnitsChange(c.Context(), s.relationUUID.String(), s.applicationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(consumer.UnitsSettingsVersions["app/0"], tc.Equals, current.Changed["app/0"])
}

func (s *settingsRevisionSuite) TestApplicationSettingsABA(c *tc.C) {
	a := map[string]string{"token": "foobar"}
	err := s.state.SetRelationApplicationSettings(c.Context(), s.relationUUID, s.applicationUUID, a)
	c.Assert(err, tc.ErrorIsNil)
	initial := s.changes(c)
	initialHash := s.getRelationApplicationSettingsHash(c, s.endpointUUID)

	err = s.state.SetRelationApplicationSettings(c.Context(), s.relationUUID, s.applicationUUID, map[string]string{"token": "other"})
	c.Assert(err, tc.ErrorIsNil)
	b := s.changes(c)
	err = s.state.SetRelationApplicationSettings(c.Context(), s.relationUUID, s.applicationUUID, a)
	c.Assert(err, tc.ErrorIsNil)
	current := s.changes(c)
	c.Check(initial.AppChanged["app"], tc.Equals, int64(1))
	c.Check(b.AppChanged["app"], tc.Equals, int64(2))
	c.Check(current.AppChanged["app"], tc.Equals, int64(3))
	c.Check(s.getRelationApplicationSettingsHash(c, s.endpointUUID), tc.Equals, initialHash)

	consumer, err := s.state.GetConsumerRelationUnitsChange(c.Context(), s.relationUUID.String(), s.applicationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(consumer.AppSettingsVersion["app"], tc.Equals, current.AppChanged["app"])
}

func (s *settingsRevisionSuite) TestNoOpKeepsRevisions(c *tc.C) {
	a := map[string]string{"token": "foobar"}
	for range 2 {
		err := s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, a, nil)
		c.Assert(err, tc.ErrorIsNil)
		err = s.state.SetRelationApplicationSettings(c.Context(), s.relationUUID, s.applicationUUID, a)
		c.Assert(err, tc.ErrorIsNil)
	}
	current := s.changes(c)
	c.Check(current.Changed["app/0"], tc.Equals, int64(1))
	c.Check(current.AppChanged["app"], tc.Equals, int64(2))

	err := s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, map[string]string{"token": "other"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.changes(c).Changed["app/0"], tc.Equals, int64(3))
}

func (s *settingsRevisionSuite) TestUnversionedSettingsInitialiseRevisions(c *tc.C) {
	settings := map[string]string{"token": "foobar"}
	err := s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, settings, nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetRelationApplicationSettings(c.Context(), s.relationUUID, s.applicationUUID, settings)
	c.Assert(err, tc.ErrorIsNil)

	// Unversioned rows retain hashes but lack revisions and a settings sequence.
	s.query(c, "UPDATE relation_unit_settings_hash SET revision = 0 WHERE relation_unit_uuid = ?", s.relationUnitUUID)
	s.query(c, "UPDATE relation_application_settings_hash SET revision = 0 WHERE relation_endpoint_uuid = ?", s.endpointUUID)
	namespace := domainsequence.MakePrefixNamespace(domainrelation.SettingsSequenceNamespace, s.relationUUID.String())
	s.query(c, "DELETE FROM sequence WHERE namespace = ?", namespace.String())
	legacy := s.changes(c)
	c.Check(legacy.Changed["app/0"], tc.Equals, int64(0))
	c.Check(legacy.AppChanged["app"], tc.Equals, int64(0))

	for range 2 {
		err = s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, settings, nil)
		c.Assert(err, tc.ErrorIsNil)
		err = s.state.SetRelationApplicationSettings(c.Context(), s.relationUUID, s.applicationUUID, settings)
		c.Assert(err, tc.ErrorIsNil)
	}
	current := s.changes(c)
	c.Check(current.Changed["app/0"], tc.Equals, int64(1))
	c.Check(current.AppChanged["app"], tc.Equals, int64(2))
}

func (s *settingsRevisionSuite) TestRevisionSurvivesScopeReentry(c *tc.C) {
	a := map[string]string{"token": "foobar"}
	err := s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, a, nil)
	c.Assert(err, tc.ErrorIsNil)
	initial := s.changes(c)

	// Remove the scope-owned rows and recreate the same unit's relation bag.
	s.query(c, "DELETE FROM relation_unit_setting WHERE relation_unit_uuid = ?", s.relationUnitUUID)
	s.query(c, "DELETE FROM relation_unit_settings_hash WHERE relation_unit_uuid = ?", s.relationUnitUUID)
	s.query(c, "DELETE FROM relation_unit WHERE uuid = ?", s.relationUnitUUID)
	s.relationUnitUUID = s.addRelationUnit(c, s.unitUUID, s.endpointUUID)
	err = s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, a, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.changes(c).Changed["app/0"], tc.Equals, initial.Changed["app/0"]+1)
}

func (s *settingsRevisionSuite) TestRelationsHaveIndependentSequences(c *tc.C) {
	a := map[string]string{"token": "foobar"}
	err := s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, a, nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetRelationApplicationSettings(c.Context(), s.relationUUID, s.applicationUUID, a)
	c.Assert(err, tc.ErrorIsNil)

	otherRelationUUID := s.addRelation(c)
	otherEndpointUUID := s.addRelationEndpoint(c, otherRelationUUID, s.applicationEndpointUUID)
	otherRelationUnitUUID := s.addRelationUnit(c, s.unitUUID, otherEndpointUUID)
	err = s.state.SetRelationUnitSettings(c.Context(), otherRelationUnitUUID, a, nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetRelationApplicationSettings(c.Context(), otherRelationUUID, s.applicationUUID, a)
	c.Assert(err, tc.ErrorIsNil)
	otherChanges := func() domainrelation.RelationUnitsChange {
		changes, err := s.state.GetRelationUnitChanges(c.Context(), otherRelationUUID.String(), []unit.UUID{s.unitUUID}, []application.UUID{s.applicationUUID})
		c.Assert(err, tc.ErrorIsNil)
		return changes
	}
	initialOther := otherChanges()
	c.Check(initialOther.Changed["app/0"], tc.Equals, int64(1))
	c.Check(initialOther.AppChanged["app"], tc.Equals, int64(2))

	err = s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, map[string]string{"token": "other"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.changes(c).Changed["app/0"], tc.Equals, int64(3))
	c.Check(otherChanges(), tc.DeepEquals, initialOther)
	err = s.state.SetRelationUnitSettings(c.Context(), otherRelationUnitUUID, map[string]string{"token": "other"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(otherChanges().Changed["app/0"], tc.Equals, int64(3))
}

func (s *settingsRevisionSuite) TestRevisionRollsBackWithSettings(c *tc.C) {
	a := map[string]string{"token": "foobar"}
	rollback := errors.New("roll back settings")
	err := s.Txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.state.setRelationUnitSettings(ctx, tx, s.relationUnitUUID.String(), a, nil); err != nil {
			return err
		}
		if err := s.state.setRelationApplicationSettings(ctx, tx, s.relationUUID.String(), s.applicationUUID.String(), a); err != nil {
			return err
		}
		return rollback
	})
	c.Assert(err, tc.ErrorIs, rollback)
	current := s.changes(c)
	c.Check(current.Changed["app/0"], tc.Equals, int64(0))
	c.Check(current.AppChanged["app"], tc.Equals, int64(0))
	c.Check(s.getRelationUnitSettings(c, s.relationUnitUUID), tc.HasLen, 0)
	c.Check(s.getRelationApplicationSettings(c, s.endpointUUID), tc.HasLen, 0)

	err = s.state.SetRelationUnitSettings(c.Context(), s.relationUnitUUID, a, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.changes(c).Changed["app/0"], tc.Equals, int64(1))
}

func (s *settingsRevisionSuite) changes(c *tc.C) domainrelation.RelationUnitsChange {
	changes, err := s.state.GetRelationUnitChanges(c.Context(), s.relationUUID.String(), []unit.UUID{s.unitUUID}, []application.UUID{s.applicationUUID})
	c.Assert(err, tc.ErrorIsNil)
	return changes
}
