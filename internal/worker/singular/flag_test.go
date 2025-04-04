// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"context"
	"time"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type FlagSuite struct {
	testing.IsolationSuite

	manager *MockManager
	claimer *MockClaimer
	clock   *MockClock

	duration time.Duration

	unitTag  names.UnitTag
	entityID string
}

var _ = gc.Suite(&FlagSuite{})

func (s *FlagSuite) SetUpTest(c *gc.C) {
	s.unitTag = names.NewUnitTag("foo/0")
	s.entityID = uuid.MustNewUUID().String()

	s.duration = time.Second
}

func (s *FlagSuite) TestValidateConfig(c *gc.C) {
	config := s.newConfig()
	c.Assert(config.Validate(), jc.ErrorIsNil)
}

func (s *FlagSuite) TestValidateConfigNotValid(c *gc.C) {
	config := s.newConfig()
	config.LeaseManager = nil
	c.Assert(config.Validate(), jc.ErrorIs, jujuerrors.NotValid)

	config = s.newConfig()
	config.ModelUUID = ""
	c.Assert(config.Validate(), jc.ErrorIs, jujuerrors.NotValid)

	config = s.newConfig()
	config.Claimant = nil
	c.Assert(config.Validate(), jc.ErrorIs, jujuerrors.NotValid)

	config = s.newConfig()
	config.Entity = nil
	c.Assert(config.Validate(), jc.ErrorIs, jujuerrors.NotValid)

	config = s.newConfig()
	config.Clock = nil
	c.Assert(config.Validate(), jc.ErrorIs, jujuerrors.NotValid)

	config = s.newConfig()
	config.Duration = -time.Second
	c.Assert(config.Validate(), jc.ErrorIs, jujuerrors.NotValid)
}

func (s *FlagSuite) TestNewWorkerValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig()
	config.LeaseManager = nil

	_, err := NewFlagWorker(context.Background(), config)
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *FlagSuite) TestSuccessClaimOnCreation(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that we claim the entity on creation. Then we wait for the claim
	// to keep it alive.

	s.manager.EXPECT().Claimer(lease.SingularControllerNamespace, "model-uuid").Return(s.claimer, nil)
	s.claimer.EXPECT().Claim(s.entityID, s.unitTag.String(), s.duration).Return(nil)

	done := make(chan struct{})
	s.clock.EXPECT().After(s.duration / 2).DoAndReturn(func(time.Duration) <-chan time.Time {
		defer close(done)

		ch := make(chan time.Time, 1)
		return ch
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for claim to expire")
	}

	c.Assert(w.Check(), jc.IsTrue)

	workertest.CleanKill(c, w)
}

func (s *FlagSuite) TestFailureClaimerOnCreation(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.manager.EXPECT().Claimer(lease.SingularControllerNamespace, "model-uuid").Return(s.claimer, errors.Errorf("boom"))

	_, err := NewFlagWorker(context.Background(), s.newConfig())
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *FlagSuite) TestFailureClaimOnCreation(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.manager.EXPECT().Claimer(lease.SingularControllerNamespace, "model-uuid").Return(s.claimer, nil)
	s.claimer.EXPECT().Claim(s.entityID, s.unitTag.String(), s.duration).Return(errors.Errorf("boom"))

	_, err := NewFlagWorker(context.Background(), s.newConfig())
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *FlagSuite) TestDeniedClaimOnCreationCausesWait(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.manager.EXPECT().Claimer(lease.SingularControllerNamespace, "model-uuid").Return(s.claimer, nil)
	s.claimer.EXPECT().Claim(s.entityID, s.unitTag.String(), s.duration).Return(lease.ErrClaimDenied)

	done := make(chan struct{})
	s.claimer.EXPECT().WaitUntilExpired(gomock.Any(), s.entityID, gomock.Any()).DoAndReturn(func(ctx context.Context, s string, c chan<- struct{}) error {
		defer close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for claim to expire")
	}

	c.Assert(w.Check(), jc.IsFalse)

	err := workertest.CheckKilled(c, w)
	c.Assert(err, jc.ErrorIs, ErrRefresh)
}

func (s *FlagSuite) TestDeniedClaimOnCreationCausesWaitError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.manager.EXPECT().Claimer(lease.SingularControllerNamespace, "model-uuid").Return(s.claimer, nil)
	s.claimer.EXPECT().Claim(s.entityID, s.unitTag.String(), s.duration).Return(lease.ErrClaimDenied)

	done := make(chan struct{})
	s.claimer.EXPECT().WaitUntilExpired(gomock.Any(), s.entityID, gomock.Any()).DoAndReturn(func(ctx context.Context, s string, c chan<- struct{}) error {
		defer close(done)
		return errors.Errorf("boom")
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for claim to expire")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *FlagSuite) TestRepeatedClaim(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that repeated claims are made to keep the entity alive.

	done := make(chan struct{})
	gomock.InOrder(
		// First claim on creation.
		s.manager.EXPECT().Claimer(lease.SingularControllerNamespace, "model-uuid").Return(s.claimer, nil),
		s.claimer.EXPECT().Claim(s.entityID, s.unitTag.String(), s.duration).Return(nil),
		s.clock.EXPECT().After(s.duration/2).DoAndReturn(func(time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		}),

		// Now wait for the next claim to be made.
		s.claimer.EXPECT().Claim(s.entityID, s.unitTag.String(), s.duration).Return(nil),
		s.clock.EXPECT().After(s.duration/2).DoAndReturn(func(time.Duration) <-chan time.Time {
			defer close(done)

			ch := make(chan time.Time, 1)
			return ch
		}),
	)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for claim to expire")
	}

	workertest.CleanKill(c, w)
}

func (s *FlagSuite) TestRepeatedClaimFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that repeated claims are made to keep the entity alive.

	done := make(chan struct{})
	gomock.InOrder(
		// First claim on creation.
		s.manager.EXPECT().Claimer(lease.SingularControllerNamespace, "model-uuid").Return(s.claimer, nil),
		s.claimer.EXPECT().Claim(s.entityID, s.unitTag.String(), s.duration).Return(nil),
		s.clock.EXPECT().After(s.duration/2).DoAndReturn(func(time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		}),

		// Now wait for the next claim to be made.
		s.claimer.EXPECT().Claim(s.entityID, s.unitTag.String(), s.duration).DoAndReturn(func(s1, s2 string, d time.Duration) error {
			defer close(done)
			return lease.ErrClaimDenied
		}),
	)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for claim to expire")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, jc.ErrorIs, ErrRefresh)
}

func (s *FlagSuite) TestRepeatedClaimFailsWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that repeated claims are made to keep the entity alive.

	done := make(chan struct{})
	gomock.InOrder(
		// First claim on creation.
		s.manager.EXPECT().Claimer(lease.SingularControllerNamespace, "model-uuid").Return(s.claimer, nil),
		s.claimer.EXPECT().Claim(s.entityID, s.unitTag.String(), s.duration).Return(nil),
		s.clock.EXPECT().After(s.duration/2).DoAndReturn(func(time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		}),

		// Now wait for the next claim to be made.
		s.claimer.EXPECT().Claim(s.entityID, s.unitTag.String(), s.duration).DoAndReturn(func(s1, s2 string, d time.Duration) error {
			defer close(done)
			return errors.Errorf("boom")
		}),
	)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for claim to expire")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *FlagSuite) newWorker(c *gc.C) *FlagWorker {
	w, err := NewFlagWorker(context.Background(), s.newConfig())
	c.Assert(err, jc.ErrorIsNil)
	return w.(*FlagWorker)
}

func (s *FlagSuite) newConfig() FlagConfig {
	return FlagConfig{
		LeaseManager: s.manager,
		ModelUUID:    "model-uuid",
		Claimant:     s.unitTag,
		Entity:       names.NewControllerTag(s.entityID),
		Clock:        s.clock,
		Duration:     s.duration,
	}
}

func (s *FlagSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.manager = NewMockManager(ctrl)
	s.claimer = NewMockClaimer(ctrl)
	s.clock = NewMockClock(ctrl)

	return ctrl
}
