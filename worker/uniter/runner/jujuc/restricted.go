// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

// ErrRestrictedContext indicates a method is not implemented in the given context.
var ErrRestrictedContext = errors.NotImplementedf("not implemented for restricted context")

// RestrictedContext is a base implementation for restricted contexts to embed,
// so that an error is returned for methods that are not explicitly
// implemented.
type RestrictedContext struct{}

// ConfigSettings implements jujuc.Context.
func (*RestrictedContext) ConfigSettings() (charm.Settings, error) { return nil, ErrRestrictedContext }

// UnitStatus implements jujuc.Context.
func (*RestrictedContext) UnitStatus() (*StatusInfo, error) { return nil, ErrRestrictedContext }

// SetUnitStatus implements jujuc.Context.
func (*RestrictedContext) SetUnitStatus(StatusInfo) error { return ErrRestrictedContext }

// ServiceStatus implements jujuc.Context.
func (*RestrictedContext) ServiceStatus() (ServiceStatusInfo, error) {
	return ServiceStatusInfo{}, ErrRestrictedContext
}

// SetServiceStatus implements jujuc.Context.
func (*RestrictedContext) SetServiceStatus(StatusInfo) error { return ErrRestrictedContext }

// AvailabilityZone implements jujuc.Context.
func (*RestrictedContext) AvailabilityZone() (string, error) { return "", ErrRestrictedContext }

// RequestReboot implements jujuc.Context.
func (*RestrictedContext) RequestReboot(prio RebootPriority) error { return ErrRestrictedContext }

// PublicAddress implements jujuc.Context.
func (*RestrictedContext) PublicAddress() (string, error) { return "", ErrRestrictedContext }

// PrivateAddress implements jujuc.Context.
func (*RestrictedContext) PrivateAddress() (string, error) { return "", ErrRestrictedContext }

// OpenPorts implements jujuc.Context.
func (*RestrictedContext) OpenPorts(protocol string, fromPort, toPort int) error {
	return ErrRestrictedContext
}

// ClosePorts implements jujuc.Context.
func (*RestrictedContext) ClosePorts(protocol string, fromPort, toPort int) error {
	return ErrRestrictedContext
}

// OpenedPorts implements jujuc.Context.
func (*RestrictedContext) OpenedPorts() []network.PortRange { return nil }

// IsLeader implements jujuc.Context.
func (*RestrictedContext) IsLeader() (bool, error) { return false, ErrRestrictedContext }

// LeaderSettings implements jujuc.Context.
func (*RestrictedContext) LeaderSettings() (map[string]string, error) {
	return nil, ErrRestrictedContext
}

// WriteLeaderSettings implements jujuc.Context.
func (*RestrictedContext) WriteLeaderSettings(map[string]string) error { return ErrRestrictedContext }

// AddMetric implements jujuc.Context.
func (*RestrictedContext) AddMetric(string, string, time.Time) error { return ErrRestrictedContext }

// StorageTags implements jujuc.Context.
func (*RestrictedContext) StorageTags() ([]names.StorageTag, error) { return nil, ErrRestrictedContext }

// Storage implements jujuc.Context.
func (*RestrictedContext) Storage(names.StorageTag) (ContextStorageAttachment, error) {
	return nil, ErrRestrictedContext
}

// HookStorage implements jujuc.Context.
func (*RestrictedContext) HookStorage() (ContextStorageAttachment, error) {
	return nil, ErrRestrictedContext
}

// AddUnitStorage implements jujuc.Context.
func (*RestrictedContext) AddUnitStorage(map[string]params.StorageConstraints) error {
	return ErrRestrictedContext
}

// Relation implements jujuc.Context.
func (*RestrictedContext) Relation(id int) (ContextRelation, error) {
	return nil, ErrRestrictedContext
}

// RelationIds implements jujuc.Context.
func (*RestrictedContext) RelationIds() ([]int, error) { return nil, ErrRestrictedContext }

// HookRelation implements jujuc.Context.
func (*RestrictedContext) HookRelation() (ContextRelation, error) {
	return nil, ErrRestrictedContext
}

// RemoteUnitName implements jujuc.Context.
func (*RestrictedContext) RemoteUnitName() (string, error) { return "", ErrRestrictedContext }

// ActionParams implements jujuc.Context.
func (*RestrictedContext) ActionParams() (map[string]interface{}, error) {
	return nil, ErrRestrictedContext
}

// UpdateActionResults implements jujuc.Context.
func (*RestrictedContext) UpdateActionResults(keys []string, value string) error {
	return ErrRestrictedContext
}

// SetActionMessage implements jujuc.Context.
func (*RestrictedContext) SetActionMessage(string) error { return ErrRestrictedContext }

// SetActionFailed implements jujuc.Context.
func (*RestrictedContext) SetActionFailed() error { return ErrRestrictedContext }

// Component implements jujc.Context.
func (*RestrictedContext) Component(string) (ContextComponent, error) {
	return nil, ErrRestrictedContext
}
