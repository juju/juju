// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/utils/ssh"
)

var _ = gc.Suite(&SSHSuite{})

type SSHSuite struct {
	SSHCommonSuite
}

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

const (
	noProxy           = `-o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 `
	args              = `-o StrictHostKeyChecking no -o ProxyCommand juju ssh --proxy=false --pty=false localhost nc %h %p -o PasswordAuthentication no -o ServerAliveInterval 30 `
	commonArgsNoProxy = noProxy + `-o UserKnownHostsFile /dev/null `
	commonArgs        = args + `-o UserKnownHostsFile /dev/null `
	sshArgs           = args + `-t -t -o UserKnownHostsFile /dev/null `
	sshArgsNoProxy    = noProxy + `-t -t -o UserKnownHostsFile /dev/null `
)

var sshTests = []struct {
	about  string
	args   []string
	result string
}{
	{
		"connect to machine 0",
		[]string{"ssh", "0"},
		sshArgs + "ubuntu@dummyenv-0.internal",
	},
	{
		"connect to machine 0 and pass extra arguments",
		[]string{"ssh", "0", "uname", "-a"},
		sshArgs + "ubuntu@dummyenv-0.internal uname -a",
	},
	{
		"connect to unit mysql/0",
		[]string{"ssh", "mysql/0"},
		sshArgs + "ubuntu@dummyenv-0.internal",
	},
	{
		"connect to unit mongodb/1 as the mongo user",
		[]string{"ssh", "mongo@mongodb/1"},
		sshArgs + "mongo@dummyenv-2.internal",
	},
	{
		"connect to unit mongodb/1 and pass extra arguments",
		[]string{"ssh", "mongodb/1", "ls", "/"},
		sshArgs + "ubuntu@dummyenv-2.internal ls /",
	},
	{
		"connect to unit mysql/0 without proxy",
		[]string{"ssh", "--proxy=false", "mysql/0"},
		sshArgsNoProxy + "ubuntu@dummyenv-0.dns",
	},
}

func (s *SSHSuite) TestSSHCommand(c *gc.C) {
	m := s.makeMachines(3, c, true)
	ch := testcharms.Repo.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	dummy, err := s.State.AddCharm(ch, curl, "dummy-path", "dummy-1-sha256")
	c.Assert(err, jc.ErrorIsNil)
	srv := s.AddTestingService(c, "mysql", dummy)
	s.addUnit(srv, m[0], c)

	srv = s.AddTestingService(c, "mongodb", dummy)
	s.addUnit(srv, m[1], c)
	s.addUnit(srv, m[2], c)

	for i, t := range sshTests {
		c.Logf("test %d: %s -> %s", i, t.about, t.args)
		ctx := coretesting.Context(c)
		jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
		jujucmd.Register(envcmd.Wrap(&SSHCommand{}))

		code := cmd.Main(jujucmd, ctx, t.args)
		c.Check(code, gc.Equals, 0)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
		c.Check(strings.TrimRight(ctx.Stdout.(*bytes.Buffer).String(), "\r\n"), gc.Equals, t.result)
	}
}

func (s *SSHSuite) TestSSHCommandEnvironProxySSH(c *gc.C) {
	s.makeMachines(1, c, true)
	// Setting proxy-ssh=false in the environment overrides --proxy.
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"proxy-ssh": false}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx := coretesting.Context(c)
	jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
	jujucmd.Register(envcmd.Wrap(&SSHCommand{}))
	code := cmd.Main(jujucmd, ctx, []string{"ssh", "0"})
	c.Check(code, gc.Equals, 0)
	c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	c.Check(strings.TrimRight(ctx.Stdout.(*bytes.Buffer).String(), "\r\n"), gc.Equals, sshArgsNoProxy+"ubuntu@dummyenv-0.dns")
}

func (s *SSHSuite) TestSSHWillWorkInUpgrade(c *gc.C) {
	// Check the API client interface used by "juju ssh" against what
	// the API server will allow during upgrades. Ensure that the API
	// server will allow all required API calls to support SSH.
	type concrete struct {
		sshAPIClient
	}
	t := reflect.TypeOf(concrete{})
	for i := 0; i < t.NumMethod(); i++ {
		name := t.Method(i).Name

		// Close isn't an API method and ServiceCharmRelations is not
		// relevant to "juju ssh".
		if name == "Close" || name == "ServiceCharmRelations" {
			continue
		}
		c.Logf("checking %q", name)
		c.Check(apiserver.IsMethodAllowedDuringUpgrade("Client", name), jc.IsTrue)
	}
}

type callbackAttemptStarter struct {
	next func() bool
}

func (s *callbackAttemptStarter) Start() attempt {
	return callbackAttempt{next: s.next}
}

type callbackAttempt struct {
	next func() bool
}

func (a callbackAttempt) Next() bool {
	return a.next()
}

func (s *SSHSuite) TestSSHCommandHostAddressRetry(c *gc.C) {
	s.testSSHCommandHostAddressRetry(c, false)
}

func (s *SSHSuite) TestSSHCommandHostAddressRetryProxy(c *gc.C) {
	s.testSSHCommandHostAddressRetry(c, true)
}

func (s *SSHSuite) testSSHCommandHostAddressRetry(c *gc.C, proxy bool) {
	m := s.makeMachines(1, c, false)
	ctx := coretesting.Context(c)

	var called int
	next := func() bool {
		called++
		return called < 2
	}
	attemptStarter := &callbackAttemptStarter{next: next}
	s.PatchValue(&sshHostFromTargetAttemptStrategy, attemptStarter)

	// Ensure that the ssh command waits for a public address, or the attempt
	// strategy's Done method returns false.
	args := []string{"--proxy=" + fmt.Sprint(proxy), "0"}
	code := cmd.Main(envcmd.Wrap(&SSHCommand{}), ctx, args)
	c.Check(code, gc.Equals, 1)
	c.Assert(called, gc.Equals, 2)
	called = 0
	attemptStarter.next = func() bool {
		called++
		if called > 1 {
			s.setAddresses(m[0], c)
		}
		return true
	}
	code = cmd.Main(envcmd.Wrap(&SSHCommand{}), ctx, args)
	c.Check(code, gc.Equals, 0)
	c.Assert(called, gc.Equals, 2)
}

func (s *SSHCommonSuite) setAddresses(m *state.Machine, c *gc.C) {
	addrPub := network.NewScopedAddress(
		fmt.Sprintf("dummyenv-%s.dns", m.Id()),
		network.ScopePublic,
	)
	addrPriv := network.NewScopedAddress(
		fmt.Sprintf("dummyenv-%s.internal", m.Id()),
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
