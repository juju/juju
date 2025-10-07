// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/offer"
	coreoffertesting "github.com/juju/juju/core/offer/testing"
	remoteapplicationtesting "github.com/juju/juju/core/remoteapplication/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type crossModelRelationSuite struct {
	baseSuite

	state *ModelState
}

func TestCrossModelRelationSuite(t *testing.T) {
	tc.Run(t, &crossModelRelationSuite{})
}

func (s *crossModelRelationSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *crossModelRelationSuite) TestApplicationUUIDForOffer(c *tc.C) {
	// Arrange
	appUUID, _ := s.createIAASApplication(c, "foo", life.Alive, false, s.workloadStatus(time.Now()))
	offerUUID := s.insertOffer(c, "test-offer", appUUID, "endpoint")

	// Act
	obtainedUUID, err := s.state.GetApplicationUUIDForOffer(c.Context(), offerUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedUUID, tc.Equals, appUUID.String())
}

func (s *crossModelRelationSuite) TestGetOfferUUIDForUnknownOffer(c *tc.C) {
	// Act
	_, err := s.state.GetApplicationUUIDForOffer(c.Context(), "unknown-offer")

	// Assert
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferNotFound)
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererUUIDByName(c *tc.C) {
	appUUID, _ := s.createIAASApplication(c, "foo", life.Alive, false, s.workloadStatus(time.Now()))
	remoteAppUUID := remoteapplicationtesting.GenRemoteApplicationUUID(c)
	s.insertRemoteApplication(c, appUUID.String(), remoteAppUUID.String())

	gotUUID, err := s.state.GetRemoteApplicationOffererUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, remoteAppUUID)
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererUUIDByNameNotRemote(c *tc.C) {
	s.createIAASApplication(c, "foo", life.Alive, false, s.workloadStatus(time.Now()))

	_, err := s.state.GetRemoteApplicationOffererUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.ApplicationNotRemote)
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererUUIDByNameNotFound(c *tc.C) {
	_, err := s.state.GetRemoteApplicationOffererUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *crossModelRelationSuite) TestSetRemoteApplicationOffererStatus(c *tc.C) {
	appUUID, _ := s.createIAASApplication(c, "foo", life.Alive, false, s.workloadStatus(time.Now()))
	remoteAppUUID := remoteapplicationtesting.GenRemoteApplicationUUID(c)
	s.insertRemoteApplication(c, appUUID.String(), remoteAppUUID.String())

	now := time.Now().UTC()
	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err := s.state.SetRemoteApplicationOffererStatus(c.Context(), remoteAppUUID.String(), expected)
	c.Assert(err, tc.ErrorIsNil)

	status, err := s.state.GetRemoteApplicationOffererStatus(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(status.Status, tc.Equals, expected.Status)
	c.Check(status.Message, tc.Equals, expected.Message)
	c.Check(status.Data, tc.DeepEquals, expected.Data)
	c.Check(status.Since, tc.DeepEquals, expected.Since)
}

func (s *crossModelRelationSuite) TestSetRemoteApplicationOffererStatusMultipleTimes(c *tc.C) {
	appUUID, _ := s.createIAASApplication(c, "foo", life.Alive, false, s.workloadStatus(time.Now()))
	remoteAppUUID := remoteapplicationtesting.GenRemoteApplicationUUID(c)
	s.insertRemoteApplication(c, appUUID.String(), remoteAppUUID.String())

	err := s.state.SetRemoteApplicationOffererStatus(c.Context(), remoteAppUUID.String(), status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusBlocked,
		Message: "blocked",
		Since:   ptr(time.Now().UTC()),
	})
	c.Assert(err, tc.ErrorIsNil)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err = s.state.SetRemoteApplicationOffererStatus(c.Context(), remoteAppUUID.String(), expected)
	c.Assert(err, tc.ErrorIsNil)

	status, err := s.state.GetRemoteApplicationOffererStatus(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(status.Status, tc.Equals, expected.Status)
	c.Check(status.Message, tc.Equals, expected.Message)
	c.Check(status.Data, tc.DeepEquals, expected.Data)
	c.Check(status.Since, tc.DeepEquals, expected.Since)
}

func (s *crossModelRelationSuite) TestSetRemoteApplicationOffererStatusNotFound(c *tc.C) {
	err := s.state.SetRemoteApplicationOffererStatus(c.Context(), "missing-uuid", status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusActive,
	})
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationNotFound)
}

func (s *crossModelRelationSuite) TestSetRemoteApplicationOffererStatusInvalidStatus(c *tc.C) {
	appUUID, _ := s.createIAASApplication(c, "foo", life.Alive, false, s.workloadStatus(time.Now()))
	remoteAppUUID := remoteapplicationtesting.GenRemoteApplicationUUID(c)
	s.insertRemoteApplication(c, appUUID.String(), remoteAppUUID.String())

	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusType(99),
	}

	err := s.state.SetRemoteApplicationOffererStatus(c.Context(), remoteAppUUID.String(), expected)
	c.Assert(err, tc.ErrorMatches, `unknown status.*`)
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererStatusNotFound(c *tc.C) {
	_, err := s.state.GetRemoteApplicationOffererStatus(c.Context(), "missing-uuid")
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationNotFound)
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererStatusNotSet(c *tc.C) {
	appUUID, _ := s.createIAASApplication(c, "foo", life.Alive, false, s.workloadStatus(time.Now()))
	remoteAppUUID := remoteapplicationtesting.GenRemoteApplicationUUID(c)
	s.insertRemoteApplication(c, appUUID.String(), remoteAppUUID.String())

	_, err := s.state.GetRemoteApplicationOffererStatus(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIs, statuserrors.RemoteApplicationStatusNotFound)
}

func (s *crossModelRelationSuite) insertOffer(c *tc.C, name string, appUUID coreapplication.UUID, endpointName string) offer.UUID {
	offerUUID := coreoffertesting.GenOfferUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var endpointUUID string
		err := tx.QueryRowContext(ctx, `
SELECT ae.uuid
FROM application_endpoint AS ae
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
WHERE ae.application_uuid = ? AND cr.name = ?
`, appUUID, endpointName).Scan(&endpointUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO offer (uuid, name) VALUES (?, ?)`, offerUUID, name)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO offer_endpoint (offer_uuid, endpoint_uuid) VALUES (?, ?)`, offerUUID, endpointUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return offerUUID
}
