// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package common contains shared handler interfaces.
package common

import "context"

// Metrics records metrics for an SSH proxy session.
type Metrics interface {
	// ObserveTimeToSession records the time taken to establish a session.
	ObserveTimeToSession(context.Context)
}

// NoopMetrics ignores metric observations.
type NoopMetrics struct{}

// ObserveTimeToSession implements Metrics.
func (NoopMetrics) ObserveTimeToSession(context.Context) {}
