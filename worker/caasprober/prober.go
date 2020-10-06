// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober

import (
	"errors"
)

type CAASProbes interface {
	LivenessProbe() Prober
	ReadinessProbe() Prober
	StartupProbe() Prober
}

type caasProbes struct {
	Liveness  Prober
	Readiness Prober
	Startup   Prober
}

var (
	ErrorProbeNotImplemented = errors.New("probe not implemented")
)

type Prober interface {
	Probe() (bool, error)
}

type ProberFunc func() (bool, error)

type ProbeNotImplemented struct{}

type ProbeSuccess struct{}

func (c *caasProbes) LivenessProbe() Prober {
	return c.Liveness
}

func (p ProberFunc) Probe() (bool, error) {
	return p()
}

func (p *ProbeNotImplemented) Probe() (bool, error) {
	return false, ErrorProbeNotImplemented
}

func (p *ProbeSuccess) Probe() (bool, error) {
	return true, nil
}

func (c *caasProbes) ReadinessProbe() Prober {
	return c.Readiness
}

func (c *caasProbes) StartupProbe() Prober {
	return c.Startup
}
