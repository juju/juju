// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallerembedded

import (
	"github.com/juju/juju/caas"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/broker_mock.go github.com/juju/juju/worker/caasfirewallerembedded CAASBroker,PortMutator,ServiceUpdater

// CAASBroker exposes CAAS broker functionality to a worker.
type CAASBroker interface {
	Application(string, caas.DeploymentType) caas.Application
}

// PortMutator exposes CAAS application functionality to a worker.
type PortMutator interface {
	UpdatePorts(ports []caas.ServicePort) error
}

// ServiceUpdater exposes CAAS application functionality to a worker.
type ServiceUpdater interface {
	UpdateService(caas.ServiceParam) error
}
