// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package tracing holds charm tracing configuration. This configuration will be
// used by the controller charm to configure OTEL collectors for Juju's
// distributed tracing. Charms will get information about the OTEL collector
// endpoints and certificate through the context for hook invocations.
//
// For more information about tracing in Juju see the following:
// - github.com/juju/juju/core/trace
// - github.com/juju/juju/internal/worker/trace
package tracing
