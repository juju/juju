// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type SSHCommonSuite struct {
	testing.JujuConnSuite
	bin string
}

func (s *SSHCommonSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(&getJujuExecutable, func() (string, error) { return "juju", nil })

	s.bin = c.MkDir()
	s.PatchEnvPathPrepend(s.bin)
	for _, name := range patchedCommands {
		f, err := os.OpenFile(filepath.Join(s.bin, name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
		c.Assert(err, jc.ErrorIsNil)
		_, err = f.Write([]byte(fakecommand))
		c.Assert(err, jc.ErrorIsNil)
		err = f.Close()
		c.Assert(err, jc.ErrorIsNil)
	}
	client, _ := ssh.NewOpenSSHClient()
	s.PatchValue(&ssh.DefaultClient, client)
}

func (s *SSHCommonSuite) setAddresses(m *state.Machine, c *gc.C) {
	addrPub := network.NewScopedAddress(
		fmt.Sprintf("admin-%s.dns", m.Id()),
		network.ScopePublic,
	)
	addrPriv := network.NewScopedAddress(
		fmt.Sprintf("admin-%s.internal", m.Id()),
		network.ScopeCloudLocal,
	)
	err := m.SetProviderAddresses(addrPub, addrPriv)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SSHCommonSuite) makeMachines(n int, c *gc.C, setAddresses bool) []*state.Machine {
	var machines = make([]*state.Machine, n)
	for i := 0; i < n; i++ {
		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		if setAddresses {
			s.setAddresses(m, c)
		}
		// must set an instance id as the ssh command uses that as a signal the
		// machine has been provisioned
		inst, md := testing.AssertStartInstance(c, s.Environ, m.Id())
		c.Assert(m.SetProvisioned(inst.Id(), "fake_nonce", md), gc.IsNil)
		machines[i] = m
	}
	return machines
}

func (s *SSHCommonSuite) addUnit(srv *state.Service, m *state.Machine, c *gc.C) {
	u, err := srv.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
}
