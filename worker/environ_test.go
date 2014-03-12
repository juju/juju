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
	var oldType string
	oldType = s.Conn.Environ.Config().AllAttrs()["type"].(string)

	// Create an invalid config by taking the current config and
	// tweaking the provider type.
	info := s.StateInfo(c)
	opts := state.DefaultDialOpts()
	st2, err := state.Open(info, opts, state.Policy(nil))
	c.Assert(err, gc.IsNil)
	defer st2.Close()
	err = st2.UpdateEnvironConfig(map[string]interface{}{"type": "unknown"}, nil, nil)
	c.Assert(err, gc.IsNil)

	w := st2.WatchForEnvironConfigChanges()
	defer stopWatcher(c, w)
	done := make(chan environs.Environ)
	go func() {
		env, err := worker.WaitForEnviron(w, st2, nil)
		c.Check(err, gc.IsNil)
		done <- env
	}()
	// Wait for the loop to process the invalid configuratrion
	<-worker.LoadedInvalid

	st2.UpdateEnvironConfig(map[string]interface{}{
		"type":   oldType,
		"secret": "environ_test",
	}, nil, nil)

	env := <-done
	c.Assert(env, gc.NotNil)
	c.Assert(env.Config().AllAttrs()["secret"], gc.Equals, "environ_test")
}
