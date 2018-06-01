// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	"io/ioutil"
	"net"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/machiner"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
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
	s.PatchValue(machiner.GetObservedNetworkConfig, func(_ common.NetworkConfigSource) ([]params.NetworkConfig, error) {
		return nil, nil
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

func (s *MachinerSuite) TestMachinerSetUpMachineNotFound(c *gc.C) {
	s.accessor.SetErrors(
		&params.Error{Code: params.CodeNotFound}, // Machine
	)
	var machineDead machineDeathTracker
	w, err := machiner.NewMachiner(machiner.Config{
		s.accessor, s.machineTag, false,
		machineDead.machineDead,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = stopWorker(w)
	c.Assert(errors.Cause(err), gc.Equals, jworker.ErrTerminateAgent)
	c.Assert(bool(machineDead), jc.IsFalse)
}

func (s *MachinerSuite) TestMachinerMachineRefreshNotFound(c *gc.C) {
	s.testMachinerMachineRefreshNotFoundOrUnauthorized(c, params.CodeNotFound)
}

func (s *MachinerSuite) TestMachinerMachineRefreshUnauthorized(c *gc.C) {
	s.testMachinerMachineRefreshNotFoundOrUnauthorized(c, params.CodeUnauthorized)
}

func (s *MachinerSuite) testMachinerMachineRefreshNotFoundOrUnauthorized(c *gc.C, code string) {
	// Accessing the machine initially yields "not found or unauthorized".
	// We don't know which, so we don't report that the machine is dead.
	s.accessor.machine.SetErrors(
		nil, // SetMachineAddresses
		nil, // SetStatus
		nil, // Watch
		&params.Error{Code: code}, // Refresh
	)
	var machineDead machineDeathTracker
	w, err := machiner.NewMachiner(machiner.Config{
		s.accessor, s.machineTag, false,
		machineDead.machineDead,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)
	c.Assert(errors.Cause(err), gc.Equals, jworker.ErrTerminateAgent)

	// the "machineDead" callback should not be invoked
	// because we don't know whether the agent is
	// legitimately not found or unauthorized; we err on
	// the side of caution, in case the password got mucked
	// up, or state got mucked up (e.g. during an upgrade).
	c.Assert(bool(machineDead), jc.IsFalse)
}

func (s *MachinerSuite) TestMachinerSetStatusStopped(c *gc.C) {
	s.accessor.machine.life = params.Dying
	s.accessor.machine.SetErrors(
		nil, // SetMachineAddresses
		nil, // SetStatus (started)
		nil, // Watch
		nil, // Refresh
		errors.New("cannot set status"), // SetStatus (stopped)
	)
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
	})
	c.Assert(err, jc.ErrorIsNil)
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
		status.Stopped,
		"",
		map[string]interface{}(nil),
	)
}

func (s *MachinerSuite) TestMachinerMachineEnsureDeadError(c *gc.C) {
	s.accessor.machine.life = params.Dying
	s.accessor.machine.SetErrors(
		nil, // SetMachineAddresses
		nil, // SetStatus
		nil, // Watch
		nil, // Refresh
		nil, // SetStatus
		errors.New("cannot ensure machine is dead"), // EnsureDead
	)
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)
	c.Check(
		err, gc.ErrorMatches,
		"machine-123 failed to set machine to dead: cannot ensure machine is dead",
	)
}

func (s *MachinerSuite) TestMachinerMachineAssignedUnits(c *gc.C) {
	s.accessor.machine.life = params.Dying
	s.accessor.machine.SetErrors(
		nil, // SetMachineAddresses
		nil, // SetStatus
		nil, // Watch
		nil, // Refresh
		nil, // SetStatus
		&params.Error{Code: params.CodeHasAssignedUnits}, // EnsureDead
	)
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
	})
	c.Assert(err, jc.ErrorIsNil)
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
			status.Started,
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
			status.Stopped,
			"",
			map[string]interface{}(nil),
		},
	}, {
		FuncName: "EnsureDead",
	}})
}

func (s *MachinerSuite) TestRunStop(c *gc.C) {
	var machineDead machineDeathTracker
	mr := s.makeMachiner(c, false, machineDead.machineDead)
	c.Assert(worker.Stop(mr), jc.ErrorIsNil)
	s.accessor.machine.CheckCallNames(c,
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
	)
}

func (s *MachinerSuite) TestStartSetsStatus(c *gc.C) {
	mr := s.makeMachiner(c, false, nil)
	err := stopWorker(mr)
	c.Assert(err, jc.ErrorIsNil)
	s.accessor.machine.CheckCallNames(c,
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
	)
	s.accessor.machine.CheckCall(
		c, 1, "SetStatus",
		status.Started, "", map[string]interface{}(nil),
	)
}

func (s *MachinerSuite) TestSetDead(c *gc.C) {
	var machineDead machineDeathTracker

	s.accessor.machine.life = params.Dying
	mr := s.makeMachiner(c, false, machineDead.machineDead)
	s.accessor.machine.watcher.changes <- struct{}{}

	err := stopWorker(mr)
	c.Assert(err, gc.Equals, jworker.ErrTerminateAgent)
	c.Assert(bool(machineDead), jc.IsTrue)
}

func (s *MachinerSuite) TestSetMachineAddresses(c *gc.C) {
	s.addresses = []net.Addr{
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

	s.PatchValue(&network.InterfaceByNameAddrs, func(name string) ([]net.Addr, error) {
		if name == "foobar" {
			// The addresses on the LXC bridge
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(10, 0, 3, 1)},
				&net.IPAddr{IP: net.IPv4(10, 0, 3, 4)},
			}, nil
		} else if name == network.DefaultLXDBridge {
			// The addresses on the LXD bridge
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(10, 0, 4, 1)},
				&net.IPAddr{IP: net.IPv4(10, 0, 4, 4)},
			}, nil
		} else if name == network.DefaultKVMBridge {
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(192, 168, 1, 1)},
			}, nil
		}
		c.Fatalf("unknown bridge in testing: %v", name)
		return nil, nil
	})

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
	s.PatchValue(&network.LXCNetDefaultConfig, lxcFakeNetConfig)

	mr := s.makeMachiner(c, false, nil)
	c.Assert(stopWorker(mr), jc.ErrorIsNil)
	s.accessor.machine.CheckCall(c, 0, "SetMachineAddresses", []network.Address{
		network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal),
		network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
		network.NewScopedAddress("::1", network.ScopeMachineLocal),
		network.NewAddress("2001:db8::1"),
	})
}

func (s *MachinerSuite) TestSetMachineAddressesEmpty(c *gc.C) {
	s.addresses = []net.Addr{}
	mr := s.makeMachiner(c, false, nil)
	c.Assert(stopWorker(mr), jc.ErrorIsNil)
	// No call to SetMachineAddresses
	s.accessor.machine.CheckCallNames(c, "SetStatus", "Watch")
}

func (s *MachinerSuite) TestMachineAddressesWithClearFlag(c *gc.C) {
	mr := s.makeMachiner(c, true, nil)
	c.Assert(stopWorker(mr), jc.ErrorIsNil)
	s.accessor.machine.CheckCall(c, 0, "SetMachineAddresses", []network.Address(nil))
}

func (s *MachinerSuite) TestGetObservedNetworkConfigEmpty(c *gc.C) {
	s.PatchValue(machiner.GetObservedNetworkConfig, func(common.NetworkConfigSource) ([]params.NetworkConfig, error) {
		return []params.NetworkConfig{}, nil
	})

	var machineDead machineDeathTracker
	mr := s.makeMachiner(c, false, machineDead.machineDead)
	s.accessor.machine.watcher.changes <- struct{}{}
	c.Assert(stopWorker(mr), jc.ErrorIsNil)

	s.accessor.machine.CheckCallNames(c,
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
		"Refresh",
		"Life",
	)
}

func (s *MachinerSuite) TestSetObservedNetworkConfig(c *gc.C) {
	s.PatchValue(machiner.GetObservedNetworkConfig, func(common.NetworkConfigSource) ([]params.NetworkConfig, error) {
		return []params.NetworkConfig{{}}, nil
	})

	var machineDead machineDeathTracker
	mr := s.makeMachiner(c, false, machineDead.machineDead)
	s.accessor.machine.watcher.changes <- struct{}{}
	c.Assert(stopWorker(mr), jc.ErrorIsNil)

	s.accessor.machine.CheckCallNames(c,
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
		"Refresh",
		"Life",
		"SetObservedNetworkConfig",
	)
}

func (s *MachinerSuite) TestAliveErrorGetObservedNetworkConfig(c *gc.C) {
	s.PatchValue(machiner.GetObservedNetworkConfig, func(common.NetworkConfigSource) ([]params.NetworkConfig, error) {
		return nil, errors.New("no config!")
	})

	var machineDead machineDeathTracker
	mr := s.makeMachiner(c, false, machineDead.machineDead)
	s.accessor.machine.watcher.changes <- struct{}{}
	c.Assert(stopWorker(mr), gc.ErrorMatches, "cannot discover observed network config: no config!")

	s.accessor.machine.CheckCallNames(c,
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
		"Refresh",
		"Life",
	)
	c.Assert(bool(machineDead), jc.IsFalse)
}

func (s *MachinerSuite) makeMachiner(
	c *gc.C,
	ignoreAddresses bool,
	machineDead func() error,
) worker.Worker {
	if machineDead == nil {
		machineDead = func() error { return nil }
	}
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
		ClearMachineAddressesOnStart: ignoreAddresses,
		NotifyMachineDead:            machineDead,
	})
	c.Assert(err, jc.ErrorIsNil)
	return w
}

type machineDeathTracker bool

func (t *machineDeathTracker) machineDead() error {
	*t = true
	return nil
}

func stopWorker(w worker.Worker) error {
	w.Kill()
	return w.Wait()
}
