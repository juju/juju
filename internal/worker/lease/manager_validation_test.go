// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/trace"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/lease"
)

type ValidationSuite struct {
	testhelpers.IsolationSuite

	config lease.ManagerConfig
}

func TestValidationSuite(t *testing.T) {
	tc.Run(t, &ValidationSuite{})
}

func (s *ValidationSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = lease.ManagerConfig{
		Store: struct{ corelease.Store }{},
		Clock: struct{ clock.Clock }{},
		SecretaryFinder: FuncSecretaryFinder(func(string) (corelease.Secretary, error) {
			return nil, nil
		}),
		MaxSleep:             time.Minute,
		Logger:               loggertesting.WrapCheckLog(c),
		PrometheusRegisterer: struct{ prometheus.Registerer }{},
		Tracer:               trace.NoopTracer{},
	}
}

func (s *ValidationSuite) TestConfigValid(c *tc.C) {
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ValidationSuite) TestMissingStore(c *tc.C) {
	s.config.Store = nil
	manager, err := lease.NewManager(s.config)
	c.Check(err, tc.ErrorMatches, "nil Store not valid")
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(manager, tc.IsNil)
}

func (s *ValidationSuite) TestMissingClock(c *tc.C) {
	s.config.Clock = nil
	manager, err := lease.NewManager(s.config)
	c.Check(err, tc.ErrorMatches, "nil Clock not valid")
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(manager, tc.IsNil)
}

func (s *ValidationSuite) TestMissingTracer(c *tc.C) {
	s.config.Tracer = nil
	manager, err := lease.NewManager(s.config)
	c.Check(err, tc.ErrorMatches, "nil Tracer not valid")
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(manager, tc.IsNil)
}

func (s *ValidationSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	manager, err := lease.NewManager(s.config)
	c.Check(err, tc.ErrorMatches, "nil Logger not valid")
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(manager, tc.IsNil)
}

func (s *ValidationSuite) TestMissingSecretary(c *tc.C) {
	s.config.SecretaryFinder = nil
	manager, err := lease.NewManager(s.config)
	c.Check(err, tc.ErrorMatches, "nil SecretaryFinder not valid")
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(manager, tc.IsNil)
}

func (s *ValidationSuite) TestMissingPrometheusRegisterer(c *tc.C) {
	s.config.PrometheusRegisterer = nil
	err := s.config.Validate()
	// Fine to miss this out for now.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ValidationSuite) TestMissingMaxSleep(c *tc.C) {
	s.config.MaxSleep = 0
	manager, err := lease.NewManager(s.config)
	c.Check(err, tc.ErrorMatches, "non-positive MaxSleep not valid")
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(manager, tc.IsNil)
}

func (s *ValidationSuite) TestNegativeMaxSleep(c *tc.C) {
	s.config.MaxSleep = -time.Nanosecond
	manager, err := lease.NewManager(s.config)
	c.Check(err, tc.ErrorMatches, "non-positive MaxSleep not valid")
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(manager, tc.IsNil)
}

func (s *ValidationSuite) TestClaim_LeaseName(c *tc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("INVALID", "bar/0", time.Minute)
		c.Check(err, tc.ErrorMatches, `cannot claim lease "INVALID": name not valid`)
		c.Check(err, tc.ErrorIs, errors.NotValid)
	})
}

func (s *ValidationSuite) TestClaim_HolderName(c *tc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("foo", "INVALID", time.Minute)
		c.Check(err, tc.ErrorMatches, `cannot claim lease for holder "INVALID": name not valid`)
		c.Check(err, tc.ErrorIs, errors.NotValid)
	})
}

func (s *ValidationSuite) TestClaim_Duration(c *tc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("foo", "bar/0", time.Second)
		c.Check(err, tc.ErrorMatches, `cannot claim lease for 1s: time not valid`)
		c.Check(err, tc.ErrorIs, errors.NotValid)
	})
}

func (s *ValidationSuite) TestToken_LeaseName(c *tc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("INVALID", "bar/0")
		err := token.Check()
		c.Check(err, tc.ErrorMatches, `cannot check lease "INVALID": name not valid`)
		c.Check(err, tc.ErrorIs, errors.NotValid)
	})
}

func (s *ValidationSuite) TestToken_HolderName(c *tc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("foo", "INVALID")
		err := token.Check()
		c.Check(err, tc.ErrorMatches, `cannot check holder "INVALID": name not valid`)
		c.Check(err, tc.ErrorIs, errors.NotValid)
	})
}

func (s *ValidationSuite) TestWaitUntilExpired_LeaseName(c *tc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).WaitUntilExpired(c.Context(), "INVALID", nil)
		c.Check(err, tc.ErrorMatches, `cannot wait for lease "INVALID" expiry: name not valid`)
		c.Check(err, tc.ErrorIs, errors.NotValid)
	})
}
