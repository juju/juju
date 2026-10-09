// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"testing"
	"time"

	"github.com/juju/tc"

	applicationservice "github.com/juju/juju/domain/application/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type resourceSuite struct {
	baseSuite
}

func TestResourceSuite(t *testing.T) {
	tc.Run(t, &resourceSuite{})
}

func (s *resourceSuite) TestResourceExists(c *tc.C) {
	resourceUUID, _ := s.createResource(c, false)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.ResourceExists(c.Context(), resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)

	exists, err = st.ResourceExists(c.Context(), "missing")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

func (s *resourceSuite) TestDeleteResourceIfUnusedDeletesOrphan(c *tc.C) {
	resourceUUID, _ := s.createResource(c, false)
	_, err := s.DB().ExecContext(c.Context(), `
DELETE FROM application_resource WHERE resource_uuid = ?`, resourceUUID)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	removed, err := st.DeleteResourceIfUnused(c.Context(), resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removed, tc.IsTrue)
	s.checkResourceCount(c, resourceUUID, 0)
}

func (s *resourceSuite) TestDeleteResourceIfUnusedMissingIsComplete(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	removed, err := st.DeleteResourceIfUnused(c.Context(), "missing")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removed, tc.IsTrue)
}

func (s *resourceSuite) TestDeleteResourceIfUnusedApplicationReference(c *tc.C) {
	resourceUUID, _ := s.createResource(c, false)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	removed, err := st.DeleteResourceIfUnused(c.Context(), resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removed, tc.IsFalse)
	s.checkResourceCount(c, resourceUUID, 1)
}

func (s *resourceSuite) TestDeleteResourceIfUnusedPendingApplicationReference(c *tc.C) {
	resourceUUID, _ := s.createResource(c, false)
	_, err := s.DB().ExecContext(c.Context(), `
DELETE FROM application_resource WHERE resource_uuid = ?`, resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO pending_application_resource (resource_uuid, application_name)
VALUES (?, 'pending-app')`, resourceUUID)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	removed, err := st.DeleteResourceIfUnused(c.Context(), resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removed, tc.IsFalse)
	s.checkResourceCount(c, resourceUUID, 1)
}

func (s *resourceSuite) TestDeleteResourceIfUnusedUnitReference(c *tc.C) {
	resourceUUID, unitUUID := s.createResource(c, true)
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO unit_resource (resource_uuid, unit_uuid, charm_resource_name, added_at)
VALUES (?, ?, 'buzz', ?)`, resourceUUID, unitUUID, time.Now().UTC())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
DELETE FROM application_resource WHERE resource_uuid = ?`, resourceUUID)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	removed, err := st.DeleteResourceIfUnused(c.Context(), resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removed, tc.IsFalse)
	s.checkResourceCount(c, resourceUUID, 1)

	_, err = s.DB().ExecContext(c.Context(), `
DELETE FROM unit_resource WHERE resource_uuid = ?`, resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	removed, err = st.DeleteResourceIfUnused(c.Context(), resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removed, tc.IsTrue)
	s.checkResourceCount(c, resourceUUID, 0)
}

func (s *resourceSuite) TestDeleteResourceIfUnusedDeletesContainerImageMetadata(c *tc.C) {
	resourceUUID, _ := s.createResource(c, false)
	const storageKey = "registry.example/image@sha256:abc"
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO resource_container_image_metadata_store (storage_key, registry_path)
VALUES (?, 'registry.example/image:latest')`, storageKey)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO resource_image_store (resource_uuid, store_storage_key)
VALUES (?, ?)`, resourceUUID, storageKey)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
DELETE FROM application_resource WHERE resource_uuid = ?`, resourceUUID)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	removed, err := st.DeleteResourceIfUnused(c.Context(), resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removed, tc.IsTrue)

	var count int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM resource_container_image_metadata_store
WHERE storage_key = ?`, storageKey).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *resourceSuite) TestDeleteResourceIfUnusedPreservesSharedContainerImageMetadata(c *tc.C) {
	resourceUUID, _ := s.createResource(c, false)
	const (
		otherResourceUUID = "22222222-2222-4222-8222-222222222222"
		storageKey        = "registry.example/image@sha256:abc"
	)
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO resource_container_image_metadata_store (storage_key, registry_path)
VALUES (?, 'registry.example/image:latest')`, storageKey)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO resource (uuid, charm_uuid, charm_resource_name, revision,
                      origin_type_id, state_id, created_at)
SELECT ?, charm_uuid, charm_resource_name, revision,
       origin_type_id, state_id, created_at
FROM   resource
WHERE  uuid = ?`, otherResourceUUID, resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO resource_image_store (resource_uuid, store_storage_key)
VALUES (?, ?), (?, ?)`, resourceUUID, storageKey, otherResourceUUID, storageKey)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
DELETE FROM application_resource WHERE resource_uuid = ?`, resourceUUID)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	removed, err := st.DeleteResourceIfUnused(c.Context(), resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removed, tc.IsTrue)

	var count int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM resource_container_image_metadata_store
WHERE storage_key = ?`, storageKey).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

func (s *resourceSuite) createResource(c *tc.C, withUnit bool) (string, string) {
	appSvc := s.setupApplicationService(c)
	var units []applicationservice.AddIAASUnitArg
	if withUnit {
		units = append(units, applicationservice.AddIAASUnitArg{})
	}
	appUUID := s.createIAASApplication(c, appSvc, "some-app", units...)

	var resourceUUID, unitUUID string
	err := s.DB().QueryRowContext(c.Context(), `
SELECT r.uuid
FROM   resource AS r
JOIN   application_resource AS ar ON ar.resource_uuid = r.uuid
WHERE  ar.application_uuid = ?
AND    r.state_id = 0`, appUUID).Scan(&resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	if withUnit {
		err = s.DB().QueryRowContext(c.Context(), `
SELECT uuid FROM unit WHERE application_uuid = ?`, appUUID).Scan(&unitUUID)
		c.Assert(err, tc.ErrorIsNil)
	}
	return resourceUUID, unitUUID
}

func (s *resourceSuite) checkResourceCount(c *tc.C, resourceUUID string, expected int) {
	var count int
	err := s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM resource WHERE uuid = ?`, resourceUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expected)
}
