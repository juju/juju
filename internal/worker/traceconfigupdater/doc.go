// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package traceconfigupdater updates the local agent config with the
// controller-wide tracing (OpenTelemetry) configuration.
//
// It is the tracing equivalent of the lokiendpointupdater worker: a
// non-controller agent watches the controller-wide workload tracing
// configuration and rewrites its agent.conf OpenTelemetry settings whenever
// the controller-side config changes.
package traceconfigupdater
