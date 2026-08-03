// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"context"

	"github.com/gliderlabs/ssh"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/core/virtualhostname"
	k8sexec "github.com/juju/juju/internal/provider/kubernetes/exec"
)

type executorFunc func(context.Context, k8sexec.ExecParams, <-chan struct{}) error

func (f executorFunc) Exec(ctx context.Context, params k8sexec.ExecParams, done <-chan struct{}) error {
	return f(ctx, params, done)
}

func (executorFunc) Status(context.Context, k8sexec.StatusParams) (*k8sexec.Status, error) {
	return nil, nil
}

func (executorFunc) Copy(context.Context, k8sexec.CopyParams, <-chan struct{}) error {
	return nil
}

func (executorFunc) RawClient() kubernetes.Interface { return nil }

func (executorFunc) NameSpace() string { return "" }

type resolverFunc func(context.Context, virtualhostname.Info) (string, string, error)

func (f resolverFunc) ResolveK8sExecInfo(ctx context.Context, destination virtualhostname.Info) (string, string, error) {
	return f(ctx, destination)
}

type k8sTestServer struct {
	listener *bufconn.Listener
}

func startK8sTestServer(c *tc.C, server *ssh.Server) *k8sTestServer {
	listener := bufconn.Listen(1024)
	c.Cleanup(func() { listener.Close() })
	go func() {
		_ = server.Serve(listener)
	}()
	return &k8sTestServer{listener: listener}
}

func (s *k8sTestServer) client() (*gossh.Client, error) {
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
