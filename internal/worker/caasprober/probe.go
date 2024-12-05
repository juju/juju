// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober

import (
	"sync"

	"github.com/juju/juju/internal/observability/probe"
)

// CaasProbes provides a private internal implementation of CAASProbes.
type CAASProbes struct {
	mut    sync.Mutex
	probes map[probe.ProbeType]*probe.Aggregate
}

// NewCAASProbes is responsible for constructing a new CAASProbes struct with
// its members initialised.
func NewCAASProbes() *CAASProbes {
	return &CAASProbes{
		probes: map[probe.ProbeType]*probe.Aggregate{
			probe.ProbeLiveness:  probe.NewAggregate(),
			probe.ProbeReadiness: probe.NewAggregate(),
			probe.ProbeStartup:   probe.NewAggregate(),
		},
	}
}

func (p *CAASProbes) ProbeAggregate(name probe.ProbeType) (*probe.Aggregate, bool) {
	p.mut.Lock()
	defer p.mut.Unlock()
	probe, ok := p.probes[name]
	return probe, ok
}
