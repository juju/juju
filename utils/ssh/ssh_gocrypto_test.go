// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os/exec"
	"path/filepath"
	"sync"

	cryptossh "code.google.com/p/go.crypto/ssh"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils/ssh"
)

var (
	testCommand     = []string{"echo", "$abc"}
	testCommandFlat = `echo "\$abc"`
)

type sshServer struct {
	cfg *cryptossh.ServerConfig
	*cryptossh.Listener
}

func newServer(c *gc.C) *sshServer {
	private, _, err := ssh.GenerateKey("test-server")
	c.Assert(err, gc.IsNil)
	key, err := cryptossh.ParsePrivateKey([]byte(private))
	c.Assert(err, gc.IsNil)
	server := &sshServer{
		cfg: &cryptossh.ServerConfig{},
	}
	server.cfg.AddHostKey(key)
	server.Listener, err = cryptossh.Listen("tcp", "127.0.0.1:0", server.cfg)
	c.Assert(err, gc.IsNil)
	return server
}

func (s *sshServer) run(c *gc.C) {
	conn, err := s.Accept()
	c.Assert(err, gc.IsNil)
	defer func() {
		err = conn.Close()
		c.Assert(err, gc.IsNil)
	}()
	err = conn.Handshake()
	c.Assert(err, gc.IsNil)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		channel, err := conn.Accept()
		c.Assert(err, gc.IsNil)
		c.Assert(channel.ChannelType(), gc.Equals, "session")
		channel.Accept()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer channel.Close()
			_, err := channel.Read(nil)
			c.Assert(err, gc.FitsTypeOf, cryptossh.ChannelRequest{})
			req := err.(cryptossh.ChannelRequest)
			c.Assert(req.Request, gc.Equals, "exec")
			c.Assert(req.WantReply, jc.IsTrue)
			n := binary.BigEndian.Uint32(req.Payload[:4])
			command := string(req.Payload[4 : n+4])
			c.Assert(command, gc.Equals, testCommandFlat)
			// TODO(axw) when gosshnew is ready, send reply to client.
		}()
	}
}

type SSHGoCryptoCommandSuite struct {
	testing.IsolationSuite
	client ssh.Client
}

var _ = gc.Suite(&SSHGoCryptoCommandSuite{})

func (s *SSHGoCryptoCommandSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	generateKeyRestorer := overrideGenerateKey(c)
	s.AddCleanup(func(*gc.C) { generateKeyRestorer.Restore() })
	client, err := ssh.NewGoCryptoClient()
	c.Assert(err, gc.IsNil)
	s.client = client
}

func (s *SSHGoCryptoCommandSuite) TestNewGoCryptoClient(c *gc.C) {
	_, err := ssh.NewGoCryptoClient()
	c.Assert(err, gc.IsNil)
	private, _, err := ssh.GenerateKey("test-client")
	c.Assert(err, gc.IsNil)
	key, err := cryptossh.ParsePrivateKey([]byte(private))
	c.Assert(err, gc.IsNil)
	_, err = ssh.NewGoCryptoClient(key)
	c.Assert(err, gc.IsNil)
}

func (s *SSHGoCryptoCommandSuite) TestClientNoKeys(c *gc.C) {
	client, err := ssh.NewGoCryptoClient()
	c.Assert(err, gc.IsNil)
	cmd := client.Command("0.1.2.3", []string{"echo", "123"}, nil)
	_, err = cmd.Output()
	c.Assert(err, gc.ErrorMatches, "no private keys available")
	defer ssh.ClearClientKeys()
	err = ssh.LoadClientKeys(c.MkDir())
	c.Assert(err, gc.IsNil)

	s.PatchValue(ssh.SSHDial, func(network, address string, cfg *cryptossh.ClientConfig) (*cryptossh.ClientConn, error) {
		return nil, errors.New("ssh.Dial failed")
	})
	cmd = client.Command("0.1.2.3", []string{"echo", "123"}, nil)
	_, err = cmd.Output()
	// error message differs based on whether using cgo or not
	c.Assert(err, gc.ErrorMatches, "ssh.Dial failed")
}

func (s *SSHGoCryptoCommandSuite) TestCommand(c *gc.C) {
	private, _, err := ssh.GenerateKey("test-server")
	c.Assert(err, gc.IsNil)
	key, err := cryptossh.ParsePrivateKey([]byte(private))
	client, err := ssh.NewGoCryptoClient(key)
	c.Assert(err, gc.IsNil)
	server := newServer(c)
	var opts ssh.Options
	opts.SetPort(server.Addr().(*net.TCPAddr).Port)
	cmd := client.Command("127.0.0.1", testCommand, &opts)
	checkedKey := false
	server.cfg.PublicKeyCallback = func(conn *cryptossh.ServerConn, user, algo string, pubkey []byte) bool {
		c.Check(pubkey, gc.DeepEquals, cryptossh.MarshalPublicKey(key.PublicKey()))
		checkedKey = true
		return true
	}
	go server.run(c)
	out, err := cmd.Output()
	c.Assert(err, gc.ErrorMatches, "ssh: could not execute command.*")
	// TODO(axw) when gosshnew is ready, expect reply from server.
	c.Assert(out, gc.IsNil)
	c.Assert(checkedKey, jc.IsTrue)
}

func (s *SSHGoCryptoCommandSuite) TestCopy(c *gc.C) {
	client, err := ssh.NewGoCryptoClient()
	c.Assert(err, gc.IsNil)
	err = client.Copy([]string{"0.1.2.3:b", c.MkDir()}, nil)
	c.Assert(err, gc.ErrorMatches, `scp command is not implemented \(OpenSSH scp not available in PATH\)`)
}

func (s *SSHGoCryptoCommandSuite) TestProxyCommand(c *gc.C) {
	realNetcat, err := exec.LookPath("nc")
	if err != nil {
		c.Skip("skipping test, couldn't find netcat: %v")
		return
	}
	netcat := filepath.Join(c.MkDir(), "nc")
	err = ioutil.WriteFile(netcat, []byte("#!/bin/sh\necho $0 \"$@\" > $0.args && exec "+realNetcat+" \"$@\""), 0755)
	c.Assert(err, gc.IsNil)

	private, _, err := ssh.GenerateKey("test-server")
	c.Assert(err, gc.IsNil)
	key, err := cryptossh.ParsePrivateKey([]byte(private))
	client, err := ssh.NewGoCryptoClient(key)
	c.Assert(err, gc.IsNil)
	server := newServer(c)
	var opts ssh.Options
	port := server.Addr().(*net.TCPAddr).Port
	opts.SetProxyCommand(netcat, "-q0", "%h", "%p")
	opts.SetPort(port)
	cmd := client.Command("127.0.0.1", testCommand, &opts)
	server.cfg.PublicKeyCallback = func(conn *cryptossh.ServerConn, user, algo string, pubkey []byte) bool {
		return true
	}
	go server.run(c)
	out, err := cmd.Output()
	c.Assert(err, gc.ErrorMatches, "ssh: could not execute command.*")
	// TODO(axw) when gosshnew is ready, expect reply from server.
	c.Assert(out, gc.IsNil)
	// Ensure the proxy command was executed with the appropriate arguments.
	data, err := ioutil.ReadFile(netcat + ".args")
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, fmt.Sprintf("%s -q0 127.0.0.1 %v\n", netcat, port))
}
