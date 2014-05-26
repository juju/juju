// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/ssh"
)

type SSHCommandSuite struct {
	testing.BaseSuite
	testbin string
	fakessh string
	fakescp string
	client  ssh.Client
}

var _ = gc.Suite(&SSHCommandSuite{})

const echoCommandScript = "#!/bin/sh\necho $0 \"$@\" | tee $0.args"

func (s *SSHCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.testbin = c.MkDir()
	s.fakessh = filepath.Join(s.testbin, "ssh")
	s.fakescp = filepath.Join(s.testbin, "scp")
	err := ioutil.WriteFile(s.fakessh, []byte(echoCommandScript), 0755)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(s.fakescp, []byte(echoCommandScript), 0755)
	c.Assert(err, gc.IsNil)
	s.PatchEnvPathPrepend(s.testbin)
	s.client, err = ssh.NewOpenSSHClient()
	c.Assert(err, gc.IsNil)
	s.PatchValue(ssh.DefaultIdentities, nil)
}

func (s *SSHCommandSuite) command(args ...string) *ssh.Cmd {
	return s.commandOptions(args, nil)
}

func (s *SSHCommandSuite) commandOptions(args []string, opts *ssh.Options) *ssh.Cmd {
	return s.client.Command("localhost", args, opts)
}

func (s *SSHCommandSuite) assertCommandArgs(c *gc.C, cmd *ssh.Cmd, expected string) {
	out, err := cmd.Output()
	c.Assert(err, gc.IsNil)
	c.Assert(strings.TrimSpace(string(out)), gc.Equals, expected)
}

func (s *SSHCommandSuite) TestDefaultClient(c *gc.C) {
	ssh.InitDefaultClient()
	c.Assert(ssh.DefaultClient, gc.FitsTypeOf, &ssh.OpenSSHClient{})
	s.PatchEnvironment("PATH", "")
	ssh.InitDefaultClient()
	c.Assert(ssh.DefaultClient, gc.FitsTypeOf, &ssh.GoCryptoClient{})
}

func (s *SSHCommandSuite) TestCommandSSHPass(c *gc.C) {
	// First create a fake sshpass, but don't set $SSHPASS
	fakesshpass := filepath.Join(s.testbin, "sshpass")
	err := ioutil.WriteFile(fakesshpass, []byte(echoCommandScript), 0755)
	s.assertCommandArgs(c, s.command("echo", "123"),
		s.fakessh+" -o StrictHostKeyChecking no -o PasswordAuthentication no localhost echo 123",
	)
	// Now set $SSHPASS.
	s.PatchEnvironment("SSHPASS", "anyoldthing")
	s.assertCommandArgs(c, s.command("echo", "123"),
		fakesshpass+" -e ssh -o StrictHostKeyChecking no -o PasswordAuthentication no localhost echo 123",
	)
	// Finally, remove sshpass from $PATH.
	err = os.Remove(fakesshpass)
	c.Assert(err, gc.IsNil)
	s.assertCommandArgs(c, s.command("echo", "123"),
		s.fakessh+" -o StrictHostKeyChecking no -o PasswordAuthentication no localhost echo 123",
	)
}

func (s *SSHCommandSuite) TestCommand(c *gc.C) {
	s.assertCommandArgs(c, s.command("echo", "123"),
		s.fakessh+" -o StrictHostKeyChecking no -o PasswordAuthentication no localhost echo 123",
	)
}

func (s *SSHCommandSuite) TestCommandEnablePTY(c *gc.C) {
	var opts ssh.Options
	opts.EnablePTY()
	s.assertCommandArgs(c, s.commandOptions([]string{"echo", "123"}, &opts),
		s.fakessh+" -o StrictHostKeyChecking no -o PasswordAuthentication no -t -t localhost echo 123",
	)
}

func (s *SSHCommandSuite) TestCommandAllowPasswordAuthentication(c *gc.C) {
	var opts ssh.Options
	opts.AllowPasswordAuthentication()
	s.assertCommandArgs(c, s.commandOptions([]string{"echo", "123"}, &opts),
		s.fakessh+" -o StrictHostKeyChecking no localhost echo 123",
	)
}

func (s *SSHCommandSuite) TestCommandIdentities(c *gc.C) {
	var opts ssh.Options
	opts.SetIdentities("x", "y")
	s.assertCommandArgs(c, s.commandOptions([]string{"echo", "123"}, &opts),
		s.fakessh+" -o StrictHostKeyChecking no -o PasswordAuthentication no -i x -i y localhost echo 123",
	)
}

func (s *SSHCommandSuite) TestCommandPort(c *gc.C) {
	var opts ssh.Options
	opts.SetPort(2022)
	s.assertCommandArgs(c, s.commandOptions([]string{"echo", "123"}, &opts),
		s.fakessh+" -o StrictHostKeyChecking no -o PasswordAuthentication no -p 2022 localhost echo 123",
	)
}

func (s *SSHCommandSuite) TestCopy(c *gc.C) {
	var opts ssh.Options
	opts.EnablePTY()
	opts.AllowPasswordAuthentication()
	opts.SetIdentities("x", "y")
	opts.SetPort(2022)
	err := s.client.Copy([]string{"/tmp/blah", "foo@bar.com:baz"}, &opts)
	c.Assert(err, gc.IsNil)
	out, err := ioutil.ReadFile(s.fakescp + ".args")
	c.Assert(err, gc.IsNil)
	// EnablePTY has no effect for Copy
	c.Assert(string(out), gc.Equals, s.fakescp+" -o StrictHostKeyChecking no -i x -i y -P 2022 /tmp/blah foo@bar.com:baz\n")

	// Try passing extra args
	err = s.client.Copy([]string{"/tmp/blah", "foo@bar.com:baz", "-r", "-v"}, &opts)
	c.Assert(err, gc.IsNil)
	out, err = ioutil.ReadFile(s.fakescp + ".args")
	c.Assert(err, gc.IsNil)
	c.Assert(string(out), gc.Equals, s.fakescp+" -o StrictHostKeyChecking no -i x -i y -P 2022 /tmp/blah foo@bar.com:baz -r -v\n")

	// Try interspersing extra args
	err = s.client.Copy([]string{"-r", "/tmp/blah", "-v", "foo@bar.com:baz"}, &opts)
	c.Assert(err, gc.IsNil)
	out, err = ioutil.ReadFile(s.fakescp + ".args")
	c.Assert(err, gc.IsNil)
	c.Assert(string(out), gc.Equals, s.fakescp+" -o StrictHostKeyChecking no -i x -i y -P 2022 -r /tmp/blah -v foo@bar.com:baz\n")
}

func (s *SSHCommandSuite) TestCommandClientKeys(c *gc.C) {
	defer overrideGenerateKey(c).Restore()
	clientKeysDir := c.MkDir()
	defer ssh.ClearClientKeys()
	err := ssh.LoadClientKeys(clientKeysDir)
	c.Assert(err, gc.IsNil)
	ck := filepath.Join(clientKeysDir, "juju_id_rsa")
	var opts ssh.Options
	opts.SetIdentities("x", "y")
	s.assertCommandArgs(c, s.commandOptions([]string{"echo", "123"}, &opts),
		s.fakessh+" -o StrictHostKeyChecking no -o PasswordAuthentication no -i x -i y -i "+ck+" localhost echo 123",
	)
}

func (s *SSHCommandSuite) TestCommandError(c *gc.C) {
	var opts ssh.Options
	err := ioutil.WriteFile(s.fakessh, []byte("#!/bin/sh\nexit 42"), 0755)
	c.Assert(err, gc.IsNil)
	command := s.client.Command("ignored", []string{"echo", "foo"}, &opts)
	err = command.Run()
	c.Assert(cmd.IsRcPassthroughError(err), gc.Equals, true)
}

func (s *SSHCommandSuite) TestCommandDefaultIdentities(c *gc.C) {
	var opts ssh.Options
	tempdir := c.MkDir()
	def1 := filepath.Join(tempdir, "def1")
	def2 := filepath.Join(tempdir, "def2")
	s.PatchValue(ssh.DefaultIdentities, []string{def1, def2})
	// If no identities are specified, then the defaults aren't added.
	s.assertCommandArgs(c, s.commandOptions([]string{"echo", "123"}, &opts),
		s.fakessh+" -o StrictHostKeyChecking no -o PasswordAuthentication no localhost echo 123",
	)
	// If identities are specified, then the defaults are must added.
	// Only the defaults that exist on disk will be added.
	err := ioutil.WriteFile(def2, nil, 0644)
	c.Assert(err, gc.IsNil)
	opts.SetIdentities("x", "y")
	s.assertCommandArgs(c, s.commandOptions([]string{"echo", "123"}, &opts),
		s.fakessh+" -o StrictHostKeyChecking no -o PasswordAuthentication no -i x -i y -i "+def2+" localhost echo 123",
	)
}
