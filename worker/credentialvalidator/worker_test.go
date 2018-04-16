// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker/credentialvalidator"
	"github.com/juju/juju/worker/workertest"
)

type WorkerSuite struct {
	testing.IsolationSuite

	facade            *mockFacade
	config            credentialvalidator.Config
	credentialChanges chan struct{}
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.credentialChanges = make(chan struct{})
	s.facade = &mockFacade{
		Stub:       &testing.Stub{},
		credential: base.StoredCredential{credentialTag, true},
		exists:     true,
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
}

func (s *WorkerSuite) TestModelCredentialError(c *gc.C) {
	s.facade.SetErrors(errors.New("mc fail"))

	worker, err := testWorker(s.config)
	c.Check(err, gc.ErrorMatches, "mc fail")
	c.Assert(worker, gc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestWatchError(c *gc.C) {
	s.facade.SetErrors(nil, errors.New("watch fail"))

	worker, err := testWorker(s.config)
	c.Assert(err, gc.ErrorMatches, "watch fail")
	c.Assert(worker, gc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential")
}

func (s *WorkerSuite) TestModelCredentialErrorWhileRunning(c *gc.C) {
	s.facade.SetErrors(nil, nil, errors.New("mc fail"))
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.sendChange(c)
	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "mc fail")
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialNotNeeded(c *gc.C) {
	s.facade.exists = false
	worker, err := testWorker(s.config)
	c.Assert(err, gc.ErrorMatches, "model is on the cloud that does not need auth")
	c.Assert(worker, gc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialUnsetWhileRunning(c *gc.C) {
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.facade.exists = false
	s.sendChange(c)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "model is on the cloud that does not need auth")
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
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

func (s *WorkerSuite) TestModelCredentialReplaced(c *gc.C) {
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.facade.credential.CloudCredential = names.NewCloudCredentialTag("such/different/credential").String()
	s.sendChange(c)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrModelCredentialChanged)
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
