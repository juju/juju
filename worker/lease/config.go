// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/trace"
)

// Secretary is responsible for validating the sanity of lease and holder names
// before bothering the manager with them.
type Secretary interface {

	// CheckLease returns an error if the supplied lease name is not valid.
	CheckLease(key lease.Key) error

	// CheckHolder returns an error if the supplied holder name is not valid.
	CheckHolder(name string) error

	// CheckDuration returns an error if the supplied duration is not valid.
	CheckDuration(duration time.Duration) error
}

// Logger represents the logging methods we use from a loggo.Logger.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warningf(string, ...interface{})
	Errorf(string, ...interface{})
}

// ManagerConfig contains the resources and information required to create a
// Manager.
type ManagerConfig struct {

	// Secretary determines validation given a namespace. The
	// secretary returned is responsible for validating lease names
	// and holder names for that namespace.
	Secretary func(namespace string) (Secretary, error)

	// Store is responsible for recording, retrieving, and expiring leases.
	Store lease.Store

	// Tracer is used to record tracing information as the manager runs.
	Tracer trace.Tracer

	// Logger is used to report debugging/status information as the
	// manager runs.
	Logger Logger

	// Clock is responsible for reporting the passage of time.
	Clock clock.Clock

	// MaxSleep is the longest time the Manager should sleep before
	// refreshing its store's leases and checking for expiries.
	MaxSleep time.Duration

	// EntityUUID is the entity that we are running this Manager for. Used for
	// logging purposes.
	EntityUUID string

	// LogDir is the directory to write a debugging log file in the
	// case that the worker times out waiting to shut down.
	LogDir string

	PrometheusRegisterer prometheus.Registerer
}

// Validate returns an error if the configuration contains invalid information
// or missing resources.
func (config ManagerConfig) Validate() error {
	if config.Secretary == nil {
		return errors.NotValidf("nil Secretary")
	}
	if config.Store == nil {
		return errors.NotValidf("nil Store")
	}
	if config.Tracer == nil {
		return errors.NotValidf("nil Tracer")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.MaxSleep <= 0 {
		return errors.NotValidf("non-positive MaxSleep")
	}
	// TODO: make the PrometheusRegisterer required when we no longer
	// have state workers managing leases.
	return nil
}
