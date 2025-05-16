// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	"net"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/core/life"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/machiner"
	"github.com/juju/juju/rpc/params"
)

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

type MachinerSuite struct {
	coretesting.BaseSuite
	accessor   *mockMachineAccessor
	machineTag names.MachineTag
	addresses  []net.Addr
}

func TestMachinerSuite(t *stdtesting.T) { tc.Run(t, &MachinerSuite{}) }
func (s *MachinerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.accessor = &mockMachineAccessor{}
	s.accessor.machine.watcher.changes = make(chan struct{})
	s.accessor.machine.life = life.Alive
	s.machineTag = names.NewMachineTag("123")
	s.addresses = []net.Addr{ // anything will do
		&net.IPAddr{IP: net.IPv4bcast},
		&net.IPAddr{IP: net.IPv4zero},
	}
	s.PatchValue(machiner.InterfaceAddrs, func() ([]net.Addr, error) {
		return s.addresses, nil
	})

	s.PatchValue(&machiner.GetObservedNetworkConfig, func(_ corenetwork.ConfigSource) (corenetwork.InterfaceInfos, error) {
		return nil, nil
	})
}

func (s *MachinerSuite) TestMachinerConfigValidate(c *tc.C) {
	_, err := machiner.NewMachiner(machiner.Config{})
	c.Assert(err, tc.ErrorMatches, "validating config: unspecified MachineAccessor not valid")
	_, err = machiner.NewMachiner(machiner.Config{
		MachineAccessor: &mockMachineAccessor{},
	})
	c.Assert(err, tc.ErrorMatches, "validating config: unspecified Tag not valid")

	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: &mockMachineAccessor{},
		Tag:             names.NewMachineTag("123"),
	})
	c.Assert(err, tc.ErrorIsNil)

	// must stop the worker to prevent a data race when cleanup suite
	// rolls back the patches
	err = stopWorker(w)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachinerSuite) TestMachinerSetUpMachineNotFound(c *tc.C) {
	s.accessor.SetErrors(
		&params.Error{Code: params.CodeNotFound}, // Machine
	)
	w, err := machiner.NewMachiner(machiner.Config{
		s.accessor, s.machineTag, false,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = stopWorker(w)
	c.Assert(errors.Cause(err), tc.Equals, jworker.ErrTerminateAgent)
}

func (s *MachinerSuite) TestMachinerMachineRefreshNotFound(c *tc.C) {
	s.testMachinerMachineRefreshNotFoundOrUnauthorized(c, params.CodeNotFound)
}

func (s *MachinerSuite) TestMachinerMachineRefreshUnauthorized(c *tc.C) {
	s.testMachinerMachineRefreshNotFoundOrUnauthorized(c, params.CodeUnauthorized)
}

func (s *MachinerSuite) testMachinerMachineRefreshNotFoundOrUnauthorized(c *tc.C, code string) {
	// Accessing the machine initially yields "not found or unauthorized".
	// We don't know which, so we don't report that the machine is dead.
	s.accessor.machine.SetErrors(
		nil,                       // SetMachineAddresses
		nil,                       // SetStatus
		nil,                       // Watch
		&params.Error{Code: code}, // Refresh
	)
	w, err := machiner.NewMachiner(machiner.Config{
		s.accessor, s.machineTag, false,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)
	c.Assert(err, tc.ErrorIs, jworker.ErrTerminateAgent)
}

func (s *MachinerSuite) TestMachinerSetStatusStopped(c *tc.C) {
	s.accessor.machine.life = life.Dying
	s.accessor.machine.SetErrors(
		nil,                             // Watch
		nil,                             // Refresh
		errors.New("cannot set status"), // SetStatus (stopped)
	)
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)
	c.Assert(
		err, tc.ErrorMatches,
		"machine-123 failed to set status stopped: cannot set status",
	)
	s.accessor.machine.CheckCallNames(c,
		"Life",
		"Watch",
		"Refresh",
		"Life",
		"SetStatus",
	)
	s.accessor.machine.CheckCall(
		c, 4, "SetStatus",
		status.Stopped,
		"",
		map[string]interface{}(nil),
	)
}

func (s *MachinerSuite) TestMachinerMachineEnsureDeadError(c *tc.C) {
	s.accessor.machine.life = life.Dying
	s.accessor.machine.SetErrors(
		nil, // Watch
		nil, // Refresh
		nil, // SetStatus
		errors.New("cannot ensure machine is dead"), // EnsureDead
	)
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)
	c.Check(
		err, tc.ErrorMatches,
		"machine-123 failed to set machine to dead: cannot ensure machine is dead",
	)
	s.accessor.machine.CheckCall(
		c, 6, "SetStatus",
		status.Error,
		"destroying machine: machine-123 failed to set machine to dead: cannot ensure machine is dead",
		map[string]interface{}(nil),
	)
}

func (s *MachinerSuite) TestMachinerMachineAssignedUnits(c *tc.C) {
	s.accessor.machine.life = life.Dying
	s.accessor.machine.SetErrors(
		nil, // Watch
		nil, // Refresh
		nil, // SetStatus
		&params.Error{Code: params.CodeHasAssignedUnits}, // EnsureDead
	)
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)

	// If EnsureDead fails with "machine has assigned units", then
	// the worker will not fail, but will wait for more events.
	c.Check(err, tc.ErrorIsNil)

	s.accessor.machine.CheckCallNames(c,
		"Life",
		"Watch",
		"Refresh",
		"Life",
		"SetStatus",
		"EnsureDead",
	)
}

func (s *MachinerSuite) TestMachinerMachineHasContainers(c *tc.C) {
	s.accessor.machine.life = life.Dying
	s.accessor.machine.SetErrors(
		nil, // Watch
		nil, // Refresh
		nil, // SetStatus
		&params.Error{Code: params.CodeMachineHasContainers}, // EnsureDead
	)
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor,
		Tag:             s.machineTag,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(w)

	// If EnsureDead fails with "machine has containers", then
	// the worker will fail and restart.
	c.Check(err, tc.Satisfies, params.IsCodeMachineHasContainers)

	s.accessor.machine.CheckCallNames(c,
		"Life",
		"Watch",
		"Refresh",
		"Life",
		"SetStatus",
		"EnsureDead",
	)
}

func (s *MachinerSuite) TestMachinerStorageAttached(c *tc.C) {
	// Machine is dying. We'll respond to "EnsureDead" by
	// saying that there are still storage attachments;
	// this should not cause an error.
	s.accessor.machine.life = life.Dying
	s.accessor.machine.SetErrors(
		nil, // Watch
		nil, // Refresh
		nil, // SetStatus
		&params.Error{Code: params.CodeMachineHasAttachedStorage},
	)

	worker, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor, Tag: s.machineTag,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(worker)
	c.Check(err, tc.ErrorIsNil)

	s.accessor.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Machine",
		Args:     []interface{}{s.machineTag},
	}})

	s.accessor.machine.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Life",
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

func (s *MachinerSuite) TestMachinerTryAgain(c *tc.C) {
	// Machine is dying. We'll respond to "EnsureDead" by
	// saying that we need to try again;
	// this should not cause an error.
	s.accessor.machine.life = life.Dying
	s.accessor.machine.SetErrors(
		nil, // Watch
		nil, // Refresh
		nil, // SetStatus
		&params.Error{Code: params.CodeTryAgain},
	)

	worker, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor: s.accessor, Tag: s.machineTag,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.accessor.machine.watcher.changes <- struct{}{}
	err = stopWorker(worker)
	c.Check(err, tc.ErrorIsNil)

	s.accessor.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Machine",
		Args:     []interface{}{s.machineTag},
	}})

	s.accessor.machine.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Life",
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

func (s *MachinerSuite) TestRunStop(c *tc.C) {
	mr := s.makeMachiner(c, false)
	c.Assert(worker.Stop(mr), tc.ErrorIsNil)
	s.accessor.machine.CheckCallNames(c,
		"Life",
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
	)
}

func (s *MachinerSuite) TestStartSetsStatus(c *tc.C) {
	mr := s.makeMachiner(c, false)
	err := stopWorker(mr)
	c.Assert(err, tc.ErrorIsNil)
	s.accessor.machine.CheckCallNames(c,
		"Life",
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
	)
	s.accessor.machine.CheckCall(
		c, 2, "SetStatus",
		status.Started, "", map[string]interface{}(nil),
	)
}

func (s *MachinerSuite) TestSetDead(c *tc.C) {
	s.accessor.machine.life = life.Dying
	mr := s.makeMachiner(c, false)
	s.accessor.machine.watcher.changes <- struct{}{}

	err := stopWorker(mr)
	c.Assert(err, tc.ErrorIs, jworker.ErrTerminateAgent)
}

func (s *MachinerSuite) TestSetMachineAddresses(c *tc.C) {
	s.addresses = []net.Addr{
		&net.IPAddr{IP: net.IPv4(10, 0, 0, 1)},
		&net.IPAddr{IP: net.IPv4(127, 0, 0, 1)},
		&net.IPAddr{IP: net.IPv6loopback},
		&net.UnixAddr{}, // not IP, ignored
		&net.IPNet{IP: net.ParseIP("2001:db8::1")},
		&net.IPAddr{IP: net.IPv4(169, 254, 1, 20)}, // LinkLocal Ignored
		&net.IPNet{IP: net.ParseIP("fe80::1")},     // LinkLocal Ignored
	}

	s.PatchValue(&network.AddressesForInterfaceName, func(name string) ([]string, error) {
		if name == network.DefaultLXDBridge {
			return []string{
				"10.0.4.1",
				"10.0.4.4",
			}, nil
		}
		c.Fatalf("unknown bridge in testing: %v", name)
		return nil, nil
	})

	mr := s.makeMachiner(c, false)
	c.Assert(stopWorker(mr), tc.ErrorIsNil)
	s.accessor.machine.CheckCall(c, 1, "SetMachineAddresses", []corenetwork.MachineAddress{
		corenetwork.NewMachineAddress("10.0.0.1", corenetwork.WithScope(corenetwork.ScopeCloudLocal)),
		corenetwork.NewMachineAddress("127.0.0.1", corenetwork.WithScope(corenetwork.ScopeMachineLocal)),
		corenetwork.NewMachineAddress("::1", corenetwork.WithScope(corenetwork.ScopeMachineLocal)),
		corenetwork.NewMachineAddress("2001:db8::1"),
	})
}

func (s *MachinerSuite) TestSetMachineAddressesEmpty(c *tc.C) {
	s.addresses = []net.Addr{}
	mr := s.makeMachiner(c, false)
	c.Assert(stopWorker(mr), tc.ErrorIsNil)
	// No call to SetMachineAddresses
	s.accessor.machine.CheckCallNames(c, "Life", "SetStatus", "Watch")
}

func (s *MachinerSuite) TestMachineAddressesWithClearFlag(c *tc.C) {
	mr := s.makeMachiner(c, true)
	c.Assert(stopWorker(mr), tc.ErrorIsNil)
	s.accessor.machine.CheckCall(c, 1, "SetMachineAddresses", []corenetwork.MachineAddress(nil))
}

func (s *MachinerSuite) TestGetObservedNetworkConfigEmpty(c *tc.C) {
	s.PatchValue(&machiner.GetObservedNetworkConfig, func(source corenetwork.ConfigSource) (corenetwork.InterfaceInfos, error) {
		return corenetwork.InterfaceInfos{}, nil
	})

	mr := s.makeMachiner(c, false)
	s.accessor.machine.watcher.changes <- struct{}{}
	c.Assert(stopWorker(mr), tc.ErrorIsNil)

	s.accessor.machine.CheckCallNames(c,
		"Life",
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
		"Refresh",
		"Life",
	)
}

func (s *MachinerSuite) TestSetObservedNetworkConfig(c *tc.C) {
	s.PatchValue(&machiner.GetObservedNetworkConfig, func(source corenetwork.ConfigSource) (corenetwork.InterfaceInfos, error) {
		return corenetwork.InterfaceInfos{{}}, nil
	})

	mr := s.makeMachiner(c, false)
	s.accessor.machine.watcher.changes <- struct{}{}
	c.Assert(stopWorker(mr), tc.ErrorIsNil)

	s.accessor.machine.CheckCallNames(c,
		"Life",
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
		"Refresh",
		"Life",
		"SetObservedNetworkConfig",
	)
}

func (s *MachinerSuite) TestAliveErrorGetObservedNetworkConfig(c *tc.C) {
	s.PatchValue(&machiner.GetObservedNetworkConfig, func(source corenetwork.ConfigSource) (corenetwork.InterfaceInfos, error) {
		return nil, errors.New("no config!")
	})

	mr := s.makeMachiner(c, false)
	s.accessor.machine.watcher.changes <- struct{}{}
	c.Assert(stopWorker(mr), tc.ErrorMatches, "cannot discover observed network config: no config!")

	s.accessor.machine.CheckCallNames(c,
		"Life",
		"SetMachineAddresses",
		"SetStatus",
		"Watch",
		"Refresh",
		"Life",
	)
}

func (s *MachinerSuite) makeMachiner(
	c *tc.C,
	ignoreAddresses bool,
) worker.Worker {
	w, err := machiner.NewMachiner(machiner.Config{
		MachineAccessor:              s.accessor,
		Tag:                          s.machineTag,
		ClearMachineAddressesOnStart: ignoreAddresses,
	})
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func stopWorker(w worker.Worker) error {
	w.Kill()
	return w.Wait()
}
