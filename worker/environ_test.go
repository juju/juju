// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type environSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestStop(c *gc.C) {
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

func (s *environSuite) TestInvalidConfig(c *gc.C) {
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

func (s *environSuite) TestErrorWhenEnvironIsInvalid(c *gc.C) {
	// reopen the state so that we can wangle a dodgy environ config in there.
	st, err := state.Open(s.StateInfo(c), state.DefaultDialOpts(), state.Policy(nil))
	c.Assert(err, gc.IsNil)
	defer st.Close()
	err = st.UpdateEnvironConfig(map[string]interface{}{"secret": 999}, nil, nil)
	c.Assert(err, gc.IsNil)
	obs, err := worker.NewEnvironObserver(s.State)
	c.Assert(err, gc.ErrorMatches, `cannot make Environ: secret: expected string, got int\(999\)`)
	c.Assert(obs, gc.IsNil)
}

func (s *environSuite) TestEnvironmentChanges(c *gc.C) {
	originalConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	logc := make(logChan, 1009)
	c.Assert(loggo.RegisterWriter("testing", logc, loggo.WARNING), gc.IsNil)
	defer loggo.RemoveWriter("testing")

	obs, err := worker.NewEnvironObserver(s.State)
	c.Assert(err, gc.IsNil)

	env := obs.Environ()
	c.Assert(env.Config().AllAttrs(), gc.DeepEquals, originalConfig.AllAttrs())
	var oldType string
	oldType = env.Config().AllAttrs()["type"].(string)

	info := s.StateInfo(c)
	opts := state.DefaultDialOpts()
	st2, err := state.Open(info, opts, state.Policy(nil))
	defer st2.Close()

	// Change to an invalid configuration and check
	// that the observer's environment remains the same.
	st2.UpdateEnvironConfig(map[string]interface{}{"type": "invalid"}, nil, nil)
	st2.StartSync()

	// Wait for the observer to register the invalid environment
	timeout := time.After(coretesting.LongWait)
loop:
	for {
		select {
		case msg := <-logc:
			if strings.Contains(msg, "error creating Environ") {
				break loop
			}
		case <-timeout:
			c.Fatalf("timed out waiting to see broken environment")
		}
	}
	// Check that the returned environ is still the same.
	env = obs.Environ()
	c.Assert(env.Config().AllAttrs(), gc.DeepEquals, originalConfig.AllAttrs())

	// Change the environment back to a valid configuration
	// with a different name and check that we see it.
	st2.UpdateEnvironConfig(map[string]interface{}{"type": oldType, "name": "a-new-name"}, nil, nil)
	st2.StartSync()

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		env := obs.Environ()
		if !a.HasNext() {
			c.Fatalf("timed out waiting for new environ")
		}
		if env.Config().Name() == "a-new-name" {
			break
		}
	}
}

type logChan chan string

func (logc logChan) Write(level loggo.Level, name, filename string, line int, timestamp time.Time, message string) {
	logc <- message
}
