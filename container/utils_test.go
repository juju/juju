package container

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&UtilsSuite{})

type UtilsSuite struct {
	testing.BaseSuite
}

func (s *UtilsSuite) TestIsLXCSupportedOnHost(c *gc.C) {
	s.PatchValue(&RunningInContainer, func() bool {
		return false
	})
	supports := ContainersSupported()
	c.Assert(supports, jc.IsTrue)
}

func (s *UtilsSuite) TestIsLXCSupportedOnLXCContainer(c *gc.C) {
	s.PatchValue(&RunningInContainer, func() bool {
		return true
	})
	supports := ContainersSupported()
	c.Assert(supports, jc.IsFalse)
}

/// fakeContainerAgentConfig

type fakeContainerAgentConfig struct {
	tag   func() names.Tag
	value func(string) string
}

func (f fakeContainerAgentConfig) Tag() names.Tag {
	if f.tag != nil {
		return f.tag()
	}
	return nil
}

func (f fakeContainerAgentConfig) Value(value string) string {
	if f.value != nil {
		return f.value(value)
	}
	return ""
}

/// fakeContainerManager

func newContainerManagerFn(manager ContainerManager) NewContainerManagerFn {
	return func(instance.ContainerType, ManagerConfig) (ContainerManager, error) {
		return manager, nil
	}
}

type fakeContainerManager struct {
	listContainers func() ([]instance.Instance, error)
	isInitialized  func() bool
}

func (f fakeContainerManager) ListContainers() ([]instance.Instance, error) {
	if f.listContainers != nil {
		return f.listContainers()
	}
	return nil, nil
}

func (f fakeContainerManager) IsInitialized() bool {
	if f.isInitialized != nil {
		return f.isInitialized()
	}
	return true
}

/// Unit tests

var _ = gc.Suite(&UnitTestSuite{})

type UnitTestSuite struct{}

func (s *UnitTestSuite) TestValidateAgentConfig_ErrIfNotMachineTag(c *gc.C) {
	cfg := fakeContainerAgentConfig{
		tag: func() names.Tag {
			return names.CharmTag{}
		},
	}
	err := validateAgentConfig(cfg)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "expected names.MachineTag, got: names.CharmTag --> charm-")
}

func (s *UnitTestSuite) TestRunningContainers_ManagerCtorErrors(c *gc.C) {
	cfg := fakeContainerAgentConfig{
		value: func(string) string { return "" },
	}
	newContainerManager := func(instance.ContainerType, ManagerConfig) (ContainerManager, error) {
		return nil, errors.New("foo")
	}
	_, err := runningContainers(cfg, newContainerManager)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "failed to get manager for container type lxc: foo")
}

func (s *UnitTestSuite) TestRunningContainers_ReturnsContainers(c *gc.C) {
	var listContainersCalled bool
	newContainerManager := newContainerManagerFn(
		fakeContainerManager{
			listContainers: func() ([]instance.Instance, error) {
				listContainersCalled = true
				return []instance.Instance{instance.Instance(nil)}, nil
			},
		},
	)
	cfg := fakeContainerAgentConfig{
		value: func(string) string { return "" },
	}
	containers, err := runningContainers(cfg, newContainerManager)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(listContainersCalled, gc.Equals, true)
	c.Assert(containers, gc.HasLen, 1*len(instance.ContainerTypes))
}

func (s *UnitTestSuite) TestContainerTeardown_StopsWhenQuitSignaled(c *gc.C) {
	cfg := fakeContainerAgentConfig{
		tag: func() names.Tag {
			return names.MachineTag{}
		},
	}
	newContainerManager := newContainerManagerFn(
		fakeContainerManager{
			// We want to always return 1 container so we keep
			// waiting.
			listContainers: func() ([]instance.Instance, error) {
				return []instance.Instance{instance.Instance(nil)}, nil
			},
		},
	)
	quit := make(chan struct{})
	teardown, err := ContainerTeardown(clock.WallClock, newContainerManager, cfg, quit)
	c.Assert(err, jc.ErrorIsNil)
	quit <- struct{}{}
	select {
	case err, ok := <-teardown:
		c.Check(err, jc.ErrorIsNil)
		c.Check(ok, gc.Equals, false)
	case <-time.After(1 * time.Microsecond):
		c.Error("the teardown channel should be closed")
		c.Fail()
	}
}

func (s *UnitTestSuite) TestContainerTeardown_SignalsTeardown(c *gc.C) {
	cfg := fakeContainerAgentConfig{
		tag: func() names.Tag {
			return names.MachineTag{}
		},
	}

	finalListContainersCall := sync.NewCond(&sync.Mutex{})
	numListContainersCalled := 0
	newContainerManager := newContainerManagerFn(
		fakeContainerManager{
			// Wait until this is called once before waiting on the
			// teardown channel.
			listContainers: func() ([]instance.Instance, error) {
				switch numListContainersCalled {
				case 0:
					numListContainersCalled++
					return []instance.Instance{instance.Instance(nil)}, nil
				case 1:
					finalListContainersCall.Signal()
					fallthrough
				default:
					return nil, nil
				}
			},
		},
	)
	teardown, err := ContainerTeardown(clock.WallClock, newContainerManager, cfg, nil)
	c.Assert(err, jc.ErrorIsNil)

	finalListContainersCall.L.Lock()
	finalListContainersCall.Wait()
	select {
	case err, ok := <-teardown:
		c.Check(err, jc.ErrorIsNil)
		c.Check(ok, gc.Equals, true)
	case <-time.After(2 * time.Second):
		c.Error("the teardown channel should be closed")
		c.Fail()
	}
}

func (s *UnitTestSuite) TestContainerTeardown_SignalsError(c *gc.C) {
	cfg := fakeContainerAgentConfig{
		tag: func() names.Tag {
			return names.MachineTag{}
		},
	}
	newContainerManager := newContainerManagerFn(
		fakeContainerManager{
			// Wait until this is called once before waiting on the
			// teardown channel.
			listContainers: func() ([]instance.Instance, error) {
				return nil, errors.New("foo")
			},
		},
	)
	teardown, err := ContainerTeardown(clock.WallClock, newContainerManager, cfg, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Unfortunately there is no automated way to test for
	// livelocks. The best we can do is a statistical heuristic.
	select {
	case err, ok := <-teardown:
		c.Assert(err, gc.Not(gc.IsNil))
		c.Check(err, gc.ErrorMatches, "failed to list containers: foo")
		c.Check(ok, gc.Equals, true)
	case <-time.After(10 * time.Second):
		c.Error("the teardown channel should be closed")
		c.Fail()
	}
}

func (s *UnitTestSuite) TestContainerTeardownOrTimeout_TimesOut(c *gc.C) {
	timeout := ContainerTeardownOrTimeout(clock.WallClock, nil, 1*time.Microsecond)
	select {
	case err, ok := <-timeout:
		c.Assert(err, gc.Not(gc.IsNil))
		c.Check(err, gc.ErrorMatches, "timeout reached waiting for containers to shutdown")
		c.Check(ok, gc.Equals, true)
	case <-time.After(10 * time.Second):
		c.Error("expected the timeout channel to be signalled")
	}
}
