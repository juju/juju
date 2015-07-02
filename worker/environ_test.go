// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"errors"
	"strings"
	"sync"
	stdtesting "testing"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type environSuite struct {
	coretesting.BaseSuite

	st *fakeState
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.st = &fakeState{
		Stub:    &testing.Stub{},
		changes: make(chan struct{}, 100),
	}
}

func (s *environSuite) TestStop(c *gc.C) {
	s.st.SetErrors(
		nil,                // WatchForEnvironConfigChanges
		errors.New("err1"), // Changes (closing the channel)
	)
	s.st.SetConfig(c, coretesting.Attrs{
		"type": "invalid",
	})

	w, err := s.st.WatchForEnvironConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	defer stopWatcher(c, w)
	stop := make(chan struct{})
	close(stop) // close immediately so the loop exits.
	done := make(chan error)
	go func() {
		env, err := worker.WaitForEnviron(w, s.st, stop)
		c.Check(env, gc.IsNil)
		done <- err
	}()
	select {
	case <-worker.LoadedInvalid:
		c.Errorf("expected changes watcher to be closed")
	case err := <-done:
		c.Assert(err, gc.Equals, tomb.ErrDying)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout waiting for the WaitForEnviron to stop")
	}
	s.st.CheckCallNames(c, "WatchForEnvironConfigChanges", "Changes")
}

func stopWatcher(c *gc.C, w apiwatcher.NotifyWatcher) {
	err := w.Stop()
	c.Check(err, jc.ErrorIsNil)
}

func (s *environSuite) TestInvalidConfig(c *gc.C) {
	s.st.SetConfig(c, coretesting.Attrs{
		"type": "unknown",
	})

	w, err := s.st.WatchForEnvironConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	defer stopWatcher(c, w)
	done := make(chan environs.Environ)
	go func() {
		env, err := worker.WaitForEnviron(w, s.st, nil)
		c.Check(err, jc.ErrorIsNil)
		done <- env
	}()
	<-worker.LoadedInvalid
	s.st.CheckCallNames(c,
		"WatchForEnvironConfigChanges",
		"Changes",
		"EnvironConfig",
		"Changes",
	)
}

func (s *environSuite) TestErrorWhenEnvironIsInvalid(c *gc.C) {
	s.st.SetConfig(c, coretesting.Attrs{
		"type": "unknown",
	})

	obs, err := worker.NewEnvironObserver(s.st)
	c.Assert(err, gc.ErrorMatches,
		`cannot create an environment: no registered provider for "unknown"`,
	)
	c.Assert(obs, gc.IsNil)
	s.st.CheckCallNames(c, "EnvironConfig")
}

func (s *environSuite) TestEnvironmentChanges(c *gc.C) {
	s.st.SetConfig(c, nil)

	logc := make(logChan, 1009)
	c.Assert(loggo.RegisterWriter("testing", logc, loggo.WARNING), gc.IsNil)
	defer loggo.RemoveWriter("testing")

	obs, err := worker.NewEnvironObserver(s.st)
	c.Assert(err, jc.ErrorIsNil)

	env := obs.Environ()
	s.st.AssertConfig(c, env.Config())

	// Change to an invalid configuration and check
	// that the observer's environment remains the same.
	originalConfig, err := s.st.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	s.st.SetConfig(c, coretesting.Attrs{
		"type": "invalid",
	})

	// Wait for the observer to register the invalid environment
loop:
	for {
		select {
		case msg := <-logc:
			if strings.Contains(msg, "error creating an environment") {
				break loop
			}
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting to see broken environment")
		}
	}
	// Check that the returned environ is still the same.
	env = obs.Environ()
	c.Assert(env.Config().AllAttrs(), jc.DeepEquals, originalConfig.AllAttrs())

	// Change the environment back to a valid configuration
	// with a different name and check that we see it.
	s.st.SetConfig(c, coretesting.Attrs{
		"name": "a-new-name",
	})

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

type fakeState struct {
	*testing.Stub
	apiwatcher.NotifyWatcher

	mu sync.Mutex

	changes chan struct{}
	config  map[string]interface{}
}

var _ worker.EnvironConfigObserver = (*fakeState)(nil)

// WatchForEnvironConfigChanges implements EnvironConfigObserver.
func (s *fakeState) WatchForEnvironConfigChanges() (apiwatcher.NotifyWatcher, error) {
	s.MethodCall(s, "WatchForEnvironConfigChanges")
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return s, nil
}

// EnvironConfig implements EnvironConfigObserver.
func (s *fakeState) EnvironConfig() (*config.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.MethodCall(s, "EnvironConfig")
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return config.New(config.NoDefaults, s.config)
}

// SetConfig changes the stored environment config with the given
// extraAttrs and triggers a change for the watcher.
func (s *fakeState) SetConfig(c *gc.C, extraAttrs coretesting.Attrs) {
	s.mu.Lock()
	defer s.mu.Unlock()

	attrs := dummy.SampleConfig()
	for k, v := range extraAttrs {
		attrs[k] = v
	}

	// Simulate it's prepared.
	attrs["broken"] = ""
	attrs["state-id"] = "42"

	s.config = coretesting.CustomEnvironConfig(c, attrs).AllAttrs()
	s.changes <- struct{}{}
}

// Err implements apiwatcher.NotifyWatcher.
func (s *fakeState) Err() error {
	s.MethodCall(s, "Err")
	return s.NextErr()
}

// Stop implements apiwatcher.NotifyWatcher.
func (s *fakeState) Stop() error {
	s.MethodCall(s, "Stop")
	return s.NextErr()
}

// Changes implements apiwatcher.NotifyWatcher.
func (s *fakeState) Changes() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.MethodCall(s, "Changes")
	if err := s.NextErr(); err != nil && s.changes != nil {
		close(s.changes) // simulate the watcher died.
		s.changes = nil
	}
	return s.changes
}

func (s *fakeState) AssertConfig(c *gc.C, expected *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c.Assert(s.config, jc.DeepEquals, expected.AllAttrs())
}
