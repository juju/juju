// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/credentialvalidator"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite

	facade                 *mockFacade
	config                 credentialvalidator.Config
	modelCredentialChanges chan struct{}

	credential *base.StoredCredential
	exists     bool
}

var _ = tc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.credential = &base.StoredCredential{CloudCredential: credentialTag, Valid: true}
	s.exists = true
	s.modelCredentialChanges = make(chan struct{})
	s.facade = &mockFacade{
		Stub:         &testhelpers.Stub{},
		credential:   s.credential,
		exists:       s.exists,
		modelWatcher: watchertest.NewMockNotifyWatcher(s.modelCredentialChanges),
	}

	s.config = credentialvalidator.Config{
		Facade: s.facade,
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *WorkerSuite) TestStartStop(c *tc.C) {
	w, err := testWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.facade.modelWatcher)
	c.Assert(err, tc.ErrorIsNil)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential")
}

func (s *WorkerSuite) TestStartStopNoCredential(c *tc.C) {
	s.facade.setupModelHasNoCredential()
	w, err := testWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.facade.modelWatcher)
	c.Assert(err, tc.ErrorIsNil)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential")
}

func (s *WorkerSuite) TestModelCredentialError(c *tc.C) {
	s.facade.SetErrors(errors.New("mc fail"))

	worker, err := testWorker(c.Context(), s.config)
	c.Check(err, tc.ErrorMatches, "mc fail")
	c.Assert(worker, tc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialWatcherError(c *tc.C) {
	s.facade.SetErrors(nil, // ModelCredential call
		errors.New("mcw fail"), // WatchModelCredential call
	)

	worker, err := testWorker(c.Context(), s.config)
	c.Check(err, tc.ErrorMatches, "mcw fail")
	c.Assert(worker, tc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential")
}

func (s *WorkerSuite) TestWatchError(c *tc.C) {
	s.facade.SetErrors(nil, // ModelCredential call
		errors.New("watch fail"), // WatchModelCredential call
	)

	worker, err := testWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorMatches, "watch fail")
	c.Assert(worker, tc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential")
}

func (s *WorkerSuite) TestModelCredentialNotNeeded(c *tc.C) {
	s.facade.setupModelHasNoCredential()
	worker, err := testWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential")
}

func (s *WorkerSuite) TestCredentialChangeToInvalid(c *tc.C) {
	worker, err := testWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)

	s.facade.credential.Valid = false
	s.sendChange(c)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.Equals, credentialvalidator.ErrValidityChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential", "ModelCredential")
}

func (s *WorkerSuite) TestCredentialChangeFromInvalid(c *tc.C) {
	s.facade.credential.Valid = false
	worker, err := testWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)

	s.facade.credential.Valid = true
	s.sendChange(c)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.Equals, credentialvalidator.ErrValidityChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential", "ModelCredential")
}

func (s *WorkerSuite) TestNoRelevantCredentialChange(c *tc.C) {
	worker, err := testWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)

	s.sendChange(c)
	s.sendChange(c)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential", "ModelCredential", "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialNoChanged(c *tc.C) {
	worker, err := testWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)

	s.sendChange(c)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential", "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialChanged(c *tc.C) {
	worker, err := testWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)

	s.facade.credential.CloudCredential = names.NewCloudCredentialTag("cloud/anotheruser/credential").String()
	s.sendChange(c)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.Equals, credentialvalidator.ErrModelCredentialChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential", "ModelCredential")
}

func (s *WorkerSuite) sendChange(c *tc.C) {
	select {
	case s.modelCredentialChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending model credential change")
	}
}

var testWorker = credentialvalidator.NewWorker
