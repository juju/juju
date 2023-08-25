// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"sync"

	"github.com/juju/juju/internal/observability/probe"
)

// Probe is a prober implementation for the uniter worker to form part of the
// Juju probe support
type Probe struct {
	hasStartedLock sync.RWMutex
	hasStarted     bool
}

// HasStarted indiciates if this probe considered the uniter to have been
// started.
func (p *Probe) HasStarted() bool {
	p.hasStartedLock.RLock()
	defer p.hasStartedLock.RUnlock()
	return p.hasStarted
}

// SetHasStarted sets the has started state for this probe. Should be called
// when the uniter has started its associated charm.
func (p *Probe) SetHasStarted(started bool) {
	p.hasStartedLock.Lock()
	defer p.hasStartedLock.Unlock()
	p.hasStarted = started
}

// SupportedProbes implements probe.ProbeProvider interface
func (p *Probe) SupportedProbes() probe.SupportedProbes {
	return probe.SupportedProbes{
		probe.ProbeLiveness: probe.ProberFn(func() (bool, error) {
			return true, nil
		}),
		probe.ProbeReadiness: probe.ProberFn(func() (bool, error) {
			return p.HasStarted(), nil
		}),
	}
}
