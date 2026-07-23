// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package k8s provides SSH handlers for Kubernetes unit containers.
package k8s

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/virtualhostname"
	k8sexec "github.com/juju/juju/internal/provider/kubernetes/exec"
)

// Resolver resolves Kubernetes pod information for a routed destination.
type Resolver interface {
	ResolveK8sExecInfo(context.Context, virtualhostname.Info) (namespace, podName string, err error)
}

// Handlers provides SSH channel handlers for a Kubernetes container target.
type Handlers struct {
	resolver    Resolver
	logger      logger.Logger
	getExecutor func(string) (k8sexec.Executor, error)
	destination virtualhostname.Info
}

// NewHandlers returns handlers for a Kubernetes container target.
func NewHandlers(destination virtualhostname.Info, resolver Resolver, logger logger.Logger, getExecutor func(string) (k8sexec.Executor, error)) (*Handlers, error) {
	if resolver == nil {
		return nil, errors.New("Kubernetes resolver is required")
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	if getExecutor == nil {
		return nil, errors.New("executor is required")
	}
	if destination.Target() != virtualhostname.ContainerTarget {
		return nil, errors.New("destination must be a container target")
	}
	return &Handlers{
		resolver:    resolver,
		logger:      logger,
		getExecutor: getExecutor,
		destination: destination,
	}, nil
}
