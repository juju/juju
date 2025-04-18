// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"fmt"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/virtualhostname"
)

type connectionDetails struct {
	startTime   time.Time
	destination virtualhostname.Info
}

type handlers struct {
	logger          Logger
	k8sHandlers     ProxyHandlers
	machineHandlers ProxyHandlers
}

func newHandlers(
	facadeClient FacadeClient,
	logger Logger,
	connector SSHConnector,
) (*handlers, error) {
	if facadeClient == nil {
		return nil, errors.NotValidf("facadeClient is required")
	}
	if logger == nil {
		return nil, errors.NotValidf("logger is required")
	}

	k8sHandlers, err := newK8sHandlers(facadeClient, logger, k8sexec.NewInCluster)
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineHandlers, err := newMachineHandlers(connector, logger)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &handlers{
		k8sHandlers:     k8sHandlers,
		machineHandlers: machineHandlers,
		logger:          logger,
	}, nil
}

// Handle is the main entry point for handling SSH sessions. It determines the
// type of session (k8s, machine) and delegates the handling to the appropriate
// session handler.
func (h *handlers) SessionHandler(session ssh.Session, details connectionDetails) error {
	handleError := func(err error) {
		h.logger.Errorf("proxy failure: %v", err)
		_, _ = session.Stderr().Write([]byte(err.Error() + "\n"))
		_ = session.Exit(1)
	}

	switch details.destination.Target() {
	case virtualhostname.ContainerTarget:
		if err := h.k8sHandlers.SessionHandler(session, details); err != nil {
			err = errors.Annotate(err, "failed to proxy k8s session")
			handleError(err)
			return err
		}
	case virtualhostname.MachineTarget:
	case virtualhostname.UnitTarget:
		if err := h.machineHandlers.SessionHandler(session, details); err != nil {
			err = errors.Annotate(err, "failed to proxy machine session")
			handleError(err)
			return err
		}
	default:
		err := errors.NotValidf("unknown virtualhostname target: %s", details.destination.Target())
		handleError(err)
		return err
	}
	return nil
}

// DirectTCPIPHandler handles direct TCP/IP connections. It determines the type
// of connection (k8s, machine) and delegates the handling to the appropriate
// handler.
func (h *handlers) DirectTCPIPHandler(details connectionDetails) ssh.ChannelHandler {
	switch details.destination.Target() {
	case virtualhostname.ContainerTarget:
		return h.k8sHandlers.DirectTCPIPHandler(details)
	case virtualhostname.MachineTarget:
	case virtualhostname.UnitTarget:
		return h.machineHandlers.DirectTCPIPHandler(details)
	default:
		h.logger.Errorf(fmt.Sprintf("unknown virtualhostname target: %d", details.destination.Target()))
		return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
			_ = newChan.Reject(gossh.ConnectionFailed, "unknown model type")
		}
	}
	return nil
}
