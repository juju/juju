// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(wallyworld) - move to instancepoller_test
package instancepoller

import (
	"time"

	"github.com/loggo/loggo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&observerSuite{})

type observerSuite struct {
	testing.JujuConnSuite
}

func (s *observerSuite) TestWaitsForValidEnviron(c *gc.C) {
	obs, err := newEnvironObserver(s.State, nil)
	c.Assert(err, gc.IsNil)
	env := obs.Environ()
	stateConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(env.Config().AllAttrs(), gc.DeepEquals, stateConfig.AllAttrs())
}

func (s *observerSuite) TestEnvironmentChanges(c *gc.C) {
	originalConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	logc := make(logChan, 1009)
	c.Assert(loggo.RegisterWriter("testing", logc, loggo.WARNING), gc.IsNil)
	defer loggo.RemoveWriter("testing")

	obs, err := newEnvironObserver(s.State, nil)
	c.Assert(err, gc.IsNil)

	env := obs.Environ()
	c.Assert(env.Config().AllAttrs(), gc.DeepEquals, originalConfig.AllAttrs())

	// Change the environment configuration with a different name and check that we see it.
	s.State.UpdateEnvironConfig(map[string]interface{}{"logging-config": "juju=ERROR"}, []string{})
	s.State.StartSync()

	// Check that the returned environ is still the same.
	env = obs.Environ()
	c.Assert(env.Config().AllAttrs(), gc.DeepEquals, originalConfig.AllAttrs())

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		env := obs.Environ()
		if !a.HasNext() {
			c.Fatalf("timed out waiting for new environ")
		}

		if env.Config().LoggingConfig() == "juju=ERROR;unit=DEBUG" {
			break
		}
	}
}

type logChan chan string

func (logc logChan) Write(level loggo.Level, name, filename string, line int, timestamp time.Time, message string) {
	logc <- message
}
