// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	coreagent "github.com/juju/juju/core/agent"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/objectstore"
)

// SecretaryFinder is responsible for returning the correct Secretary for a
// given namespace.
type SecretaryFinder struct {
	secretaries map[string]lease.Secretary
}

// Register adds a Secretary to the Cabinet.
func (c SecretaryFinder) Register(namespace string, secretary lease.Secretary) {
	c.secretaries[namespace] = secretary
}

// SecretaryFor returns the Secretary for the given namespace.
// Returns an error if the namespace is not valid.
func (c SecretaryFinder) SecretaryFor(namespace string) (lease.Secretary, error) {
	result, found := c.secretaries[namespace]
	if !found {
		return nil, errors.NotValidf("namespace %q", namespace)
	}
	return result, nil
}

// NewCabinet returns a SecretaryFinder with default set of secretaries
// registered with it.
// Note: a cabinet is a group of secretaries.
func NewSecretaryFinder(controllerUUID string) lease.SecretaryFinder {
	finder := SecretaryFinder{
		secretaries: map[string]lease.Secretary{
			lease.SingularControllerNamespace: SingularSecretary{
				ControllerUUID: controllerUUID,
			},
			lease.ApplicationLeadershipNamespace: LeadershipSecretary{},
			lease.ObjectStoreNamespace:           ObjectStoreSecretary{},
		},
	}
	return finder
}

type baseSecretary struct{}

// CheckDuration is part of the lease.Secretary interface.
func (baseSecretary) CheckDuration(duration time.Duration) error {
	if duration <= 0 {
		return errors.NewNotValid(nil, "non-positive")
	}
	return nil
}

// SingularSecretary implements Secretary to restrict claims to either
// a lease for the controller or the specific model it's asking for,
// holdable only by machine-tag strings.
type SingularSecretary struct {
	baseSecretary
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
	tag, err := names.ParseTag(name)
	if err != nil {
		return errors.Annotate(err, "expected a valid tag")
	}
	if !coreagent.IsAllowedControllerTag(tag.Kind()) {
		return errors.New("expected machine or controller tag")
	}
	return nil
}

// LeadershipSecretary implements Secretary; it checks that leases are
// application names, and holders are unit names.
type LeadershipSecretary struct {
	baseSecretary
}

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

// ObjectStoreSecretary implements Secretary; it checks that leases are
// application names, and holders are unit names.
type ObjectStoreSecretary struct {
	baseSecretary
}

// CheckLease is part of the lease.Secretary interface.
func (ObjectStoreSecretary) CheckLease(key lease.Key) error {
	if key.Lease == "" {
		return errors.NewNotValid(nil, "empty lease name")
	}
	return nil
}

// CheckHolder is part of the lease.Secretary interface.
func (ObjectStoreSecretary) CheckHolder(name string) error {
	return objectstore.ParseLeaseHolderName(name)
}
