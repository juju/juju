// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// ErrRestrictedContext indicates a method is not implemented in the given context.
var ErrRestrictedContext = errors.NotImplementedf("not implemented for restricted context")

// RestrictedContext is a base implementation for restricted contexts to embed,
// so that an error is returned for methods that are not explicitly
// implemented.
type RestrictedContext struct{}

// ConfigSettings implements hooks.Context.
func (*RestrictedContext) ConfigSettings(context.Context) (charm.Settings, error) {
	return nil, ErrRestrictedContext
}

// GoalState implements hooks.Context.
func (*RestrictedContext) GoalState(context.Context) (*application.GoalState, error) {
	return &application.GoalState{}, ErrRestrictedContext
}

// GetCharmState implements jujuc.unitCharmStateContext.
func (*RestrictedContext) GetCharmState(context.Context) (map[string]string, error) {
	return nil, ErrRestrictedContext
}

// GetCharmStateValue implements jujuc.unitCharmStateContext.
func (*RestrictedContext) GetCharmStateValue(context.Context, string) (string, error) {
	return "", ErrRestrictedContext
}

// DeleteCharmStateValue implements jujuc.unitCharmStateContext.
func (*RestrictedContext) DeleteCharmStateValue(context.Context, string) error {
	return ErrRestrictedContext
}

// SetCharmStateValue implements jujuc.unitCharmStateContext.
func (*RestrictedContext) SetCharmStateValue(context.Context, string, string) error {
	return ErrRestrictedContext
}

// UnitStatus implements hooks.Context.
func (*RestrictedContext) UnitStatus(context.Context) (*StatusInfo, error) {
	return nil, ErrRestrictedContext
}

// CloudSpec implements hooks.Context.
func (c *RestrictedContext) CloudSpec(context.Context) (*params.CloudSpec, error) {
	return nil, ErrRestrictedContext
}

// SetUnitStatus implements hooks.Context.
func (*RestrictedContext) SetUnitStatus(context.Context, StatusInfo) error {
	return ErrRestrictedContext
}

// ApplicationStatus implements hooks.Context.
func (*RestrictedContext) ApplicationStatus(context.Context) (ApplicationStatusInfo, error) {
	return ApplicationStatusInfo{}, ErrRestrictedContext
}

// SetApplicationStatus implements hooks.Context.
func (*RestrictedContext) SetApplicationStatus(context.Context, StatusInfo) error {
	return ErrRestrictedContext
}

// AvailabilityZone implements hooks.Context.
func (*RestrictedContext) AvailabilityZone() (string, error) { return "", ErrRestrictedContext }

// RequestReboot implements hooks.Context.
func (*RestrictedContext) RequestReboot(_ context.Context, prio RebootPriority) error {
	return ErrRestrictedContext
}

// PublicAddress implements hooks.Context.
func (*RestrictedContext) PublicAddress(_ context.Context) (string, error) {
	return "", ErrRestrictedContext
}

// PrivateAddress implements hooks.Context.
func (*RestrictedContext) PrivateAddress() (string, error) { return "", ErrRestrictedContext }

// OpenPortRange implements hooks.Context.
func (*RestrictedContext) OpenPortRange(context.Context, string, network.PortRange) error {
	return ErrRestrictedContext
}

// ClosePortRange implements hooks.Context.
func (*RestrictedContext) ClosePortRange(context.Context, string, network.PortRange) error {
	return ErrRestrictedContext
}

// OpenedPortRanges implements hooks.Context.
func (*RestrictedContext) OpenedPortRanges() network.GroupedPortRanges { return nil }

// NetworkInfo implements hooks.Context.
func (*RestrictedContext) NetworkInfo(_ context.Context, bindingNames []string, relationId int) (map[string]params.NetworkInfoResult, error) {
	return map[string]params.NetworkInfoResult{}, ErrRestrictedContext
}

// IsLeader implements hooks.Context.
func (*RestrictedContext) IsLeader() (bool, error) { return false, ErrRestrictedContext }

// LeaderSettings implements hooks.Context.
func (*RestrictedContext) LeaderSettings(_ context.Context) (map[string]string, error) {
	return nil, ErrRestrictedContext
}

// WriteLeaderSettings implements hooks.Context.
func (*RestrictedContext) WriteLeaderSettings(context.Context, map[string]string) error {
	return ErrRestrictedContext
}

// AddMetric implements hooks.Context.
func (*RestrictedContext) AddMetric(string, string, time.Time) error { return ErrRestrictedContext }

// AddMetricLabels implements hooks.Context.
func (*RestrictedContext) AddMetricLabels(string, string, time.Time, map[string]string) error {
	return ErrRestrictedContext
}

// StorageTags implements hooks.Context.
func (*RestrictedContext) StorageTags(_ context.Context) ([]names.StorageTag, error) {
	return nil, ErrRestrictedContext
}

// Storage implements hooks.Context.
func (*RestrictedContext) Storage(context.Context, names.StorageTag) (ContextStorageAttachment, error) {
	return nil, ErrRestrictedContext
}

// HookStorage implements hooks.Context.
func (*RestrictedContext) HookStorage(_ context.Context) (ContextStorageAttachment, error) {
	return nil, ErrRestrictedContext
}

// AddUnitStorage implements hooks.Context.
func (*RestrictedContext) AddUnitStorage(map[string]params.StorageDirectives) error {
	return ErrRestrictedContext
}

// DownloadResource implements hooks.Context.
func (ctx *RestrictedContext) DownloadResource(_ context.Context, name string) (filePath string, _ error) {
	return "", ErrRestrictedContext
}

// GetPayload implements hooks.Context.
func (ctx *RestrictedContext) GetPayload(_ context.Context, class, id string) (*payloads.Payload, error) {
	return nil, ErrRestrictedContext
}

// TrackPayload implements hooks.Context.
func (ctx *RestrictedContext) TrackPayload(_ context.Context, payload payloads.Payload) error {
	return ErrRestrictedContext
}

// UntrackPayload implements hooks.Context.
func (ctx *RestrictedContext) UntrackPayload(_ context.Context, class, id string) error {
	return ErrRestrictedContext
}

// SetPayloadStatus implements hooks.Context.
func (ctx *RestrictedContext) SetPayloadStatus(_ context.Context, class, id, status string) error {
	return ErrRestrictedContext
}

// ListPayloads implements hooks.Context.
func (ctx *RestrictedContext) ListPayloads(_ context.Context) ([]string, error) {
	return nil, ErrRestrictedContext
}

// FlushPayloads pushes the hook context data out to state.
func (ctx *RestrictedContext) FlushPayloads(_ context.Context) error {
	return ErrRestrictedContext
}

// Relation implements hooks.Context.
func (*RestrictedContext) Relation(id int) (ContextRelation, error) {
	return nil, ErrRestrictedContext
}

// RelationIds implements hooks.Context.
func (*RestrictedContext) RelationIds(context.Context) ([]int, error) {
	return nil, ErrRestrictedContext
}

// HookRelation implements hooks.Context.
func (*RestrictedContext) HookRelation() (ContextRelation, error) {
	return nil, ErrRestrictedContext
}

// RemoteUnitName implements hooks.Context.
func (*RestrictedContext) RemoteUnitName() (string, error) { return "", ErrRestrictedContext }

// RemoteApplicationName implements hooks.Context.
func (*RestrictedContext) RemoteApplicationName() (string, error) { return "", ErrRestrictedContext }

// ActionParams implements hooks.Context.
func (*RestrictedContext) ActionParams() (map[string]interface{}, error) {
	return nil, ErrRestrictedContext
}

// UpdateActionResults implements hooks.Context.
func (*RestrictedContext) UpdateActionResults(keys []string, value interface{}) error {
	return ErrRestrictedContext
}

// LogActionMessage implements hooks.Context.
func (*RestrictedContext) LogActionMessage(context.Context, string) error {
	return ErrRestrictedContext
}

// SetActionMessage implements hooks.Context.
func (*RestrictedContext) SetActionMessage(string) error { return ErrRestrictedContext }

// SetActionFailed implements hooks.Context.
func (*RestrictedContext) SetActionFailed() error { return ErrRestrictedContext }

// UnitWorkloadVersion implements hooks.Context.
func (*RestrictedContext) UnitWorkloadVersion(context.Context) (string, error) {
	return "", ErrRestrictedContext
}

// SetUnitWorkloadVersion implements hooks.Context.
func (*RestrictedContext) SetUnitWorkloadVersion(context.Context, string) error {
	return ErrRestrictedContext
}

// WorkloadName implements hooks.Context.
func (*RestrictedContext) WorkloadName() (string, error) {
	return "", ErrRestrictedContext
}

// GetSecret implements runner.Context.
func (ctx *RestrictedContext) GetSecret(context.Context, *secrets.URI, string, bool, bool) (secrets.SecretValue, error) {
	return nil, ErrRestrictedContext
}

// CreateSecret implements runner.Context.
func (ctx *RestrictedContext) CreateSecret(_ context.Context, args *SecretCreateArgs) (*secrets.URI, error) {
	return nil, ErrRestrictedContext
}

// UpdateSecret implements runner.Context.
func (ctx *RestrictedContext) UpdateSecret(*secrets.URI, *SecretUpdateArgs) error {
	return ErrRestrictedContext
}

func (ctx *RestrictedContext) RemoveSecret(*secrets.URI, *int) error {
	return ErrRestrictedContext
}

func (ctx *RestrictedContext) SecretMetadata(context.Context) (map[string]SecretMetadata, error) {
	return nil, ErrRestrictedContext
}

// GrantSecret implements runner.Context.
func (c *RestrictedContext) GrantSecret(context.Context, *secrets.URI, *SecretGrantRevokeArgs) error {
	return ErrRestrictedContext
}

// RevokeSecret implements runner.Context.
func (c *RestrictedContext) RevokeSecret(*secrets.URI, *SecretGrantRevokeArgs) error {
	return ErrRestrictedContext
}
