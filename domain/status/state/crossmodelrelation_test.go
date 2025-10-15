// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"sort"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/offer"
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
	s.baseSuite.SetUpTest(c)

	s.state = NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *crossModelRelationSuite) TestApplicationUUIDForOffer(c *tc.C) {
	// Arrange
	appUUID, _ := s.createIAASApplication(c, "foo", life.Alive, s.workloadStatus(time.Now()))
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
	_, remoteAppUUID := s.createIAASRemoteApplicationOfferer(c, "foo")

	gotUUID, err := s.state.GetRemoteApplicationOffererUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, remoteAppUUID)
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererUUIDByNameNotRemote(c *tc.C) {
	s.createIAASApplication(c, "foo", life.Alive, s.workloadStatus(time.Now()))

	_, err := s.state.GetRemoteApplicationOffererUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.ApplicationNotRemote)
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererUUIDByNameNotFound(c *tc.C) {
	_, err := s.state.GetRemoteApplicationOffererUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *crossModelRelationSuite) TestSetRemoteApplicationOffererStatus(c *tc.C) {
	_, remoteAppUUID := s.createIAASRemoteApplicationOfferer(c, "foo")

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
	_, remoteAppUUID := s.createIAASRemoteApplicationOfferer(c, "foo")

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
	_, remoteAppUUID := s.createIAASRemoteApplicationOfferer(c, "foo")

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
	_, remoteAppUUID := s.createIAASRemoteApplicationOfferer(c, "foo")

	s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM application_remote_offerer_status`)
		return err
	})

	_, err := s.state.GetRemoteApplicationOffererStatus(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIs, statuserrors.RemoteApplicationStatusNotFound)
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererStatusesNoRemoteApplications(c *tc.C) {
	statuses, err := s.state.GetRemoteApplicationOffererStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statuses, tc.DeepEquals, map[string]status.RemoteApplicationOfferer{})
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererStatusesNoRemoteApplicationsButAnApplication(c *tc.C) {
	s.createIAASApplication(c, "foo", life.Alive, s.workloadStatus(time.Now()))

	statuses, err := s.state.GetRemoteApplicationOffererStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statuses, tc.DeepEquals, map[string]status.RemoteApplicationOfferer{})
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererStatuses(c *tc.C) {
	s.createIAASRemoteApplicationOfferer(c, "foo")
	s.createIAASRemoteApplicationOfferer(c, "bar")

	statuses, err := s.state.GetRemoteApplicationOffererStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Sort the endpoints
	for _, app := range statuses {
		sort.Slice(app.Endpoints, func(i, j int) bool {
			return app.Endpoints[i].Name < app.Endpoints[j].Name
		})
	}

	c.Assert(statuses, tc.HasLen, 2)
	c.Check(statuses, tc.DeepEquals, map[string]status.RemoteApplicationOfferer{
		"foo": {
			Status: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusUnknown,
				Message: "waiting for first status update",
				Since:   &s.now,
			},
			OfferURL: "controller:qualifier/model.foo",
			Life:     life.Alive,
			Endpoints: []status.Endpoint{{
				Name:      "endpoint",
				Interface: "interf",
				Role:      "provider",
			}, {
				Name:      "juju-info",
				Interface: "juju-info",
				Role:      "provider",
			}, {
				Name:      "misc",
				Interface: "interf",
				Role:      "provider",
			}},
		},
		"bar": {
			Status: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusUnknown,
				Message: "waiting for first status update",
				Since:   &s.now,
			},
			OfferURL: "controller:qualifier/model.bar",
			Life:     life.Alive,
			Endpoints: []status.Endpoint{{
				Name:      "endpoint",
				Interface: "interf",
				Role:      "provider",
			}, {
				Name:      "juju-info",
				Interface: "juju-info",
				Role:      "provider",
			}, {
				Name:      "misc",
				Interface: "interf",
				Role:      "provider",
			}},
		},
	})
}

func (s *crossModelRelationSuite) TestGetRemoteApplicationOffererStatusesWithRelations(c *tc.C) {
	appUUID, _ := s.createIAASRemoteApplicationOfferer(c, "foo")
	relationUUID1 := s.addRelationWithLifeAndID(c, corelife.Alive, 1)
	relationUUID2 := s.addRelationWithLifeAndID(c, corelife.Alive, 2)
	s.addRelationToApplication(c, appUUID, relationUUID1)
	s.addRelationToApplication(c, appUUID, relationUUID2)

	statuses, err := s.state.GetRemoteApplicationOffererStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses["foo"].Relations, tc.SameContents, []string{relationUUID1.String(), relationUUID2.String()})
}

func (s *crossModelRelationSuite) insertOffer(c *tc.C, name string, appUUID coreapplication.UUID, endpointName string) offer.UUID {
	offerUUID := tc.Must(c, offer.NewUUID)
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
