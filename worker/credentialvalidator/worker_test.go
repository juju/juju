// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker/credentialvalidator"
)

type WorkerSuite struct {
	testing.IsolationSuite

	facade            *mockFacade
	config            credentialvalidator.Config
	credentialChanges chan struct{}

	credential *base.StoredCredential
	exists     bool
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.credential = &base.StoredCredential{credentialTag, true}
	s.credentialChanges = make(chan struct{})
	s.exists = true
	s.facade = &mockFacade{
		Stub:       &testing.Stub{},
		credential: s.credential,
		exists:     s.exists,
		watcher:    watchertest.NewMockNotifyWatcher(s.credentialChanges),
	}

	s.config = credentialvalidator.Config{
		Facade: s.facade,
	}
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	w, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.facade.watcher)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(credentialvalidator.WorkerCredentialDeleted(w), jc.IsFalse)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential")
}

func (s *WorkerSuite) TestStartStopNoCredential(c *gc.C) {
	s.facade.setupModelHasNoCredential()
	w, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckNilOrKill(c, s.facade.watcher)
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialError(c *gc.C) {
	s.facade.SetErrors(errors.New("mc fail"))

	worker, err := testWorker(s.config)
	c.Check(err, gc.ErrorMatches, "mc fail")
	c.Assert(worker, gc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialDoesNotExist(c *gc.C) {
	s.facade.SetErrors(common.ServerError(errors.NotFoundf("lost")))

	w, err := testWorker(s.config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(credentialvalidator.WorkerCredentialDeleted(w), jc.IsTrue)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)

	workertest.CheckKilled(c, w)
	c.Assert(c.GetTestLog(), jc.Contains, "cloud credential reference is set for the model but the credential content is no longer on the controller")
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialNeededButUnset(c *gc.C) {
	s.facade.setupModelHasNoCredential()
	s.credential.Valid = false
	w, err := testWorker(s.config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(credentialvalidator.WorkerCredentialDeleted(w), jc.IsTrue)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)

	workertest.CheckKilled(c, w)
	c.Assert(c.GetTestLog(), jc.Contains, "model credential is not set for the model but the cloud requires it")
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestWatchError(c *gc.C) {
	s.facade.SetErrors(nil, errors.New("watch fail"))

	worker, err := testWorker(s.config)
	c.Assert(err, gc.ErrorMatches, "watch fail")
	c.Assert(worker, gc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential")
}

func (s *WorkerSuite) TestModelCredentialStopsExistingWhileRunning(c *gc.C) {
	s.facade.SetErrors(
		nil, // starting worker: ModelCredential
		nil, // starting worker: WatchCredential
		common.ServerError(errors.NotFoundf("deep loss")), // within worker loop: ModelCredential
	)
	w, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(credentialvalidator.WorkerCredentialDeleted(w), jc.IsFalse)

	s.sendChange(c)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "cloud credential validity has changed")
	c.Check(credentialvalidator.WorkerCredentialDeleted(w), jc.IsTrue)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialNotNeeded(c *gc.C) {
	s.facade.setupModelHasNoCredential()
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestCredentialChangeToInvalid(c *gc.C) {
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.facade.credential.Valid = false
	s.sendChange(c)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrValidityChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestCredentialChangeFromInvalid(c *gc.C) {
	s.facade.credential.Valid = false
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.facade.credential.Valid = true
	s.sendChange(c)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrValidityChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestNoRelevantCredentialChange(c *gc.C) {
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.sendChange(c)
	s.sendChange(c)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential", "ModelCredential")
}

func (s *WorkerSuite) sendChange(c *gc.C) {
	select {
	case s.credentialChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending credential change")
	}

}

var testWorker = credentialvalidator.NewWorker
