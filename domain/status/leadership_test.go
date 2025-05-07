// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
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
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/domain/status/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type leadershipSuite struct {
	changestreamtesting.ModelSuite

	leadership *MockChecker
}

var _ = gc.Suite(&leadershipSuite{})

func (s *leadershipSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *leadershipSuite) TestSetApplicationStatusForUnitLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.leadership.EXPECT().WaitUntilExpired(gomock.Any(), "foo", gomock.Any()).DoAndReturn(func(ctx context.Context, s string, ch chan<- struct{}) error {
		close(ch)
		// Block until the call is done.
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for done")
		}
		return nil
	})
	s.leadership.EXPECT().Token("foo", "foo/0").Return(leaseToken{})

	svc := s.setupService(c)

	u1 := application.AddUnitArg{}
	s.createApplication(c, "foo", u1)

	err := svc.SetApplicationStatusForUnitLeader(context.Background(), "foo/0", status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, jc.ErrorIsNil)

	close(done)

	appStatus, err := svc.GetApplicationDisplayStatus(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appStatus.Status, gc.Equals, status.Active)
}

func (s *leadershipSuite) TestSetApplicationStatusForUnitLeaderNotTheLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.leadership.EXPECT().WaitUntilExpired(gomock.Any(), "foo", gomock.Any()).DoAndReturn(func(ctx context.Context, s string, ch chan<- struct{}) error {
		close(ch)
		// Block until the call is done.
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for done")
		}
		return nil
	})
	s.leadership.EXPECT().Token("foo", "foo/0").Return(leaseToken{
		error: lease.ErrNotHeld,
	})

	svc := s.setupService(c)

	u1 := application.AddUnitArg{}
	s.createApplication(c, "foo", u1)

	err := svc.SetApplicationStatusForUnitLeader(context.Background(), "foo/0", status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotLeader)

	close(done)
}

func (s *leadershipSuite) TestSetApplicationStatusForUnitLeaderCancelled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// This triggers the started flow, but won't wait till the call, so it
	// will cancel the context, forcing the call to be cancelled.

	s.leadership.EXPECT().WaitUntilExpired(gomock.Any(), "foo", gomock.Any()).DoAndReturn(func(ctx context.Context, s string, c chan<- struct{}) error {
		close(c)
		return nil
	})
	s.leadership.EXPECT().Token("foo", "foo/0").DoAndReturn(func(s1, s2 string) lease.Token {
		return leaseToken{}
	}).AnyTimes()

	svc := s.setupService(c)

	u1 := application.AddUnitArg{}
	s.createApplication(c, "foo", u1)

	err := svc.SetApplicationStatusForUnitLeader(context.Background(), "foo/0", status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, jc.ErrorIs, context.Canceled)
}

func (s *leadershipSuite) setupService(c *gc.C) *service.LeadershipService {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	return service.NewLeadershipService(
		state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domain.NewLeaseService(leaseGetter{
			Checker: s.leadership,
		}),
		model.UUID(s.ModelUUID()),
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		func() (service.StatusHistoryReader, error) {
			return nil, errors.Errorf("status history reader not available")
		},
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *leadershipSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	// In an ideal world, this would be a real lease manager, but for now, we
	// just need to check the leadership token.
	s.leadership = NewMockChecker(ctrl)

	return ctrl
}

func (s *leadershipSuite) createApplication(c *gc.C, name string, units ...application.AddUnitArg) coreapplication.ID {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

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
	ctx := context.Background()

	appID, err := appState.CreateApplication(ctx, name, application.AddApplicationArg{
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
		Scale: len(units),
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	charmUUID, err := appState.GetCharmIDByApplicationName(ctx, "foo")
	c.Assert(err, jc.ErrorIsNil)

	_, err = appState.AddIAASUnits(ctx, appID, charmUUID, units...)
	c.Assert(err, jc.ErrorIsNil)

	return appID
}

func (s *leadershipSuite) minimalManifest(c *gc.C) charm.Manifest {
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
	lease.Checker
}

func (l leaseGetter) GetLeaseManager() (lease.Checker, error) {
	return l, nil
}

type leaseToken struct {
	error
}

func (l leaseToken) Check() error {
	return l.error
}
