// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"github.com/juju/errors"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/rpc/params"
)

// Resolver is an interface that defines the methods required to resolve
// a model UUID and unit name to a Kubernetes pod and namespace.
type Resolver interface {
	ResolveK8sExecInfo(arg params.SSHK8sExecArg) (params.SSHK8sExecResult, error)
}

// Logger is an interface that defines the methods required to log messages.
type Logger interface {
	Errorf(string, ...interface{})
	Debugf(string, ...interface{})
}

// Handlers provides a set of handlers for SSH sessions to Kubernetes units.
type Handlers struct {
	resolver    Resolver
	logger      Logger
	getExecutor func(string) (k8sexec.Executor, error)
	destination virtualhostname.Info
}

// NewHandlers creates a new set of Kubernetes handlers.
func NewHandlers(destination virtualhostname.Info, resolver Resolver, logger Logger, getExecutor func(string) (k8sexec.Executor, error)) (*Handlers, error) {
	if resolver == nil {
		return nil, errors.NotValidf("k8s resolver is required")
	}
	if logger == nil {
		return nil, errors.NotValidf("logger is required")
	}
	if getExecutor == nil {
		return nil, errors.NotValidf("executor is required")
	}
	if destination.Target() != virtualhostname.ContainerTarget {
		return nil, errors.NotValidf("destination must be a container target")
	}
	return &Handlers{
		resolver:    resolver,
		logger:      logger,
		getExecutor: getExecutor,
		destination: destination,
	}, nil
}
