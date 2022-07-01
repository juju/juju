// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober

import (
	"github.com/juju/juju/v2/observability/probe"
)

// CaasProbes provides a private internal implementation of CAASProbes.
type CAASProbes struct {
	Liveness  *probe.Aggregate
	Readiness *probe.Aggregate
	Startup   *probe.Aggregate
}

func NewCAASProbes() *CAASProbes {
	return &CAASProbes{
		Liveness:  probe.NewAggregate(),
		Readiness: probe.NewAggregate(),
		Startup:   probe.NewAggregate(),
	}
}
