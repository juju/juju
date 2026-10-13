// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"testing"
	"time"

	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type charmSuite struct {
	baseSuite
}

func TestCharmSuite(t *testing.T) {
	tc.Run(t, &charmSuite{})
}

func (s *charmSuite) TestCharmScheduleRemoval(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.CharmScheduleRemoval(c.Context(), "removal-uuid", "charm-uuid", when)
	c.Assert(err, tc.ErrorIsNil)

	row := s.DB().QueryRow(`
SELECT t.name, r.entity_uuid, r.force, r.scheduled_for
FROM   removal r JOIN removal_type t ON r.removal_type_id = t.id
WHERE  r.uuid = ?`, "removal-uuid",
	)
	var (
		removalType  string
		rUUID        string
		force        bool
		scheduledFor time.Time
	)
	err = row.Scan(&removalType, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalType, tc.Equals, "charm")
	c.Check(rUUID, tc.Equals, "charm-uuid")
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *charmSuite) TestCharmExists(c *tc.C) {
	charmUUID := s.createBareCharm(c, true, s.now.Add(-2*time.Hour))
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.CharmExists(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)

	exists, err = st.CharmExists(c.Context(), "no-such-charm")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

func (s *charmSuite) TestCharmHasReferencedResourcesNoResources(c *tc.C) {
	charmUUID := s.createBareCharm(c, true, s.now.Add(-2*time.Hour))
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	pinned, err := st.CharmHasReferencedResources(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pinned, tc.IsFalse)
}

func (s *charmSuite) TestCharmHasReferencedResourcesUnreferenced(c *tc.C) {
	charmUUID := s.createBareCharm(c, true, s.now.Add(-2*time.Hour))
	s.createBareResource(c, charmUUID, "free-res", s.now.Add(-2*time.Hour))
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	pinned, err := st.CharmHasReferencedResources(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pinned, tc.IsFalse)
}

func (s *charmSuite) TestCharmHasReferencedResourcesApplicationReference(c *tc.C) {
	charmUUID := s.createBareCharm(c, true, s.now.Add(-2*time.Hour))
	resourceUUID := s.createBareResource(c, charmUUID, "app-res", s.now.Add(-2*time.Hour))

	appSvc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, appSvc, "some-app")
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO application_resource (application_uuid, resource_uuid)
VALUES (?, ?)`, appUUID.String(), resourceUUID)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	pinned, err := st.CharmHasReferencedResources(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pinned, tc.IsTrue)
}

func (s *charmSuite) TestCharmHasReferencedResourcesPendingReference(c *tc.C) {
	charmUUID := s.createBareCharm(c, true, s.now.Add(-2*time.Hour))
	resourceUUID := s.createBareResource(c, charmUUID, "pending-res", s.now.Add(-2*time.Hour))

	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO pending_application_resource (resource_uuid, application_name)
VALUES (?, 'some-app')`, resourceUUID)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	pinned, err := st.CharmHasReferencedResources(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pinned, tc.IsTrue)
}
