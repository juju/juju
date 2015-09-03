// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package ssh_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/utils/ssh"
)

const (
	echoCommand = "/bin/echo"
	echoScript  = "#!/bin/sh\n" + echoCommand + " $0 \"$@\" | /usr/bin/tee $0.args"
)

type SSHCommandSuite struct {
	testing.IsolationSuite
	originalPath string
	testbin      string
	fakessh      string
	fakescp      string
	client       ssh.Client
}

var _ = gc.Suite(&SSHCommandSuite{})

func (s *SSHCommandSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.testbin = c.MkDir()
	s.fakessh = filepath.Join(s.testbin, "ssh")
	s.fakescp = filepath.Join(s.testbin, "scp")
	err := ioutil.WriteFile(s.fakessh, []byte(echoScript), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(s.fakescp, []byte(echoScript), 0755)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchEnvPathPrepend(s.testbin)
	s.client, err = ssh.NewOpenSSHClient()
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
	err := ioutil.WriteFile(fakesshpass, []byte(echoScript), 0755)
	s.assertCommandArgs(c, s.command(echoCommand, "123"),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 localhost %s 123",
			s.fakessh, echoCommand),
	)
	// Now set $SSHPASS.
	s.PatchEnvironment("SSHPASS", "anyoldthing")
	s.assertCommandArgs(c, s.command(echoCommand, "123"),
		fmt.Sprintf("%s -e ssh -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 localhost %s 123",
			fakesshpass, echoCommand),
	)
	// Finally, remove sshpass from $PATH.
	err = os.Remove(fakesshpass)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCommandArgs(c, s.command(echoCommand, "123"),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 localhost %s 123",
			s.fakessh, echoCommand),
	)
}

func (s *SSHCommandSuite) TestCommand(c *gc.C) {
	s.assertCommandArgs(c, s.command(echoCommand, "123"),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 localhost %s 123",
			s.fakessh, echoCommand),
	)
}

func (s *SSHCommandSuite) TestCommandEnablePTY(c *gc.C) {
	var opts ssh.Options
	opts.EnablePTY()
	s.assertCommandArgs(c, s.commandOptions([]string{echoCommand, "123"}, &opts),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 -t -t localhost %s 123",
			s.fakessh, echoCommand),
	)
}

func (s *SSHCommandSuite) TestCommandSetKnownHostsFile(c *gc.C) {
	var opts ssh.Options
	opts.SetKnownHostsFile("/tmp/known hosts")
	s.assertCommandArgs(c, s.commandOptions([]string{echoCommand, "123"}, &opts),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 -o UserKnownHostsFile \"/tmp/known hosts\" localhost %s 123",
			s.fakessh, echoCommand),
	)
}

func (s *SSHCommandSuite) TestCommandAllowPasswordAuthentication(c *gc.C) {
	var opts ssh.Options
	opts.AllowPasswordAuthentication()
	s.assertCommandArgs(c, s.commandOptions([]string{echoCommand, "123"}, &opts),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o ServerAliveInterval 30 localhost %s 123",
			s.fakessh, echoCommand),
	)
}

func (s *SSHCommandSuite) TestCommandIdentities(c *gc.C) {
	var opts ssh.Options
	opts.SetIdentities("x", "y")
	s.assertCommandArgs(c, s.commandOptions([]string{echoCommand, "123"}, &opts),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 -i x -i y localhost %s 123",
			s.fakessh, echoCommand),
	)
}

func (s *SSHCommandSuite) TestCommandPort(c *gc.C) {
	var opts ssh.Options
	opts.SetPort(2022)
	s.assertCommandArgs(c, s.commandOptions([]string{echoCommand, "123"}, &opts),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 -p 2022 localhost %s 123",
			s.fakessh, echoCommand),
	)
}

func (s *SSHCommandSuite) TestCopy(c *gc.C) {
	var opts ssh.Options
	opts.EnablePTY()
	opts.AllowPasswordAuthentication()
	opts.SetIdentities("x", "y")
	opts.SetPort(2022)
	err := s.client.Copy([]string{"/tmp/blah", "foo@bar.com:baz"}, &opts)
	c.Assert(err, jc.ErrorIsNil)
	out, err := ioutil.ReadFile(s.fakescp + ".args")
	c.Assert(err, jc.ErrorIsNil)
	// EnablePTY has no effect for Copy
	c.Assert(string(out), gc.Equals, s.fakescp+" -o StrictHostKeyChecking no -o ServerAliveInterval 30 -i x -i y -P 2022 /tmp/blah foo@bar.com:baz\n")

	// Try passing extra args
	err = s.client.Copy([]string{"/tmp/blah", "foo@bar.com:baz", "-r", "-v"}, &opts)
	c.Assert(err, jc.ErrorIsNil)
	out, err = ioutil.ReadFile(s.fakescp + ".args")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, s.fakescp+" -o StrictHostKeyChecking no -o ServerAliveInterval 30 -i x -i y -P 2022 /tmp/blah foo@bar.com:baz -r -v\n")

	// Try interspersing extra args
	err = s.client.Copy([]string{"-r", "/tmp/blah", "-v", "foo@bar.com:baz"}, &opts)
	c.Assert(err, jc.ErrorIsNil)
	out, err = ioutil.ReadFile(s.fakescp + ".args")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, s.fakescp+" -o StrictHostKeyChecking no -o ServerAliveInterval 30 -i x -i y -P 2022 -r /tmp/blah -v foo@bar.com:baz\n")
}

func (s *SSHCommandSuite) TestCommandClientKeys(c *gc.C) {
	defer overrideGenerateKey(c).Restore()
	clientKeysDir := c.MkDir()
	defer ssh.ClearClientKeys()
	err := ssh.LoadClientKeys(clientKeysDir)
	c.Assert(err, jc.ErrorIsNil)
	ck := filepath.Join(clientKeysDir, "juju_id_rsa")
	var opts ssh.Options
	opts.SetIdentities("x", "y")
	s.assertCommandArgs(c, s.commandOptions([]string{echoCommand, "123"}, &opts),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 -i x -i y -i %s localhost %s 123",
			s.fakessh, ck, echoCommand),
	)
}

func (s *SSHCommandSuite) TestCommandError(c *gc.C) {
	var opts ssh.Options
	err := ioutil.WriteFile(s.fakessh, []byte("#!/bin/sh\nexit 42"), 0755)
	c.Assert(err, jc.ErrorIsNil)
	command := s.client.Command("ignored", []string{echoCommand, "foo"}, &opts)
	err = command.Run()
	c.Assert(cmd.IsRcPassthroughError(err), jc.IsTrue)
}

func (s *SSHCommandSuite) TestCommandDefaultIdentities(c *gc.C) {
	var opts ssh.Options
	tempdir := c.MkDir()
	def1 := filepath.Join(tempdir, "def1")
	def2 := filepath.Join(tempdir, "def2")
	s.PatchValue(ssh.DefaultIdentities, []string{def1, def2})
	// If no identities are specified, then the defaults aren't added.
	s.assertCommandArgs(c, s.commandOptions([]string{echoCommand, "123"}, &opts),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 localhost %s 123",
			s.fakessh, echoCommand),
	)
	// If identities are specified, then the defaults are must added.
	// Only the defaults that exist on disk will be added.
	err := ioutil.WriteFile(def2, nil, 0644)
	c.Assert(err, jc.ErrorIsNil)
	opts.SetIdentities("x", "y")
	s.assertCommandArgs(c, s.commandOptions([]string{echoCommand, "123"}, &opts),
		fmt.Sprintf("%s -o StrictHostKeyChecking no -o PasswordAuthentication no -o ServerAliveInterval 30 -i x -i y -i %s localhost %s 123",
			s.fakessh, def2, echoCommand),
	)
}

func (s *SSHCommandSuite) TestCopyReader(c *gc.C) {
	client := &fakeClient{}
	r := bytes.NewBufferString("<data>")

	err := ssh.TestCopyReader(client, "foo@bar.com:baz", "/tmp/blah", r, nil)
	c.Assert(err, jc.ErrorIsNil)

	client.checkCalls(c, "foo@bar.com:baz", []string{"cat - > /tmp/blah"}, nil, nil, "Command")
	client.impl.checkCalls(c, r, nil, nil, "SetStdio", "Start", "Wait")
}
