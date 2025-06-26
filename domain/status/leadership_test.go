// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"context"
	"database/sql"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/testing"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	schematesting "github.com/juju/juju/domain/schema/testing"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/domain/status/state"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type leadershipSuite struct {
	schematesting.ModelSuite
	controllerState *MockControllerState

	leaseManager *MockLeaseManager
}

func TestLeadershipSuite(t *stdtesting.T) {
	tc.Run(t, &leadershipSuite{})
}

func (s *leadershipSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.ModelSuite.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *leadershipSuite) TestSetApplicationStatusForUnitLeader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.leaseManager.EXPECT().WaitUntilExpired(gomock.Any(), "foo", gomock.Any()).DoAndReturn(func(ctx context.Context, s string, ch chan<- struct{}) error {
		close(ch)
		// Block until the call is done.
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for done")
		}
		return nil
	})
	s.leaseManager.EXPECT().Token("foo", "foo/0").Return(leaseToken{})

	svc := s.setupService(c)

	u1 := application.AddIAASUnitArg{}
	s.createApplication(c, "foo", u1)

	err := svc.SetApplicationStatusForUnitLeader(c.Context(), "foo/0", status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, tc.ErrorIsNil)

	close(done)

	appStatus, err := svc.GetApplicationDisplayStatus(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appStatus.Status, tc.Equals, status.Active)
}

func (s *leadershipSuite) TestSetApplicationStatusForUnitLeaderNotTheLeader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.leaseManager.EXPECT().WaitUntilExpired(gomock.Any(), "foo", gomock.Any()).DoAndReturn(func(ctx context.Context, s string, ch chan<- struct{}) error {
		close(ch)
		// Block until the call is done.
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for done")
		}
		return nil
	})
	s.leaseManager.EXPECT().Token("foo", "foo/0").Return(leaseToken{
		error: lease.ErrNotHeld,
	})

	svc := s.setupService(c)

	u1 := application.AddIAASUnitArg{}
	s.createApplication(c, "foo", u1)

	err := svc.SetApplicationStatusForUnitLeader(c.Context(), "foo/0", status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotLeader)

	close(done)
}

func (s *leadershipSuite) TestSetApplicationStatusForUnitLeaderCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// This triggers the started flow, but won't wait till the call, so it
	// will cancel the context, forcing the call to be cancelled.

	s.leaseManager.EXPECT().WaitUntilExpired(gomock.Any(), "foo", gomock.Any()).DoAndReturn(func(ctx context.Context, s string, c chan<- struct{}) error {
		close(c)
		return nil
	}).AnyTimes()
	s.leaseManager.EXPECT().Token("foo", "foo/0").DoAndReturn(func(s1, s2 string) lease.Token {
		return leaseToken{}
	}).AnyTimes()

	svc := s.setupService(c)

	u1 := application.AddIAASUnitArg{}
	s.createApplication(c, "foo", u1)

	// WithLeader is racy on the context cancellation on heavily loaded systems.
	// So if this succeeds, try again.
	for {
		err := svc.SetApplicationStatusForUnitLeader(c.Context(), "foo/0", status.StatusInfo{
			Status: status.Active,
		})
		if err != nil {
			c.Assert(err, tc.ErrorIs, context.Canceled)
			break
		}
	}
}

func (s *leadershipSuite) setupService(c *tc.C) *service.LeadershipService {

	return service.NewLeadershipService(
		state.NewModelState(s.ModelSuite.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c)),
		s.controllerState,
		domain.NewLeaseService(leaseGetter{
			LeaseManager: s.leaseManager,
		}),
		model.UUID(s.ModelSuite.ModelUUID()),
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		func() (service.StatusHistoryReader, error) {
			return nil, errors.Errorf("status history reader not available")
		},
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *leadershipSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	// In an ideal world, this would be a real lease manager, but for now, we
	// just need to check the leaseManager token.
	s.leaseManager = NewMockLeaseManager(ctrl)
	s.controllerState = NewMockControllerState(ctrl)

	return ctrl
}

func (s *leadershipSuite) createApplication(c *tc.C, name string, units ...application.AddIAASUnitArg) coreapplication.ID {
	appState := applicationstate.NewState(s.ModelSuite.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := c.Context()

	appID, _, err := appState.CreateIAASApplication(ctx, name, application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: name,
					Provides: map[string]charm.Relation{
						"endpoint": {
							Name:  "endpoint",
							Role:  charm.RoleProvider,
							Scope: charm.ScopeGlobal,
						},
						"misc": {
							Name:  "misc",
							Role:  charm.RoleProvider,
							Scope: charm.ScopeGlobal,
						},
					},
				},
				Manifest:      s.minimalManifest(c),
				ReferenceName: name,
				Source:        charm.CharmHubSource,
				Revision:      42,
				Hash:          "hash",
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident",
				DownloadURL:        "https://example.com",
				DownloadSize:       42,
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = appState.AddIAASUnits(ctx, appID, units...)
	c.Assert(err, tc.ErrorIsNil)

	return appID
}

func (s *leadershipSuite) minimalManifest(c *tc.C) charm.Manifest {
	return charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.RiskStable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}
}

type leaseGetter struct {
	lease.LeaseManager
}

func (l leaseGetter) GetLeaseManager() (lease.LeaseManager, error) {
	return l, nil
}

type leaseToken struct {
	error
}

func (l leaseToken) Check() error {
	return l.error
}
