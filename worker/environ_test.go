// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
)

type waitForEnvironSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&waitForEnvironSuite{})

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

func (s *waitForEnvironSuite) TestStop(c *gc.C) {
	w := s.State.WatchForEnvironConfigChanges()
	defer stopWatcher(c, w)
	stop := make(chan struct{})
	done := make(chan error)
	go func() {
		env, err := worker.WaitForEnviron(w, s.State, stop)
		c.Check(env, gc.IsNil)
		done <- err
	}()
	close(stop)
	c.Assert(<-done, gc.Equals, tomb.ErrDying)
}

func stopWatcher(c *gc.C, w state.NotifyWatcher) {
	err := w.Stop()
	c.Check(err, gc.IsNil)
}

func (s *waitForEnvironSuite) TestInvalidConfig(c *gc.C) {
	// Create an invalid config by taking the current config and
	// tweaking the provider type.
	var oldType string
	testing.ChangeEnvironConfig(c, s.State, func(attrs coretesting.Attrs) coretesting.Attrs {
		oldType = attrs["type"].(string)
		return attrs.Merge(coretesting.Attrs{"type": "unknown"})
	})
	w := s.State.WatchForEnvironConfigChanges()
	defer stopWatcher(c, w)
	done := make(chan environs.Environ)
	go func() {
		env, err := worker.WaitForEnviron(w, s.State, nil)
		c.Check(err, gc.IsNil)
		done <- env
	}()
	// Wait for the loop to process the invalid configuratrion
	<-worker.LoadedInvalid

	testing.ChangeEnvironConfig(c, s.State, func(attrs coretesting.Attrs) coretesting.Attrs {
		return attrs.Merge(coretesting.Attrs{
			"type":   oldType,
			"secret": "environ_test",
		})
	})

	env := <-done
	c.Assert(env, gc.NotNil)
	c.Assert(env.Config().AllAttrs()["secret"], gc.Equals, "environ_test")
}
