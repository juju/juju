// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"encoding/binary"
	"net"
	"sync"

	cryptossh "code.google.com/p/go.crypto/ssh"
	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
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
	testbase.LoggingSuite
	client ssh.Client
}

var _ = gc.Suite(&SSHGoCryptoCommandSuite{})

func (s *SSHGoCryptoCommandSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
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
	cmd = client.Command("0.1.2.3", []string{"echo", "123"}, nil)
	_, err = cmd.Output()
	// error message differs based on whether using cgo or not
	c.Assert(err, gc.ErrorMatches, `(dial tcp )?0\.1\.2\.3:22: invalid argument`)
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
	err = client.Copy("0.1.2.3:b", c.MkDir(), nil)
	c.Assert(err, gc.ErrorMatches, "Copy is not implemented")
}
