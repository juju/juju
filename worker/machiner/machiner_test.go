// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	"io/ioutil"
	"net"
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/machiner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/machiner"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type MachinerSuite struct {
	coretesting.BaseSuite
	accessor   *mockMachineAccessor
	machineTag names.MachineTag
	addresses  []net.Addr
}

var _ = gc.Suite(&MachinerSuite{})

func (s *MachinerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.accessor = &mockMachineAccessor{}
	s.accessor.machine.watcher.changes = make(chan struct{})
	s.accessor.machine.life = params.Alive
	s.machineTag = names.NewMachineTag("123")
	s.addresses = []net.Addr{ // anything will do
		&net.IPAddr{IP: net.IPv4bcast},
		&net.IPAddr{IP: net.IPv4zero},
	}
	s.PatchValue(machiner.InterfaceAddrs, func() ([]net.Addr, error) {
		return s.addresses, nil
	})
}

func (s *MachinerSuite) TestMachinerConfigValidate(c *gc.C) {
	_, err := machiner.NewMachiner(machiner.Config{})
	c.Assert(err, gc.ErrorMatches, "validating config: unspecified MachineAccessor not valid")
	_, err = machiner.NewMachiner(machiner.Config{
		MachineAccessor: &mockMachineAccessor{},
	})
	c.Assert(err, gc.ErrorMatches, "validating config: unspecified Tag not valid")

	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: &mockMachineAccessor{},
		Tag:             names.NewMachineTag("123"),
	})
	c.Assert(err, jc.ErrorIsNil)

	// must stop the worker to prevent a data race when cleanup suite
	// rolls back the patches
	err = stopWorker(w)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachinerSuite) TestMachinerMachineNotFound(c *gc.C) {
	// Accessing the machine initially yields "not found or unauthorized".
	// We don't know which, so we don't report that the machine is dead.
	var machineDead machineDeathTracker
	w, err := machiner.NewMachiner(machiner.Config{
		s.accessor, s.machineTag, false,
		machineDead.machineDead,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.accessor.machine.SetErrors(
		nil, // SetMachineAddresses
		nil, // SetStatus
		nil, // Watch
		&params.Error{Code: params.CodeNotFound}, // Refresh
	)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)
	c.Assert(errors.Cause(err), gc.Equals, worker.ErrTerminateAgent)
	c.Assert(bool(machineDead), jc.IsFalse)
}

func (s *MachinerSuite) TestMachinerSetStatusStopped(c *gc.C) {
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.accessor.machine.life = params.Dying
	s.accessor.machine.SetErrors(
		nil, // SetMachineAddresses
		nil, // SetStatus (started)
		nil, // Watch
		nil, // Refresh
		errors.New("cannot set status"), // SetStatus (stopped)
	)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)
	c.Assert(
		err, gc.ErrorMatches,
		"machine-123 failed to set status stopped: cannot set status",
	)
	s.accessor.machine.CheckCallNames(c,
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
		"Refresh",
		"Life",
		"SetStatus",
	)
	s.accessor.machine.CheckCall(
		c, 5, "SetStatus",
		params.StatusStopped,
		"",
		map[string]interface{}(nil),
	)
}

func (s *MachinerSuite) TestMachinerMachineEnsureDeadError(c *gc.C) {
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.accessor.machine.life = params.Dying
	s.accessor.machine.SetErrors(
		nil, // SetMachineAddresses
		nil, // SetStatus
		nil, // Watch
		nil, // Refresh
		nil, // SetStatus
		errors.New("cannot ensure machine is dead"), // EnsureDead
	)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)
	c.Check(
		err, gc.ErrorMatches,
		"machine-123 failed to set machine to dead: cannot ensure machine is dead",
	)
}

func (s *MachinerSuite) TestMachinerMachineAssignedUnits(c *gc.C) {
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.accessor.machine.life = params.Dying
	s.accessor.machine.SetErrors(
		nil, // SetMachineAddresses
		nil, // SetStatus
		nil, // Watch
		nil, // Refresh
		nil, // SetStatus
		&params.Error{Code: params.CodeHasAssignedUnits}, // EnsureDead
	)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)

	// If EnsureDead fails with "machine has assigned units", then
	// the worker will not fail, but will wait for more events.
	c.Check(err, jc.ErrorIsNil)

	s.accessor.machine.CheckCallNames(c,
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
		"Refresh",
		"Life",
		"SetStatus",
		"EnsureDead",
	)
}

func (s *MachinerSuite) TestMachinerStorageAttached(c *gc.C) {
	// Machine is dying. We'll respond to "EnsureDead" by
	// saying that there are still storage attachments;
	// this should not cause an error.
	s.accessor.machine.life = params.Dying
	s.accessor.machine.SetErrors(
		nil, // SetMachineAddresses
		nil, // SetStatus
		nil, // Watch
		nil, // Refresh
		nil, // SetStatus
		&params.Error{Code: params.CodeMachineHasAttachedStorage},
	)

	worker, err := machiner.NewMachiner(machiner.Config{
		s.accessor, s.machineTag, false,
		func() error { return nil },
	})
	c.Assert(err, jc.ErrorIsNil)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(worker)
	c.Check(err, jc.ErrorIsNil)

	s.accessor.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "Machine",
		Args:     []interface{}{s.machineTag},
	}})

	s.accessor.machine.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "SetMachineAddresses",
		Args: []interface{}{
			network.NewAddresses(
				"255.255.255.255",
				"0.0.0.0",
			),
		},
	}, {
		FuncName: "SetStatus",
		Args: []interface{}{
			params.StatusStarted,
			"",
			map[string]interface{}(nil),
		},
	}, {
		FuncName: "Watch",
	}, {
		FuncName: "Refresh",
	}, {
		FuncName: "Life",
	}, {
		FuncName: "SetStatus",
		Args: []interface{}{
			params.StatusStopped,
			"",
			map[string]interface{}(nil),
		},
	}, {
		FuncName: "EnsureDead",
	}})
}

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 5 * time.Second

type MachinerStateSuite struct {
	testing.JujuConnSuite

	st            api.Connection
	machinerState *apimachiner.State
	machine       *state.Machine
	apiMachine    *apimachiner.Machine
}

var _ = gc.Suite(&MachinerStateSuite{})

func (s *MachinerStateSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.st, s.machine = s.OpenAPIAsNewMachine(c)

	// Create the machiner API facade.
	s.machinerState = s.st.Machiner()
	c.Assert(s.machinerState, gc.NotNil)

	// Get the machine through the facade.
	var err error
	s.apiMachine, err = s.machinerState.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiMachine.Tag(), gc.Equals, s.machine.Tag())
	// Isolate tests better by not using real interface addresses.
	s.PatchValue(machiner.InterfaceAddrs, func() ([]net.Addr, error) {
		return nil, nil
	})
	s.PatchValue(&network.InterfaceByNameAddrs, func(string) ([]net.Addr, error) {
		return nil, nil
	})
	s.PatchValue(&network.LXCNetDefaultConfig, "")

}

func (s *MachinerStateSuite) waitMachineStatus(c *gc.C, m *state.Machine, expectStatus state.Status) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for machine status to change")
		case <-time.After(10 * time.Millisecond):
			statusInfo, err := m.Status()
			c.Assert(err, jc.ErrorIsNil)
			if statusInfo.Status != expectStatus {
				c.Logf("machine %q status is %s, still waiting", m, statusInfo.Status)
				continue
			}
			return
		}
	}
}

func (s *MachinerStateSuite) TestNotFoundOrUnauthorized(c *gc.C) {
	mr, err := machiner.NewMachiner(machiner.Config{
		machiner.APIMachineAccessor{s.machinerState},
		names.NewMachineTag("99"),
		false,
		// the "machineDead" callback should not be invoked
		// because we don't know whether the agent is
		// legimitately not found or unauthorized; we err on
		// the side of caution, in case the password got mucked
		// up, or state got mucked up (e.g. during an upgrade).
		func() error { return errors.New("should not be called") },
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mr.Wait(), gc.Equals, worker.ErrTerminateAgent)
}

func (s *MachinerStateSuite) makeMachiner(
	c *gc.C,
	ignoreAddresses bool,
	machineDead func() error,
) worker.Worker {
	if machineDead == nil {
		machineDead = func() error { return nil }
	}
	w, err := machiner.NewMachiner(machiner.Config{
		machiner.APIMachineAccessor{s.machinerState},
		s.apiMachine.Tag().(names.MachineTag),
		ignoreAddresses,
		machineDead,
	})
	c.Assert(err, jc.ErrorIsNil)
	return w
}

type machineDeathTracker bool

func (t *machineDeathTracker) machineDead() error {
	*t = true
	return nil
}

func (s *MachinerStateSuite) TestRunStop(c *gc.C) {
	var machineDead machineDeathTracker
	mr := s.makeMachiner(c, false, machineDead.machineDead)
	c.Assert(worker.Stop(mr), jc.ErrorIsNil)
	c.Assert(s.apiMachine.Refresh(), jc.ErrorIsNil)
	c.Assert(s.apiMachine.Life(), gc.Equals, params.Alive)
	c.Assert(bool(machineDead), jc.IsFalse)
}

func (s *MachinerStateSuite) TestStartSetsStatus(c *gc.C) {
	statusInfo, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusPending)
	c.Assert(statusInfo.Message, gc.Equals, "")

	mr := s.makeMachiner(c, false, nil)
	defer worker.Stop(mr)

	s.waitMachineStatus(c, s.machine, state.StatusStarted)
}

func (s *MachinerStateSuite) TestSetsStatusWhenDying(c *gc.C) {
	mr := s.makeMachiner(c, false, nil)
	defer worker.Stop(mr)
	c.Assert(s.machine.Destroy(), jc.ErrorIsNil)
	s.waitMachineStatus(c, s.machine, state.StatusStopped)
}

func (s *MachinerStateSuite) TestSetDead(c *gc.C) {
	var machineDead machineDeathTracker
	mr := s.makeMachiner(c, false, machineDead.machineDead)
	defer worker.Stop(mr)
	c.Assert(s.machine.Destroy(), jc.ErrorIsNil)
	s.State.StartSync()
	c.Assert(mr.Wait(), gc.Equals, worker.ErrTerminateAgent)
	c.Assert(s.machine.Refresh(), jc.ErrorIsNil)
	c.Assert(s.machine.Life(), gc.Equals, state.Dead)
	c.Assert(bool(machineDead), jc.IsTrue)
}

func (s *MachinerStateSuite) TestSetDeadWithDyingUnit(c *gc.C) {
	var machineDead machineDeathTracker
	mr := s.makeMachiner(c, false, machineDead.machineDead)
	defer worker.Stop(mr)

	// Add a service, assign to machine.
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)

	// Service alive, can't destroy machine.
	err = s.machine.Destroy()
	c.Assert(err, jc.Satisfies, state.IsHasAssignedUnitsError)

	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// With dying unit, machine can now be marked as dying.
	c.Assert(s.machine.Destroy(), jc.ErrorIsNil)
	s.State.StartSync()
	c.Assert(s.machine.Refresh(), jc.ErrorIsNil)
	c.Assert(s.machine.Life(), gc.Equals, state.Dying)
	c.Assert(bool(machineDead), jc.IsFalse)

	// When the unit is ultimately destroyed, the machine becomes dead.
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()
	c.Assert(mr.Wait(), gc.Equals, worker.ErrTerminateAgent)
	c.Assert(bool(machineDead), jc.IsTrue)

}

func (s *MachinerStateSuite) setupSetMachineAddresses(c *gc.C, ignore bool) {
	lxcFakeNetConfig := filepath.Join(c.MkDir(), "lxc-net")
	netConf := []byte(`
  # comments ignored
LXC_BR= ignored
LXC_ADDR = "fooo"
LXC_BRIDGE="foobar" # detected
anything else ignored
LXC_BRIDGE="ignored"`[1:])
	err := ioutil.WriteFile(lxcFakeNetConfig, netConf, 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(machiner.InterfaceAddrs, func() ([]net.Addr, error) {
		addrs := []net.Addr{
			&net.IPAddr{IP: net.IPv4(10, 0, 0, 1)},
			&net.IPAddr{IP: net.IPv4(127, 0, 0, 1)},
			&net.IPAddr{IP: net.IPv4(10, 0, 3, 1)}, // lxc bridge address ignored
			&net.IPAddr{IP: net.IPv6loopback},
			&net.UnixAddr{},                        // not IP, ignored
			&net.IPAddr{IP: net.IPv4(10, 0, 3, 4)}, // lxc bridge address ignored
			&net.IPNet{IP: net.ParseIP("2001:db8::1")},
			&net.IPAddr{IP: net.IPv4(169, 254, 1, 20)}, // LinkLocal Ignored
			&net.IPNet{IP: net.ParseIP("fe80::1")},     // LinkLocal Ignored
		}
		return addrs, nil
	})
	s.PatchValue(&network.InterfaceByNameAddrs, func(name string) ([]net.Addr, error) {
		c.Assert(name, gc.Equals, "foobar")
		return []net.Addr{
			&net.IPAddr{IP: net.IPv4(10, 0, 3, 1)},
			&net.IPAddr{IP: net.IPv4(10, 0, 3, 4)},
		}, nil
	})
	s.PatchValue(&network.LXCNetDefaultConfig, lxcFakeNetConfig)

	mr := s.makeMachiner(c, ignore, nil)
	defer worker.Stop(mr)
	c.Assert(s.machine.Destroy(), jc.ErrorIsNil)
	s.State.StartSync()
	c.Assert(mr.Wait(), gc.Equals, worker.ErrTerminateAgent)
	c.Assert(s.machine.Refresh(), jc.ErrorIsNil)
}

func (s *MachinerStateSuite) TestMachineAddresses(c *gc.C) {
	s.setupSetMachineAddresses(c, false)
	c.Assert(s.machine.MachineAddresses(), jc.DeepEquals, []network.Address{
		network.NewAddress("2001:db8::1"),
		network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal),
		network.NewScopedAddress("::1", network.ScopeMachineLocal),
		network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
	})
}

func (s *MachinerStateSuite) TestMachineAddressesWithIgnoreFlag(c *gc.C) {
	s.setupSetMachineAddresses(c, true)
	c.Assert(s.machine.MachineAddresses(), gc.HasLen, 0)
}

func stopWorker(w worker.Worker) error {
	w.Kill()
	return w.Wait()
}
