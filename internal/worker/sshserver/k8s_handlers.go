// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/rpc/params"
)

type k8sHandlers struct {
	facadeClient FacadeClient
	logger       Logger
	getExecutor  func(string) (k8sexec.Executor, error)
}

func newK8sHandlers(facadeClient FacadeClient, logger Logger, getExecutor func(string) (k8sexec.Executor, error)) (*k8sHandlers, error) {
	if facadeClient == nil {
		return nil, errors.NotValidf("facadeClient is required")
	}
	if logger == nil {
		return nil, errors.NotValidf("logger is required")
	}
	if getExecutor == nil {
		return nil, errors.NotValidf("executor is required")
	}
	return &k8sHandlers{
		facadeClient: facadeClient,
		logger:       logger,
		getExecutor:  getExecutor,
	}, nil
}

// SessionHandler handles SSH sessions for Kubernetes pods. It resolves the
// Kubernetes execution information and executes the command in the specified
// pod and container.
func (k8sHandlers *k8sHandlers) SessionHandler(session ssh.Session, details connectionDetails) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	unitName, _ := details.destination.Unit()
	res, err := k8sHandlers.facadeClient.ResolveK8sExecInfo(params.SSHK8sExecArg{
		UnitName:  unitName,
		ModelUUID: details.destination.ModelUUID(),
	})
	if err != nil {
		return errors.Annotate(err, "failed to resolve k8s exec info")
	}

	executor, err := k8sHandlers.getExecutor(res.Namespace)
	if err != nil {
		return errors.Annotate(err, "failed to get executor")
	}
	containerName, _ := details.destination.Container()
	// TODO(JUJU-7880): improve pty sessions
	_, _, pty := session.Pty()
	err = executor.Exec(
		k8sexec.ExecParams{
			PodName:       res.PodName,
			ContainerName: containerName,
			Commands:      session.Command(),
			Stdout:        session,
			Stderr:        session.Stderr(),
			Stdin:         session,
			TTY:           pty,
			Env:           session.Environ(),
		},
		ctx.Done(),
	)
	if err != nil {
		return errors.Annotate(err, "failed to execute command in k8s pod")
	}
	return nil
}

// DirectTCPIPHandler is not supported	and returns a rejection message.
func (k8sHandlers) DirectTCPIPHandler(details connectionDetails) ssh.ChannelHandler {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		_ = newChan.Reject(gossh.Prohibited, "not implemented")
		return
	}
}
