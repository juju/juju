// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/unit"
	apperrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/resource"
	schematesting "github.com/juju/juju/domain/schema/testing"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type resourceSuite struct {
	schematesting.ModelSuite

	state *State

	constants struct {
		fakeApplicationUUID1 string
		fakeApplicationUUID2 string
		fakeUnitUUID1        string
		fakeUnitUUID2        string
	}
}

var _ = gc.Suite(&resourceSuite{})

const fakeCharmUUID = "fake-charm-uuid"

func (s *resourceSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	s.constants.fakeApplicationUUID1 = "fake-application-1-uuid"
	s.constants.fakeApplicationUUID2 = "fake-application-2-uuid"
	s.constants.fakeUnitUUID1 = "fake-unit-1-uuid"
	s.constants.fakeUnitUUID2 = "fake-unit-2-uuid"

	// Populate DB with two application and a charm
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		fakeNetNodeUUID := "fake-net-node-uuid"

		_, err = tx.ExecContext(ctx, `INSERT INTO charm (uuid) VALUES (?)`, fakeCharmUUID)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO net_node (uuid) VALUES (?)`, fakeNetNodeUUID)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO application (uuid, name, life_id, charm_uuid) VALUES (?, ?, ?, ?),(?, ?, ?, ?)`,
			s.constants.fakeApplicationUUID1, "app1", 0 /* alive */, fakeCharmUUID,
			s.constants.fakeApplicationUUID2, "app2", 0 /* alive */, fakeCharmUUID)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?),(?, ?, ?, ?, ?)`,
			s.constants.fakeUnitUUID1, "unit1", 0 /* alive */, s.constants.fakeApplicationUUID1, fakeNetNodeUUID,
			s.constants.fakeUnitUUID2, "unit2", 0 /* alive */, s.constants.fakeApplicationUUID2, fakeNetNodeUUID)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("failed to populate DB with applications: %v", errors.ErrorStack(err)))
}

// TestGetApplicationResourceID tests that the resource ID can be correctly
// retrieved from the database, given a name and an application
func (s *resourceSuite) TestGetApplicationResourceID(c *gc.C) {
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
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		for _, input := range []resourceData{found, other} {
			if err := input.insert(context.Background(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: Get application resource ID
	id, err := s.state.GetApplicationResourceID(context.Background(), resource.GetApplicationResourceIDArgs{
		ApplicationID: application.ID(s.constants.fakeApplicationUUID1),
		Name:          found.Name,
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to get application resource ID: %v", errors.ErrorStack(err)))
	c.Assert(id, gc.Equals, coreresource.UUID(found.UUID),
		gc.Commentf("(Act) unexpected application resource ID"))
}

// TestGetApplicationResourceIDNotFound verifies the behavior when attempting
// to retrieve a resource ID for a non-existent resource within a specified
// application.
func (s *resourceSuite) TestGetApplicationResourceIDNotFound(c *gc.C) {
	// Arrange: No resources
	// Act: Get application resource ID
	_, err := s.state.GetApplicationResourceID(context.Background(), resource.GetApplicationResourceIDArgs{
		ApplicationID: application.ID(s.constants.fakeApplicationUUID1),
		Name:          "resource-name-not-found",
	})
	c.Assert(err, jc.ErrorIs, apperrors.ResourceNotFound, gc.Commentf("(Act) unexpected error"))
}

// TestGetResourceNotFound verifies that attempting to retrieve a non-existent
// resource results in a ResourceNotFound error.
func (s *resourceSuite) TestGetResourceNotFound(c *gc.C) {
	// Arrange : no resource
	resID := coreresource.UUID("resource-id")

	// Act
	_, err := s.state.GetResource(context.Background(), resID)

	// Assert
	c.Assert(err, jc.ErrorIs, apperrors.ResourceNotFound, gc.Commentf("(Assert) unexpected error"))
}

// TestGetResource verifies the successful retrieval of a resource from the
// database by its ID.
func (s *resourceSuite) TestGetResource(c *gc.C) {
	// Arrange : a simple resource
	resID := coreresource.UUID("resource-id")
	now := time.Now().Truncate(time.Second).UTC()
	expected := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "resource-name",
				Path:        "/path/to/resource",
				Description: "this is a test resource",
			},
			Revision: 42,
			Origin:   0,
			// todo(gfouillet): handle size/fingerprint
			//Fingerprint: charmresource.Fingerprint{},
			//Size:        0,
		},
		UUID:            resID,
		ApplicationID:   application.ID(s.constants.fakeApplicationUUID1),
		RetrievedBy:     "johnDoe",
		RetrievedByType: "user",
		Timestamp:       now,
	}
	input := resourceData{
		UUID:            resID.String(),
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Revision:        expected.Revision,
		OriginType:      "upload", // 0 in db
		CreatedAt:       now,
		Name:            expected.Name,
		Kind:            "file", // 0 in db
		Path:            expected.Path,
		Description:     expected.Description,
		RetrievedByType: string(expected.RetrievedByType),
		RetrievedByName: expected.RetrievedBy,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := input.insert(context.Background(), tx)
		return errors.Capture(err)
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	obtained, err := s.state.GetResource(context.Background(), resID)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute GetResource: %v", errors.ErrorStack(err)))

	// Assert
	c.Assert(obtained, jc.DeepEquals, expected, gc.Commentf("(Assert) resource different than expected"))
}

type setTest struct {
	Summary         string
	ResourceUUID    coreresource.UUID
	Name            string
	Revision        int
	OriginType      charmresource.Origin     // OriginType is a string representing the source type of the resource (should be a valid value from resource_origin_type or empty).
	RetrievedByType resource.RetrievedByType // should be a valid value from resource_supplied_by_type
	RetrievedByName string
	Increment       resource.IncrementCharmModifiedVersionType
}

func (s *resourceSuite) TestSetResource(c *gc.C) {
	// Arrange:
	name := s.insertTestCharmResource(c)
	res := s.testResource(c, name)
	increment := resource.IncrementCharmModifiedVersion
	originalCharmModifiedVersion := s.getCharmModifiedVersion(c)

	// Action: Set the resource.
	err := s.state.SetResource(
		context.Background(),
		res,
		increment,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("error: %v", errors.ErrorStack(err)))

	// Check that the resource was added.
	found := s.getResource(c, res.UUID.String())
	c.Check(err, jc.ErrorIsNil, gc.Commentf("error: %v", errors.ErrorStack(err)))
	c.Check(found.CharmUUID, gc.Equals, fakeCharmUUID)
	c.Check(found.CharmResourceName, gc.Equals, name)
	c.Check(found.Revision, gc.NotNil)
	c.Check(*found.Revision, gc.Equals, res.Revision)
	c.Check(found.OriginTypeID, gc.Equals, OriginTypeID(res.Origin.String()))
	c.Check(found.StateID, gc.Equals, 0)
	c.Check(found.CreatedAt, gc.Equals, res.Timestamp)
	c.Check(found.LastPolled, gc.IsNil)

	// Check "added by" was added.
	foundRetrievedByTypeID, foundName := s.getResourceRetrievedBy(c, res.UUID.String())
	c.Check(foundRetrievedByTypeID, gc.Equals, RetrievedByTypeID(string(res.RetrievedByType)))
	c.Check(foundName, gc.Equals, res.RetrievedBy)

	// Check resource was added to application resource table.
	foundApplicationUUID := s.getApplicationResource(c, res.UUID.String())
	c.Check(foundApplicationUUID, gc.Equals, res.ApplicationID.String())

	// Check charm modified version was incremented.
	foundCharmModifiedVersion := s.getCharmModifiedVersion(c)
	c.Check(foundCharmModifiedVersion, gc.Equals, originalCharmModifiedVersion+1)
}

func (s *resourceSuite) TestSetResourceRetrievedByUnit(c *gc.C) {
	// Arrange:
	name := s.insertTestCharmResource(c)
	res := s.testResource(c, name)
	res.RetrievedBy = "unit-app1-0"
	res.RetrievedByType = resource.Unit

	// Action: Set the resource.
	err := s.state.SetResource(
		context.Background(),
		res,
		resource.DoNotIncrementCharmModifiedVersion,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("error: %v", errors.ErrorStack(err)))

	// Check "added by" was added.
	foundRetrievedByTypeID, foundName := s.getResourceRetrievedBy(c, res.UUID.String())
	c.Check(foundRetrievedByTypeID, gc.Equals, RetrievedByTypeID(string(res.RetrievedByType)))
	c.Check(foundName, gc.Equals, res.RetrievedBy)
}

func (s *resourceSuite) TestSetResourceNoRevision(c *gc.C) {
	// Arrange.
	name := s.insertTestCharmResource(c)
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: name,
			},
			Origin:   charmresource.OriginUpload,
			Revision: -1,
		},
		UUID:            resourcetesting.GenResourceUUID(c),
		ApplicationID:   application.ID(s.constants.fakeApplicationUUID1),
		RetrievedBy:     "bill",
		RetrievedByType: resource.User,
		Timestamp:       time.Now(),
	}

	// Action: Set the resource.
	err := s.state.SetResource(
		context.Background(),
		res,
		resource.IncrementCharmModifiedVersion,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("error: %v", errors.ErrorStack(err)))

	// Check revision is NULL.
	found := s.getResource(c, res.UUID.String())
	c.Check(err, jc.ErrorIsNil, gc.Commentf("error: %v", errors.ErrorStack(err)))
	c.Check(found.Revision, gc.IsNil)
}

func (s *resourceSuite) TestSetResourceCharmModifiedVersionIncremented(c *gc.C) {
	// Arrange:
	name := s.insertTestCharmResource(c)
	res := s.testResource(c, name)
	originalCharmModifiedVersion := s.getCharmModifiedVersion(c)

	// Action:
	err := s.state.SetResource(
		context.Background(),
		res,
		resource.IncrementCharmModifiedVersion,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("error: %v", errors.ErrorStack(err)))

	// Check charm modified version was incremented.
	foundCharmModifiedVersion := s.getCharmModifiedVersion(c)
	c.Assert(foundCharmModifiedVersion, gc.Equals, originalCharmModifiedVersion+1)
}

func (s *resourceSuite) TestSetResourceCharmModifiedVersionNotIncremented(c *gc.C) {
	// Arrange:
	name := s.insertTestCharmResource(c)
	res := s.testResource(c, name)
	originalCharmModifiedVersion := s.getCharmModifiedVersion(c)

	// Action:
	err := s.state.SetResource(
		context.Background(),
		res,
		resource.DoNotIncrementCharmModifiedVersion,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("error: %v", errors.ErrorStack(err)))

	// Check charm modified version was not incremented.
	foundCharmModifiedVersion := s.getCharmModifiedVersion(c)
	c.Assert(foundCharmModifiedVersion, gc.Equals, originalCharmModifiedVersion)
}

func (s *resourceSuite) TestSetResourceApplicationNotFound(c *gc.C) {
	// Arrange:
	name := s.insertTestCharmResource(c)
	res := s.testResource(c, name)
	res.ApplicationID = "bad-uuid"

	// Action: Set the resource.
	err := s.state.SetResource(
		context.Background(),
		res,
		resource.IncrementCharmModifiedVersion,
	)

	// Assert expected error.
	c.Assert(err, jc.ErrorIs, apperrors.ApplicationNotFound)
}

func (s *resourceSuite) TestSetResourceCharmResourceNotFound(c *gc.C) {
	// Arrange:
	name := "bad-name"
	res := s.testResource(c, name)

	// Action: Set the resource.
	err := s.state.SetResource(
		context.Background(),
		res,
		resource.IncrementCharmModifiedVersion,
	)

	// Assert expected error.
	c.Assert(err, jc.ErrorIs, apperrors.CharmResourceNotFound)
}

func (s *resourceSuite) TestSetResourceUnknownOriginType(c *gc.C) {
	// Arrange:
	name := s.insertTestCharmResource(c)
	res := s.testResource(c, name)
	// 0 is the unknown origin.
	res.Origin = 0

	// Action: Set the resource.
	err := s.state.SetResource(
		context.Background(),
		res,
		resource.IncrementCharmModifiedVersion,
	)

	// Assert expected error.
	c.Assert(err, jc.ErrorIs, apperrors.UnknownResourceOriginType)
}

func (s *resourceSuite) TestSetResourceUnknownResourceRetrievedByType(c *gc.C) {
	// Arrange:
	name := s.insertTestCharmResource(c)
	res := s.testResource(c, name)
	res.RetrievedByType = resource.Unknown

	// Action: Set the resource.
	err := s.state.SetResource(
		context.Background(),
		res,
		resource.IncrementCharmModifiedVersion,
	)

	// Assert expected error.
	c.Assert(err, jc.ErrorIs, apperrors.UnknownResourceRetrievedByType)
}

// TestSetRepositoryResource ensures that the SetRepositoryResources function
// updates the resource poll dates correctly.
func (s *resourceSuite) TestSetRepositoryResource(c *gc.C) {
	// Arrange : Insert 4 resources, two have been already polled, and two other not yet.
	now := time.Now().Truncate(time.Second).UTC()
	previousPoll := now.Add(-1 * time.Hour)
	defaultResource := resourceData{
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		RetrievedByType: "user",
		RetrievedByName: "John Doe",
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
	}

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		for _, input := range append(notPolled, alreadyPolled...) {
			if err := input.insert(context.Background(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: update resource 1 and 2 (not 3)
	err = s.state.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: application.ID(s.constants.fakeApplicationUUID1),
		Info: []charmresource.Resource{{
			Meta: charmresource.Meta{
				Name: "not-polled-1",
			},
		}, {
			Meta: charmresource.Meta{
				Name: "polled-1",
			},
		}},
		LastPolled: now,
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute TestSetRepositoryResource: %v", errors.ErrorStack(err)))

	// Assert
	type obtainedRow struct {
		ResourceUUID string
		LastPolled   *time.Time
	}
	var obtained []obtainedRow
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT uuid, last_polled FROM resource`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row obtainedRow
			if err := rows.Scan(&row.ResourceUUID, &row.LastPolled); err != nil {
				return err
			}
			obtained = append(obtained, row)
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) failed to get expected changes in db: %v", errors.ErrorStack(err)))
	c.Assert(obtained, jc.SameContents, []obtainedRow{
		{
			ResourceUUID: "polled-id-1", // updated
			LastPolled:   &now,
		},
		{
			ResourceUUID: "polled-id-2",
			LastPolled:   &previousPoll, // not updated
		},
		{
			ResourceUUID: "not-polled-id-1", // created
			LastPolled:   &now,
		},
		{
			ResourceUUID: "not-polled-id-2", // not polled
			LastPolled:   nil,
		},
	})
}

// TestSetRepositoryResourceUnknownResource validates that attempting to set
// repository resources for unknown resources logs the correct errors.
func (s *resourceSuite) TestSetRepositoryResourceUnknownResource(c *gc.C) {
	// Act: update unexisting resources
	err := s.state.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
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
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute TestSetRepositoryResource: %v", errors.ErrorStack(err)))

	// Assert
	c.Check(c.GetTestLog(), jc.Contains, fmt.Sprintf("Resource not found for application app1 (%s)", s.constants.fakeApplicationUUID1), gc.Commentf("(Assert) application not found in log"))
	c.Check(c.GetTestLog(), jc.Contains, "not-a-resource-1", gc.Commentf("(Assert) missing resource name log"))
	c.Check(c.GetTestLog(), jc.Contains, "not-a-resource-2", gc.Commentf("(Assert) missing resource name log"))
}

// TestSetRepositoryResourceApplicationNotFound verifies that setting repository
// resources for a non-existent application results in an ApplicationNotFound error.
func (s *resourceSuite) TestSetRepositoryResourceApplicationNotFound(c *gc.C) {
	// Act: request an unexisting application
	err := s.state.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: "not-an-application",
		Info:          []charmresource.Resource{{}}, // Non empty info
		LastPolled:    time.Now(),                   // not used
	})

	// Assert: check expected error
	c.Assert(err, jc.ErrorIs, apperrors.ApplicationNotFound, gc.Commentf("(Act) unexpected error: %v", errors.ErrorStack(err)))
}

// TestSetUnitResourceNotYetSupplied verifies that a unit resource is correctly
// set when the resource has no initial supplier. It sets up a resource in the
// database, calls the SetUnitResource method, and checks if the resource is
// updated as expected.
func (s *resourceSuite) TestSetUnitResourceNotYetSupplied(c *gc.C) {
	// Arrange: insert a resource with no supplier
	now := time.Now().Truncate(time.Second).UTC()
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		Name:            "resource-name",
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with application type
	result, err := s.state.SetUnitResource(context.Background(), resource.SetUnitResourceArgs{
		ResourceUUID:    coreresource.UUID(resID),
		UnitUUID:        unit.UUID(s.constants.fakeUnitUUID1),
		RetrievedBy:     "app1",
		RetrievedByType: "application",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute SetUnitResource: %v", errors.ErrorStack(err)))

	// Assert
	c.Check(result.UUID.String(), gc.Equals, resID,
		gc.Commentf("(Assert) unexpected resource ID"))
	c.Check(result.Timestamp, jc.TimeBetween(now, time.Now()))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var discard string
		return tx.QueryRow(`
SELECT resource_uuid FROM unit_resource
WHERE resource_uuid = ? and unit_uuid = ? and added_at = ?`,
			resID, s.constants.fakeUnitUUID1, result.Timestamp).Scan(&discard) // only fetch a possible error
	})
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) unit_resource table not updated: %v", errors.ErrorStack(err)))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var discard string
		return tx.QueryRow(`
SELECT resource_uuid FROM resource_retrieved_by rrb
JOIN resource_retrieved_by_type rrbt on rrb.retrieved_by_type_id = rrbt.id
WHERE rrb.resource_uuid = ? and rrb.name = ? and rrbt.name = ?`,
			resID, "app1", "application").Scan(&discard) // only fetch a possible error
	})
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) application_resource and resource_supplied_by not updated: %v", errors.ErrorStack(err)))
}

// TestSetUnitResourceAlreadyRetrievedByApplication verifies that attempting to
// set a resource as retrieved by a unit correctly handles the case where the
// resource was previously retrieved by an application. It ensures that no
// erroneous updates are made to the resource_retrieved_by table and that the
// unit_resource table is updated appropriately.
func (s *resourceSuite) TestSetUnitResourceAlreadyRetrievedByApplication(c *gc.C) {
	// Arrange: insert a resource and data implying it has been retrieved by an application (not an unit)
	now := time.Now().Truncate(time.Second).UTC()
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		Name:            "resource-name",
		RetrievedByName: s.constants.fakeApplicationUUID1,
		RetrievedByType: "application",
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return errors.Capture(input.insert(context.Background(), tx))
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with application type
	result, err := s.state.SetUnitResource(context.Background(), resource.SetUnitResourceArgs{
		ResourceUUID:    coreresource.UUID(resID),
		UnitUUID:        unit.UUID(s.constants.fakeUnitUUID1),
		RetrievedBy:     s.constants.fakeUnitUUID1,
		RetrievedByType: "unit",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute SetUnitResource: %v", errors.ErrorStack(err)))

	// Assert
	c.Check(result.UUID.String(), gc.Equals, resID,
		gc.Commentf("(Assert) unexpected resource ID"))
	c.Check(result.Timestamp, jc.TimeBetween(now, time.Now()))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var discard string
		return tx.QueryRow(`
SELECT resource_uuid FROM unit_resource
WHERE resource_uuid = ? and unit_uuid = ? and added_at = ?`,
			resID, s.constants.fakeUnitUUID1, result.Timestamp).Scan(&discard) // only fetch a possible error
	})
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) unit_resource table not updated: %v", errors.ErrorStack(err)))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var discard string
		return tx.QueryRow(`
SELECT resource_uuid FROM resource_retrieved_by
WHERE resource_uuid = ? AND retrieved_by_type_id = ? AND name = ?`,
			resID, 2 /* application */, s.constants.fakeApplicationUUID1).Scan(&discard) // only fetch a possible error
	})
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) resource_retrieved_by has been updated, which is incorrect: %v", errors.ErrorStack(err)))
}

// TestSetUnitResourceAlreadySetted checks if set unit resource correctly
// identifies an already set resource and skips updating.
func (s *resourceSuite) TestSetUnitResourceAlreadySetted(c *gc.C) {
	// Arrange: insert a resource and data implying that everything is already setted
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
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return errors.Capture(input.insert(context.Background(), tx))
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with user type
	result, err := s.state.SetUnitResource(context.Background(), resource.SetUnitResourceArgs{
		ResourceUUID:    coreresource.UUID(resID),
		UnitUUID:        unit.UUID(s.constants.fakeUnitUUID1),
		RetrievedBy:     "john",
		RetrievedByType: "user",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute SetUnitResource: %v", errors.ErrorStack(err)))

	// Assert
	c.Check(result.UUID.String(), gc.Equals, resID,
		gc.Commentf("(Assert) unexpected resource ID"))
	c.Check(result.Timestamp, gc.Equals, previousInsertTime)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var discard string
		return tx.QueryRow(`
SELECT resource_uuid FROM unit_resource
WHERE resource_uuid = ? and unit_uuid = ? and added_at = ?`,
			resID, s.constants.fakeUnitUUID1, result.Timestamp).Scan(&discard) // only fetch a possible error
	})
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
}

// TestSetUnitResourceNotYetSuppliedExistingSupplierWrongType ensures that
// setting a unit resource with an unexpected supplier type returns an error
// and does not update the database.
func (s *resourceSuite) TestSetUnitResourceNotYetSuppliedExistingSupplierWrongType(c *gc.C) {
	// Arrange: insert a resource
	now := time.Now().Truncate(time.Second).UTC()
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		Name:            "resource-name",
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return errors.Capture(input.insert(context.Background(), tx))
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with unexpected type
	_, err = s.state.SetUnitResource(context.Background(), resource.SetUnitResourceArgs{
		ResourceUUID:    coreresource.UUID(resID),
		UnitUUID:        unit.UUID(s.constants.fakeUnitUUID1),
		RetrievedBy:     "john",
		RetrievedByType: "unexpected",
	})

	// Assert: an error is returned, nothing is updated in the db
	c.Check(err, jc.ErrorIs, apperrors.UnknownRetrievedByType)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var discard string
		err = tx.QueryRow(`SELECT * FROM unit_resource`).Scan(&discard)
		c.Check(err, jc.ErrorIs, sql.ErrNoRows, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
		err = tx.QueryRow(`SELECT * FROM resource_retrieved_by`).Scan(&discard)
		c.Check(err, jc.ErrorIs, sql.ErrNoRows, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
		return nil
	})
}

// TestSetUnitResourceNotFound verifies that attempting to set a resource for a
// unit when the resource does not exist results in a ResourceNotFound error.
// The test ensures that no updates are made to the unit_resource and
// resource_retrieved_by tables in the database.
func (s *resourceSuite) TestSetUnitResourceNotFound(c *gc.C) {
	// Arrange: No resource
	resID := "resource-id"

	// Act: set unknown resource
	_, err := s.state.SetUnitResource(context.Background(), resource.SetUnitResourceArgs{
		ResourceUUID:    coreresource.UUID(resID),
		UnitUUID:        unit.UUID(s.constants.fakeUnitUUID1),
		RetrievedBy:     "john",
		RetrievedByType: "unexpected",
	})

	// Assert: an error is returned, nothing is updated in the db
	c.Check(err, jc.ErrorIs, apperrors.ResourceNotFound)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var discard string
		err = tx.QueryRow(`SELECT * FROM unit_resource`).Scan(&discard)
		c.Check(err, jc.ErrorIs, sql.ErrNoRows, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
		err = tx.QueryRow(`SELECT * FROM resource_retrieved_by`).Scan(&discard)
		c.Check(err, jc.ErrorIs, sql.ErrNoRows, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
		return nil
	})
}

// TestSetUnitResourceUnitNotFound tests that setting a unit resource with an
// unexpected unit ID results in an error.
func (s *resourceSuite) TestSetUnitResourceUnitNotFound(c *gc.C) {
	// Arrange: insert a resource
	now := time.Now().Truncate(time.Second).UTC()
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		Name:            "resource-name",
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return errors.Capture(input.insert(context.Background(), tx))
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with unexpected unit
	_, err = s.state.SetUnitResource(context.Background(), resource.SetUnitResourceArgs{
		ResourceUUID:    coreresource.UUID(resID),
		UnitUUID:        "unexpected-unit-id",
		RetrievedBy:     "john",
		RetrievedByType: "user",
	})

	// Assert: an error is returned, nothing is updated in the db
	c.Check(err, jc.ErrorIs, apperrors.UnitNotFound)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var discard string
		err = tx.QueryRow(`SELECT * FROM unit_resource`).Scan(&discard)
		c.Check(err, jc.ErrorIs, sql.ErrNoRows, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
		err = tx.QueryRow(`SELECT * FROM resource_retrieved_by`).Scan(&discard)
		c.Check(err, jc.ErrorIs, sql.ErrNoRows, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
		return nil
	})
}

// TestListResourcesNoResources verifies that no resources are listed for an
// application when no resources exist. It checks that the resulting lists for
// unit resources, general resources, and repository resources are all empty.
func (s *resourceSuite) TestListResourcesNoResources(c *gc.C) {
	// Arrange: No resources
	// Act
	results, err := s.state.ListResources(context.Background(), application.ID(s.constants.fakeApplicationUUID1))
	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Assert(results.UnitResources, gc.HasLen, 0)
	c.Assert(results.Resources, gc.HasLen, 0)
	c.Assert(results.RepositoryResources, gc.HasLen, 0)
}

// TestListResources tests the retrieval and organization of resources from the
// database.
func (s *resourceSuite) TestListResources(c *gc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	// Arrange : Insert several resources
	// - 1 with no unit nor polled
	// - 1 with unit but no polled
	// - 1 with polled but no unit
	// - 1 with polled and unit
	// - 1 not polled and another unit
	simpleRes := resourceData{
		UUID:            "simple-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "simple",
		CreatedAt:       now,
	}
	polledRes := resourceData{
		UUID:            "polled-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "polled",
		CreatedAt:       now,
		PolledAt:        now,
	}
	unitRes := resourceData{
		UUID:            "unit-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "unit",
		CreatedAt:       now,
		UnitUUID:        s.constants.fakeUnitUUID1,
		AddedAt:         now,
	}
	bothRes := resourceData{
		UUID:            "both-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "both",
		UnitUUID:        s.constants.fakeUnitUUID1,
		AddedAt:         now,
		PolledAt:        now,
	}
	anotherUnitRes := resourceData{
		UUID:            "another-unit-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "anotherUnit",
		CreatedAt:       now,
		UnitUUID:        s.constants.fakeUnitUUID2,
		AddedAt:         now,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		for _, input := range []resourceData{simpleRes, polledRes, unitRes, bothRes, anotherUnitRes} {
			if err := input.insert(context.Background(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	results, err := s.state.ListResources(context.Background(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Assert(results.UnitResources, gc.DeepEquals, []resource.UnitResources{
		{
			ID: unit.UUID(s.constants.fakeUnitUUID1),
			Resources: []resource.Resource{
				unitRes.toResource(),
				bothRes.toResource(),
			},
		},
		{
			ID: unit.UUID(s.constants.fakeUnitUUID2),
			Resources: []resource.Resource{
				anotherUnitRes.toResource(),
			},
		},
	})
	c.Assert(results.Resources, gc.DeepEquals, []resource.Resource{
		simpleRes.toResource(),
		polledRes.toResource(),
		unitRes.toResource(),
		bothRes.toResource(),
		anotherUnitRes.toResource(),
	})
	c.Assert(results.RepositoryResources, gc.DeepEquals, []charmresource.Resource{
		{}, // not polled
		polledRes.toCharmResource(),
		{}, // not polled
		bothRes.toCharmResource(),
		{}, // not polled
	})
}

func (s *resourceSuite) getCharmModifiedVersion(c *gc.C) int {
	var foundCharmModifiedVersion *int
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT charm_modified_version
FROM   application
WHERE  uuid = ?`, s.constants.fakeApplicationUUID1).Scan(
			&foundCharmModifiedVersion,
		)
	})
	c.Assert(err, jc.ErrorIsNil)

	// If the charm modified version is NULL, then return 0.
	if foundCharmModifiedVersion != nil {
		return *foundCharmModifiedVersion
	}
	return 0
}

func (s *resourceSuite) getResourceRetrievedBy(c *gc.C, resourceUUID string) (int, string) {
	var (
		foundRetrievedByTypeID int
		foundName              string
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT added_by_type_id, name
FROM   resource_added_by
WHERE  resource_uuid = ?`, resourceUUID).Scan(
			&foundRetrievedByTypeID, &foundName,
		)
	})
	c.Assert(err, jc.ErrorIsNil)
	return foundRetrievedByTypeID, foundName
}

func (s *resourceSuite) getApplicationResource(c *gc.C, resourceUUID string) string {
	var foundApplicationUUID string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT application_uuid
FROM   application_resource
WHERE  resource_uuid = ?`, resourceUUID).Scan(&foundApplicationUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
	return foundApplicationUUID
}

func (s *resourceSuite) testResource(c *gc.C, name string) resource.Resource {
	return resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: name,
			},
			Origin:   charmresource.OriginUpload,
			Revision: 42,
		},
		UUID:            resourcetesting.GenResourceUUID(c),
		ApplicationID:   application.ID(s.constants.fakeApplicationUUID1),
		RetrievedBy:     "bill",
		RetrievedByType: resource.User,
		Timestamp:       time.Now().Truncate(time.Second).UTC(),
	}
}

func (s *resourceSuite) insertTestCharmResource(c *gc.C) string {
	// Put in test charm.
	name := "resource-name"
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return resourceData{
			Name:        "resource-name",
			Path:        "/path/to/resource",
			Description: "this is a test resource",
			Kind:        "file",
		}.insertCharmResource(tx)
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("inserting test charm"))
	return name
}

type gotResource struct {
	CharmUUID, CharmResourceName string
	Revision                     *int
	OriginTypeID, StateID        int
	CreatedAt                    time.Time
	LastPolled                   *time.Time
}

func (s *resourceSuite) getResource(c *gc.C, resourceUUID string) gotResource {
	var res gotResource
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT charm_uuid, charm_resource_name, revision, origin_type_id, state_id, created_at, last_polled
FROM   resource
WHERE  uuid = ?`, resourceUUID).Scan(
			&res.CharmUUID, &res.CharmResourceName,
			&res.Revision,
			&res.OriginTypeID, &res.StateID,
			&res.CreatedAt,
			&res.LastPolled,
		)
	})
	c.Assert(err, jc.ErrorIsNil)
	return res
}

// resourceData represents a structure containing meta-information about a resource in the system.
type resourceData struct {
	// from resource table
	UUID            string
	ApplicationUUID string
	Name            string
	Revision        int
	OriginType      string // OriginType is a string representing the source type of the resource (should be a valid value from resource_origin_type or empty).
	State           string // State represents the current state of the resource (should be a valid value resource_state or empty)
	CreatedAt       time.Time
	PolledAt        time.Time
	RetrievedByType string // should be a valid value from resource_supplied_by_type
	RetrievedByName string
	Kind            string // Type of resource (should be a valid value from charm_resource_kind or empty)
	Path            string
	Description     string
	UnitUUID        string
	AddedAt         time.Time
}

// toCharmResource converts a resourceData object to a charmresource.Resource object.
func (d resourceData) toCharmResource() charmresource.Resource {
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        d.Name,
			Type:        charmresource.Type(TypeID(d.Kind)),
			Path:        d.Path,
			Description: d.Description,
		},
		Origin:   charmresource.Origin(OriginTypeID(d.OriginType)),
		Revision: d.Revision,
		// todo(gfouillet): deal with fingerprint & size
		Fingerprint: charmresource.Fingerprint{},
		Size:        0,
	}
}

// toResource converts a resourceData object to a resource.Resource object with
// enriched metadata.
func (d resourceData) toResource() resource.Resource {
	return resource.Resource{
		Resource:        d.toCharmResource(),
		UUID:            coreresource.UUID(d.UUID),
		ApplicationID:   application.ID(d.ApplicationUUID),
		RetrievedBy:     d.RetrievedByName,
		RetrievedByType: resource.RetrievedByType(d.RetrievedByType),
		Timestamp:       d.CreatedAt,
	}
}

// DeepCopy creates a deep copy of the resourceData instance and returns it.
func (d resourceData) DeepCopy() resourceData {
	result := d
	return result
}

// insert inserts the resource data into multiple related tables within a transaction.
// It populates charm_resource, resource, application_resource,
// resource_retrieved_by (if necessary), and unit_resource (if required).
func (input resourceData) insert(ctx context.Context, tx *sql.Tx) (err error) {
	//  Populate resource table
	nilZeroTime := func(t time.Time) *time.Time {
		if t.IsZero() {
			return nil
		}
		return &t
	}

	// Populate charm_resource table
	err = input.insertCharmResource(tx)
	if err != nil {
		return errors.Capture(err)
	}

	// Populate resource table
	_, err = tx.Exec(`INSERT INTO resource (uuid, charm_uuid, charm_resource_name, revision, origin_type_id, state_id, created_at, last_polled) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		input.UUID, fakeCharmUUID, input.Name, nilZero(input.Revision), OriginTypeID(input.OriginType), StateID(input.State), input.CreatedAt, nilZeroTime(input.PolledAt))
	if err != nil {
		return errors.Capture(err)
	}

	// Populate application_resource table
	_, err = tx.Exec(`INSERT INTO application_resource (resource_uuid, application_uuid) VALUES (?, ?)`,
		input.UUID, input.ApplicationUUID)
	if err != nil {
		return errors.Capture(err)
	}

	// Populate resource_retrieved_by table of necessary
	if input.RetrievedByName != "" {
		_, err = tx.Exec(`INSERT INTO resource_retrieved_by (resource_uuid, retrieved_by_type_id, name) VALUES (?, ?, ?)`,
			input.UUID, RetrievedByTypeID(input.RetrievedByType), input.RetrievedByName)
		if err != nil {
			return errors.Capture(err)
		}
	}

	// Populate unit resource if required
	if input.UnitUUID != "" {
		_, err = tx.Exec(`INSERT INTO unit_resource (resource_uuid, unit_uuid, added_at) VALUES (?, ?, ?)`, input.UUID, input.UnitUUID, input.AddedAt)
		return errors.Capture(err)
	}

	return nil
}

func (input resourceData) insertCharmResource(tx *sql.Tx) error {
	_, err := tx.Exec(`INSERT INTO charm_resource (charm_uuid, name, kind_id, path, description) VALUES (?, ?, ?, ?, ?)`,
		fakeCharmUUID, input.Name, TypeID(input.Kind), nilZero(input.Path), nilZero(input.Description))
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// nilZero returns a pointer to the input value unless the value is its type's zero value, in which case it returns nil.
func nilZero[T comparable](t T) *T {
	var zero T
	if t == zero {
		return nil
	}
	return &t
}

// RetrievedByTypeID maps the RetrievedByType string to an integer ID based on
// predefined categories.
func RetrievedByTypeID(RetrievedByType string) int {
	res, _ := map[string]int{
		"user":        0,
		"unit":        1,
		"application": 2,
	}[RetrievedByType]
	return res
}

// TypeID returns the integer ID corresponding to the resource kind stored in d.Kind.
func TypeID(Kind string) int {
	res, _ := map[string]int{
		"file":      0,
		"oci-image": 1,
	}[Kind]
	return res
}

// OriginTypeID maps the OriginType string to its corresponding integer ID
// based on predefined categories.
func OriginTypeID(OriginType string) int {
	res, _ := map[string]int{
		"upload": 0,
		"store":  1,
	}[OriginType]
	return res
}

// StateID returns the integer ID corresponding to the state stored in d.State.
func StateID(State string) int {
	res, _ := map[string]int{
		"available": 0,
		"potential": 1,
	}[State]
	return res
}
