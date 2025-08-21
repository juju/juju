// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/resource/store"
	resourcestoretesting "github.com/juju/juju/core/resource/store/testing"
	coreresourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/unit"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type resourceSuite struct {
	schematesting.ModelSuite

	state *State

	constants struct {
		fakeApplicationUUID1    string
		fakeApplicationUUID2    string
		fakeApplicationName1    string
		fakeApplicationName2    string
		fakeUnitUUID1           string
		fakeUnitUUID2           string
		fakeUnitUUID3           string
		fakeUnitName1           string
		fakeUnitName2           string
		fakeUnitName3           string
		applicationNameFromUUID map[string]string
	}
}

func TestResourceSuite(t *stdtesting.T) {
	tc.Run(t, &resourceSuite{})
}

const fakeCharmUUID = "fake-charm-uuid"

var fingerprint = []byte("123456789012345678901234567890123456789012345678")

func (s *resourceSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	s.constants.fakeApplicationUUID1 = "fake-application-1-uuid"
	s.constants.fakeApplicationUUID2 = "fake-application-2-uuid"
	s.constants.fakeApplicationName1 = "fake-application-1"
	s.constants.fakeApplicationName2 = "fake-application-2"
	s.constants.fakeUnitUUID1 = "fake-unit-1-uuid"
	s.constants.fakeUnitUUID2 = "fake-unit-2-uuid"
	s.constants.fakeUnitUUID3 = "fake-unit-3-uuid"
	s.constants.fakeUnitName1 = "fake-unit/0"
	s.constants.fakeUnitName2 = "fake-unit/1"
	s.constants.fakeUnitName3 = "fake-unit/2"
	s.constants.applicationNameFromUUID = map[string]string{
		s.constants.fakeApplicationUUID1: s.constants.fakeApplicationName1,
		s.constants.fakeApplicationUUID2: s.constants.fakeApplicationName2,
	}

	// Populate DB with two application and a charm
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		fakeNetNodeUUID := "fake-net-node-uuid"

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, architecture_id, source_id)
VALUES (?, 'app', 0, 1)
`, fakeCharmUUID)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO net_node (uuid) VALUES (?)
`, fakeNetNodeUUID)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
VALUES (?, ?, ?, ?, ?),(?, ?, ?, ?, ?)`,
			s.constants.fakeApplicationUUID1, s.constants.fakeApplicationName1, life.Alive, fakeCharmUUID, network.AlphaSpaceId,
			s.constants.fakeApplicationUUID2, s.constants.fakeApplicationName2, life.Alive, fakeCharmUUID, network.AlphaSpaceId)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid)
VALUES (?, ?, ?, ?, ?, ?),(?, ?, ?, ?, ?, ?),(?, ?, ?, ?, ?, ?)`,
			s.constants.fakeUnitUUID1, s.constants.fakeUnitName1, life.Alive, s.constants.fakeApplicationUUID1, fakeCharmUUID, fakeNetNodeUUID,
			s.constants.fakeUnitUUID2, s.constants.fakeUnitName2, life.Alive, s.constants.fakeApplicationUUID1, fakeCharmUUID, fakeNetNodeUUID,
			s.constants.fakeUnitUUID3, s.constants.fakeUnitName3, life.Alive, s.constants.fakeApplicationUUID1, fakeCharmUUID, fakeNetNodeUUID,
		)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("failed to populate DB with applications: %v", errors.ErrorStack(err)))
}

// TestDeleteApplicationResources is a test method that verifies the deletion of resources
// associated with a specific application in the database.
func (s *resourceSuite) TestDeleteApplicationResources(c *tc.C) {
	// Arrange: populate db with some resources, belonging to app1 (2 res) and app2 (1 res)
	res1 := resourceData{
		UUID:            "app1-res1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res1",
		// populate table "resource_retrieved_by"
		RetrievedByType: "user",
		RetrievedByName: "john",
	}
	res2 := resourceData{
		UUID:            "app1-res2-uuid",
		Name:            "res2",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
	}
	other := resourceData{
		UUID:            "res-uuid",
		Name:            "res3",
		ApplicationUUID: s.constants.fakeApplicationUUID2,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		for _, input := range []resourceData{res1, res2, other} {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: delete resources from application 1
	err = s.state.DeleteApplicationResources(c.Context(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert: check that resources have been deleted in expected tables
	// without errors
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to delete resources from application 1: %v", errors.ErrorStack(err)))
	var remainingResources []resourceData
	var noRowsInResourceRetrievedBy bool
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		rows, err := tx.Query(`
SELECT uuid, charm_resource_name, application_uuid
FROM resource AS r
LEFT JOIN application_resource AS ar ON r.uuid = ar.resource_uuid`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var uuid string
			var resName string
			var appUUID string
			if err := rows.Scan(&uuid, &resName, &appUUID); err != nil {
				return err
			}
			remainingResources = append(remainingResources,
				resourceData{UUID: uuid, ApplicationUUID: appUUID,
					Name: resName})
		}
		// fetch resource_retrieved_by
		err = s.runQuery(c, `SELECT resource_uuid from resource_retrieved_by`)
		if errors.Is(err, sql.ErrNoRows) {
			noRowsInResourceRetrievedBy = true
			return nil
		}
		return err
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to check db: %v",
		errors.ErrorStack(err)))
	c.Check(noRowsInResourceRetrievedBy, tc.Equals, true, tc.Commentf("(Assert) resource_retrieved_by table should be empty"))
	c.Check(remainingResources, tc.DeepEquals, []resourceData{other},
		tc.Commentf("(Assert) only resource from %q should be there",
			s.constants.fakeApplicationUUID2))
}

// TestDeleteApplicationResourcesErrorRemainingUnits tests resource deletion with linked units.
//
// This method populates the database with a resource linked to a unit, attempts to delete
// the application's resources, then verifies that an error is returned due to the remaining unit
// and that no resources have been deleted. This enforces constraints on cleaning up resources
// with active dependencies.
func (s *resourceSuite) TestDeleteApplicationResourcesErrorRemainingUnits(c *tc.C) {
	// Arrange: populate db with some resource a resource linked to a unit
	input := resourceData{
		UUID:            "app1-res1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res1",
		// Populate table resource_unit
		UnitUUID: s.constants.fakeUnitUUID1,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		return input.insert(c.Context(), tx)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: delete resources from application 1
	err = s.state.DeleteApplicationResources(c.Context(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert: check an error is returned and no resource deleted
	c.Check(err, tc.ErrorIs, resourceerrors.CleanUpStateNotValid,
		tc.Commentf("(Assert) unexpected error: %v", errors.ErrorStack(err)))
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		var discard string
		return tx.QueryRow(`
SELECT uuid FROM v_application_resource
WHERE uuid = ? AND application_uuid = ? AND name = ?`,
			input.UUID, input.ApplicationUUID, input.Name,
		).Scan(&discard)
	})
	c.Check(err, tc.ErrorIsNil, tc.Commentf("(Assert) resource deleted or cannot check db: %v",
		errors.ErrorStack(err)))
}

// TestDeleteApplicationResourcesErrorRemainingObjectStoreData verifies that attempting to delete application
// resources will fail when there are remaining object store data linked to the resource,
// and no resource will be deleted.
func (s *resourceSuite) TestDeleteApplicationResourcesErrorRemainingObjectStoreData(c *tc.C) {
	// Arrange: populate db with some resource linked with some data
	input := resourceData{
		UUID:            "res1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res1",
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// Insert the data
		if err := input.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		// Insert some data linked to the resource
		if _, err := tx.Exec(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384,size)
VALUES ('store-uuid','','',0)`); err != nil {
			return errors.Capture(err)
		}
		if _, err := tx.Exec(`
INSERT INTO resource_file_store (resource_uuid, store_uuid)
VALUES (?,'store-uuid')`, input.UUID); err != nil {
			return errors.Capture(err)
		}
		return
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: delete resources from application 1
	err = s.state.DeleteApplicationResources(c.Context(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert: check an error is returned and no resource deleted
	c.Check(err, tc.ErrorIs, resourceerrors.CleanUpStateNotValid,
		tc.Commentf("(Assert) unexpected error: %v", errors.ErrorStack(err)))
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		var discard string
		return tx.QueryRow(`
SELECT uuid FROM v_application_resource
WHERE uuid = ? AND application_uuid = ? AND name = ?`,
			input.UUID, input.ApplicationUUID, input.Name,
		).Scan(&discard)
	})
	c.Check(err, tc.ErrorIsNil, tc.Commentf("(Assert) resource deleted or cannot check db: %v",
		errors.ErrorStack(err)))
}

// TestDeleteApplicationResourcesErrorRemainingImageStoreData verifies that attempting to delete application
// resources will fail when there are remaining image store data linked to the resource,
// and no resource will be deleted.
func (s *resourceSuite) TestDeleteApplicationResourcesErrorRemainingImageStoreData(c *tc.C) {
	// Arrange: populate db with some resource linked with some data
	input := resourceData{
		UUID:            "res1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res1",
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// Insert the data
		if err := input.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		// Insert some data linked to the resource
		if _, err := tx.Exec(`
INSERT INTO resource_container_image_metadata_store (storage_key, registry_path)
VALUES ('store-uuid','')`); err != nil {
			return errors.Capture(err)
		}
		if _, err := tx.Exec(`
INSERT INTO resource_image_store (resource_uuid, store_storage_key)
VALUES (?,'store-uuid')`, input.UUID); err != nil {
			return errors.Capture(err)
		}
		return
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: delete resources from application 1
	err = s.state.DeleteApplicationResources(c.Context(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert: check an error is returned and no resource deleted
	c.Check(err, tc.ErrorIs, resourceerrors.CleanUpStateNotValid,
		tc.Commentf("(Assert) unexpected error: %v", errors.ErrorStack(err)))
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		var discard string
		return tx.QueryRow(`
SELECT uuid FROM v_application_resource
WHERE uuid = ? AND application_uuid = ? AND name = ?`,
			input.UUID, input.ApplicationUUID, input.Name,
		).Scan(&discard)
	})
	c.Check(err, tc.ErrorIsNil, tc.Commentf("(Assert) resource deleted or cannot check db: %v",
		errors.ErrorStack(err)))
}

// TestDeleteUnitResources verifies that resources linked to a specific unit are deleted correctly.
func (s *resourceSuite) TestDeleteUnitResources(c *tc.C) {
	// Arrange: populate db with some resource a resource linked to a unit
	resUnit1 := resourceData{
		UUID:            "res-unit1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res-unit1",
		// Populate table resource_unit
		UnitUUID: s.constants.fakeUnitUUID1,
	}
	other := resourceData{
		UUID:            "res-unit2-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res-unit2",
		// Populate table resource_unit
		UnitUUID: s.constants.fakeUnitUUID2,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		for _, input := range []resourceData{resUnit1, other} {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: delete resources from application 1
	err = s.state.DeleteUnitResources(c.Context(), unit.UUID(s.constants.fakeUnitUUID1))

	// Assert: check that resources link to unit 1 have been deleted in expected tables
	// without errors
	c.Assert(err, tc.ErrorIsNil,
		tc.Commentf("(Assert) failed to delete resources link to unit 1: %v",
			errors.ErrorStack(err)))
	var obtained []resourceData
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		rows, err := tx.Query(`
SELECT uuid, name, application_uuid, unit_uuid
FROM v_application_resource AS rv
LEFT JOIN unit_resource AS ur ON rv.uuid = ur.resource_uuid`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var uuid string
			var resName string
			var appUUID string
			var unitUUID *string
			if err := rows.Scan(&uuid, &resName, &appUUID, &unitUUID); err != nil {
				return err
			}
			obtained = append(obtained,
				resourceData{UUID: uuid, ApplicationUUID: appUUID,
					Name: resName, UnitUUID: zeroPtr(unitUUID)})
		}
		return err
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to check db: %v",
		errors.ErrorStack(err)))
	expectedResUnit1 := resUnit1
	expectedResUnit1.UnitUUID = ""
	c.Assert(obtained, tc.SameContents, []resourceData{expectedResUnit1, other}, tc.Commentf("(Assert) unexpected resources: %v", obtained))
}

// TestGetApplicationResourceID tests that the resource ID can be correctly
// retrieved from the database, given a name and an application
func (s *resourceSuite) TestGetApplicationResourceID(c *tc.C) {
	// Arrange: Populate state with two resources on application 1.
	found := resourceData{
		UUID:            "resource-uuid-found",
		Name:            "resource-name-found",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
	}
	other := resourceData{
		UUID:            "resource-uuid-other",
		Name:            "resource-name-other",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		for _, input := range []resourceData{found, other} {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: Get application resource ID
	id, err := s.state.GetApplicationResourceID(c.Context(), resource.GetApplicationResourceIDArgs{
		ApplicationID: application.ID(s.constants.fakeApplicationUUID1),
		Name:          found.Name,
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to get application resource ID: %v", errors.ErrorStack(err)))
	c.Assert(id, tc.Equals, coreresource.UUID(found.UUID),
		tc.Commentf("(Act) unexpected application resource ID"))
}

// TestGetApplicationResourceIDNotFound verifies the behavior when attempting
// to retrieve a resource ID for a non-existent resource within a specified
// application.
func (s *resourceSuite) TestGetApplicationResourceIDNotFound(c *tc.C) {
	// Arrange: No resources
	// Act: Get application resource ID
	_, err := s.state.GetApplicationResourceID(c.Context(), resource.GetApplicationResourceIDArgs{
		ApplicationID: application.ID(s.constants.fakeApplicationUUID1),
		Name:          "resource-name-not-found",
	})
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNotFound, tc.Commentf("(Act) unexpected error"))
}

// TestGetApplicationResourceIDCannotGetPotentialResource verifies that
// potential resources cannot be found by the method.
func (s *resourceSuite) TestGetApplicationResourceIDCannotGetPotentialResource(c *tc.C) {
	// Arrange: Add only a potential resource.
	potentialRes := resourceData{
		UUID:            "with-potential-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "potential-resource-name",
		Type:            charmresource.TypeFile,
		State:           resource.StatePotential.String(),
		Revision:        2,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		for _, input := range []resourceData{potentialRes} {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})

	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: Get application resource ID.
	_, err = s.state.GetApplicationResourceID(c.Context(), resource.GetApplicationResourceIDArgs{
		ApplicationID: application.ID(s.constants.fakeApplicationUUID1),
		Name:          "potential-resource-name",
	})

	// Assert: No resource can be found.
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNotFound, tc.Commentf("(Act) unexpected error"))
}

// TestGetResourceUUIDByApplicationAndResourceName tests that the resource ID can be correctly
// retrieved from the database, given a name and an application
func (s *resourceSuite) TestGetResourceUUIDByApplicationAndResourceName(c *tc.C) {
	// Arrange: Populate state with two resources on application 1.
	found := resourceData{
		UUID:            "resource-uuid-found",
		Name:            "resource-name-found",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
	}
	other := resourceData{
		UUID:            "resource-uuid-other",
		Name:            "resource-name-other",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		for _, input := range []resourceData{found, other} {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: Get application resource ID
	id, err := s.state.GetResourceUUIDByApplicationAndResourceName(
		c.Context(),
		s.constants.fakeApplicationName1,
		found.Name,
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to get resource UUID: %v", errors.ErrorStack(err)))
	c.Assert(id, tc.Equals, coreresource.UUID(found.UUID), tc.Commentf("(Act) unexpected resource UUID"))
}

// TestGetResourceUUIDByApplicationAndResourceNameNotFound verifies the behavior when attempting
// to retrieve a resource ID for a non-existent resource within a specified
// application.
func (s *resourceSuite) TestGetResourceUUIDByApplicationAndResourceNameNotFound(c *tc.C) {
	// Arrange: No resources
	// Act: Get application resource ID
	_, err := s.state.GetResourceUUIDByApplicationAndResourceName(
		c.Context(),
		s.constants.fakeApplicationName1,
		"resource-name-not-found",
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNotFound, tc.Commentf("(Act) unexpected error"))
}

// TestGetResourceUUIDByApplicationAndResourceNameNotFound verifies the behavior when attempting
// to retrieve a resource ID for a non-existent resource within a specified
// application.
func (s *resourceSuite) TestGetResourceUUIDByApplicationAndResourceNameApplicationNameNotFound(c *tc.C) {
	// Arrange: No resources
	// Act: Get application resource ID
	_, err := s.state.GetResourceUUIDByApplicationAndResourceName(
		c.Context(),
		"bad-app-name",
		"resource-name-found",
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationNotFound, tc.Commentf("(Act) unexpected error"))
}

// TestGetResourceUUIDByApplicationAndResourceNameCannotGetPotentialResource verifies that
// potential resources cannot be found by the method.
func (s *resourceSuite) TestGetResourceUUIDByApplicationAndResourceNameCannotGetPotentialResource(c *tc.C) {
	// Arrange: Add only a potential resource.
	potentialRes := resourceData{
		UUID:            "with-potential-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "potential-resource-name",
		Type:            charmresource.TypeFile,
		State:           resource.StatePotential.String(),
		Revision:        2,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		for _, input := range []resourceData{potentialRes} {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})

	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: Get application resource ID.
	_, err = s.state.GetResourceUUIDByApplicationAndResourceName(c.Context(),
		s.constants.fakeApplicationName1,
		"potential-resource-name",
	)

	// Assert: No resource can be found.
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNotFound, tc.Commentf("(Act) unexpected error"))
}

// TestGetResourceNotFound verifies that attempting to retrieve a non-existent
// resource results in a ResourceNotFound error.
func (s *resourceSuite) TestGetResourceNotFound(c *tc.C) {
	// Arrange : no resource
	resID := coreresource.UUID("resource-id")

	// Act
	_, err := s.state.GetResource(c.Context(), resID)

	// Assert
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNotFound, tc.Commentf("(Assert) unexpected error"))
}

// TestGetResource verifies the successful retrieval of a resource from the
// database by its ID.
func (s *resourceSuite) TestGetResource(c *tc.C) {
	// Arrange : a simple resource
	resID := coreresource.UUID("resource-id")
	now := time.Now().Truncate(time.Second).UTC()
	expected := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "resource-name",
				Path:        "/path/to/resource",
				Description: "this is a test resource",
				Type:        charmresource.TypeFile,
			},
			Revision: 42,
			Origin:   charmresource.OriginUpload,
			// todo(gfouillet): handle size/fingerprint
			//Fingerprint: charmresource.Fingerprint{},
			//Size:        0,
		},
		UUID:            resID,
		ApplicationName: s.constants.fakeApplicationName1,
		RetrievedBy:     "johnDoe",
		Timestamp:       now,
	}
	input := resourceData{
		UUID:            resID.String(),
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Revision:        expected.Revision,
		OriginType:      "uploaded",
		CreatedAt:       now,
		Name:            expected.Name,
		Type:            charmresource.TypeFile,
		Path:            expected.Path,
		Description:     expected.Description,
		RetrievedByType: coreresource.Application.String(),
		RetrievedByName: expected.RetrievedBy,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := input.insert(c.Context(), tx)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	obtained, err := s.state.GetResource(c.Context(), resID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute GetResource: %v", errors.ErrorStack(err)))

	// Assert
	c.Assert(obtained, tc.DeepEquals, expected, tc.Commentf("(Assert) resource different than expected"))
}

// TestGetResourcePending verifies the successful retrieval of a resource
// from the database by its ID, even if the application does not yet exist.
// Required to add a pending resource.
func (s *resourceSuite) TestGetResourcePending(c *tc.C) {
	// Arrange : a simple resource
	resID := coreresource.UUID("resource-id")
	now := time.Now().Truncate(time.Second).UTC()
	expected := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "resource-name",
				Path:        "/path/to/resource",
				Description: "this is a test resource",
				Type:        charmresource.TypeFile,
			},
			Revision: 42,
			Origin:   charmresource.OriginUpload,
		},
		UUID:      resID,
		Timestamp: now,
	}
	input := resourceData{
		UUID:        resID.String(),
		Revision:    expected.Revision,
		OriginType:  "uploaded",
		CreatedAt:   now,
		Name:        expected.Name,
		Type:        charmresource.TypeFile,
		Path:        expected.Path,
		Description: expected.Description,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := input.insert(c.Context(), tx)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act
	obtained, err := s.state.GetResource(c.Context(), resID)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	c.Assert(obtained, tc.DeepEquals, expected)
}

func (s *resourceSuite) TestGetResourceWithStoredFile(c *tc.C) {
	// Arrange : a simple resource
	resID := coreresource.UUID("resource-id")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	expected := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: charmresource.TypeFile,
			},
			Fingerprint: fp,
			Size:        42,
			// origin is upload by default if not specified in test input value
			Origin: charmresource.OriginUpload,
		},
		UUID:            resID,
		ApplicationName: s.constants.fakeApplicationName1,
	}
	input := resourceData{
		UUID:            resID.String(),
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            expected.Type,
		ObjectStoreUUID: "object-store-uuid",
		Size:            int(expected.Size),
		SHA384:          expected.Fingerprint.String(),
	}
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := input.insert(c.Context(), tx)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	obtained, err := s.state.GetResource(c.Context(), resID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute GetResource: %v", errors.ErrorStack(err)))

	// Assert
	c.Assert(obtained, tc.DeepEquals, expected, tc.Commentf("(Assert) resource different than expected"))
}

func (s *resourceSuite) TestGetResourceWithStoredImage(c *tc.C) {
	// Arrange : a simple resource
	resID := coreresource.UUID("resource-id")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	expected := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: charmresource.TypeContainerImage,
			},
			Fingerprint: fp,
			Size:        42,
			// origin is upload by default if not specified in test input value
			Origin: charmresource.OriginUpload,
		},
		UUID:            resID,
		ApplicationName: s.constants.fakeApplicationName1,
	}
	input := resourceData{
		UUID:                     resID.String(),
		ApplicationUUID:          s.constants.fakeApplicationUUID1,
		Type:                     expected.Type,
		ContainerImageStorageKey: "container-image-key",
		Size:                     int(expected.Size),
		SHA384:                   expected.Fingerprint.String(),
	}
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := input.insert(c.Context(), tx)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	obtained, err := s.state.GetResource(c.Context(), resID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute GetResource: %v", errors.ErrorStack(err)))

	// Assert
	c.Assert(obtained, tc.DeepEquals, expected, tc.Commentf("(Assert) resource different than expected"))
}

// TestSetRepositoryResource ensures that the SetRepositoryResources function
// updates the resource poll dates correctly.
func (s *resourceSuite) TestSetRepositoryResource(c *tc.C) {
	// Arrange : Insert 4 resources, two have been already polled, and two other not yet.
	now := time.Now().Truncate(time.Second).UTC()
	previousPoll := now.Add(-1 * time.Hour)
	defaultResource := resourceData{
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		RetrievedByType: "user",
		RetrievedByName: "John Doe",
		State:           resource.StatePotential.String(),
	}
	notPolled := []resourceData{
		defaultResource.DeepCopy(),
		defaultResource.DeepCopy(),
	}
	notPolled[0].UUID = "not-polled-id-1"
	notPolled[0].Name = "not-polled-1"
	notPolled[1].UUID = "not-polled-id-2"
	notPolled[1].Name = "not-polled-2"
	alreadyPolled := []resourceData{
		defaultResource.DeepCopy(),
		defaultResource.DeepCopy(),
	}
	alreadyPolled[0].UUID = "polled-id-1"
	alreadyPolled[0].Name = "polled-1"
	alreadyPolled[1].UUID = "polled-id-2"
	alreadyPolled[1].Name = "polled-2"
	for i := range alreadyPolled {
		alreadyPolled[i].PolledAt = previousPoll
		alreadyPolled[i].Revision = 1
	}

	newCharmUUID := "new-charm-uuid"

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// add the new charm
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, architecture_id, revision) VALUES (?, 'app',0, 1)
`, newCharmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		// populate charm resources table for existing resources
		for _, d := range append(notPolled, alreadyPolled...) {
			_, err = tx.Exec(`
INSERT INTO charm_resource (charm_uuid, name, kind_id, path, description)
VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
				newCharmUUID, d.Name, TypeID(d.Type), nilZero(d.Path), nilZero(d.Description))
			if err != nil {
				return errors.Capture(err)
			}
		}

		for _, input := range append(notPolled, alreadyPolled...) {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: update resource 1 and 2 (not 3)
	err = s.state.SetRepositoryResources(c.Context(), resource.SetRepositoryResourcesArgs{
		ApplicationID: application.ID(s.constants.fakeApplicationUUID1),
		CharmID:       charm.ID(newCharmUUID),
		Info: []charmresource.Resource{{
			Meta: charmresource.Meta{
				Name: "not-polled-1",
			},
			Revision: 2,
		}, {
			Meta: charmresource.Meta{
				Name: "polled-1",
			},
			Revision: 2,
		}},
		LastPolled: now,
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute TestSetRepositoryResource: %v", errors.ErrorStack(err)))

	// Assert
	type obtainedRow struct {
		ResourceUUID string
		Revision     int
		CharmID      string
		LastPolled   *time.Time
	}
	var obtained []obtainedRow
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT uuid, revision, charm_uuid, last_polled FROM resource WHERE state_id = 1`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row obtainedRow
			var revision *int
			if err := rows.Scan(&row.ResourceUUID, &revision, &row.CharmID, &row.LastPolled); err != nil {
				return err
			}
			if revision != nil {
				row.Revision = *revision
			}
			obtained = append(obtained, row)
		}
		return err
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to get expected changes in db: %v", errors.ErrorStack(err)))
	c.Assert(obtained, tc.SameContents, []obtainedRow{
		{
			ResourceUUID: "polled-id-1", // updated
			LastPolled:   &now,
			CharmID:      newCharmUUID,
			Revision:     2,
		},
		{
			ResourceUUID: "polled-id-2",
			Revision:     1,
			CharmID:      fakeCharmUUID,
			LastPolled:   &previousPoll, // not updated
		},
		{
			ResourceUUID: "not-polled-id-1", // created
			LastPolled:   &now,
			CharmID:      newCharmUUID,
			Revision:     2,
		},
		{
			ResourceUUID: "not-polled-id-2", // not polled
			LastPolled:   nil,
			CharmID:      fakeCharmUUID,
		},
	})
}

// TestSetRepositoryResourceUnknownResource validates that attempting to set
// repository resources for unknown resources logs the correct errors.
func (s *resourceSuite) TestSetRepositoryResourceUnknownResource(c *tc.C) {
	// Act: update non-existent resources
	err := s.state.SetRepositoryResources(c.Context(), resource.SetRepositoryResourcesArgs{
		ApplicationID: application.ID(s.constants.fakeApplicationUUID1),
		Info: []charmresource.Resource{{
			Meta: charmresource.Meta{
				Name: "not-a-resource-1",
			},
		}, {
			Meta: charmresource.Meta{
				Name: "not-a-resource-2",
			},
		}},
		LastPolled: time.Now(),
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute TestSetRepositoryResource: %v", errors.ErrorStack(err)))

	// Assert
	//c.Check(c.GetTestLog(), tc.Contains, fmt.Sprintf("Resource not found for application %s (%s)", s.constants.fakeApplicationName1,
	//	s.constants.fakeApplicationUUID1), tc.Commentf("(Assert) application not found in log"))
	//c.Check(c.GetTestLog(), tc.Contains, "not-a-resource-1", tc.Commentf("(Assert) missing resource name log"))
	//c.Check(c.GetTestLog(), tc.Contains, "not-a-resource-2", tc.Commentf("(Assert) missing resource name log"))
}

// TestSetRepositoryResourceApplicationNotFound verifies that setting repository
// resources for a non-existent application results in an ApplicationNotFound error.
func (s *resourceSuite) TestSetRepositoryResourceApplicationNotFound(c *tc.C) {
	// Act: request a non-existent application.
	err := s.state.SetRepositoryResources(c.Context(), resource.SetRepositoryResourcesArgs{
		ApplicationID: "not-an-application",
		Info:          []charmresource.Resource{{}}, // Non empty info
		LastPolled:    time.Now(),                   // not used
	})

	// Assert: check expected error
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationNotFound, tc.Commentf("(Act) unexpected error: %v", errors.ErrorStack(err)))
}

// TestRecordStoredResourceWithContainerImage tests recording that a container
// image resource has been stored.
func (s *resourceSuite) TestRecordStoredResourceWithContainerImage(c *tc.C) {
	// Arrange: Create a container image blob and resource record.
	resID, storeID, size, hash := s.createContainerImageResourceAndBlob(c)

	// Act: store the resource blob.
	retrievedBy := "retrieved-by-app"
	retrievedByType := coreresource.Application
	err := s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID:    resID,
			StorageID:       storeID,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
			ResourceType:    charmresource.TypeContainerImage,
			Size:            size,
			SHA384:          hash,
		},
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))

	// Assert: Check that the resource has been linked to the stored blob
	var (
		foundStorageKey, foundHash string
		foundSize                  int64
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT store_storage_key, size, sha384 FROM resource_image_store
WHERE resource_uuid = ?`, resID).Scan(&foundStorageKey, &foundSize, &foundHash)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) resource_image_store table not updated: %v", errors.ErrorStack(err)))
	storageKey, err := storeID.ContainerImageMetadataStoreID()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundStorageKey, tc.Equals, storageKey)

	// Assert: Check that retrieved by has been set.
	var foundRetrievedByType, foundRetrievedBy string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT rrb.name, rrbt.name AS type
FROM   resource_retrieved_by rrb
JOIN   resource_retrieved_by_type rrbt ON rrb.retrieved_by_type_id = rrbt.id
WHERE  resource_uuid = ?`, resID).Scan(&foundRetrievedBy, &foundRetrievedByType)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundRetrievedByType, tc.Equals, string(retrievedByType))
	c.Check(foundRetrievedBy, tc.Equals, retrievedBy)
	c.Check(foundHash, tc.Equals, hash)
	c.Check(foundSize, tc.Equals, size)
}

// TestRecordStoredResourceWithFile tests recording that a file resource has
// been stored.
func (s *resourceSuite) TestRecordStoredResourceWithFile(c *tc.C) {
	// Arrange: Create file resource.
	resID, storeID, size, hash := s.createFileResourceAndBlob(c)

	// Act: store the resource blob.
	retrievedBy := "retrieved-by-unit"
	retrievedByType := coreresource.Unit
	err := s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID:    resID,
			StorageID:       storeID,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
			ResourceType:    charmresource.TypeFile,
			Size:            size,
			SHA384:          hash,
		},
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))

	// Assert: Check that the resource has been linked to the stored blob
	var (
		foundStoreUUID, foundHash string
		foundSize                 int64
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT store_uuid, size, sha384 FROM resource_file_store
WHERE resource_uuid = ?`, resID).Scan(&foundStoreUUID, &foundSize, &foundHash)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) resource_file_store table not updated: %v", errors.ErrorStack(err)))
	objectStoreUUID, err := storeID.ObjectStoreUUID()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundStoreUUID, tc.Equals, objectStoreUUID.String())
	c.Check(foundHash, tc.Equals, hash)
	c.Check(foundSize, tc.Equals, size)

	// Assert: Check that retrieved by has been set.
	var foundRetrievedByType, foundRetrievedBy string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT rrb.name, rrbt.name AS type
FROM   resource_retrieved_by rrb
JOIN   resource_retrieved_by_type rrbt ON rrb.retrieved_by_type_id = rrbt.id
WHERE  resource_uuid = ?`, resID).Scan(&foundRetrievedBy, &foundRetrievedByType)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundRetrievedByType, tc.Equals, string(retrievedByType))
	c.Check(foundRetrievedBy, tc.Equals, retrievedBy)
}

// TestRecordStoredResourceIncrementCharmModifiedVersion checks that the charm
// modified version is incremented correctly when the indicator field is true,
// both from 0/NULL to 1 and after that.
func (s *resourceSuite) TestRecordStoredResourceIncrementCharmModifiedVersion(c *tc.C) {
	// Arrange: create two resources and  blobs storage and get the initial charm
	// modified version.
	resID, storeID, size, hash := s.createContainerImageResourceAndBlob(c)
	resID2, storeID2, size2, hash2 := s.createContainerImageResourceAndBlob(c)
	initialCharmModifiedVersion := s.getCharmModifiedVersion(c, resID.String())

	// Act: store the resource and increment the CMV.
	err := s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID:                  resID,
			StorageID:                     storeID,
			ResourceType:                  charmresource.TypeContainerImage,
			IncrementCharmModifiedVersion: true,
			SHA384:                        hash,
			Size:                          size,
		},
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))

	foundCharmModifiedVersion1 := s.getCharmModifiedVersion(c, resID.String())

	err = s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID:                  resID2,
			StorageID:                     storeID2,
			ResourceType:                  charmresource.TypeContainerImage,
			IncrementCharmModifiedVersion: true,
			SHA384:                        hash2,
			Size:                          size2,
		},
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))

	foundCharmModifiedVersion2 := s.getCharmModifiedVersion(c, resID2.String())

	// Assert: Check the charm modified version has been incremented.
	c.Assert(foundCharmModifiedVersion1, tc.Equals, initialCharmModifiedVersion+1)
	c.Assert(foundCharmModifiedVersion2, tc.Equals, initialCharmModifiedVersion+2)
}

// TestRecordStoredResourceDoNotIncrementCharmModifiedVersion checks that the
// charm modified version is not updated by RecordStoredResource if the variable
// is false.
func (s *resourceSuite) TestRecordStoredResourceDoNotIncrementCharmModifiedVersion(c *tc.C) {
	// Arrange: insert a resource and get charm modified version.
	resID, storeID, size, hash := s.createContainerImageResourceAndBlob(c)
	initialCharmModifiedVersion := s.getCharmModifiedVersion(c, resID.String())

	// Act: store the resource.
	err := s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID: resID,
			StorageID:    storeID,
			ResourceType: charmresource.TypeContainerImage,
			SHA384:       hash,
			Size:         size,
		},
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))

	// Assert: Check the charm modified version has not been incremented.
	foundCharmModifiedVersion := s.getCharmModifiedVersion(c, resID.String())
	c.Assert(foundCharmModifiedVersion, tc.Equals, initialCharmModifiedVersion)
}

func (s *resourceSuite) TestRecordStoredResourceWithContainerImageAlreadyStored(c *tc.C) {
	// Arrange: insert a resource record and generate 2 blobs.
	resID, storeID1, size1, hash1 := s.createContainerImageResourceAndBlob(c)
	retrievedBy1 := "ubuntu/0"
	retrievedByType1 := coreresource.Unit

	// Arrange: store the first resource.
	err := s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID:    resID,
			StorageID:       storeID1,
			ResourceType:    charmresource.TypeContainerImage,
			SHA384:          hash1,
			Size:            size1,
			RetrievedBy:     retrievedBy1,
			RetrievedByType: retrievedByType1,
		},
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))

	storageKey2 := "storage-key-2"
	storeID2 := resourcestoretesting.GenContainerImageMetadataResourceID(c, storageKey2)
	retrievedBy2 := "user-name"
	retrievedByType2 := coreresource.User
	size2 := int64(422)
	hash2 := "hash2"
	err = s.addContainerImage(c, storageKey2)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to add container image: %v", errors.ErrorStack(err)))

	// Act: try to store a second resource.
	err = s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID:    resID,
			StorageID:       storeID2,
			ResourceType:    charmresource.TypeContainerImage,
			SHA384:          hash2,
			Size:            size2,
			RetrievedBy:     retrievedBy2,
			RetrievedByType: retrievedByType2,
		},
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.StoredResourceAlreadyExists)
}

func (s *resourceSuite) TestStoreWithFileResourceAlreadyStored(c *tc.C) {
	// Arrange: insert a resource.
	resID, storeID1, size1, hash1 := s.createFileResourceAndBlob(c)
	retrievedBy1 := "ubuntu/0"
	retrievedByType1 := coreresource.Unit

	// Arrange: store the first resource.
	err := s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID:    resID,
			StorageID:       storeID1,
			ResourceType:    charmresource.TypeFile,
			RetrievedBy:     retrievedBy1,
			RetrievedByType: retrievedByType1,
			SHA384:          hash1,
			Size:            size1,
		},
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))

	objectStoreUUID2 := objectstoretesting.GenObjectStoreUUID(c)
	storeID2 := resourcestoretesting.GenFileResourceStoreID(c, objectStoreUUID2)
	retrievedBy2 := "ubuntu/0"
	retrievedByType2 := coreresource.Unit
	size2 := int64(422)
	hash2 := "hash2"
	err = s.addObjectStoreBlobMetadata(c, objectStoreUUID2)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to add object store blob: %v", errors.ErrorStack(err)))

	// Act: try and store the second resource.
	err = s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID:    resID,
			StorageID:       storeID2,
			ResourceType:    charmresource.TypeFile,
			RetrievedBy:     retrievedBy2,
			RetrievedByType: retrievedByType2,
			SHA384:          hash2,
			Size:            size2,
		},
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.StoredResourceAlreadyExists)
}

func (s *resourceSuite) TestRecordStoredResourceSameBlobAlreadyStoredContainerImage(c *tc.C) {
	// Arrange: insert a resource record and generate 2 blobs.
	resID, storeID, size, hash := s.createContainerImageResourceAndBlob(c)

	// Arrange: store the first resource.
	err := s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID: resID,
			StorageID:    storeID,
			ResourceType: charmresource.TypeContainerImage,
			SHA384:       hash,
			Size:         size,
		},
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))

	// Act: try to store the same resource again.
	err = s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID: resID,
			StorageID:    storeID,
			ResourceType: charmresource.TypeContainerImage,
			SHA384:       hash,
			Size:         size,
		},
	)
	// Assert: That when record a blob twice, the second recording does not
	// return an error.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceSuite) TestRecordStoredResourceSameBlobAlreadyStoredFile(c *tc.C) {
	// Arrange: insert a resource record and generate 2 blobs.
	resID, storeID, size, hash := s.createFileResourceAndBlob(c)

	// Arrange: store the first resource.
	err := s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID: resID,
			StorageID:    storeID,
			ResourceType: charmresource.TypeFile,
			SHA384:       hash,
			Size:         size,
		},
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))

	// Act: try to store the same resource again.
	err = s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID: resID,
			StorageID:    storeID,
			ResourceType: charmresource.TypeFile,
			SHA384:       hash,
			Size:         size,
		},
	)
	// Assert: That when record a blob twice, the second recording does not
	// return an error.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceSuite) TestRecordStoredResourceFileStoredResourceNotFoundInObjectStore(c *tc.C) {
	// Arrange: insert a resource.
	resID := s.addResource(c, charmresource.TypeFile)

	// Arrange: generate a valid store ID.
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)
	storeID := resourcestoretesting.GenFileResourceStoreID(c, objectStoreUUID)

	// Act: try and store the resource.
	err := s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID: resID,
			StorageID:    storeID,
			ResourceType: charmresource.TypeFile,
		},
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.StoredResourceNotFound)
}

func (s *resourceSuite) TestRecordStoredResourceContainerImageStoredResourceNotFoundInStore(c *tc.C) {
	// Arrange: insert a resource and generate a valid store ID.
	resID := s.addResource(c, charmresource.TypeContainerImage)
	storeID := resourcestoretesting.GenContainerImageMetadataResourceID(c, "bad-storage-key")

	// Act: try and store the resource.
	err := s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID: resID,
			StorageID:    storeID,
			ResourceType: charmresource.TypeContainerImage,
		},
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.StoredResourceNotFound)
}

func (s *resourceSuite) TestRecordStoredResourceWithRetrievedByUnit(c *tc.C) {
	resourceUUID := s.addResource(c, charmresource.TypeFile)
	retrievedBy := "app-test/0"
	retrievedByType := coreresource.Unit
	err := s.setWithRetrievedBy(c, resourceUUID, retrievedBy, retrievedByType)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))
	foundRetrievedBy, foundRetrievedByType := s.getRetrievedByType(c, resourceUUID)
	c.Check(foundRetrievedBy, tc.Equals, retrievedBy)
	c.Check(foundRetrievedByType, tc.Equals, retrievedByType)
}

func (s *resourceSuite) TestRecordStoredResourceWithRetrievedByApplication(c *tc.C) {
	resourceUUID := s.addResource(c, charmresource.TypeFile)
	retrievedBy := "app-test"
	retrievedByType := coreresource.Application
	err := s.setWithRetrievedBy(c, resourceUUID, retrievedBy, retrievedByType)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))
	foundRetrievedBy, foundRetrievedByType := s.getRetrievedByType(c, resourceUUID)
	c.Check(foundRetrievedBy, tc.Equals, retrievedBy)
	c.Check(foundRetrievedByType, tc.Equals, retrievedByType)
}

func (s *resourceSuite) TestRecordStoredResourceWithRetrievedByUser(c *tc.C) {
	resourceUUID := s.addResource(c, charmresource.TypeFile)
	retrievedBy := "jim"
	retrievedByType := coreresource.User
	err := s.setWithRetrievedBy(c, resourceUUID, retrievedBy, retrievedByType)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute RecordStoredResource: %v", errors.ErrorStack(err)))
	foundRetrievedBy, foundRetrievedByType := s.getRetrievedByType(c, resourceUUID)
	c.Check(foundRetrievedBy, tc.Equals, retrievedBy)
	c.Check(foundRetrievedByType, tc.Equals, retrievedByType)
}

func (s *resourceSuite) TestRecordStoredResourceWithRetrievedByNotSet(c *tc.C) {
	// Retrieve by should not be set if it is blank and the type is unknown.
	resourceUUID := s.addResource(c, charmresource.TypeFile)
	retrievedBy := ""
	retrievedByType := coreresource.Unknown
	err := s.setWithRetrievedBy(c, resourceUUID, retrievedBy, retrievedByType)
	c.Assert(err, tc.ErrorIsNil)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT rrb.name, rrbt.name AS type
FROM   resource_retrieved_by rrb
JOIN   resource_retrieved_by_type rrbt ON rrb.retrieved_by_type_id = rrbt.id
WHERE  resource_uuid = ?`, resourceUUID.String()).Scan(&retrievedBy, &retrievedByType)
	})
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
}

// TestSetUnitResource verifies that a unit resource is correctly set.
func (s *resourceSuite) TestSetUnitResource(c *tc.C) {
	// Arrange: insert a resource.
	startTime := time.Now().Truncate(time.Second).UTC()
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       startTime,
		Name:            "resource-name",
		Type:            charmresource.TypeFile,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with application type
	err = s.state.SetUnitResource(
		c.Context(),
		coreresource.UUID(resID),
		unit.UUID(s.constants.fakeUnitUUID1),
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute SetUnitResource: %v", errors.ErrorStack(err)))

	// Assert: check the unit resource has been added.
	var addedAt time.Time
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT added_at FROM unit_resource
WHERE resource_uuid = ? and unit_uuid = ?`,
			resID, s.constants.fakeUnitUUID1).Scan(&addedAt)
	})
	c.Check(addedAt, tc.TimeBetween(startTime, time.Now()))
	c.Check(err, tc.ErrorIsNil, tc.Commentf("(Assert) unit_resource table not updated: %v", errors.ErrorStack(err)))
}

// TestSetUnitResourceUnsetExisting verifies that a unit resource is set and
// an existing resource with the same charm resource as the new one is unset.
func (s *resourceSuite) TestSetUnitResourceUnsetExisting(c *tc.C) {
	// Arrange: insert a resource link it to a unit then insert a second
	// resource without linking.
	startTime := time.Now().Truncate(time.Second).UTC()
	time1 := startTime.Add(time.Hour * -1)
	resID1 := "resource-id-1"
	resID2 := "resource-id-2"
	input1 := resourceData{
		UUID:            resID1,
		UnitUUID:        s.constants.fakeUnitUUID1,
		AddedAt:         time1,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            charmresource.TypeFile,
	}

	input2 := resourceData{
		UUID:            resID2,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            charmresource.TypeFile,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input1.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		if err := input2.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set unit resource and check the old resource has been reset.
	err = s.state.SetUnitResource(
		c.Context(),
		coreresource.UUID(resID2),
		unit.UUID(s.constants.fakeUnitUUID1),
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute SetUnitResource: %v", errors.ErrorStack(err)))

	// Assert: check the unit resource has been added and the old one removed.
	var addedAts []time.Time
	var uuids []string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query(`
SELECT added_at, resource_uuid FROM unit_resource
WHERE  unit_uuid = ?`, s.constants.fakeUnitUUID1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var addedAt time.Time
			var uuid string
			err := rows.Scan(&addedAt, &uuid)
			if err != nil {
				return err
			}
			addedAts = append(addedAts, addedAt)
			uuids = append(uuids, uuid)
		}
		return nil
	})
	c.Check(err, tc.ErrorIsNil, tc.Commentf("(Assert) cannot check unit_resource table: %v", errors.ErrorStack(err)))
	c.Check(uuids, tc.DeepEquals, []string{resID2})
	c.Assert(addedAts, tc.HasLen, 1)
	c.Check(addedAts[0], tc.TimeBetween(startTime, time.Now()))
}

// TestSetUnitResourceUnsetExistingOtherUnits verifies that setting a unit
// resource that unsets an old one doesn't affect other units using the same
// resource.
func (s *resourceSuite) TestSetUnitResourceUnsetExistingOtherUnits(c *tc.C) {
	// Arrange: insert a resource link it to a unit then insert a second
	// resource without linking.
	startTime := time.Now().Truncate(time.Second).UTC()
	time1 := startTime.Add(time.Hour * -1)
	resID1 := "resource-id-1"
	resID2 := "resource-id-2"
	inputUnit1 := resourceData{
		UUID:            resID1,
		UnitUUID:        s.constants.fakeUnitUUID1,
		AddedAt:         time1,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            charmresource.TypeFile,
	}

	inputUnit2 := resourceData{
		UUID:            resID1,
		UnitUUID:        s.constants.fakeUnitUUID2,
		AddedAt:         time1,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            charmresource.TypeFile,
	}

	inputNewResource := resourceData{
		UUID:            resID2,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            charmresource.TypeFile,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := inputUnit1.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		if err := inputUnit2.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		if err := inputNewResource.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set unit resource, this should remove the old resource from unit 1.
	err = s.state.SetUnitResource(
		c.Context(),
		coreresource.UUID(resID2),
		unit.UUID(s.constants.fakeUnitUUID1),
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute SetUnitResource: %v", errors.ErrorStack(err)))

	// Assert: check the unit resource has been added and the old one removed.
	var unit1AddedAt time.Time
	var unit1ResUUID string
	var unit2AddedAt time.Time
	var unit2ResUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT added_at, resource_uuid FROM unit_resource
WHERE  unit_uuid = ?`, s.constants.fakeUnitUUID1).Scan(&unit1AddedAt, &unit1ResUUID)
		if err != nil {
			return err
		}
		// Check the second unit is unaffected
		return tx.QueryRow(`
SELECT added_at, resource_uuid FROM unit_resource
WHERE  unit_uuid = ?`, s.constants.fakeUnitUUID2).Scan(&unit2AddedAt, &unit2ResUUID)
	})
	c.Check(err, tc.ErrorIsNil, tc.Commentf("(Assert) cannot check unit_resource table: %v", errors.ErrorStack(err)))
	c.Check(unit1ResUUID, tc.Equals, resID2)
	c.Check(unit1AddedAt, tc.TimeBetween(startTime, time.Now()))

	c.Check(unit2AddedAt, tc.Equals, time1)
	c.Check(unit2ResUUID, tc.Equals, resID1)
}
func (s *resourceSuite) TestSetUnitResourceNotFound(c *tc.C) {
	// Act set supplied by with application type
	err := s.state.SetUnitResource(
		c.Context(),
		"bad-uuid",
		unit.UUID(s.constants.fakeUnitUUID1),
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNotFound)
}

// TestSetUnitResourceAlreadySet checks if set unit resource correctly
// identifies an already set resource and skips updating.
func (s *resourceSuite) TestSetUnitResourceAlreadySet(c *tc.C) {
	// Arrange: insert a resource and data implying that everything is already
	// set.
	now := time.Now().Truncate(time.Second).UTC()
	previousInsertTime := now.Add(-1 * time.Hour)
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		Name:            "resource-name",
		UnitUUID:        s.constants.fakeUnitUUID1,
		AddedAt:         previousInsertTime,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return errors.Capture(input.insert(c.Context(), tx))
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with user type
	err = s.state.SetUnitResource(c.Context(),
		coreresource.UUID(resID),
		unit.UUID(s.constants.fakeUnitUUID1),
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to execute SetUnitResource: %v", errors.ErrorStack(err)))

	// Assert
	var addedAt time.Time
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT added_at FROM unit_resource
WHERE resource_uuid = ? and unit_uuid = ?`,
			resID, s.constants.fakeUnitUUID1).Scan(&addedAt)
	})
	c.Check(addedAt, tc.Equals, previousInsertTime)
	c.Check(err, tc.ErrorIsNil, tc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
}

// TestSetUnitResourceUnitNotFound tests that setting a unit resource with an
// unexpected unit ID results in an error.
func (s *resourceSuite) TestSetUnitResourceUnitNotFound(c *tc.C) {
	// Arrange: insert a resource
	now := time.Now().Truncate(time.Second).UTC()
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		Name:            "resource-name",
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return errors.Capture(input.insert(c.Context(), tx))
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with unexpected unit
	err = s.state.SetUnitResource(c.Context(),
		coreresource.UUID(resID),
		"unexpected-unit-id",
	)

	// Assert: an error is returned, nothing is updated in the db
	c.Check(err, tc.ErrorIs, resourceerrors.UnitNotFound)
	err = s.runQuery(c, `SELECT * FROM unit_resource`)
	c.Check(err, tc.ErrorIs, sql.ErrNoRows, tc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
	err = s.runQuery(c, `SELECT * FROM resource_retrieved_by`)
	c.Check(err, tc.ErrorIs, sql.ErrNoRows, tc.Commentf("(Assert) resource_retrieved_by table has been updated: %v", errors.ErrorStack(err)))
}

func (s *resourceSuite) TestGetResourceTypeContainerImage(c *tc.C) {
	// Arrange: insert a resource.
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            charmresource.TypeContainerImage,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: Get the resource type.
	resourceType, err := s.state.GetResourceType(
		c.Context(),
		coreresource.UUID(resID),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resourceType, tc.Equals, charmresource.TypeContainerImage)
}

func (s *resourceSuite) TestGetResourceTypeFile(c *tc.C) {
	// Arrange: insert a resource.
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            charmresource.TypeFile,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: Get the resource type.
	resourceType, err := s.state.GetResourceType(
		c.Context(),
		coreresource.UUID(resID),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resourceType, tc.Equals, charmresource.TypeFile)
}

func (s *resourceSuite) TestGetResourceTypeNotFound(c *tc.C) {
	// Arrange: Make fake resource-uuid.
	resID := "resource-id"

	// Act: Get the resource type.
	_, err := s.state.GetResourceType(
		c.Context(),
		coreresource.UUID(resID),
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNotFound)
}

// TestListResourcesNoResources verifies that no resources are listed for an
// application when no resources exist. It checks that the resulting lists for
// unit resources, general resources, and repository resources are all empty.
func (s *resourceSuite) TestListResourcesNoResources(c *tc.C) {
	// Arrange: No resources
	// Act
	results, err := s.state.ListResources(c.Context(), application.ID(s.constants.fakeApplicationUUID1))
	// Assert
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Check(results.UnitResources, tc.DeepEquals, []coreresource.UnitResources{
		{
			Name: unit.Name(s.constants.fakeUnitName1),
			// No resources
		},
		{
			Name: unit.Name(s.constants.fakeUnitName2),
			// No resources
		},
		{
			Name: unit.Name(s.constants.fakeUnitName3),
			// No resources
		},
	})
	c.Check(results.Resources, tc.HasLen, 0)
	c.Check(results.RepositoryResources, tc.HasLen, 0)
}

// TestListResources tests the retrieval and organization of resources from the
// database.
func (s *resourceSuite) TestListResources(c *tc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	// Arrange : Insert several resources
	// - 1 with no unit (state available)
	// - 1 with no unit (state potential)
	// - 1 with no unit (state potential, but without revision)
	// - 1 associated with two units (state available)
	// - 1 with the same name as above, no unit (state potential)
	// - 1 associated with one unit (state available)
	noUnitAvailableRes := resourceData{
		UUID:            "no-unit-available-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "no-unit",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
	}
	noUnitPotentialRes := resourceData{
		UUID:            "no-unit-potential-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "no-unit",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
		State:           resource.StatePotential.String(),
		Revision:        2,
	}
	noUnitPotentialNoRevRes := resourceData{
		UUID:            "no-unit-potential-placedholder-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "no-unit-placeholder",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
		State:           resource.StatePotential.String(),
		// No revision
	}
	withUnit1AvailableRes := resourceData{
		UUID:            "with-unit-available-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "with-unit",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
		UnitUUID:        s.constants.fakeUnitUUID1,
	}
	withUnit2AvailableRes := resourceData{
		UUID: "with-unit-available-no-app-uuid",
		// this one is not linked to the application (maybe it has been updated)
		Name:      "with-unit",
		CreatedAt: now,
		Type:      charmresource.TypeFile,
		UnitUUID:  s.constants.fakeUnitUUID2,
	}
	withUnitPotentialRes := resourceData{
		UUID:            "with-unit-potential-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "with-unit",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
		State:           resource.StatePotential.String(),
		Revision:        2,
	}
	withUnitBisAvailableRes := resourceData{
		UUID:            "with-unit-bis-available-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "with-unit-bis",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
		UnitUUID:        s.constants.fakeUnitUUID1,
	}

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, input := range []resourceData{
			noUnitPotentialNoRevRes,
			noUnitAvailableRes,
			noUnitPotentialRes,
			withUnit1AvailableRes,
			withUnit2AvailableRes,
			withUnitPotentialRes,
			withUnitBisAvailableRes} {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	results, err := s.state.ListResources(c.Context(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert
	// the application, even if not directly linked to this unit resource, should be properly retrieved
	withUnit2AvailableRes.ApplicationUUID = s.constants.fakeApplicationUUID1
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Check(results.UnitResources, tc.DeepEquals, []coreresource.UnitResources{
		{
			Name: unit.Name(s.constants.fakeUnitName1),
			Resources: []coreresource.Resource{
				withUnit1AvailableRes.toResource(s, c),
				withUnitBisAvailableRes.toResource(s, c),
			},
		},
		{
			Name: unit.Name(s.constants.fakeUnitName2),
			Resources: []coreresource.Resource{
				withUnit2AvailableRes.toResource(s, c),
			},
		},
		{
			Name: unit.Name(s.constants.fakeUnitName3),
			// No resources
		},
	})
	c.Check(results.Resources, tc.DeepEquals, []coreresource.Resource{
		noUnitAvailableRes.toResource(s, c),
		withUnit1AvailableRes.toResource(s, c),
		// withUnit2AvailableRes is the same resource as above on another unit
		withUnitBisAvailableRes.toResource(s, c),
	})
	c.Check(results.RepositoryResources, tc.DeepEquals, []charmresource.Resource{
		noUnitPotentialRes.toCharmResource(c),
		withUnitPotentialRes.toCharmResource(c),
	})
}

// TestGetResourcesByApplicationIDWrongApplicationID verifies the behavior
// when querying non-existing application ID.
// Ensures the method returns the correct `ApplicationNotFound` error for an
// invalid application ID.
func (s *resourceSuite) TestGetResourcesByApplicationIDWrongApplicationID(c *tc.C) {
	// Arrange: No resources
	// Act
	_, err := s.state.GetResourcesByApplicationID(c.Context(), "not-an-application-id")
	// Assert
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationNotFound,
		tc.Commentf("(Assert) should fails with specific error: %v",
			errors.ErrorStack(err)))
}

// TestGetResourcesByApplicationIDNoResources verifies that no resources are listed for an
// application when no resources exist. It checks that the resulting lists
// is empty.
func (s *resourceSuite) TestGetResourcesByApplicationIDNoResources(c *tc.C) {
	// Arrange: No resources
	// Act
	results, err := s.state.GetResourcesByApplicationID(c.Context(), application.ID(s.constants.fakeApplicationUUID1))
	// Assert
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Assert(results, tc.HasLen, 0)
}

// TestGetResourcesByApplicationID tests the retrieval and organization of resources from the
// database.
func (s *resourceSuite) TestGetResourcesByApplicationID(c *tc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	// Arrange : Insert several resources
	simpleRes := resourceData{
		UUID:            "simple-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "simple",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
	}
	polledRes := resourceData{
		UUID:            "polled-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "polled",
		CreatedAt:       now,
		PolledAt:        now,
		Type:            charmresource.TypeContainerImage,
	}
	unitRes := resourceData{
		UUID:            "unit-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "unit",
		CreatedAt:       now,
		UnitUUID:        s.constants.fakeUnitUUID1,
		AddedAt:         now,
		Type:            charmresource.TypeFile,
	}
	bothRes := resourceData{
		UUID:            "both-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "both",
		UnitUUID:        s.constants.fakeUnitUUID1,
		AddedAt:         now,
		PolledAt:        now,
		Type:            charmresource.TypeContainerImage,
	}
	anotherUnitRes := resourceData{
		UUID:            "another-unit-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "anotherUnit",
		CreatedAt:       now,
		UnitUUID:        s.constants.fakeUnitUUID2,
		AddedAt:         now,
		Type:            charmresource.TypeFile,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, input := range []resourceData{simpleRes, polledRes, unitRes, bothRes, anotherUnitRes} {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	results, err := s.state.GetResourcesByApplicationID(c.Context(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Assert(results, tc.DeepEquals, []coreresource.Resource{
		simpleRes.toResource(s, c),
		polledRes.toResource(s, c),
		unitRes.toResource(s, c),
		bothRes.toResource(s, c),
		anotherUnitRes.toResource(s, c),
	})
}

// TestGetResourcesByApplicationIDWithStatePotential tests retrieving resources
// by application ID where state filters are applied. It ensures that only
// resources with the "available" state are returned, excluding any with the
// "potential" state.
func (s *resourceSuite) TestGetResourcesByApplicationIDWithStatePotential(c *tc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	// Arrange : Insert several resources
	availableRes := resourceData{
		UUID:            "simple-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "simple",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
		State:           "available",
	}
	potentialRes := resourceData{
		UUID:            "simple-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "simple",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
		State:           "potential",
	}

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, input := range []resourceData{availableRes, potentialRes} {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	results, err := s.state.GetResourcesByApplicationID(c.Context(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Assert(results, tc.DeepEquals, []coreresource.Resource{
		availableRes.toResource(s, c),
		// potential resources are not returned
	})
}

// TestAddResourcesBeforeApplication tests inserting given resource docs
// referencing the given charm and linking them to an application name for
// later resolution.
func (s *resourceSuite) TestAddResourcesBeforeApplication(c *tc.C) {
	// Setup charm resources only
	charmUUID := testing.GenCharmID(c)
	data := []resourceData{
		{
			CharmUUID:   charmUUID.String(),
			Name:        "one",
			Type:        charmresource.TypeFile,
			Path:        "/tmp/one.txt",
			Description: "testing",
		}, {
			CharmUUID:   charmUUID.String(),
			Name:        "two",
			Type:        charmresource.TypeFile,
			Path:        "/tmp/two.txt",
			Description: "testing",
		},
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		if err = insertCharmStateWithRevision(ctx, tx, charmUUID.String(), 42); err != nil {
			return err
		}
		for _, input := range data {
			if err := input.insertCharmResource(tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Define AddResourcesBeforeApplication arguments
	rev := 7
	args := resource.AddResourcesBeforeApplicationArgs{
		ApplicationName: "test-app",
		CharmLocator: applicationcharm.CharmLocator{
			Name:     "ubuntu",
			Revision: 42,
			Source:   applicationcharm.CharmHubSource,
		},
		ResourceDetails: []resource.AddResourceDetails{
			{
				Name:     data[0].Name,
				Origin:   charmresource.OriginStore,
				Revision: &rev,
			}, {
				Name:   data[1].Name,
				Origin: charmresource.OriginUpload,
			},
		},
	}

	// Run the command and validate results.
	obtainedUUIDs, err := s.state.AddResourcesBeforeApplication(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedUUIDs, tc.HasLen, 2)
	for i, resID := range obtainedUUIDs {
		s.checkPendingApplication(c, resID.String(), args.ApplicationName)
		obtainedResource, err := s.getPendingResource(c, resID.String())
		if !c.Check(err, tc.ErrorIsNil) {
			continue
		}
		c.Check(obtainedResource.CharmUUID, tc.Equals, charmUUID.String())
		c.Check(obtainedResource.UUID, tc.Equals, resID.String())
		c.Check(obtainedResource.Name, tc.Equals, args.ResourceDetails[i].Name)
		c.Check(obtainedResource.OriginType, tc.Equals, args.ResourceDetails[i].Origin.String())
		if args.ResourceDetails[i].Revision != nil {
			c.Check(obtainedResource.Revision, tc.Equals, *args.ResourceDetails[i].Revision)
		}
	}
}

// TestAddResourcesBeforeApplicationNotFound tests inserting given resource
// docs referencing the given charm and linking them to an application name
// for later resolution in the case where the charm does not exist yet.
func (s *resourceSuite) TestAddResourcesBeforeApplicationNotFound(c *tc.C) {
	rev := 7
	args := resource.AddResourcesBeforeApplicationArgs{
		ApplicationName: "test-app",
		CharmLocator: applicationcharm.CharmLocator{
			Name:     "ubuntu",
			Revision: 42,
			Source:   applicationcharm.CharmHubSource,
		},
		ResourceDetails: []resource.AddResourceDetails{
			{
				Name:     "one",
				Origin:   charmresource.OriginStore,
				Revision: &rev,
			}, {
				Name:   "two",
				Origin: charmresource.OriginUpload,
			},
		},
	}

	// Run the command and validate the error.
	_, err := s.state.AddResourcesBeforeApplication(c.Context(), args)
	c.Assert(err, tc.ErrorIs, resourceerrors.CharmResourceNotFound)
}

func (s *resourceSuite) getPendingResource(c *tc.C, resID string) (pendingResourceTest, error) {
	retVal := pendingResourceTest{}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT r.uuid, r.charm_uuid, r.charm_resource_name, IFNULL(r.revision , 0) , rot.name
FROM   resource r
JOIN   resource_origin_type rot ON r.origin_type_id = rot.id
WHERE  r.uuid = ?`, resID).Scan(&retVal.UUID, &retVal.CharmUUID, &retVal.Name, &retVal.Revision, &retVal.OriginType)
	})

	return retVal, err
}

// pendingResourceTest represents data to be verified when testing
// the AddPendingResource method.
type pendingResourceTest struct {
	UUID       string
	CharmUUID  string
	Name       string
	Revision   int
	OriginType string
}

func (s *resourceSuite) checkPendingApplication(c *tc.C, resID, expectedAppName string) {
	var obtainedAppName string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		return tx.QueryRow(`
SELECT application_name
FROM   pending_application_resource
WHERE  resource_uuid = ?`,
			resID,
		).Scan(&obtainedAppName)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedAppName, tc.Equals, expectedAppName)
}

// TestUpdateResourceRevisionAndDeletePriorVersion tests that a resource
// revision and type are updated via the UpdateResourceRevisionAndDeletePriorVersion
// method. Check that the application's charm modified version is also
// incremented. Verify the resource file record has been deleted and the
// correct hash returned.
func (s *resourceSuite) TestUpdateResourceRevisionAndDeletePriorVersionFile(c *tc.C) {
	// Arrange : a simple resource
	resID := coreresource.UUID("resource-id")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	expected := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: charmresource.TypeFile,
			},
			Fingerprint: fp,
			Size:        42,
			// origin is upload by default if not specified in test input value
			Origin: charmresource.OriginUpload,
		},
		UUID:            resID,
		ApplicationName: s.constants.fakeApplicationName1,
	}
	input := resourceData{
		UUID:            resID.String(),
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            expected.Type,
		ObjectStoreUUID: "object-store-uuid",
		Size:            int(expected.Size),
		SHA384:          expected.Fingerprint.String(),
	}
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := input.insert(c.Context(), tx)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	expectedCharmModifiedVersion := s.getCharmModifiedVersion(c, resID.String()) + 1
	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resID,
		Revision:     5,
	}

	obtainedUUID, err := s.state.UpdateResourceRevisionAndDeletePriorVersion(c.Context(), args, charmresource.TypeFile)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedUUID, tc.Not(tc.Equals), resID)

	obtainedCharmModifiedVersion := s.getCharmModifiedVersion(c, obtainedUUID.String())
	c.Check(obtainedCharmModifiedVersion, tc.Equals, expectedCharmModifiedVersion)
	s.checkResourceOriginAndRevision(c, obtainedUUID.String(), "store", 5)
	// Assert: Check that the resource has been remove from the stored blob
	s.checkResourceFileStore(c, resID)
}

func (s *resourceSuite) checkResourceFileStore(c *tc.C, resID coreresource.UUID) {
	// Assert: Check that the resource has been remove from the stored blob
	var (
		foundStoreUUID string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT store_uuid
FROM   resource_file_store
WHERE  resource_uuid = ?`, resID).Scan(&foundStoreUUID)
	})
	c.Check(err, tc.ErrorMatches, "sql: no rows in result set")
}

// TestUpdateResourceRevisionAndDeletePriorVersionImage tests that a resource
// revision and type are updated via the UpdateResourceRevisionAndDeletePriorVersion
// method. Check that the application's charm modified version is also incremented.
// Verify the resource image record has been deleted and the correct hash returned.
func (s *resourceSuite) TestUpdateResourceRevisionAndDeletePriorVersionImage(c *tc.C) {
	// Arrange : a simple resource
	resID := coreresource.UUID("resource-id")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	expected := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: charmresource.TypeContainerImage,
			},
			Fingerprint: fp,
			Size:        42,
			// origin is upload by default if not specified in test input value
			Origin: charmresource.OriginUpload,
		},
		UUID:            resID,
		ApplicationName: s.constants.fakeApplicationName1,
	}
	input := resourceData{
		UUID:                     resID.String(),
		ApplicationUUID:          s.constants.fakeApplicationUUID1,
		Type:                     expected.Type,
		ContainerImageStorageKey: "file-store-uuid",
		Size:                     int(expected.Size),
		SHA384:                   expected.Fingerprint.String(),
	}
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := input.insert(c.Context(), tx)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	expectedCharmModifiedVersion := s.getCharmModifiedVersion(c, resID.String()) + 1
	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resID,
		Revision:     5,
	}

	obtainedUUID, err := s.state.UpdateResourceRevisionAndDeletePriorVersion(c.Context(), args, charmresource.TypeContainerImage)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedUUID, tc.Not(tc.Equals), resID)

	obtainedCharmModifiedVersion := s.getCharmModifiedVersion(c, obtainedUUID.String())
	c.Check(obtainedCharmModifiedVersion, tc.Equals, expectedCharmModifiedVersion)
	s.checkResourceOriginAndRevision(c, obtainedUUID.String(), charmresource.OriginStore.String(), 5)
	s.checkResourceImageStore(c, resID)
}

func (s *resourceSuite) checkResourceImageStore(c *tc.C, resID coreresource.UUID) {
	// Assert: Check that the resource has been remove from the stored blob
	var (
		foundStoreUUID string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT store_storage_key
FROM   resource_image_store
WHERE  resource_uuid = ?`, resID).Scan(&foundStoreUUID)
	})
	c.Check(err, tc.ErrorMatches, "sql: no rows in result set")
}

// TestUpdateResourceRevisionAndDeletePriorVersionFileNotStored tests that a
// resource revision and type are updated via the UpdateResourceRevisionAndDeletePriorVersion
// method. Check that the application's charm modified version is also incremented.
func (s *resourceSuite) TestUpdateResourceRevisionAndDeletePriorVersionFileNotStored(c *tc.C) {
	// Arrange : a simple resource
	resID := s.addResourceWithOrigin(c, charmresource.TypeFile, "upload")

	expectedCharmModifiedVersion := s.getCharmModifiedVersion(c, resID.String()) + 1
	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resID,
		Revision:     5,
	}

	// Assert: Check that the resource file store record does not exist
	// before running the test.
	var (
		foundStoreUUID string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT store_uuid
FROM   resource_file_store
WHERE  resource_uuid = ?`, resID).Scan(&foundStoreUUID)
	})
	c.Assert(err, tc.ErrorIs, sqlair.ErrNoRows)

	obtainedUUID, err := s.state.UpdateResourceRevisionAndDeletePriorVersion(c.Context(), args, charmresource.TypeFile)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedUUID, tc.Not(tc.Equals), resID)

	obtainedCharmModifiedVersion := s.getCharmModifiedVersion(c, obtainedUUID.String())
	c.Check(obtainedCharmModifiedVersion, tc.Equals, expectedCharmModifiedVersion)
	s.checkResourceOriginAndRevision(c, obtainedUUID.String(), charmresource.OriginStore.String(), 5)
}

// TestUpdateResourceRevisionAndDeletePriorVersionImageNotStored tests that a
// resource revision and type are updated via the UpdateResourceRevisionAndDeletePriorVersion
// method. Check that the application's charm modified version is also incremented.
func (s *resourceSuite) TestUpdateResourceRevisionAndDeletePriorVersionImageNotStored(c *tc.C) {
	// Arrange : a simple resource
	resID := s.addResourceWithOrigin(c, charmresource.TypeContainerImage, "upload")

	expectedCharmModifiedVersion := s.getCharmModifiedVersion(c, resID.String()) + 1
	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resID,
		Revision:     5,
	}

	// Assert: Check that the resource image store record does not exist
	// before running the test.
	var (
		foundStoreUUID string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT store_storage_key
FROM   resource_image_store
WHERE  resource_uuid = ?`, resID).Scan(&foundStoreUUID)
	})
	c.Check(err, tc.ErrorMatches, "sql: no rows in result set")

	obtainedUUID, err := s.state.UpdateResourceRevisionAndDeletePriorVersion(c.Context(), args, charmresource.TypeContainerImage)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedUUID, tc.Not(tc.Equals), resID)

	obtainedCharmModifiedVersion := s.getCharmModifiedVersion(c, obtainedUUID.String())
	c.Check(obtainedCharmModifiedVersion, tc.Equals, expectedCharmModifiedVersion)
	s.checkResourceOriginAndRevision(c, obtainedUUID.String(), "store", 5)
}

// TestUpdateResourceStoreToUpload tests updating a resource with origin store,
// to a resource with origin upload.
func (s *resourceSuite) TestUpdateUploadResourceAndDeletePriorVersionUpload(c *tc.C) {
	s.testUpdateUploadResourceAndDeletePriorVersion(c, charmresource.OriginUpload)
}

// TestUpdateResourceStoreToUpload tests updating a resource with origin store,
// to a resource with origin upload. Start with a store origin and revision
func (s *resourceSuite) TestUpdateUploadResourceAndDeletePriorVersionRevision(c *tc.C) {
	s.testUpdateUploadResourceAndDeletePriorVersion(c, charmresource.OriginStore)
}

func (s *resourceSuite) testUpdateUploadResourceAndDeletePriorVersion(c *tc.C, origin charmresource.Origin) {
	// Arrange: a resource to update.
	originalUUID := coreresource.UUID("resource-id")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	input := resourceData{
		UUID:                     originalUUID.String(),
		ApplicationUUID:          s.constants.fakeApplicationUUID1,
		Type:                     charmresource.TypeContainerImage,
		Name:                     "resource-name",
		Revision:                 17,
		OriginType:               origin.String(),
		ContainerImageStorageKey: "file-store-uuid",
		Size:                     42,
		SHA384:                   fp.String(),
	}
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := input.insert(c.Context(), tx)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	args := resource.StateUpdateUploadResourceArgs{
		ResourceType: charmresource.TypeContainerImage,
		ResourceUUID: originalUUID,
	}

	// Act: update resource to expect upload.
	obtainedUUID, err := s.state.UpdateUploadResourceAndDeletePriorVersion(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to update resource: %v", errors.ErrorStack(err)))

	// Assert check the application resource was updated to the newly inserted
	// record and that it has the correct origin and revision.
	s.checkApplicationResourceUpdated(c, input.ApplicationUUID, obtainedUUID.String())

	// Check that the resource_image_store no longer references the old resource
	s.checkResourceImageStore(c, originalUUID)
}

func (s *resourceSuite) checkApplicationResourceUpdated(c *tc.C, appID, expectedResourceUUID string) {
	var (
		foundOrigin string
		foundUUID   string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT uuid, origin_type
FROM   v_application_resource
WHERE  application_uuid = ?
`, appID).Scan(&foundUUID, &foundOrigin)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundOrigin, tc.Equals, charmresource.OriginUpload.String())
	// Check that the uuid of the resource has been updated.
	c.Check(foundUUID, tc.Equals, expectedResourceUUID)
}

func (s *resourceSuite) TestUpdateUploadResourceAndDeletePriorVersionFileStore(c *tc.C) {
	// Arrange: a resource to update.
	originalUUID := coreresource.UUID("resource-id")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	input := resourceData{
		UUID:            originalUUID.String(),
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            charmresource.TypeFile,
		Name:            "resource-name",
		OriginType:      charmresource.OriginUpload.String(),
		ObjectStoreUUID: "object-store-uuid",
		Size:            42,
		SHA384:          fp.String(),
	}
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := input.insert(c.Context(), tx)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	args := resource.StateUpdateUploadResourceArgs{
		ResourceType: charmresource.TypeFile,
		ResourceUUID: originalUUID,
	}

	// Act: update resource to expect upload.
	obtainedUUID, err := s.state.UpdateUploadResourceAndDeletePriorVersion(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to update resource: %v", errors.ErrorStack(err)))

	// Assert check the application resource was updated to the newly inserted
	// record and that it has the correct origin and revision.
	s.checkApplicationResourceUpdated(c, input.ApplicationUUID, obtainedUUID.String())

	// Check that the resource_image_store no longer references the old resource
	s.checkResourceFileStore(c, originalUUID)
}

// TestDeleteResourcesAddedBeforeApplication tests the happy path for
// DeleteResourcesAddedBeforeApplication.
func (s *resourceSuite) TestDeleteResourcesAddedBeforeApplication(c *tc.C) {
	resourceUUID := coreresourcetesting.GenResourceUUID(c)
	resourceName := "testResource"
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) (err error) {
		_, err = tx.Exec(`
INSERT INTO charm_resource (charm_uuid, name, kind_id, path, description)
VALUES (?, ?, ?, ?, ?)`,
			fakeCharmUUID, resourceName, TypeID(charmresource.TypeFile), nilZero(""), nilZero(""))
		if err != nil {
			return errors.Capture(err)
		}

		// Populate resource table. Don't recreate the resource if it already
		// exists.
		_, err = tx.Exec(`
INSERT INTO resource (uuid, charm_uuid, charm_resource_name, revision, origin_type_id, state_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, resourceUUID.String(), fakeCharmUUID, resourceName, nilZero(3),
			OriginTypeID("uploaded"), StateID("available"), time.Now().Truncate(time.Second).UTC(),
		)
		if err != nil {
			return errors.Capture(err)
		}

		// Populate pending_application_resource table.
		_, err = tx.Exec(`
INSERT INTO pending_application_resource (resource_uuid, application_name)
VALUES (?, ?)`, resourceUUID.String(), "test-app")
		if err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	err = s.state.DeleteResourcesAddedBeforeApplication(c.Context(), []coreresource.UUID{resourceUUID})
	c.Assert(err, tc.ErrorIsNil)
	s.checkPendingApplicationDeleted(c, resourceUUID.String())
	s.checkResourceDeleted(c, resourceUUID.String())
}

func (s *resourceSuite) TestImportResources(c *tc.C) {
	// Arrange: Add charm resources for the resources we are going to set.
	app1Res1Name := "app-1-resource-1"
	app1Res2Name := "app-1-resource-2"
	app2ResName := "app-2-resource-1"
	s.addCharmResource(c, fakeCharmUUID, charmresource.Meta{
		Name: app1Res1Name,
		Type: charmresource.TypeFile,
	})
	s.addCharmResource(c, fakeCharmUUID, charmresource.Meta{
		Name: app1Res2Name,
		Type: charmresource.TypeFile,
	})
	s.addCharmResource(c, fakeCharmUUID, charmresource.Meta{
		Name: app2ResName,
		Type: charmresource.TypeContainerImage,
	})

	// Arrange: Create arguments for ImportResources containing the resources we
	// want to set.
	app1Res1 := resource.ImportResourceInfo{
		Name:      app1Res1Name,
		Origin:    charmresource.OriginStore,
		Revision:  3,
		Timestamp: time.Now().Truncate(time.Second).UTC(),
	}
	app1Res1Unit := resource.ImportUnitResourceInfo{
		UnitName:           s.constants.fakeUnitName1,
		ImportResourceInfo: app1Res1,
	}

	app1Res2 := resource.ImportResourceInfo{
		Name:      app1Res2Name,
		Origin:    charmresource.OriginUpload,
		Revision:  -1,
		Timestamp: time.Now().Truncate(time.Second).UTC(),
	}
	app1Res2Unit := resource.ImportUnitResourceInfo{
		UnitName:           s.constants.fakeUnitName1,
		ImportResourceInfo: app1Res2,
	}
	app2Res := resource.ImportResourceInfo{
		Name:      app2ResName,
		Origin:    charmresource.OriginStore,
		Revision:  2,
		Timestamp: time.Now().Truncate(time.Second).UTC(),
	}
	args := []resource.ImportResourcesArg{{
		ApplicationName: s.constants.fakeApplicationName1,
		Resources:       []resource.ImportResourceInfo{app1Res1, app1Res2},
		UnitResources:   []resource.ImportUnitResourceInfo{app1Res1Unit, app1Res2Unit},
	}, {
		ApplicationName: s.constants.fakeApplicationName2,
		Resources:       []resource.ImportResourceInfo{app2Res},
	}}

	// Act: Set the resources.
	err := s.state.ImportResources(c.Context(), args)
	// Assert:
	c.Assert(err, tc.ErrorIsNil)

	// Assert: Check the resources were set.
	app1Res1UUID := s.checkResourceSet(c, fakeCharmUUID, app1Res1)
	app1Res2UUID := s.checkResourceSet(c, fakeCharmUUID, app1Res2)
	app2ResUUID := s.checkResourceSet(c, fakeCharmUUID, app2Res)

	// Assert: Check the application resources were set.
	s.checkApplicationResourceSet(c, s.constants.fakeApplicationUUID1, app1Res1UUID)
	s.checkApplicationResourceSet(c, s.constants.fakeApplicationUUID1, app1Res2UUID)
	s.checkApplicationResourceSet(c, s.constants.fakeApplicationUUID2, app2ResUUID)

	// Assert: Check the repo resources were set and linked ot the application
	// (the testing charm has source "charmhub" so we expect these to be set).
	s.checkRepoResourceSet(c, s.constants.fakeApplicationUUID1, app1Res1)
	s.checkRepoResourceSet(c, s.constants.fakeApplicationUUID1, app1Res2)
	s.checkRepoResourceSet(c, s.constants.fakeApplicationUUID2, app2Res)

	// Assert: Check the unit resources were set.
	s.checkUnitResourceSet(c, app1Res1UUID, s.constants.fakeUnitUUID1, app1Res1Unit)
	s.checkUnitResourceSet(c, app1Res2UUID, s.constants.fakeUnitUUID1, app1Res2Unit)
}

// TestImportResourcesOnLocalCharm checks that repository resources are not set for
// local charms.
func (s *resourceSuite) TestImportResourcesOnLocalCharm(c *tc.C) {
	// Arrange: Add a local charm and an app on it.
	charmUUID := "local-charm-uuid"
	appName := "local-charm-app"
	appUUID := "local-charm-app-uuid"
	s.addLocalCharmAndApp(c, charmUUID, appName, appUUID)

	// Arrange: Add charm resources for the resources we are going to set.
	resName := "resource-name"
	s.addCharmResource(c, charmUUID, charmresource.Meta{
		Name: resName,
		Type: charmresource.TypeFile,
	})

	// Arrange: Create arguments for ImportResources containing the resources we
	// want to set.
	setRes := resource.ImportResourceInfo{
		Name:      resName,
		Origin:    charmresource.OriginUpload,
		Revision:  -1,
		Timestamp: time.Now().Truncate(time.Second).UTC(),
	}
	args := []resource.ImportResourcesArg{{
		ApplicationName: appName,
		Resources:       []resource.ImportResourceInfo{setRes},
	}}

	// Act: Set the resources.
	err := s.state.ImportResources(c.Context(), args)
	// Assert:
	c.Assert(err, tc.ErrorIsNil)

	// Assert: Check the resources were set and linked to the application.
	resUUID := s.checkResourceSet(c, charmUUID, setRes)
	s.checkApplicationResourceSet(c, appUUID, resUUID)

	// Assert: Check the resource has no repo resources associated with it.
	s.checkRepoResourceNotSet(c, charmUUID, setRes)
}

// TestImportResourcesUnitResourceNotMatchingApplicationResources checks that we
// correctly import unit resources that have a revision and origin that do not
// match those of the application resource with the same name. These should have
// a row in the resource table created for them that the unit resource links to.
func (s *resourceSuite) TestImportResourcesUnitResourceNotMatchingApplicationResources(c *tc.C) {
	// Arrange: Add charm resources for the resources we are going to set.
	resName := "resource-name"
	s.addCharmResource(c, fakeCharmUUID, charmresource.Meta{
		Name: resName,
		Type: charmresource.TypeFile,
	})

	// Arrange: Create arguments for ImportResources containing the resources we
	// want to set.
	res := resource.ImportResourceInfo{
		Name:      resName,
		Origin:    charmresource.OriginStore,
		Revision:  3,
		Timestamp: time.Now().Truncate(time.Second).UTC(),
	}
	resUnit1 := resource.ImportUnitResourceInfo{
		UnitName: s.constants.fakeUnitName1,
		ImportResourceInfo: resource.ImportResourceInfo{
			Name:      resName,
			Origin:    charmresource.OriginUpload,
			Revision:  -1,
			Timestamp: time.Now().Truncate(time.Second).UTC(),
		},
	}
	resUnit2 := resource.ImportUnitResourceInfo{
		UnitName: s.constants.fakeUnitName2,
		ImportResourceInfo: resource.ImportResourceInfo{
			Name:      resName,
			Origin:    charmresource.OriginStore,
			Revision:  2,
			Timestamp: time.Now().Truncate(time.Second).UTC(),
		},
	}
	resUnit3 := resource.ImportUnitResourceInfo{
		UnitName:           s.constants.fakeUnitName3,
		ImportResourceInfo: res,
	}

	args := []resource.ImportResourcesArg{{
		ApplicationName: s.constants.fakeApplicationName1,
		Resources:       []resource.ImportResourceInfo{res},
		UnitResources:   []resource.ImportUnitResourceInfo{resUnit1, resUnit2, resUnit3},
	}}

	// Act: Set the resources.
	err := s.state.ImportResources(c.Context(), args)
	// Assert:
	c.Assert(err, tc.ErrorIsNil)

	// Assert: Check the application resources were set and linked ot the
	// application.
	resUUID := s.checkResourceSet(c, fakeCharmUUID, res)
	s.checkApplicationResourceSet(c, s.constants.fakeApplicationUUID1, resUUID)

	// Assert: Rows in the resource table were set for the unit resource.
	resUnit1UUID := s.checkResourceSet(c, fakeCharmUUID, resUnit1.ImportResourceInfo)
	resUnit2UUID := s.checkResourceSet(c, fakeCharmUUID, resUnit2.ImportResourceInfo)
	resUnit3UUID := s.checkResourceSet(c, fakeCharmUUID, resUnit3.ImportResourceInfo)

	// Assert: the application resource had the same origin and revision as unit
	// resource 3, so it should be the same resource.
	c.Assert(resUUID, tc.Equals, resUnit3UUID)

	// Assert: Check the unit resources were set.
	s.checkUnitResourceSet(c, resUnit1UUID, s.constants.fakeUnitUUID1, resUnit1)
	s.checkUnitResourceSet(c, resUnit2UUID, s.constants.fakeUnitUUID2, resUnit2)
	s.checkUnitResourceSet(c, resUnit3UUID, s.constants.fakeUnitUUID3, resUnit3)
}

func (s *resourceSuite) TestImportResourcesEmpty(c *tc.C) {
	// Act:
	err := s.state.ImportResources(c.Context(), nil)

	// Assert:
	c.Check(err, tc.ErrorIsNil)
}

// TestImportResourcesOnLocalCharm checks that repository resources are not set for
// local charms.
func (s *resourceSuite) TestImportResourcesApplicationNotFound(c *tc.C) {
	// Arrange: Create arguments for ImportResources containing a bad application
	// name.
	args := []resource.ImportResourcesArg{{
		ApplicationName: "bad-app-name",
	}}

	// Act: Set the resources.
	err := s.state.ImportResources(c.Context(), args)

	// Assert:
	c.Check(err, tc.ErrorIs, resourceerrors.ApplicationNotFound)
}

// TestImportResourcesOnLocalCharm checks that repository resources are not set for
// local charms.
func (s *resourceSuite) TestImportResourcesResourceNotFound(c *tc.C) {
	// Arrange: Create arguments for ImportResources containing the resources we
	// want to set.
	setRes := resource.ImportResourceInfo{
		Name:      "bad-res-name",
		Origin:    charmresource.OriginUpload,
		Revision:  -1,
		Timestamp: time.Now().Truncate(time.Second).UTC(),
	}
	args := []resource.ImportResourcesArg{{
		ApplicationName: s.constants.fakeApplicationName1,
		Resources:       []resource.ImportResourceInfo{setRes},
	}}

	// Act: Set the resources.
	err := s.state.ImportResources(c.Context(), args)

	// Assert:
	c.Check(err, tc.ErrorIs, resourceerrors.ResourceNotFound)
}

func (s *resourceSuite) TestImportResourcesUnitNotFound(c *tc.C) {
	// Arrange: Add charm resources for the resources we are going to set.
	app1Res1Name := "app-1-resource-1"
	s.addCharmResource(c, fakeCharmUUID, charmresource.Meta{
		Name: app1Res1Name,
		Type: charmresource.TypeFile,
	})

	// Arrange: Create arguments for ImportResources containing the resources we
	// want to set.
	app1Res1 := resource.ImportResourceInfo{
		Name:   app1Res1Name,
		Origin: charmresource.OriginStore,
	}
	app1Res1Unit := resource.ImportUnitResourceInfo{
		UnitName:           "bad-unit-name",
		ImportResourceInfo: app1Res1,
	}

	args := []resource.ImportResourcesArg{{
		ApplicationName: s.constants.fakeApplicationName1,
		Resources:       []resource.ImportResourceInfo{app1Res1},
		UnitResources:   []resource.ImportUnitResourceInfo{app1Res1Unit},
	}}

	// Act: Set the resources.
	err := s.state.ImportResources(c.Context(), args)

	// Assert:
	c.Check(err, tc.ErrorIs, resourceerrors.UnitNotFound)
}

func (s *resourceSuite) TestImportResourcesOriginNotValid(c *tc.C) {
	// Arrange: Add charm resources for the resources we are going to set.
	app1Res1Name := "app-1-resource-1"
	s.addCharmResource(c, fakeCharmUUID, charmresource.Meta{
		Name: app1Res1Name,
		Type: charmresource.TypeFile,
	})

	// Arrange: Create arguments for ImportResources containing the resources we
	// want to set.
	app1Res1 := resource.ImportResourceInfo{
		Name:   app1Res1Name,
		Origin: 0,
	}

	args := []resource.ImportResourcesArg{{
		ApplicationName: s.constants.fakeApplicationName1,
		Resources:       []resource.ImportResourceInfo{app1Res1},
	}}

	// Act: Set the resources.
	err := s.state.ImportResources(c.Context(), args)

	// Assert:
	c.Check(err, tc.ErrorIs, resourceerrors.OriginNotValid)
}

// TestExportResources tests the retrieval and organization of resources from the
// database.
func (s *resourceSuite) TestExportResources(c *tc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()

	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)

	// Arrange : Insert several resources
	// - 1 with no unit
	// - 1 associated with two units (state available)
	// - 1 associated with one unit (state available)
	resource1 := resourceData{
		UUID:            "resource-1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "resource-1",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
		OriginType:      "store",
		Revision:        17,
		Size:            100,
		SHA384:          fp.String(),
		RetrievedByName: "retrieved-by-name",
		ObjectStoreUUID: "object-store-uuid",
	}
	unit1Resource1 := resourceData{
		UUID:            "resource-1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "resource-1",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
		UnitUUID:        s.constants.fakeUnitUUID1,
	}
	unit2Resource1 := resourceData{
		UUID:            "resource-1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "resource-1",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
		UnitUUID:        s.constants.fakeUnitUUID2,
	}
	resource2 := resourceData{
		UUID:                     "resource-2",
		ApplicationUUID:          s.constants.fakeApplicationUUID1,
		Name:                     "resource-2",
		CreatedAt:                now,
		Type:                     charmresource.TypeContainerImage,
		OriginType:               "upload",
		RetrievedByName:          "retrieved-by-name",
		ContainerImageStorageKey: "container-image-store-uuid",
		Size:                     200,
		SHA384:                   fp.String(),
	}

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, input := range []resourceData{
			resource1,
			unit1Resource1,
			unit2Resource1,
			resource2,
		} {
			if err := input.insert(c.Context(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	exportedResources, err := s.state.ExportResources(c.Context(), s.constants.fakeApplicationName1)

	// Assert
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Check(exportedResources.Resources, tc.DeepEquals, []coreresource.Resource{
		resource1.toResource(s, c),
		resource2.toResource(s, c),
	})
	c.Check(exportedResources.UnitResources, tc.DeepEquals, []coreresource.UnitResources{
		{
			Name: unit.Name(s.constants.fakeUnitName1),
			Resources: []coreresource.Resource{
				resource1.toResource(s, c),
			},
		},
		{
			Name: unit.Name(s.constants.fakeUnitName2),
			Resources: []coreresource.Resource{
				resource1.toResource(s, c),
			},
		},
		{
			Name: unit.Name(s.constants.fakeUnitName3),
			// No resources.
		},
	})
}

// TestExportResourcesNoResources verifies that no resources are returned for an
// application when no resources exist. It checks that the resulting lists for
// unit resources, general resources, and repository resources are all empty.
func (s *resourceSuite) TestExportResourcesNoResources(c *tc.C) {
	// Arrange: No resources
	// Act
	exportedResources, err := s.state.ExportResources(c.Context(), s.constants.fakeApplicationName1)
	// Assert
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Check(exportedResources.Resources, tc.IsNil)
	c.Check(exportedResources.UnitResources, tc.DeepEquals, []coreresource.UnitResources{
		{
			Name: unit.Name(s.constants.fakeUnitName1),
			// No resources
		},
		{
			Name: unit.Name(s.constants.fakeUnitName2),
			// No resources
		},
		{
			Name: unit.Name(s.constants.fakeUnitName3),
			// No resources
		},
	})
}

func (s *resourceSuite) TestExportResourcesApplicationNotFound(c *tc.C) {
	// Arrange: No resources
	// Act
	_, err := s.state.ExportResources(c.Context(), "bad-app-name")
	// Assert
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationNotFound)
}

// TestDeleteImportedApplicationResources checks the importing and then deleting
// resources leaves the database in the same state it started.
func (s *resourceSuite) TestDeleteImportedApplicationResources(c *tc.C) {
	// Arrange: Add charm resources for the resources we are going to set.
	app1Res1Name := "app-1-resource-1"
	app1Res2Name := "app-1-resource-2"
	app2ResName := "app-2-resource-1"
	s.addCharmResource(c, fakeCharmUUID, charmresource.Meta{
		Name: app1Res1Name,
		Type: charmresource.TypeFile,
	})
	s.addCharmResource(c, fakeCharmUUID, charmresource.Meta{
		Name: app1Res2Name,
		Type: charmresource.TypeFile,
	})
	s.addCharmResource(c, fakeCharmUUID, charmresource.Meta{
		Name: app2ResName,
		Type: charmresource.TypeContainerImage,
	})

	// Arrange: Create arguments for ImportResources containing the resources we
	// want to set.
	app1Res1 := resource.ImportResourceInfo{
		Name:      app1Res1Name,
		Origin:    charmresource.OriginStore,
		Revision:  3,
		Timestamp: time.Now().Truncate(time.Second).UTC(),
	}
	app1Res1Unit := resource.ImportUnitResourceInfo{
		UnitName:           s.constants.fakeUnitName1,
		ImportResourceInfo: app1Res1,
	}

	app1Res2 := resource.ImportResourceInfo{
		Name:      app1Res2Name,
		Origin:    charmresource.OriginUpload,
		Revision:  -1,
		Timestamp: time.Now().Truncate(time.Second).UTC(),
	}
	app1Res2Unit := resource.ImportUnitResourceInfo{
		UnitName:           s.constants.fakeUnitName1,
		ImportResourceInfo: app1Res2,
	}
	app2Res := resource.ImportResourceInfo{
		Name:      app2ResName,
		Origin:    charmresource.OriginStore,
		Revision:  2,
		Timestamp: time.Now().Truncate(time.Second).UTC(),
	}
	args := []resource.ImportResourcesArg{{
		ApplicationName: s.constants.fakeApplicationName1,
		Resources:       []resource.ImportResourceInfo{app1Res1, app1Res2},
		UnitResources:   []resource.ImportUnitResourceInfo{app1Res1Unit, app1Res2Unit},
	}, {
		ApplicationName: s.constants.fakeApplicationName2,
		Resources:       []resource.ImportResourceInfo{app2Res},
	}}

	// Arrange: Import the resources.
	err := s.state.ImportResources(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	// Act: Delete imported resources.
	err = s.state.DeleteImportedResources(
		c.Context(),
		[]string{s.constants.fakeApplicationName1, s.constants.fakeApplicationName2},
	)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) failed to delete resources: %v", errors.ErrorStack(err)))

	// Assert: Check that all the resources have been removed.
	s.checkResourceTablesEmpty(c)
}

func (s *resourceSuite) addLocalCharmAndApp(c *tc.C, charmUUID, appName, appUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, source_id) VALUES (?, 'app', 0 /* local */)
`, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)
`, appUUID, appName, charmUUID, network.AlphaSpaceId,
		)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceSuite) addCharmResource(c *tc.C, charmUUID string, m charmresource.Meta) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO charm_resource (charm_uuid, name, kind_id, path, description)
VALUES (?, ?, ?, ?, ?)`,
			charmUUID, m.Name, TypeID(m.Type), nilZero(m.Path), nilZero(m.Description))
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceSuite) checkResourceSet(
	c *tc.C,
	charmUUID string,
	res resource.ImportResourceInfo,
) string {
	var (
		uuid      string
		createdAt time.Time
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT uuid, created_at
FROM   resource
WHERE  charm_resource_name = ?
AND    charm_uuid = ?
AND    COALESCE(revision, '') = COALESCE(?, '') -- Revision may be NULL
AND    origin_type_id = ?
AND    state_id = 0 -- "available"
`, res.Name, charmUUID, NullableRevision(res.Revision), OriginTypeID(res.Origin.String())).Scan(
			&uuid, &createdAt,
		)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Not(tc.Equals), "")
	c.Check(createdAt, tc.Equals, res.Timestamp)

	return uuid
}

func (s *resourceSuite) checkApplicationResourceSet(
	c *tc.C,
	expectedAppID string,
	uuid string,
) {
	var appID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT application_uuid
FROM   application_resource
WHERE  resource_uuid = ?
`, uuid).Scan(&appID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appID, tc.Equals, expectedAppID)
}

// checkRepoResourceSet checks for the repository resource record ("potential"
// resource) and application link.
func (s *resourceSuite) checkRepoResourceSet(
	c *tc.C,
	expectedAppID string,
	res resource.ImportResourceInfo,
) {
	var (
		repoResourceUUID string
		revision         sql.NullInt64
		originTypeID     int
		lastPolled       sql.NullTime
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT uuid, revision, origin_type_id, last_polled
FROM   resource
WHERE  charm_resource_name = ?
AND    charm_uuid = ?
AND    state_id = 1 -- "potential"
`, res.Name, fakeCharmUUID).Scan(&repoResourceUUID, &revision, &originTypeID, &lastPolled)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(repoResourceUUID, tc.Not(tc.Equals), "")
	c.Check(originTypeID, tc.Equals, OriginTypeID(charmresource.OriginStore.String()))
	c.Check(revision.Valid, tc.IsFalse)
	c.Check(lastPolled.Valid, tc.IsFalse)

	var appID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT application_uuid
FROM   application_resource
WHERE  resource_uuid = ?
`, repoResourceUUID).Scan(&appID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appID, tc.Equals, expectedAppID)
}

func (s *resourceSuite) checkUnitResourceSet(
	c *tc.C,
	resourceUUID string,
	expectedUnitUUID string,
	res resource.ImportUnitResourceInfo) {
	var (
		unitUUID string
		addedAt  time.Time
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT unit_uuid, added_at
FROM   unit_resource
WHERE  resource_uuid = ?
`, resourceUUID).Scan(&unitUUID, &addedAt)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitUUID, tc.Equals, expectedUnitUUID)
	c.Check(addedAt, tc.Equals, res.Timestamp)
}

// checkRepoResourceNotSet checks there is no potential resource for this charm
// resource.
func (s *resourceSuite) checkRepoResourceNotSet(
	c *tc.C,
	charmUUID string,
	res resource.ImportResourceInfo,
) {
	var repoResourceUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT uuid
FROM   resource
WHERE  charm_resource_name = ?
AND    charm_uuid = ?
AND    state_id = 1 -- "potential"
`, res.Name, charmUUID).Scan(&repoResourceUUID)
	})
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
}

func (s *resourceSuite) checkResourceTablesEmpty(
	c *tc.C,
) {
	var uuid string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT uuid
FROM   resource
`).Scan(&uuid)
	})
	c.Check(err, tc.ErrorIs, sql.ErrNoRows)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT resource_uuid
FROM   application_resource
`).Scan(&uuid)
	})
	c.Check(err, tc.ErrorIs, sql.ErrNoRows)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT resource_uuid
FROM   unit_resource
`).Scan(&uuid)
	})
	c.Check(err, tc.ErrorIs, sql.ErrNoRows)
}

func (s *resourceSuite) addResource(c *tc.C, resType charmresource.Type) coreresource.UUID {
	return s.addResourceWithOrigin(c, resType, "upload")
}

func (s *resourceSuite) addResourceWithOrigin(c *tc.C, resType charmresource.Type, origin string) coreresource.UUID {
	createdAt := time.Now().Truncate(time.Second).UTC()
	resourceUUID := coreresource.UUID("resource-uuid")
	resID := resourceUUID.String()
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       createdAt,
		Name:            "resource-name",
		Type:            resType,
		OriginType:      origin,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to add resource: %v", errors.ErrorStack(err)))
	return resourceUUID
}

func (s *resourceSuite) createFileResourceAndBlob(c *tc.C) (_ coreresource.UUID, _ store.ID, size int64, hash string) {
	// Arrange: insert a resource.
	resID := coreresourcetesting.GenResourceUUID(c)
	input := resourceData{
		UUID:            resID.String(),
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            charmresource.TypeFile,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to add resource: %v", errors.ErrorStack(err)))

	// Arrange: add a blob to the object store.
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)
	storeID := resourcestoretesting.GenFileResourceStoreID(c, objectStoreUUID)
	err = s.addObjectStoreBlobMetadata(c, objectStoreUUID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to add object store blob: %v", errors.ErrorStack(err)))

	return resID, storeID, 42, hash
}

func (s *resourceSuite) createContainerImageResourceAndBlob(c *tc.C) (_ coreresource.UUID, _ store.ID, size int64, hash string) {
	// Arrange: insert a resource.
	resID := coreresourcetesting.GenResourceUUID(c)
	input := resourceData{
		UUID:            resID.String(),
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Type:            charmresource.TypeContainerImage,
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(c.Context(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to add resource: %v", errors.ErrorStack(err)))

	// Arrange: add a container image using the resource UUID as the storage key.
	storageKey := resID.String()
	storeID := resourcestoretesting.GenContainerImageMetadataResourceID(c, storageKey)
	err = s.addContainerImage(c, storageKey)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to add container image: %v", errors.ErrorStack(err)))

	return resID, storeID, 24, "hash"
}

func (s *resourceSuite) addContainerImage(c *tc.C, storageKey string) error {
	return s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO resource_container_image_metadata_store (storage_key, registry_path)
VALUES      (?, 'testing@sha256:beef-deed')`, storageKey)
		return err
	})
}

func (s *resourceSuite) addObjectStoreBlobMetadata(c *tc.C, uuid objectstore.UUID) error {
	return s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Use the uuid as the hash to avoid uniqueness issues while testing.
		_, err := tx.ExecContext(ctx, `
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size) VALUES (?, ?, ?, 42)
`, uuid.String(), uuid.String(), uuid.String())
		return err
	})
}

// setWithRetrievedBy sets a resource with the specified retrievedBy and
// retrievedByType.
func (s *resourceSuite) setWithRetrievedBy(
	c *tc.C,
	resourceUUID coreresource.UUID,
	retrievedBy string,
	retrievedByType coreresource.RetrievedByType,
) error {
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)
	storeID := resourcestoretesting.GenFileResourceStoreID(c, objectStoreUUID)
	err := s.addObjectStoreBlobMetadata(c, objectStoreUUID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to add object store blob: %v", errors.ErrorStack(err)))
	err = s.state.RecordStoredResource(
		c.Context(),
		resource.RecordStoredResourceArgs{
			ResourceUUID:    resourceUUID,
			StorageID:       storeID,
			ResourceType:    charmresource.TypeFile,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
		},
	)
	return err
}

func (s *resourceSuite) getRetrievedByType(c *tc.C, resourceUUID coreresource.UUID) (retrievedBy string,
	retrievedByType coreresource.RetrievedByType) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT rrb.name, rrbt.name AS type
FROM   resource_retrieved_by rrb
JOIN   resource_retrieved_by_type rrbt ON rrb.retrieved_by_type_id = rrbt.id
WHERE  resource_uuid = ?`, resourceUUID.String()).Scan(&retrievedBy, &retrievedByType)
	})
	c.Assert(err, tc.ErrorIsNil)
	return retrievedBy, retrievedByType
}

func (s *resourceSuite) getCharmModifiedVersion(c *tc.C, resID string) int {
	var charmModifiedVersion sql.NullInt64
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT a.charm_modified_version
FROM   application a
JOIN   application_resource ar ON a.uuid = ar.application_uuid
WHERE  ar.resource_uuid = ?`, resID).Scan(&charmModifiedVersion)
	})
	c.Assert(err, tc.ErrorIsNil)
	if charmModifiedVersion.Valid {
		return int(charmModifiedVersion.Int64)
	}
	return 0
}

func (s *resourceSuite) checkResourceOriginAndRevision(c *tc.C, resID, expectedOrigin string, expectedRevision int) {
	// Assert: Check that the origin and revision have been set.
	var (
		obtainedOrigin   string
		obtainedRevision int
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT rot.name, r.revision
FROM   resource r
JOIN   resource_origin_type rot ON r.origin_type_id = rot.id
WHERE  r.uuid = ?`, resID).Scan(&obtainedOrigin, &obtainedRevision)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) origin and revision in resource table not updated: %v", errors.ErrorStack(err)))
	c.Check(obtainedOrigin, tc.Equals, expectedOrigin)
	c.Check(obtainedRevision, tc.Equals, expectedRevision)
}

func (s *resourceSuite) checkPendingApplicationDeleted(c *tc.C, resID string) {
	var foundAppName string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT application_name
FROM   pending_application_resource
WHERE  resource_uuid = ?`, resID).Scan(&foundAppName)
	})
	c.Check(err, tc.ErrorMatches, "sql: no rows in result set")
}

func (s *resourceSuite) checkResourceDeleted(c *tc.C, resID string) {
	var foundResName string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT charm_resource_name
FROM   resource
WHERE  uuid = ?`, resID).Scan(&foundResName)
	})
	c.Check(err, tc.ErrorMatches, "sql: no rows in result set")
}

func insertCharmStateWithRevision(ctx context.Context, tx *sql.Tx, uuid string, revision int) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, archive_path, available, reference_name, revision, version, architecture_id)
VALUES (?, 'archive', true, 'ubuntu', ?, 'deadbeef', 0)
`, uuid, revision)
	if err != nil {
		return errors.Capture(err)
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes)
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// resourceData represents a structure containing meta-information about a resource in the system.
type resourceData struct {
	// from resource table
	UUID            string
	ApplicationUUID string
	CharmUUID       string
	Name            string
	Revision        int
	// OriginType is a string representing the source type of the resource
	// (should be a valid value from resource_origin_type or empty).
	OriginType string
	// State represents the current state of the resource (should be a valid
	// value resource_state or empty)
	State     string
	CreatedAt time.Time
	PolledAt  time.Time
	// RetrievedByType should be a valid value from resource_supplied_by_type.
	RetrievedByType string
	RetrievedByName string
	Type            charmresource.Type
	Path            string
	Description     string
	UnitUUID        string
	AddedAt         time.Time
	// ObjectStoreUUID indicates if the resource is a file type resource stored
	// in the object store. If it is then it will be inserted along with the
	// Size and SHA384.
	ObjectStoreUUID string
	// ContainerImageStorageKey indicates if the resource is a container image
	// type resource stored in the object store. If it is then it will be
	// inserted along with the Size and SHA384.
	ContainerImageStorageKey string
	Size                     int
	SHA384                   string
}

// toCharmResource converts a resourceData object to a charmresource.Resource object.
func (d resourceData) toCharmResource(c *tc.C) charmresource.Resource {
	origin, err := charmresource.ParseOrigin(d.OriginType)
	if err != nil {
		// default case
		origin = charmresource.OriginUpload
	}
	var fingerprint charmresource.Fingerprint
	if d.SHA384 != "" {
		fingerprint, err = charmresource.ParseFingerprint(d.SHA384)
		c.Assert(err, tc.ErrorIsNil)
	}
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        d.Name,
			Type:        d.Type,
			Path:        d.Path,
			Description: d.Description,
		},
		Origin:      origin,
		Revision:    d.Revision,
		Fingerprint: fingerprint,
		Size:        int64(d.Size),
	}
}

// toResource converts a resourceData object to a resource.Resource object with
// enriched metadata.
func (d resourceData) toResource(s *resourceSuite, c *tc.C) coreresource.Resource {
	return coreresource.Resource{
		Resource:        d.toCharmResource(c),
		UUID:            coreresource.UUID(d.UUID),
		ApplicationName: s.constants.applicationNameFromUUID[d.ApplicationUUID],
		RetrievedBy:     d.RetrievedByName,
		Timestamp:       d.CreatedAt,
	}
}

// DeepCopy creates a deep copy of the resourceData instance and returns it.
func (d resourceData) DeepCopy() resourceData {
	result := d
	return result
}

// insertCharmResource inserts a charm_resource into the testing db.
func (d resourceData) insertCharmResource(tx *sql.Tx) (err error) {
	// Populate charm_resource table. Don't recreate the charm resource if it
	// already exists.
	_, err = tx.Exec(`
INSERT INTO charm_resource (charm_uuid, name, kind_id, path, description)
VALUES (?, ?, ?, ?, ?)`,
		d.CharmUUID, d.Name, TypeID(d.Type), nilZero(d.Path), nilZero(d.Description))
	return
}

// insert inserts the resource data into multiple related tables within a transaction.
// It populates charm_resource, resource, application_resource,
// resource_retrieved_by (if necessary), and unit_resource (if required).
func (d resourceData) insert(ctx context.Context, tx *sql.Tx) (err error) {
	//  Populate resource table
	nilZeroTime := func(t time.Time) *time.Time {
		if t.IsZero() {
			return nil
		}
		return &t
	}
	// Populate charm_resource table. Don't recreate the charm resource if it
	// already exists.
	_, err = tx.Exec(`
INSERT INTO charm_resource (charm_uuid, name, kind_id, path, description)
VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		fakeCharmUUID, d.Name, TypeID(d.Type), nilZero(d.Path), nilZero(d.Description))
	if err != nil {
		return errors.Capture(err)
	}

	// Populate resource table. Don't recreate the resource if it already
	// exists.
	_, err = tx.Exec(`
INSERT INTO resource (uuid, charm_uuid, charm_resource_name, revision, origin_type_id, state_id, created_at, last_polled)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`, d.UUID, fakeCharmUUID, d.Name, nilZero(d.Revision),
		OriginTypeID(d.OriginType), StateID(d.State), d.CreatedAt, nilZeroTime(d.PolledAt),
	)
	if err != nil {
		return errors.Capture(err)
	}

	// Populate application_resource table. Don't recreate the link if it already
	// exists.
	if d.ApplicationUUID != "" {
		_, err = tx.Exec(`
INSERT INTO application_resource (resource_uuid, application_uuid)
VALUES (?, ?) ON CONFLICT DO NOTHING`, d.UUID, d.ApplicationUUID)
		if err != nil {
			return errors.Capture(err)
		}
	}

	// Populate resource_retrieved_by table of necessary.
	if d.RetrievedByName != "" {
		_, err = tx.Exec(`
INSERT INTO resource_retrieved_by (resource_uuid, retrieved_by_type_id, name)
VALUES (?, ?, ?)`, d.UUID, RetrievedByTypeID(d.RetrievedByType), d.RetrievedByName)
		if err != nil {
			return errors.Capture(err)
		}
	}

	// Populate unit resource if required.
	if d.UnitUUID != "" {
		_, err = tx.Exec(`
INSERT INTO unit_resource (resource_uuid, unit_uuid, added_at)
VALUES (?, ?, ?)`, d.UUID, d.UnitUUID, d.AddedAt)
		if err != nil {
			return errors.Capture(err)
		}
	}

	if d.ObjectStoreUUID != "" {
		if _, err := tx.Exec(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES (?, '', '', 0)`, d.ObjectStoreUUID); err != nil {
			return errors.Capture(err)
		}
		if _, err := tx.Exec(`
INSERT INTO resource_file_store (resource_uuid, store_uuid, size, sha384)
VALUES (?, ?, ?, ?)`, d.UUID, d.ObjectStoreUUID, d.Size, d.SHA384); err != nil {
			return errors.Capture(err)
		}

	} else if d.ContainerImageStorageKey != "" {
		if _, err := tx.Exec(`
INSERT INTO resource_container_image_metadata_store (storage_key, registry_path)
VALUES (?,'')`, d.ContainerImageStorageKey); err != nil {
			return errors.Capture(err)
		}
		if _, err := tx.Exec(`
INSERT INTO resource_image_store (resource_uuid, store_storage_key, size, sha384)
VALUES (?, ?, ?, ?)`, d.UUID, d.ContainerImageStorageKey, d.Size, d.SHA384); err != nil {
			return errors.Capture(err)
		}
	}

	return err
}

// runQuery executes a SQL query within a transaction and discards the result.
func (s *resourceSuite) runQuery(c *tc.C, query string) error {
	var discard string
	return s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(query).Scan(&discard)
	})
}

// nilZero returns a pointer to the input value unless the value is its type's
// zero value, in which case it returns nil.
func nilZero[T comparable](t T) *T {
	var zero T
	if t == zero {
		return nil
	}
	return &t
}

// zeroPtr returns the value pointed to by t or the zero value if the pointer is
// nil.
func zeroPtr[T comparable](t *T) T {
	var zero T
	if t == nil {
		return zero
	}
	return *t
}

// RetrievedByTypeID maps the RetrievedByType string to an integer ID based on
// predefined categories.
func RetrievedByTypeID(retrievedByType string) int {
	res, _ := map[string]int{
		"user":        0,
		"unit":        1,
		"application": 2,
	}[retrievedByType]
	return res
}

// TypeID returns the integer ID corresponding to the resource kind stored in d.Type.
func TypeID(kind charmresource.Type) int {
	res, _ := map[charmresource.Type]int{
		charmresource.TypeFile:           0,
		charmresource.TypeContainerImage: 1,
	}[kind]
	return res
}

// OriginTypeID maps the OriginType string to its corresponding integer ID
// based on predefined categories.
func OriginTypeID(originType string) int {
	res, _ := map[string]int{
		"upload": 0,
		"store":  1,
	}[originType]
	return res
}

// StateID returns the integer ID corresponding to the state stored in d.State.
func StateID(state string) int {
	res, _ := map[string]int{
		"available": 0,
		"potential": 1,
	}[state]
	return res
}

func NullableRevision(revision int) sql.NullInt64 {
	if revision <= 0 {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Valid: true, Int64: int64(revision)}
}
