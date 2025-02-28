// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	context "context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/testing"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/secretbackend/errors"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
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
	s.leadership.EXPECT().Token("foo", "foo/666").Return(leaseToken{})

	svc := s.setupService(c, nil)

	u1 := service.AddUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, svc, "foo", u1)

	err := svc.SetApplicationStatusForUnitLeader(context.Background(), "foo/666", &status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, jc.ErrorIsNil)

	close(done)

	appStatus, err := svc.GetApplicationStatus(context.Background(), appID)
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
	s.leadership.EXPECT().Token("foo", "foo/666").Return(leaseToken{
		error: errors.NotValid,
	})

	svc := s.setupService(c, nil)

	u1 := service.AddUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, svc, "foo", u1)

	err := svc.SetApplicationStatusForUnitLeader(context.Background(), "foo/666", &status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, jc.ErrorIs, errors.NotValid)

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
	s.leadership.EXPECT().Token("foo", "foo/666").DoAndReturn(func(s1, s2 string) lease.Token {
		return leaseToken{}
	}).AnyTimes()

	svc := s.setupService(c, nil)

	u1 := service.AddUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, svc, "foo", u1)

	err := svc.SetApplicationStatusForUnitLeader(context.Background(), "foo/666", &status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, jc.ErrorIs, context.Canceled)
}

func (s *leadershipSuite) setupService(c *gc.C, factory domain.WatchableDBFactory) *service.Service {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	return service.NewService(
		state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domain.NewLeaseService(leaseGetter{
			Checker: s.leadership,
		}),
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return provider.CommonStorageProviders()
		}),
		nil,
		statushistory.NewStatusHistory(loggertesting.WrapCheckLog(c)),
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

func (s *leadershipSuite) createApplication(c *gc.C, svc *service.Service, name string, units ...service.AddUnitArg) coreapplication.ID {
	ctx := context.Background()
	appID, err := svc.CreateApplication(ctx, name, &stubCharm{}, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, service.AddApplicationArgs{
		ReferenceName: name,
		DownloadInfo: &charm.DownloadInfo{
			Provenance:  charm.ProvenanceDownload,
			DownloadURL: "http://example.com",
		},
	}, units...)
	c.Assert(err, jc.ErrorIsNil)
	return appID
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
