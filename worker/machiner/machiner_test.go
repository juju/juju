// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	"io/ioutil"
	"net"
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
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

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 5 * time.Second

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type MachinerSuite struct {
	testing.JujuConnSuite

	st            *api.State
	machinerState *apimachiner.State
	machine       *state.Machine
	apiMachine    *apimachiner.Machine
}

var _ = gc.Suite(&MachinerSuite{})

func (s *MachinerSuite) SetUpTest(c *gc.C) {
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

func (s *MachinerSuite) waitMachineStatus(c *gc.C, m *state.Machine, expectStatus state.Status) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for machine status to change")
		case <-time.After(10 * time.Millisecond):
			status, _, _, err := m.Status()
			c.Assert(err, jc.ErrorIsNil)
			if status != expectStatus {
				c.Logf("machine %q status is %s, still waiting", m, status)
				continue
			}
			return
		}
	}
}

var _ worker.NotifyWatchHandler = (*machiner.Machiner)(nil)

type mockConfig struct {
	agent.Config
	tag names.Tag
}

func (mock *mockConfig) Tag() names.Tag {
	return mock.tag
}

func agentConfig(tag names.Tag) agent.Config {
	return &mockConfig{tag: tag}
}

func (s *MachinerSuite) TestNotFoundOrUnauthorized(c *gc.C) {
	mr := machiner.NewMachiner(s.machinerState, agentConfig(names.NewMachineTag("99")))
	c.Assert(mr.Wait(), gc.Equals, worker.ErrTerminateAgent)
}

func (s *MachinerSuite) makeMachiner() worker.Worker {
	return machiner.NewMachiner(s.machinerState, agentConfig(s.apiMachine.Tag()))
}

func (s *MachinerSuite) TestRunStop(c *gc.C) {
	mr := s.makeMachiner()
	c.Assert(worker.Stop(mr), gc.IsNil)
	c.Assert(s.apiMachine.Refresh(), gc.IsNil)
	c.Assert(s.apiMachine.Life(), gc.Equals, params.Alive)
}

func (s *MachinerSuite) TestStartSetsStatus(c *gc.C) {
	status, info, _, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusPending)
	c.Assert(info, gc.Equals, "")

	mr := s.makeMachiner()
	defer worker.Stop(mr)

	s.waitMachineStatus(c, s.machine, state.StatusStarted)
}

func (s *MachinerSuite) TestSetsStatusWhenDying(c *gc.C) {
	mr := s.makeMachiner()
	defer worker.Stop(mr)
	c.Assert(s.machine.Destroy(), gc.IsNil)
	s.waitMachineStatus(c, s.machine, state.StatusStopped)
}

func (s *MachinerSuite) TestSetDead(c *gc.C) {
	mr := s.makeMachiner()
	defer worker.Stop(mr)
	c.Assert(s.machine.Destroy(), gc.IsNil)
	s.State.StartSync()
	c.Assert(mr.Wait(), gc.Equals, worker.ErrTerminateAgent)
	c.Assert(s.machine.Refresh(), gc.IsNil)
	c.Assert(s.machine.Life(), gc.Equals, state.Dead)
}

func (s *MachinerSuite) TestMachineAddresses(c *gc.C) {
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

	mr := s.makeMachiner()
	defer worker.Stop(mr)
	c.Assert(s.machine.Destroy(), gc.IsNil)
	s.State.StartSync()
	c.Assert(mr.Wait(), gc.Equals, worker.ErrTerminateAgent)
	c.Assert(s.machine.Refresh(), gc.IsNil)
	c.Assert(s.machine.MachineAddresses(), jc.DeepEquals, []network.Address{
		network.NewAddress("2001:db8::1"),
		network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal),
		network.NewScopedAddress("::1", network.ScopeMachineLocal),
		network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
	})
}
