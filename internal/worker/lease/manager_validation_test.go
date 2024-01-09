// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/worker/lease"
)

type ValidationSuite struct {
	testing.IsolationSuite

	config lease.ManagerConfig
}

var _ = gc.Suite(&ValidationSuite{})

func (s *ValidationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = lease.ManagerConfig{
		Store: struct{ corelease.Store }{},
		Clock: struct{ clock.Clock }{},
		SecretaryFinder: FuncSecretaryFinder(func(string) (lease.Secretary, error) {
			return nil, nil
		}),
		MaxSleep:             time.Minute,
		Logger:               loggo.GetLogger("lease_test"),
		PrometheusRegisterer: struct{ prometheus.Registerer }{},
		Tracer:               trace.NoopTracer{},
	}
}

func (s *ValidationSuite) TestConfigValid(c *gc.C) {
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ValidationSuite) TestMissingStore(c *gc.C) {
	s.config.Store = nil
	manager, err := lease.NewManager(s.config)
	c.Check(err, gc.ErrorMatches, "nil Store not valid")
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingClock(c *gc.C) {
	s.config.Clock = nil
	manager, err := lease.NewManager(s.config)
	c.Check(err, gc.ErrorMatches, "nil Clock not valid")
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingTracer(c *gc.C) {
	s.config.Tracer = nil
	manager, err := lease.NewManager(s.config)
	c.Check(err, gc.ErrorMatches, "nil Tracer not valid")
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	manager, err := lease.NewManager(s.config)
	c.Check(err, gc.ErrorMatches, "nil Logger not valid")
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingSecretary(c *gc.C) {
	s.config.SecretaryFinder = nil
	manager, err := lease.NewManager(s.config)
	c.Check(err, gc.ErrorMatches, "nil SecretaryFinder not valid")
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingPrometheusRegisterer(c *gc.C) {
	s.config.PrometheusRegisterer = nil
	err := s.config.Validate()
	// Fine to miss this out for now.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ValidationSuite) TestMissingMaxSleep(c *gc.C) {
	s.config.MaxSleep = 0
	manager, err := lease.NewManager(s.config)
	c.Check(err, gc.ErrorMatches, "non-positive MaxSleep not valid")
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestNegativeMaxSleep(c *gc.C) {
	s.config.MaxSleep = -time.Nanosecond
	manager, err := lease.NewManager(s.config)
	c.Check(err, gc.ErrorMatches, "non-positive MaxSleep not valid")
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestClaim_LeaseName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("INVALID", "bar/0", time.Minute)
		c.Check(err, gc.ErrorMatches, `cannot claim lease "INVALID": name not valid`)
		c.Check(err, jc.ErrorIs, errors.NotValid)
	})
}

func (s *ValidationSuite) TestClaim_HolderName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("foo", "INVALID", time.Minute)
		c.Check(err, gc.ErrorMatches, `cannot claim lease for holder "INVALID": name not valid`)
		c.Check(err, jc.ErrorIs, errors.NotValid)
	})
}

func (s *ValidationSuite) TestClaim_Duration(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("foo", "bar/0", time.Second)
		c.Check(err, gc.ErrorMatches, `cannot claim lease for 1s: time not valid`)
		c.Check(err, jc.ErrorIs, errors.NotValid)
	})
}

func (s *ValidationSuite) TestToken_LeaseName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("INVALID", "bar/0")
		err := token.Check()
		c.Check(err, gc.ErrorMatches, `cannot check lease "INVALID": name not valid`)
		c.Check(err, jc.ErrorIs, errors.NotValid)
	})
}

func (s *ValidationSuite) TestToken_HolderName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("foo", "INVALID")
		err := token.Check()
		c.Check(err, gc.ErrorMatches, `cannot check holder "INVALID": name not valid`)
		c.Check(err, jc.ErrorIs, errors.NotValid)
	})
}

func (s *ValidationSuite) TestWaitUntilExpired_LeaseName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).WaitUntilExpired("INVALID", nil)
		c.Check(err, gc.ErrorMatches, `cannot wait for lease "INVALID" expiry: name not valid`)
		c.Check(err, jc.ErrorIs, errors.NotValid)
	})
}
