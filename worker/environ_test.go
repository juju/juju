// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
)

type suite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&suite{})

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

func (s *suite) TestStop(c *gc.C) {
	w := s.State.WatchEnvironConfig()
	defer stopWatcher(c, w)
	stop := make(chan struct{})
	done := make(chan error)
	go func() {
		env, err := worker.WaitForEnviron(w, stop)
		c.Check(env, gc.IsNil)
		done <- err
	}()
	close(stop)
	c.Assert(<-done, gc.Equals, tomb.ErrDying)
}

func stopWatcher(c *gc.C, w *state.EnvironConfigWatcher) {
	err := w.Stop()
	c.Check(err, gc.IsNil)
}

func (s *suite) TestInvalidConfig(c *gc.C) {
	// Create an invalid config by taking the current config and
	// tweaking the provider type.
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	m := cfg.AllAttrs()
	m["type"] = "unknown"
	invalidCfg, err := config.New(config.NoDefaults, m)
	c.Assert(err, gc.IsNil)

	err = s.State.SetEnvironConfig(invalidCfg)
	c.Assert(err, gc.IsNil)

	w := s.State.WatchEnvironConfig()
	defer stopWatcher(c, w)
	done := make(chan environs.Environ)
	go func() {
		env, err := worker.WaitForEnviron(w, nil)
		c.Check(err, gc.IsNil)
		done <- env
	}()
	// Wait for the loop to process the invalid configuratrion
	<-worker.LoadedInvalid

	// Then load a valid configuration back in.
	m = cfg.AllAttrs()
	m["secret"] = "environ_test"
	validCfg, err := config.New(config.NoDefaults, m)
	c.Assert(err, gc.IsNil)

	err = s.State.SetEnvironConfig(validCfg)
	c.Assert(err, gc.IsNil)
	s.State.StartSync()

	env := <-done
	c.Assert(env, gc.NotNil)
	c.Assert(env.Config().AllAttrs()["secret"], gc.Equals, "environ_test")
}
