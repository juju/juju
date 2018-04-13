// Copyright 2018 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

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
}

func (s *WorkerSuite) TestCredentialValidityPanicOnStartup(c *gc.C) {
	s.facade.SetErrors(errors.New("gaah"))
	config := credentialvalidator.Config{
		Facade: s.facade,
		Check:  panicCheck,
	}
	worker, err := credentialvalidator.New(config)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "gaah")
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestWatchError(c *gc.C) {
	s.facade.SetErrors(nil, errors.New("boff"))
	config := credentialvalidator.Config{
		Facade: s.facade,
		Check:  neverCheck,
	}
	worker, err := credentialvalidator.New(config)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boff")
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential")
}

func (s *WorkerSuite) TestModelCredentialErrorWhileRunning(c *gc.C) {
	s.facade.SetErrors(nil, nil, errors.New("glug"))
	config := credentialvalidator.Config{
		Facade: s.facade,
		Check:  neverCheck,
	}
	worker, err := credentialvalidator.New(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "glug")
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialNotNeeded(c *gc.C) {
	s.facade.exists = false
	config := credentialvalidator.Config{
		Facade: s.facade,
		Check:  neverCheck,
	}
	worker, err := credentialvalidator.New(config)
	c.Assert(err, gc.ErrorMatches, "model is on the cloud that does not need auth")
	c.Assert(worker, gc.IsNil)
	s.facade.CheckCallNames(c, "ModelCredential")
}

func (s *WorkerSuite) TestCredentialChangeToInvalid(c *gc.C) {
	s.facade.credentials = []base.StoredCredential{
		{credentialTag, true},
		{credentialTag, false},
	}

	config := credentialvalidator.Config{
		Facade: s.facade,
		Check:  credentialvalidator.IsValid,
	}
	worker, err := credentialvalidator.New(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsTrue)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestCredentialChangeFromInvalid(c *gc.C) {
	s.facade.credentials = []base.StoredCredential{
		{credentialTag, false},
		{credentialTag, true},
	}

	config := credentialvalidator.Config{
		Facade: s.facade,
		Check:  credentialvalidator.IsValid,
	}
	worker, err := credentialvalidator.New(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, credentialvalidator.ErrChanged)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential")
}

func (s *WorkerSuite) TestModelCredentialReplaced(c *gc.C) {
	s.facade.credentials = []base.StoredCredential{
		{credentialTag, true},
		{names.NewCloudCredentialTag("such/different/credential").String(), false},
	}
	config := credentialvalidator.Config{
		Facade: s.facade,
		Check:  credentialvalidator.IsValid,
	}
	worker, err := credentialvalidator.New(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsTrue)

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
	config := credentialvalidator.Config{
		Facade: s.facade,
		Check:  credentialvalidator.IsValid,
	}
	worker, err := credentialvalidator.New(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsTrue)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	s.facade.CheckCallNames(c, "ModelCredential", "WatchCredential", "ModelCredential", "ModelCredential", "ModelCredential")
}
