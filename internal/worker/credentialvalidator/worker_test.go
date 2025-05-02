// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/credentialvalidator"
)

type WorkerSuite struct {
	testing.IsolationSuite

	facade                 *mockFacade
	config                 credentialvalidator.Config
	credentialChanges      chan struct{}
	modelCredentialChanges chan struct{}

	credential *base.StoredCredential
	exists     bool
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.credential = &base.StoredCredential{CloudCredential: credentialTag, Valid: true}
	s.credentialChanges = make(chan struct{})
	s.exists = true
	s.modelCredentialChanges = make(chan struct{})
	s.facade = &mockFacade{
		Stub:         &testing.Stub{},
		credential:   s.credential,
		exists:       s.exists,
		modelWatcher: watchertest.NewMockNotifyWatcher(s.modelCredentialChanges),
	}

	s.config = credentialvalidator.Config{
		Facade: s.facade,
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	w, err := testWorker(context.Background(), s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.facade.modelWatcher)
	c.Assert(err, jc.ErrorIsNil)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential")
}

func (s *WorkerSuite) TestStartStopNoCredential(c *gc.C) {
	s.facade.setupModelHasNoCredential()
	w, err := testWorker(context.Background(), s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.facade.modelWatcher)
	c.Assert(err, jc.ErrorIsNil)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential")
}

func (s *WorkerSuite) TestModelCredentialError(c *gc.C) {
	s.facade.SetErrors(errors.New("mc fail"))

	worker, err := testWorker(context.Background(), s.config)
	c.Check(err, gc.ErrorMatches, "mc fail")
	c.Assert(worker, gc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialWatcherError(c *gc.C) {
	s.facade.SetErrors(nil, // ModelCredential call
		errors.New("mcw fail"), // WatchModelCredential call
	)

	worker, err := testWorker(context.Background(), s.config)
	c.Check(err, gc.ErrorMatches, "mcw fail")
	c.Assert(worker, gc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential")
}

func (s *WorkerSuite) TestWatchError(c *gc.C) {
	s.facade.SetErrors(nil, // ModelCredential call
		errors.New("watch fail"), // WatchModelCredential call
	)

	worker, err := testWorker(context.Background(), s.config)
	c.Assert(err, gc.ErrorMatches, "watch fail")
	c.Assert(worker, gc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential")
}

func (s *WorkerSuite) TestModelCredentialNotNeeded(c *gc.C) {
	s.facade.setupModelHasNoCredential()
	worker, err := testWorker(context.Background(), s.config)
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential")
}

func (s *WorkerSuite) TestCredentialChangeToInvalid(c *gc.C) {
	worker, err := testWorker(context.Background(), s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.facade.credential.Valid = false
	s.sendChange(c)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrValidityChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential", "ModelCredential")
}

func (s *WorkerSuite) TestCredentialChangeFromInvalid(c *gc.C) {
	s.facade.credential.Valid = false
	worker, err := testWorker(context.Background(), s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.facade.credential.Valid = true
	s.sendChange(c)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrValidityChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential", "ModelCredential")
}

func (s *WorkerSuite) TestNoRelevantCredentialChange(c *gc.C) {
	worker, err := testWorker(context.Background(), s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.sendChange(c)
	s.sendChange(c)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential", "ModelCredential", "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialNoChanged(c *gc.C) {
	worker, err := testWorker(context.Background(), s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.sendChange(c)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential", "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialChanged(c *gc.C) {
	worker, err := testWorker(context.Background(), s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.facade.credential.CloudCredential = names.NewCloudCredentialTag("cloud/anotheruser/credential").String()
	s.sendChange(c)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrModelCredentialChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchModelCredential", "ModelCredential")
}

func (s *WorkerSuite) sendChange(c *gc.C) {
	select {
	case s.modelCredentialChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending model credential change")
	}
}

var testWorker = credentialvalidator.NewWorker
