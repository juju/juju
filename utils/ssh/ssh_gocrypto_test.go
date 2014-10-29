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
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/utils/ssh"
)

var (
	testCommand     = []string{"echo", "$abc"}
	testCommandFlat = `echo "\$abc"`
)

type sshServer struct {
	cfg      *cryptossh.ServerConfig
	listener net.Listener
	client   *cryptossh.Client
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
	server.listener, err = net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, gc.IsNil)
	return server
}

func (s *sshServer) run(c *gc.C) {
	netconn, err := s.listener.Accept()
	c.Assert(err, gc.IsNil)
	defer func() {
		err = netconn.Close()
		c.Assert(err, gc.IsNil)
	}()
	conn, chans, reqs, err := cryptossh.NewServerConn(netconn, s.cfg)
	c.Assert(err, gc.IsNil)
	s.client = cryptossh.NewClient(conn, chans, reqs)
	var wg sync.WaitGroup
	defer wg.Wait()
	sessionChannels := s.client.HandleChannelOpen("session")
	c.Assert(sessionChannels, gc.NotNil)
	for newChannel := range sessionChannels {
		c.Assert(newChannel.ChannelType(), gc.Equals, "session")
		channel, reqs, err := newChannel.Accept()
		c.Assert(err, gc.IsNil)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer channel.Close()
			for req := range reqs {
				switch req.Type {
				case "exec":
					c.Assert(req.WantReply, jc.IsTrue)
					n := binary.BigEndian.Uint32(req.Payload[:4])
					command := string(req.Payload[4 : n+4])
					c.Assert(command, gc.Equals, testCommandFlat)
					req.Reply(true, nil)
					channel.Write([]byte("abc value\n"))
					_, err := channel.SendRequest("exit-status", false, cryptossh.Marshal(&struct{ n uint32 }{0}))
					c.Assert(err, gc.IsNil)
					return
				default:
					c.Fatalf("Unexpected request type: %v", req.Type)
				}
			}
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

	s.PatchValue(ssh.SSHDial, func(network, address string, cfg *cryptossh.ClientConfig) (*cryptossh.Client, error) {
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
	opts.SetPort(server.listener.Addr().(*net.TCPAddr).Port)
	cmd := client.Command("127.0.0.1", testCommand, &opts)
	checkedKey := false
	server.cfg.PublicKeyCallback = func(conn cryptossh.ConnMetadata, pubkey cryptossh.PublicKey) (*cryptossh.Permissions, error) {
		c.Check(pubkey, gc.DeepEquals, key.PublicKey())
		checkedKey = true
		return nil, nil
	}
	go server.run(c)
	out, err := cmd.Output()
	c.Assert(err, gc.IsNil)
	c.Assert(string(out), gc.Equals, "abc value\n")
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
	port := server.listener.Addr().(*net.TCPAddr).Port
	opts.SetProxyCommand(netcat, "-q0", "%h", "%p")
	opts.SetPort(port)
	cmd := client.Command("127.0.0.1", testCommand, &opts)
	server.cfg.PublicKeyCallback = func(_ cryptossh.ConnMetadata, pubkey cryptossh.PublicKey) (*cryptossh.Permissions, error) {
		return nil, nil
	}
	go server.run(c)
	out, err := cmd.Output()
	c.Assert(err, gc.IsNil)
	c.Assert(string(out), gc.Equals, "abc value\n")
	// Ensure the proxy command was executed with the appropriate arguments.
	data, err := ioutil.ReadFile(netcat + ".args")
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, fmt.Sprintf("%s -q0 127.0.0.1 %v\n", netcat, port))
}
