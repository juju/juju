// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/lease"
)

// SingularSecretary implements Secretary to restrict claims to either
// a lease for the controller or the specific model it's asking for,
// holdable only by machine-tag strings.
type SingularSecretary struct {
	ControllerUUID string
}

// CheckLease is part of the lease.Secretary interface.
func (s SingularSecretary) CheckLease(key lease.Key) error {
	if key.Lease != s.ControllerUUID && key.Lease != key.ModelUUID {
		return errors.New("expected controller or model UUID")
	}
	return nil
}

// CheckHolder is part of the lease.Secretary interface.
func (s SingularSecretary) CheckHolder(name string) error {
	if _, err := names.ParseMachineTag(name); err != nil {
		return errors.New("expected machine tag")
	}
	return nil
}

// CheckDuration is part of the lease.Secretary interface.
func (s SingularSecretary) CheckDuration(duration time.Duration) error {
	if duration <= 0 {
		return errors.NewNotValid(nil, "non-positive")
	}
	return nil
}

// LeadershipSecretary implements Secretary; it checks that leases are
// application names, and holders are unit names.
type LeadershipSecretary struct{}

// CheckLease is part of the lease.Secretary interface.
func (LeadershipSecretary) CheckLease(key lease.Key) error {
	if !names.IsValidApplication(key.Lease) {
		return errors.NewNotValid(nil, "not an application name")
	}
	return nil
}

// CheckHolder is part of the lease.Secretary interface.
func (LeadershipSecretary) CheckHolder(name string) error {
	if !names.IsValidUnit(name) {
		return errors.NewNotValid(nil, "not a unit name")
	}
	return nil
}

// CheckDuration is part of the lease.Secretary interface.
func (LeadershipSecretary) CheckDuration(duration time.Duration) error {
	if duration <= 0 {
		return errors.NewNotValid(nil, "non-positive")
	}
	return nil
}

// SecretaryFinder returns a function to find the correct secretary to
// use for validation for a specific lease namespace (or an error if
// the namespace isn't valid).
func SecretaryFinder(controllerUUID string) func(string) (Secretary, error) {
	secretaries := map[string]Secretary{
		lease.ApplicationLeadershipNamespace: LeadershipSecretary{},
		lease.SingularControllerNamespace: SingularSecretary{
			ControllerUUID: controllerUUID,
		},
	}
	return func(namespace string) (Secretary, error) {
		result, found := secretaries[namespace]
		if !found {
			return nil, errors.NotValidf("namespace %q", namespace)
		}
		return result, nil
	}
}
