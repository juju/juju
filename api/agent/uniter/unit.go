// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// Unit represents a juju unit as seen by a uniter worker.
type Unit struct {
	client     *Client
	tag        names.UnitTag
	life       life.Value
	providerID string
}

// Tag returns the unit's tag.
func (u *Unit) Tag() names.UnitTag {
	return u.tag
}

// ProviderID returns the provider Id of the unit.
func (u *Unit) ProviderID() string {
	return u.providerID
}

// Name returns the name of the unit.
func (u *Unit) Name() string {
	return u.tag.Id()
}

// String returns the unit as a string.
func (u *Unit) String() string {
	return u.Name()
}

// Life returns the unit's lifecycle value.
func (u *Unit) Life() life.Value {
	return u.life
}

// Resolved returns the unit's resolved mode value.
func (u *Unit) Resolved(ctx context.Context) (params.ResolvedMode, error) {
	var results params.ResolvedModeResults
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: u.tag.String()},
		},
	}
	err := u.client.facade.FacadeCall(ctx, "Resolved", args, &results)
	if err != nil {
		return "", errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		// We should be able to use apiserver.common.RestoreError here,
		// but because of poor design, it causes import errors.
		if params.IsCodeNotFound(result.Error) {
			return "", errors.NewNotFound(result.Error, "")
		}
		return "", errors.Trace(result.Error)
	}

	return result.Mode, nil
}

// Refresh updates the cached local copy of the unit's data.
//
// Deprecated: Please use a purpose-built getter instead.
func (u *Unit) Refresh(ctx context.Context) error {
	var results params.UnitRefreshResults
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: u.tag.String()},
		},
	}
	err := u.client.facade.FacadeCall(ctx, "Refresh", args, &results)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		// We should be able to use apiserver.common.RestoreError here,
		// but because of poor design, it causes import errors.
		if params.IsCodeNotFound(result.Error) {
			return errors.NewNotFound(result.Error, "")
		}
		return errors.Trace(result.Error)
	}

	u.life = result.Life
	u.providerID = result.ProviderID
	return nil
}

// SetUnitStatus sets the status of the unit.
func (u *Unit) SetUnitStatus(ctx context.Context, unitStatus status.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: u.tag.String(), Status: unitStatus.String(), Info: info, Data: data},
		},
	}
	err := u.client.facade.FacadeCall(ctx, "SetUnitStatus", args, &result)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return result.OneError()
}

// UnitStatus gets the status details of the unit.
func (u *Unit) UnitStatus(ctx context.Context) (params.StatusResult, error) {
	var results params.StatusResults
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: u.tag.String()},
		},
	}
	err := u.client.facade.FacadeCall(ctx, "UnitStatus", args, &results)
	if err != nil {
		return params.StatusResult{}, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return params.StatusResult{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.StatusResult{}, result.Error
	}
	return result, nil
}

// SetAgentStatus sets the status of the unit agent.
func (u *Unit) SetAgentStatus(ctx context.Context, agentStatus status.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: u.tag.String(), Status: agentStatus.String(), Info: info, Data: data},
		},
	}
	err := u.client.facade.FacadeCall(ctx, "SetAgentStatus", args, &result)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return result.OneError()
}

// EnsureDead sets the unit lifecycle to Dead if it is Alive or
// Dying. It does nothing otherwise.
func (u *Unit) EnsureDead(ctx context.Context) error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "EnsureDead", args, &result)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return result.OneError()
}

// Watch returns a watcher for observing changes to an application.
func (s *Unit) Watch(ctx context.Context) (watcher.NotifyWatcher, error) {
	arg := params.Entity{Tag: s.tag.String()}
	var result params.NotifyWatchResult

	err := s.client.facade.FacadeCall(ctx, "WatchUnit", arg, &result)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}

	if result.Error != nil {
		return nil, result.Error
	}
	return apiwatcher.NewNotifyWatcher(s.client.facade.RawAPICaller(), result), nil
}

// WatchResolveMode returns a NotifyWatcher that will send notifications when
// the resolve mode of the unit changes.
func (s *Unit) WatchResolveMode(ctx context.Context) (watcher.NotifyWatcher, error) {
	arg := params.Entity{Tag: s.tag.String()}
	var result params.NotifyWatchResult

	err := s.client.facade.FacadeCall(ctx, "WatchUnitResolveMode", arg, &result)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}

	if result.Error != nil {
		return nil, result.Error
	}
	return apiwatcher.NewNotifyWatcher(s.client.facade.RawAPICaller(), result), nil
}

// WatchRelations returns a StringsWatcher that notifies of changes to
// the lifecycles of relations involving u.
func (u *Unit) WatchRelations(ctx context.Context) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "WatchUnitRelations", args, &results)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(u.client.facade.RawAPICaller(), result)
	return w, nil
}

// Application returns the unit's application.
func (u *Unit) Application(ctx context.Context) (*Application, error) {
	application := &Application{
		client: u.client,
		tag:    u.ApplicationTag(),
	}
	// Call Refresh() immediately to get the up-to-date
	// life and other needed locally cached fields.
	err := application.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	return application, nil
}

// ConfigSettings returns the complete set of application charm config settings
// available to the unit. Unset values will be replaced with the default
// value for the associated option, and may thus be nil when no default is
// specified.
func (u *Unit) ConfigSettings(ctx context.Context) (charm.Settings, error) {
	var results params.ConfigSettingsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "ConfigSettings", args, &results)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return charm.Settings(result.Settings), nil
}

// ApplicationName returns the application name.
func (u *Unit) ApplicationName() string {
	application, err := names.UnitApplication(u.Name())
	if err != nil {
		panic(err)
	}
	return application
}

// ApplicationTag returns the application tag.
func (u *Unit) ApplicationTag() names.ApplicationTag {
	return names.NewApplicationTag(u.ApplicationName())
}

// Destroy, when called on a Alive unit, advances its lifecycle as far as
// possible; it otherwise has no effect. In most situations, the unit's
// life is just set to Dying; but if a principal unit that is not assigned
// to a provisioned machine is Destroyed, it will be removed from state
// directly.
func (u *Unit) Destroy(ctx context.Context) error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "Destroy", args, &result)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return result.OneError()
}

// DestroyAllSubordinates destroys all subordinates of the unit.
func (u *Unit) DestroyAllSubordinates(ctx context.Context) error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "DestroyAllSubordinates", args, &result)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return result.OneError()
}

// AssignedMachine returns the unit's assigned machine tag or an error
// satisfying params.IsCodeNotAssigned when the unit has no assigned
// machine..
func (u *Unit) AssignedMachine(ctx context.Context) (names.MachineTag, error) {
	var invalidTag names.MachineTag
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "AssignedMachine", args, &results)
	if err != nil {
		return invalidTag, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return invalidTag, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return invalidTag, result.Error
	}
	return names.ParseMachineTag(result.Result)
}

// PrincipalName returns the principal unit name and true for subordinates.
// For principal units the function returns "" and false.
//
// NOTE: This differs from state.Unit.PrincipalName() by returning an
// error as well, because it needs to make an API call.
func (u *Unit) PrincipalName(ctx context.Context) (string, bool, error) {
	var results params.StringBoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "GetPrincipal", args, &results)
	if err != nil {
		return "", false, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return "", false, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", false, result.Error
	}
	var unitName string
	if result.Ok {
		unitTag, err := names.ParseUnitTag(result.Result)
		if err != nil {
			return "", false, err
		}
		unitName = unitTag.Id()
	}
	return unitName, result.Ok, nil
}

// HasSubordinates returns the tags of any subordinate units.
func (u *Unit) HasSubordinates(ctx context.Context) (bool, error) {
	var results params.BoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "HasSubordinates", args, &results)
	if err != nil {
		return false, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return false, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return false, result.Error
	}
	return result.Result, nil
}

// PublicAddress returns the public address of the unit and whether it
// is valid.
//
// NOTE: This differs from state.Unit.PublicAddres() by returning
// an error instead of a bool, because it needs to make an API call.
//
// TODO(dimitern): We might be able to drop this, once we have machine
// addresses implemented fully. See also LP bug 1221798.
func (u *Unit) PublicAddress(ctx context.Context) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "PublicAddress", args, &results)
	if err != nil {
		return "", errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// PrivateAddress returns the private address of the unit and whether
// it is valid.
//
// NOTE: This differs from state.Unit.PrivateAddress() by returning
// an error instead of a bool, because it needs to make an API call.
//
// TODO(dimitern): We might be able to drop this, once we have machine
// addresses implemented fully. See also LP bug 1221798.
func (u *Unit) PrivateAddress(ctx context.Context) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "PrivateAddress", args, &results)
	if err != nil {
		return "", errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// AvailabilityZone returns the availability zone of the unit.
func (u *Unit) AvailabilityZone(ctx context.Context) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	if err := u.client.facade.FacadeCall(ctx, "AvailabilityZone", args, &results); err != nil {
		return "", errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", errors.Trace(result.Error)
	}
	return result.Result, nil
}

var ErrNoCharmURLSet = errors.New("unit has no charm url set")

// CharmURL returns the charm URL this unit is currently using.
func (u *Unit) CharmURL(ctx context.Context) (string, error) {
	var results params.StringBoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "CharmURL", args, &results)
	if err != nil {
		return "", errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	if result.Result != "" {
		return result.Result, nil
	}
	return "", ErrNoCharmURLSet
}

// SetCharm marks the unit as currently using the supplied charm URL.
// An error will be returned if the unit is dead, or the charm URL not known.
func (u *Unit) SetCharm(ctx context.Context, curl string) error {
	if curl == "" {
		return errors.Errorf("charm URL cannot be nil")
	}
	var result params.ErrorResults
	args := params.EntitiesCharmURL{
		Entities: []params.EntityCharmURL{
			{Tag: u.tag.String(), CharmURL: curl},
		},
	}
	err := u.client.facade.FacadeCall(ctx, "SetCharm", args, &result)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return result.OneError()
}

// ClearResolved removes any resolved setting on the unit.
func (u *Unit) ClearResolved(ctx context.Context) error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "ClearResolved", args, &result)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return result.OneError()
}

// WatchConfigSettingsHash returns a watcher for observing changes to
// the unit's charm configuration settings (with a hash of the
// settings content so we can determine whether it has changed since
// it was last seen by the uniter). The unit must have a charm URL set
// before this method is called, and the returned watcher will be
// valid only while the unit's charm URL is not changed.
func (u *Unit) WatchConfigSettingsHash(ctx context.Context) (watcher.StringsWatcher, error) {
	return getHashWatcher(ctx, u, "WatchConfigSettingsHash")
}

// WatchTrustConfigSettingsHash returns a watcher for observing changes to
// the unit's application configuration settings (with a hash of the
// settings content so we can determine whether it has changed since
// it was last seen by the uniter).
func (u *Unit) WatchTrustConfigSettingsHash(ctx context.Context) (watcher.StringsWatcher, error) {
	return getHashWatcher(ctx, u, "WatchTrustConfigSettingsHash")
}

func getHashWatcher(ctx context.Context, u *Unit, methodName string) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, methodName, args, &results)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(u.client.facade.RawAPICaller(), result)
	return w, nil
}

// WatchAddressesHash returns a watcher for observing changes to the
// hash of the unit's addresses.
// For IAAS models, the unit must be assigned to a machine before
// this method is called, and the returned watcher will be valid
// only while the unit's assigned machine is not changed.
// For CAAS models, the watcher observes changes to the address
// of the pod associated with the unit.
func (u *Unit) WatchAddressesHash(ctx context.Context) (watcher.StringsWatcher, error) {
	return getHashWatcher(ctx, u, "WatchUnitAddressesHash")
}

// WatchActionNotifications returns a StringsWatcher for observing the
// ids of Actions added to the Unit. The initial event will contain the
// ids of any Actions pending at the time the Watcher is made.
func (u *Unit) WatchActionNotifications(ctx context.Context) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "WatchActionNotifications", args, &results)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(u.client.facade.RawAPICaller(), result)
	return w, nil
}

// LogActionMessage logs a progress message for the specified action.
func (u *Unit) LogActionMessage(ctx context.Context, tag names.ActionTag, message string) error {
	var result params.ErrorResults
	args := params.ActionMessageParams{
		Messages: []params.EntityString{{Tag: tag.String(), Value: message}},
	}
	err := u.client.facade.FacadeCall(ctx, "LogActionsMessages", args, &result)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return result.OneError()
}

// RequestReboot sets the reboot flag for its machine agent
func (u *Unit) RequestReboot(ctx context.Context) error {
	machineId, err := u.AssignedMachine(ctx)
	if err != nil {
		return err
	}
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: machineId.String()}},
	}
	err = u.client.facade.FacadeCall(ctx, "RequestReboot", args, &result)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return result.OneError()
}

// RelationStatus holds information about a relation's scope and status.
type RelationStatus struct {
	// Tag is the relation tag.
	Tag names.RelationTag

	// Suspended is true if the relation is suspended.
	Suspended bool

	// InScope is true if the relation unit is in scope.
	InScope bool
}

// RelationsStatus returns the tags of the relations the unit has joined
// and entered scope, or the relation is suspended.
func (u *Unit) RelationsStatus(ctx context.Context) ([]RelationStatus, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	var results params.RelationUnitStatusResults
	err := u.client.facade.FacadeCall(ctx, "RelationsStatus", args, &results)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	var statusResult []RelationStatus
	for _, result := range result.RelationResults {
		tag, err := names.ParseRelationTag(result.RelationTag)
		if err != nil {
			return nil, err
		}
		statusResult = append(statusResult, RelationStatus{
			Tag:       tag,
			InScope:   result.InScope,
			Suspended: result.Suspended,
		})
	}
	return statusResult, nil
}

// WatchStorage returns a watcher for observing changes to the
// unit's storage attachments.
func (u *Unit) WatchStorage(ctx context.Context) (watcher.StringsWatcher, error) {
	return u.client.WatchUnitStorageAttachments(ctx, u.tag)
}

// WatchInstanceData returns a watcher for observing changes to the
// instanceData of the unit's machine.  Primarily used for watching
// LXDProfile changes.
func (u *Unit) WatchInstanceData(ctx context.Context) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "WatchInstanceData", args, &results)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(u.client.facade.RawAPICaller(), result)
	return w, nil
}

// LXDProfileName returns the name of the lxd profile applied to the unit's
// machine for the current charm version.
func (u *Unit) LXDProfileName(ctx context.Context) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "LXDProfileName", args, &results)
	if err != nil {
		return "", errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// CanApplyLXDProfile returns true if an lxd profile can be applied to
// this unit, e.g. this is an lxd machine or container and not maunal
func (u *Unit) CanApplyLXDProfile(ctx context.Context) (bool, error) {
	var results params.BoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.client.facade.FacadeCall(ctx, "CanApplyLXDProfile", args, &results)
	if err != nil {
		return false, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return false, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return false, result.Error
	}
	return result.Result, nil
}

// NetworkInfo returns network interfaces/addresses for specified bindings.
func (u *Unit) NetworkInfo(ctx context.Context, bindings []string, relationId *int) (map[string]params.NetworkInfoResult, error) {
	var results params.NetworkInfoResults
	args := params.NetworkInfoParams{
		Unit:       u.tag.String(),
		Endpoints:  bindings,
		RelationId: relationId,
	}

	err := u.client.facade.FacadeCall(ctx, "NetworkInfo", args, &results)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}

	return results.Results, nil
}

// State returns the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *Unit) State(ctx context.Context) (params.UnitStateResult, error) {
	return u.client.State(ctx)
}

// SetState sets the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *Unit) SetState(ctx context.Context, unitState params.SetUnitStateArg) error {
	return u.client.SetState(ctx, unitState)
}

// CommitHookChanges batches together all required API calls for applying
// a set of changes after a hook successfully completes and executes them in a
// single transaction.
func (u *Unit) CommitHookChanges(ctx context.Context, req params.CommitHookChangesArgs) error {
	var results params.ErrorResults
	err := u.client.facade.FacadeCall(ctx, "CommitHookChanges", req, &results)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return apiservererrors.RestoreError(results.OneError())
}

// CommitHookParamsBuilder is a helper type for populating the set of
// parameters used to perform a CommitHookChanges API call.
type CommitHookParamsBuilder struct {
	arg params.CommitHookChangesArg
}

// NewCommitHookParamsBuilder returns a new builder for assembling the
// parameters for a CommitHookChanges API call.
func NewCommitHookParamsBuilder(unitTag names.UnitTag) *CommitHookParamsBuilder {
	return &CommitHookParamsBuilder{
		arg: params.CommitHookChangesArg{
			Tag: unitTag.String(),
		},
	}
}

// OpenPortRange records a request to open a particular port range.
func (b *CommitHookParamsBuilder) OpenPortRange(endpoint string, portRange network.PortRange) {
	b.arg.OpenPorts = append(b.arg.OpenPorts, params.EntityPortRange{
		// The Tag is optional as the call uses the Tag from the
		// CommitHookChangesArg; it is included here for consistency.
		Tag:      b.arg.Tag,
		Endpoint: endpoint,
		Protocol: portRange.Protocol,
		FromPort: portRange.FromPort,
		ToPort:   portRange.ToPort,
	})
}

// ClosePortRange records a request to close a particular port range.
func (b *CommitHookParamsBuilder) ClosePortRange(endpoint string, portRange network.PortRange) {
	b.arg.ClosePorts = append(b.arg.ClosePorts, params.EntityPortRange{
		// The Tag is optional as the call uses the Tag from the
		// CommitHookChangesArg; it is included here for consistency.
		Tag:      b.arg.Tag,
		Endpoint: endpoint,
		Protocol: portRange.Protocol,
		FromPort: portRange.FromPort,
		ToPort:   portRange.ToPort,
	})
}

// UpdateRelationUnitSettings records a request to update the unit/application
// settings for a relation.
func (b *CommitHookParamsBuilder) UpdateRelationUnitSettings(relName string, unitSettings, appSettings params.Settings) {
	b.arg.RelationUnitSettings = append(b.arg.RelationUnitSettings, params.RelationUnitSettings{
		Relation:            relName,
		Unit:                b.arg.Tag,
		Settings:            unitSettings,
		ApplicationSettings: appSettings,
	})
}

// UpdateNetworkInfo records a request to update the network information
// settings for each joined relation.
func (b *CommitHookParamsBuilder) UpdateNetworkInfo() {
	b.arg.UpdateNetworkInfo = true
}

// UpdateCharmState records a request to update the server-persisted charm state.
func (b *CommitHookParamsBuilder) UpdateCharmState(state map[string]string) {
	b.arg.SetUnitState = &params.SetUnitStateArg{
		// The Tag is optional as the call uses the Tag from the
		// CommitHookChangesArg; it is included here for consistency.
		Tag:        b.arg.Tag,
		CharmState: &state,
	}
}

// AddStorage records a request for adding storage.
func (b *CommitHookParamsBuilder) AddStorage(constraints map[string][]params.StorageDirectives) {
	storageReqs := make([]params.StorageAddParams, 0, len(constraints))
	for storage, cons := range constraints {
		for _, one := range cons {
			storageReqs = append(storageReqs, params.StorageAddParams{
				UnitTag:     b.arg.Tag,
				StorageName: storage,
				Directives:  one,
			})
		}
	}

	b.arg.AddStorage = storageReqs
}

// SecretUpsertArg holds parameters for creating or updating a secret.
type SecretUpsertArg struct {
	URI          *secrets.URI
	RotatePolicy *secrets.RotatePolicy
	ExpireTime   *time.Time
	Description  *string
	Label        *string
	Value        secrets.SecretValue
	ValueRef     *secrets.ValueRef
	Checksum     string
}

// SecretCreateArg holds parameters for creating a secret.
type SecretCreateArg struct {
	SecretUpsertArg
	Owner secrets.Owner
}

// SecretUpdateArg holds parameters for updating a secret.
type SecretUpdateArg struct {
	SecretUpsertArg
	CurrentRevision int
}

// SecretDeleteArg holds parameters for deleting a secret.
type SecretDeleteArg struct {
	URI      *secrets.URI
	Revision *int
}

// AddSecretCreates records requests to create secrets.
func (b *CommitHookParamsBuilder) AddSecretCreates(creates []SecretCreateArg) error {
	if len(creates) == 0 {
		return nil
	}
	b.arg.SecretCreates = make([]params.CreateSecretArg, len(creates))
	for i, c := range creates {

		var data secrets.SecretData
		if c.Value != nil {
			data = c.Value.EncodedValues()
		}
		if len(data) == 0 {
			data = nil
		}

		uriStr := c.URI.String()
		var valueRef *params.SecretValueRef
		if c.ValueRef != nil {
			valueRef = &params.SecretValueRef{
				BackendID:  c.ValueRef.BackendID,
				RevisionID: c.ValueRef.RevisionID,
			}
		}

		ownerTag, err := common.OwnerTagFromSecretOwner(c.Owner)
		if err != nil {
			return errors.Trace(err)
		}
		b.arg.SecretCreates[i] = params.CreateSecretArg{
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: c.RotatePolicy,
				ExpireTime:   c.ExpireTime,
				Description:  c.Description,
				Label:        c.Label,
				Content: params.SecretContentParams{
					Data:     data,
					ValueRef: valueRef,
					Checksum: c.Checksum,
				},
			},
			URI:      &uriStr,
			OwnerTag: ownerTag.String(),
		}
	}
	return nil
}

// AddSecretUpdates records requests to update secrets.
func (b *CommitHookParamsBuilder) AddSecretUpdates(updates []SecretUpsertArg) {
	if len(updates) == 0 {
		return
	}
	b.arg.SecretUpdates = make([]params.UpdateSecretArg, len(updates))
	for i, u := range updates {

		var data secrets.SecretData
		if u.Value != nil {
			data = u.Value.EncodedValues()
		}
		if len(data) == 0 {
			data = nil
		}

		var valueRef *params.SecretValueRef
		if u.ValueRef != nil {
			valueRef = &params.SecretValueRef{
				BackendID:  u.ValueRef.BackendID,
				RevisionID: u.ValueRef.RevisionID,
			}
		}

		b.arg.SecretUpdates[i] = params.UpdateSecretArg{
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: u.RotatePolicy,
				ExpireTime:   u.ExpireTime,
				Description:  u.Description,
				Label:        u.Label,
				Content: params.SecretContentParams{
					Data:     data,
					ValueRef: valueRef,
					Checksum: u.Checksum,
				},
			},
			URI: u.URI.String(),
		}
	}
}

// AddTrackLatest records the URIs for which the latest revision should be tracked.
func (b *CommitHookParamsBuilder) AddTrackLatest(trackLatest []string) {
	if len(trackLatest) == 0 {
		return
	}
	b.arg.TrackLatest = make([]string, len(trackLatest))
	copy(b.arg.TrackLatest, trackLatest)
}

// SecretGrantRevokeArgs holds parameters for updating a secret's access.
type SecretGrantRevokeArgs struct {
	URI             *secrets.URI
	ApplicationName *string
	UnitName        *string
	RelationKey     *string
	Role            secrets.SecretRole
}

// Equal returns true if the two SecretGrantRevokeArgs are equal.
func (arg SecretGrantRevokeArgs) Equal(other SecretGrantRevokeArgs) bool {
	return arg.URI.ID == other.URI.ID &&
		arg.Role == other.Role &&
		((arg.ApplicationName == nil && other.ApplicationName == nil) ||
			(arg.ApplicationName != nil && other.ApplicationName != nil && *arg.ApplicationName == *other.ApplicationName)) &&
		((arg.UnitName == nil && other.UnitName == nil) ||
			(arg.UnitName != nil && other.UnitName != nil && *arg.UnitName == *other.UnitName)) &&
		((arg.RelationKey == nil && other.RelationKey == nil) ||
			(arg.RelationKey != nil && other.RelationKey != nil && *arg.RelationKey == *other.RelationKey))
}

// AddSecretGrants records requests to grant secret access.
func (b *CommitHookParamsBuilder) AddSecretGrants(grants []SecretGrantRevokeArgs) {
	if len(grants) == 0 {
		return
	}
	b.arg.SecretGrants = make([]params.GrantRevokeSecretArg, len(grants))
	for i, g := range grants {
		b.arg.SecretGrants[i] = g.ToParams()
	}
}

// AddSecretRevokes records requests to revoke secret access.
func (b *CommitHookParamsBuilder) AddSecretRevokes(revokes []SecretGrantRevokeArgs) {
	if len(revokes) == 0 {
		return
	}
	b.arg.SecretRevokes = make([]params.GrantRevokeSecretArg, len(revokes))
	for i, g := range revokes {
		b.arg.SecretRevokes[i] = g.ToParams()
	}
}

// ToParams converts a SecretGrantRevokeArgs to a params.GrantRevokeSecretArg.
func (arg SecretGrantRevokeArgs) ToParams() params.GrantRevokeSecretArg {
	var subjectTag, scopeTag string
	if arg.ApplicationName != nil {
		subjectTag = names.NewApplicationTag(*arg.ApplicationName).String()
	}
	if arg.UnitName != nil {
		subjectTag = names.NewUnitTag(*arg.UnitName).String()
	}
	if arg.RelationKey != nil {
		scopeTag = names.NewRelationTag(*arg.RelationKey).String()
	} else {
		scopeTag = subjectTag
	}
	return params.GrantRevokeSecretArg{
		URI:         arg.URI.String(),
		ScopeTag:    scopeTag,
		SubjectTags: []string{subjectTag},
		Role:        string(arg.Role),
	}
}

// AddSecretDeletes records requests to delete secrets.
func (b *CommitHookParamsBuilder) AddSecretDeletes(deletes []SecretDeleteArg) {
	if len(deletes) == 0 {
		return
	}
	b.arg.SecretDeletes = make([]params.DeleteSecretArg, len(deletes))
	for i, d := range deletes {
		var revs []int
		if d.Revision != nil {
			revs = []int{*d.Revision}
		}
		b.arg.SecretDeletes[i] = params.DeleteSecretArg{
			URI:       d.URI.String(),
			Revisions: revs,
		}
	}
}

// Build assembles the recorded change requests into a CommitHookChangesArgs
// instance that can be passed as an argument to the CommitHookChanges API
// call.
func (b *CommitHookParamsBuilder) Build() (params.CommitHookChangesArgs, int) {
	return params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{
			b.arg,
		},
	}, b.changeCount()
}

// changeCount returns the number of changes recorded by this builder instance.
func (b *CommitHookParamsBuilder) changeCount() int {
	var count int
	if b.arg.UpdateNetworkInfo {
		count++
	}
	if b.arg.SetUnitState != nil {
		count++
	}
	count += len(b.arg.RelationUnitSettings)
	count += len(b.arg.OpenPorts)
	count += len(b.arg.ClosePorts)
	count += len(b.arg.AddStorage)
	count += len(b.arg.SecretCreates)
	count += len(b.arg.SecretUpdates)
	count += len(b.arg.SecretDeletes)
	count += len(b.arg.SecretGrants)
	count += len(b.arg.SecretRevokes)
	return count
}
