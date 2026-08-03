// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"

	"github.com/juju/juju/core/watcher"
	tracingservice "github.com/juju/juju/domain/tracing/service"
)

// ControllerTracingConfigService is an interface that provides access to the
// controller-wide tracing configuration.
type ControllerTracingConfigService interface {
	// GetWorkloadTracingConfig reports the controller-wide workload tracing
	// configuration.
	GetWorkloadTracingConfig(ctx context.Context) (tracingservice.WorkloadTracingConfig, error)

	// WatchWorkloadTracingConfig starts a watcher for controller-wide
	// workload tracing configuration changes.
	WatchWorkloadTracingConfig(ctx context.Context) (watcher.NotifyWatcher, error)
}
