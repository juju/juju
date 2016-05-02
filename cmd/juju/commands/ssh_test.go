// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&SSHSuite{})

type SSHSuite struct {
	SSHCommonSuite
}

const (
	args                = `-o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 `
	withProxy           = `-o StrictHostKeyChecking no -o ProxyCommand juju ssh --proxy=false --pty=false localhost nc %h %p -o PasswordAuthentication no -o ServerAliveInterval 30 `
	commonArgsWithProxy = withProxy + `-o UserKnownHostsFile /dev/null `
	commonArgs          = args + `-o UserKnownHostsFile /dev/null `
	sshArgs             = args + `-t -t -o UserKnownHostsFile /dev/null `
	sshArgsWithProxy    = withProxy + `-t -t -o UserKnownHostsFile /dev/null `
)

var sshTests = []struct {
	about  string
	args   []string
	result string
}{
	{
		"connect to machine 0",
		[]string{"ssh", "0"},
		sshArgs + "ubuntu@admin-0.dns",
	},
	{
		"connect to machine 0 and pass extra arguments",
		[]string{"ssh", "0", "uname", "-a"},
		sshArgs + "ubuntu@admin-0.dns uname -a",
	},
	{
		"connect to unit mysql/0",
		[]string{"ssh", "mysql/0"},
		sshArgs + "ubuntu@admin-0.dns",
	},
	{
		"connect to unit mongodb/1 as the mongo user",
		[]string{"ssh", "mongo@mongodb/1"},
		sshArgs + "mongo@admin-2.dns",
	},
	{
		"connect to unit mongodb/1 and pass extra arguments",
		[]string{"ssh", "mongodb/1", "ls", "/"},
		sshArgs + "ubuntu@admin-2.dns ls /",
	},
	{
		"connect to unit mysql/0 with proxy",
		[]string{"ssh", "--proxy=true", "mysql/0"},
		sshArgsWithProxy + "ubuntu@admin-0.internal",
	},
}

func (s *SSHSuite) TestSSHCommand(c *gc.C) {
	m := s.makeMachines(3, c, true)
	ch := testcharms.Repo.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-path",
		SHA256:      "dummy-1-sha256",
	}
	dummy, err := s.State.AddCharm(info)
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
		jujucmd.Register(newSSHCommand())

		code := cmd.Main(jujucmd, ctx, t.args)
		c.Check(code, gc.Equals, 0)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
		c.Check(strings.TrimRight(ctx.Stdout.(*bytes.Buffer).String(), "\r\n"), gc.Equals, t.result)
	}
}

func (s *SSHSuite) TestSSHCommandEnvironProxySSH(c *gc.C) {
	s.makeMachines(1, c, true)
	// Setting proxy-ssh=true in the environment overrides --proxy.
	err := s.State.UpdateModelConfig(map[string]interface{}{"proxy-ssh": true}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx := coretesting.Context(c)
	jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
	jujucmd.Register(newSSHCommand())
	code := cmd.Main(jujucmd, ctx, []string{"ssh", "0"})
	c.Check(code, gc.Equals, 0)
	c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	c.Check(strings.TrimRight(ctx.Stdout.(*bytes.Buffer).String(), "\r\n"), gc.Equals, sshArgsWithProxy+"ubuntu@admin-0.internal")
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

		// Close isn't an API method.
		if name == "Close" {
			continue
		}
		c.Logf("checking %q", name)
		c.Check(apiserver.IsMethodAllowedDuringUpgrade("SSHClient", name), jc.IsTrue)
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
	code := cmd.Main(newSSHCommand(), ctx, args)
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
	code = cmd.Main(newSSHCommand(), ctx, args)
	c.Check(code, gc.Equals, 0)
	c.Assert(called, gc.Equals, 2)
}
