// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"time"

	"github.com/juju/errors"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/internal/worker/sshserver/handlers/k8s"
	"github.com/juju/juju/internal/worker/sshserver/handlers/machine"
)

type proxyFactory struct {
	k8sResolver k8s.Resolver
	logger      Logger
	connector   machine.SSHConnector
}

// ConnectionInfo contains details about the connection to be proxied.
type ConnectionInfo struct {
	startTime   time.Time
	destination virtualhostname.Info
}

// New creates a new set of proxy handlers based on the destination type.
func (b proxyFactory) New(info ConnectionInfo) (ProxyHandlers, error) {
	switch info.destination.Target() {
	case virtualhostname.ContainerTarget:
		k8sHandlers, err := k8s.NewHandlers(info.destination, b.k8sResolver, b.logger, k8sexec.NewInCluster)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return k8sHandlers, nil
	case virtualhostname.MachineTarget, virtualhostname.UnitTarget:
		// We validate the hostname prior, ensuring unit targets are machines.
		machineHandlers, err := machine.NewHandlers(info.destination, b.connector, b.logger)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return machineHandlers, nil
	default:
		return nil, errors.NotValidf("unknown virtualhostname target: %s", info.destination.Target())
	}
}
