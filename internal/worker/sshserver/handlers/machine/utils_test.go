// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"

	"github.com/gliderlabs/ssh"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"

	"github.com/juju/juju/core/virtualhostname"
)

type connectorFunc func(context.Context, virtualhostname.Info) (*gossh.Client, error)

func (f connectorFunc) Connect(ctx context.Context, destination virtualhostname.Info) (*gossh.Client, error) {
	return f(ctx, destination)
}

type sshTestServer struct {
	listener *bufconn.Listener
}

func startSSHTestServer(c *tc.C, server *ssh.Server) *sshTestServer {
	listener := bufconn.Listen(1024)
	c.Cleanup(func() { listener.Close() })
	go func() {
		_ = server.Serve(listener)
	}()
	return &sshTestServer{listener: listener}
}

func (s *sshTestServer) client() (*gossh.Client, error) {
	conn, err := s.listener.Dial()
	if err != nil {
		return nil, err
	}
	sshConn, channels, requests, err := gossh.NewClientConn(conn, "", &gossh.ClientConfig{
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return nil, err
	}
	return gossh.NewClient(sshConn, channels, requests), nil
}

func connectorForServer(server *sshTestServer) SSHConnector {
	return connectorFunc(func(context.Context, virtualhostname.Info) (*gossh.Client, error) {
		return server.client()
	})
}
