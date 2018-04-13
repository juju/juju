// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/credentialvalidator"
	"github.com/juju/juju/worker/workertest"
)

type WorkerSuite struct {
	testing.IsolationSuite

	facade *mockFacade
	config credentialvalidator.Config
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.facade = &mockFacade{
		Stub: &testing.Stub{},
		credentials: []base.StoredCredential{
			{credentialTag, true},
		},
		exists: true,
	}
	s.config = credentialvalidator.Config{
		Facade: s.facade,
	}
}

func (s *WorkerSuite) TestModelCredentialError(c *gc.C) {
	s.facade.SetErrors(errors.New("mc fail"))

	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "mc fail")
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestWatchError(c *gc.C) {
	s.facade.SetErrors(nil, errors.New("watch fail"))

	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "watch fail")
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential")
}

func (s *WorkerSuite) TestModelCredentialErrorWhileRunning(c *gc.C) {
	s.facade.SetErrors(nil, nil, errors.New("mc fail"))
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "mc fail")
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialNotNeeded(c *gc.C) {
	s.facade.exists = false
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "model is on the cloud that does not need auth")
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestCredentialChangeToInvalid(c *gc.C) {
	s.facade.credentials = []base.StoredCredential{
		{credentialTag, true},
		{credentialTag, false},
	}

	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrValidityChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestCredentialChangeFromInvalid(c *gc.C) {
	s.facade.credentials = []base.StoredCredential{
		{credentialTag, false},
		{credentialTag, true},
	}

	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrValidityChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialReplaced(c *gc.C) {
	s.facade.credentials = []base.StoredCredential{
		{credentialTag, true},
		{names.NewCloudCredentialTag("such/different/credential").String(), false},
	}
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrModelCredentialChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestNoRelevantCredentialChange(c *gc.C) {
	s.facade.credentials = []base.StoredCredential{
		{credentialTag, true},
		{credentialTag, true},
		{credentialTag, true},
		{credentialTag, true},
	}
	worker, err := testWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential", "ModelCredential", "ModelCredential")
}

var testWorker = credentialvalidator.NewWorker
