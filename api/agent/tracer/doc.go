// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package tracer provides access to the tracer facade for agent clients.
//
// The tracer facade exposes the controller-wide workload tracing
// configuration so that non-controller agents can keep their local
// agent.conf OpenTelemetry settings in sync with the controller.
package tracer
