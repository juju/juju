// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package probe

import (
	"github.com/juju/errors"
)

// LivenessProber is an interface for probing targets that implement liveness
// probe support.
type LivenessProber interface {
	// LivenessProbe returns the Prober to use for Liveness probes.
	LivenessProbe() Prober
}

// Prober represents something that can be probed and return a true or false
// statement about it's success or optionally error if it's not able to
// assertain a probes success.
type Prober interface {
	// Probe this thing and return true or false as to it's success.
	// Alternatively an error can be raise when making this decision in which
	// case the probe should be considered a failed with extra context through
	// the error.
	Probe() (bool, error)
}

// ProberProvider is implemented by entities that wish to controbute probes to
// the overall applications probe support.
type ProbeProvider interface {
	SupportedProbes() SupportedProbes
}

type probeProvider struct {
	Probes SupportedProbes
}

// ProbeType is an alias type to describe the type of probe in question.
type ProbeType string

// SupportedProbes provides a map of supported probes to the caller referenced
// on ProbeType.
type SupportedProbes map[ProbeType]Prober

const (
	// ProbeLiveness represents a liveness probe
	ProbeLiveness = ProbeType("liveness")

	// ProbeReadiness represents a readiness probe
	ProbeReadiness = ProbeType("readiness")

	// ProbeStartup represents a startup probe
	ProbeStartup = ProbeType("startup")
)

// ProberFn is a convenience wrapper to transform a function into a Prober
// interface
type ProberFn func() (bool, error)

var (
	// Failure is a convenience wrapper probe that always evaluates to failure.
	Failure Prober = ProberFn(func() (bool, error) {
		return false, nil
	})

	// NotImplemented is a convenience wrapper for supplying a probe that
	// indicates to it's caller that it's not implemented. The resulting error
	// will be of type errors.NotImplemented
	NotImplemented Prober = ProberFn(func() (bool, error) {
		return false, errors.NotImplementedf("probe")
	})

	// Success is a convenience wrapper probe that always evaluates to success.
	Success Prober = ProberFn(func() (bool, error) {
		return true, nil
	})
)

// LivenessProvider is a utility function for returning a ProbeProvider for the
// provided liveness probe.
func LivenessProvider(probe Prober) ProbeProvider {
	return Provider(SupportedProbes{
		ProbeLiveness: probe,
	})
}

// ReadinessProvider is a utility function for returning a ProbeProvider for the
// provided readiness probe.
func ReadinessProvider(probe Prober) ProbeProvider {
	return Provider(SupportedProbes{
		ProbeReadiness: probe,
	})
}

// StartupProvider is a utility function for returning a ProbeProvider for the
// provided startup probe.
func StartupProvider(probe Prober) ProbeProvider {
	return Provider(SupportedProbes{
		ProbeStartup: probe,
	})
}

// Probe implements Prober interface
func (p ProberFn) Probe() (bool, error) {
	return p()
}

// Provider is a utility function for returning a ProbeProvider based on the
// SupportedProbes passed in.
func Provider(supported SupportedProbes) ProbeProvider {
	return &probeProvider{
		Probes: supported,
	}
}

// Supports indicates if the supplied ProbeType is in the map of supported
// probe types.
func (s SupportedProbes) Supports(t ProbeType) bool {
	_, has := s[t]
	return has
}

// SupportedProbes implements ProbeProvider interface.
func (p *probeProvider) SupportedProbes() SupportedProbes {
	return p.Probes
}
