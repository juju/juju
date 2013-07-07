// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"fmt"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/common"
	//"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/upgrader"
)

var shortWait = 5 * time.Millisecond
var longWait = 500 * time.Millisecond

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type UpgraderSuite struct {
	jujutesting.JujuConnSuite
	//SimpleToolsFixture
}

var _ = gc.Suite(&UpgraderSuite{})

func (s *UpgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	//s.SimpleToolsFixture.SetUp(c, s.DataDir())
}

func (s *UpgraderSuite) TearDownTest(c *gc.C) {
	//s.SimpleToolsFixture.TearDown(c)
	s.JujuConnSuite.TearDownTest(c)
}

type MockCaller struct {
}

func (mc *MockCaller) Call(objType, id, request string, params, response interface{}) error {
	return nil
}

var _ common.Caller = (*MockCaller)(nil)

type Stopper interface {
	// most Worker objects implement Stopper, though it isn't officially
	// required
	Stop() error
}

// Tests of the Upgrader code that can use a MockCaller
type UpgraderNoStateSuite struct {
	caller   common.Caller
	upgrader worker.Worker
}

var _ = gc.Suite(&UpgraderNoStateSuite{})

func (s *UpgraderNoStateSuite) SetUpTest(c *gc.C) {
	s.caller = &MockCaller{}
	s.upgrader = upgrader.NewUpgrader(s.caller, "machine-tag")
}

func (s *UpgraderNoStateSuite) TearDownTest(c *gc.C) {
	stopper, ok := s.upgrader.(Stopper)
	c.Assert(ok, jc.IsTrue)
	err := stopper.Stop()
	c.Assert(err, gc.IsNil)
}

func (s *UpgraderNoStateSuite) TestString(c *gc.C) {
	c.Assert(fmt.Sprint(s.upgrader), gc.Equals, `upgrader for "machine-tag"`)
}

func WaitShort(c *gc.C, w worker.Worker) error {
	done := make(chan error)
	go func() {
		done <- w.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(shortWait):
		c.Errorf("Wait() failed to return after %.3fs", shortWait.Seconds())
	}
	return nil
}

func (s *UpgraderNoStateSuite) TestKill(c *gc.C) {
	s.upgrader.Kill()
	err := WaitShort(c, s.upgrader)
	c.Assert(err, gc.IsNil)
}

func (s *UpgraderNoStateSuite) TestStop(c *gc.C) {
	upg := s.upgrader.(Stopper)
	err := upg.Stop()
	c.Assert(err, gc.IsNil)
	// After stop, Wait should return right away
	err = WaitShort(c, s.upgrader)
	c.Assert(err, gc.IsNil)
}

func (s *UpgraderNoStateSuite) TestWait(c *gc.C) {
	done := make(chan error)
	go func() {
		done <- s.upgrader.Wait()
	}()
	select {
	case err := <-done:
		c.Errorf("Wait() didn't wait until we stopped it. err: %v", err)
	case <-time.After(shortWait):
	}
	s.upgrader.Kill()
	select {
	case err := <-done:
		c.Assert(err, gc.IsNil)
	case <-time.After(longWait):
		c.Errorf("Wait() failed to return after we stopped.")
	}
}
