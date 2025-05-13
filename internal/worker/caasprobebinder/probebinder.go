// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprobebinder

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/internal/observability/probe"
	"github.com/juju/juju/internal/worker/caasprober"
)

// ProbeBinder is a worker that binds a set of probe providers
// onto a caasprober worker.
type ProbeBinder struct {
	catacomb catacomb.Catacomb

	probes    *caasprober.CAASProbes
	providers map[string]probe.ProbeProvider
}

// NewProbeBinder constructs a new caas probe binder worker.
func NewProbeBinder(probes *caasprober.CAASProbes, providers map[string]probe.ProbeProvider) (*ProbeBinder, error) {
	pb := &ProbeBinder{
		probes:    probes,
		providers: providers,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "caas-probe-binder",
		Site: &pb.catacomb,
		Work: pb.loop,
	}); err != nil {
		return pb, errors.Trace(err)
	}
	return pb, nil
}

// Kill implements worker.Kill
func (c *ProbeBinder) Kill() {
	c.catacomb.Kill(nil)
}

// loop adds all the probers to the probe aggregates and
// removes them when the worker dies.
func (c *ProbeBinder) loop() error {
	for id, provider := range c.providers {
		for k, v := range provider.SupportedProbes() {
			if agg, ok := c.probes.ProbeAggregate(k); ok {
				agg.AddProber(id, v)
				defer agg.RemoveProber(id)
			}
		}
	}
	<-c.catacomb.Dying()
	return c.catacomb.ErrDying()
}

// Wait implements worker.Wait
func (c *ProbeBinder) Wait() error {
	return c.catacomb.Wait()
}
