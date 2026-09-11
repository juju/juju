// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/internal/sshproxy"
	"github.com/juju/juju/internal/worker/sshserver/handlers/common"
	"github.com/juju/juju/internal/worker/sshserver/handlers/k8s"
	"github.com/juju/juju/internal/worker/sshserver/handlers/machine"
)

type proxyFactory struct {
	k8sResolver k8s.Resolver
	logger      logger.Logger
	connector   machine.SSHConnector
	getExecutor k8s.ExecutorGetter
	metrics     *Collector
}

type sessionMetrics struct {
	collector *Collector
	modelType string
}

func (m sessionMetrics) ObserveTimeToSession(ctx context.Context) {
	started, ok := ctx.Value(connectionStartTime{}).(time.Time)
	if !ok {
		return
	}
	m.collector.timeToSession.WithLabelValues(m.modelType).Observe(time.Since(started).Seconds())
}

// New returns a set of handlers for the given target based
// on whether the target is a container, unit or machine.
func (f proxyFactory) New(destination virtualhostname.Info) (sshproxy.ProxyHandlers, error) {
	modelType := "machine"
	if destination.Target() == virtualhostname.ContainerTarget {
		modelType = "k8s"
	}
	var metrics common.Metrics = sessionMetrics{collector: f.metrics, modelType: modelType}

	switch destination.Target() {
	case virtualhostname.ContainerTarget:
		return k8s.NewHandlers(destination, f.k8sResolver, f.getExecutor, f.logger, metrics)
	case virtualhostname.MachineTarget, virtualhostname.UnitTarget:
		return machine.NewHandlers(destination, f.connector, f.logger, metrics)
	default:
		return nil, errors.NotValidf("unknown virtual hostname target %d", destination.Target())
	}
}
