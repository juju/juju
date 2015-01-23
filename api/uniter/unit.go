// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

// Unit represents a juju unit as seen by a uniter worker.
type Unit struct {
	st   *State
	tag  names.UnitTag
	life params.Life
}

// Tag returns the unit's tag.
func (u *Unit) Tag() names.UnitTag {
	return u.tag
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
func (u *Unit) Life() params.Life {
	return u.life
}

// Refresh updates the cached local copy of the unit's data.
func (u *Unit) Refresh() error {
	life, err := u.st.life(u.tag)
	if err != nil {
		return err
	}
	u.life = life
	return nil
}

// SetStatus sets the status of the unit agent.
func (u *Unit) SetStatus(status params.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatus{
			{Tag: u.tag.String(), Status: status, Info: info, Data: data},
		},
	}
	err := u.st.facade.FacadeCall("SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// AddMetrics adds the metrics for the unit.
func (u *Unit) AddMetrics(metrics []params.Metric) error {
	var result params.ErrorResults
	args := params.MetricsParams{
		Metrics: []params.MetricsParam{{
			Tag:     u.tag.String(),
			Metrics: metrics,
		}},
	}
	err := u.st.facade.FacadeCall("AddMetrics", args, &result)
	if err != nil {
		return errors.Annotate(err, "unable to add metric")
	}
	return result.OneError()
}

// EnsureDead sets the unit lifecycle to Dead if it is Alive or
// Dying. It does nothing otherwise.
func (u *Unit) EnsureDead() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Watch returns a watcher for observing changes to the unit.
func (u *Unit) Watch() (watcher.NotifyWatcher, error) {
	return common.Watch(u.st.facade, u.tag)
}

// Service returns the service.
func (u *Unit) Service() (*Service, error) {
	service := &Service{
		st:  u.st,
		tag: u.ServiceTag(),
	}
	// Call Refresh() immediately to get the up-to-date
	// life and other needed locally cached fields.
	err := service.Refresh()
	if err != nil {
		return nil, err
	}
	return service, nil
}

// ConfigSettings returns the complete set of service charm config settings
// available to the unit. Unset values will be replaced with the default
// value for the associated option, and may thus be nil when no default is
// specified.
func (u *Unit) ConfigSettings() (charm.Settings, error) {
	var results params.ConfigSettingsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("ConfigSettings", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return charm.Settings(result.Settings), nil
}

// ServiceName returns the service name.
func (u *Unit) ServiceName() string {
	service, err := names.UnitService(u.Name())
	if err != nil {
		panic(err)
	}
	return service
}

// ServiceTag returns the service tag.
func (u *Unit) ServiceTag() names.ServiceTag {
	return names.NewServiceTag(u.ServiceName())
}

// Destroy, when called on a Alive unit, advances its lifecycle as far as
// possible; it otherwise has no effect. In most situations, the unit's
// life is just set to Dying; but if a principal unit that is not assigned
// to a provisioned machine is Destroyed, it will be removed from state
// directly.
func (u *Unit) Destroy() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("Destroy", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// DestroyAllSubordinates destroys all subordinates of the unit.
func (u *Unit) DestroyAllSubordinates() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("DestroyAllSubordinates", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Resolved returns the resolved mode for the unit.
//
// NOTE: This differs from state.Unit.Resolved() by returning an
// error as well, because it needs to make an API call
func (u *Unit) Resolved() (params.ResolvedMode, error) {
	var results params.ResolvedModeResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("Resolved", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Mode, nil
}

// AssignedMachine returns the unit's assigned machine tag or an error
// satisfying params.IsCodeNotAssigned when the unit has no assigned
// machine..
func (u *Unit) AssignedMachine() (names.MachineTag, error) {
	if u.st.BestAPIVersion() < 1 {
		return names.MachineTag{}, errors.NotImplementedf("unit.AssignedMachine() (need V1+)")
	}
	var invalidTag names.MachineTag
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("AssignedMachine", args, &results)
	if err != nil {
		return invalidTag, err
	}
	if len(results.Results) != 1 {
		return invalidTag, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return invalidTag, result.Error
	}
	return names.ParseMachineTag(result.Result)
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
//
// NOTE: This differs from state.Unit.IsPrincipal() by returning an
// error as well, because it needs to make an API call.
func (u *Unit) IsPrincipal() (bool, error) {
	var results params.StringBoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("GetPrincipal", args, &results)
	if err != nil {
		return false, err
	}
	if len(results.Results) != 1 {
		return false, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return false, result.Error
	}
	// GetPrincipal returns false when the unit is subordinate.
	return !result.Ok, nil
}

// HasSubordinates returns the tags of any subordinate units.
func (u *Unit) HasSubordinates() (bool, error) {
	var results params.BoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("HasSubordinates", args, &results)
	if err != nil {
		return false, err
	}
	if len(results.Results) != 1 {
		return false, fmt.Errorf("expected 1 result, got %d", len(results.Results))
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
func (u *Unit) PublicAddress() (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("PublicAddress", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
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
func (u *Unit) PrivateAddress() (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("PrivateAddress", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// AvailabilityZone returns the availability zone of the unit.
func (u *Unit) AvailabilityZone() (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	if err := u.st.facade.FacadeCall("AvailabilityZone", args, &results); err != nil {
		return "", errors.Trace(err)
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

// OpenPorts sets the policy of the port range with protocol to be
// opened.
func (u *Unit) OpenPorts(protocol string, fromPort, toPort int) error {
	var result params.ErrorResults
	args := params.EntitiesPortRanges{
		Entities: []params.EntityPortRange{{
			Tag:      u.tag.String(),
			Protocol: protocol,
			FromPort: fromPort,
			ToPort:   toPort,
		}},
	}
	err := u.st.facade.FacadeCall("OpenPorts", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// ClosePorts sets the policy of the port range with protocol to be
// closed.
func (u *Unit) ClosePorts(protocol string, fromPort, toPort int) error {
	var result params.ErrorResults
	args := params.EntitiesPortRanges{
		Entities: []params.EntityPortRange{{
			Tag:      u.tag.String(),
			Protocol: protocol,
			FromPort: fromPort,
			ToPort:   toPort,
		}},
	}
	err := u.st.facade.FacadeCall("ClosePorts", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// OpenPort sets the policy of the port with protocol and number to be
// opened.
//
// TODO(dimitern): This is deprecated and is kept for
// backwards-compatibility. Use OpenPorts instead.
func (u *Unit) OpenPort(protocol string, number int) error {
	var result params.ErrorResults
	args := params.EntitiesPorts{
		Entities: []params.EntityPort{
			{Tag: u.tag.String(), Protocol: protocol, Port: number},
		},
	}
	err := u.st.facade.FacadeCall("OpenPort", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// ClosePort sets the policy of the port with protocol and number to
// be closed.
//
// TODO(dimitern): This is deprecated and is kept for
// backwards-compatibility. Use ClosePorts instead.
func (u *Unit) ClosePort(protocol string, number int) error {
	var result params.ErrorResults
	args := params.EntitiesPorts{
		Entities: []params.EntityPort{
			{Tag: u.tag.String(), Protocol: protocol, Port: number},
		},
	}
	err := u.st.facade.FacadeCall("ClosePort", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

var ErrNoCharmURLSet = errors.New("unit has no charm url set")

// CharmURL returns the charm URL this unit is currently using.
//
// NOTE: This differs from state.Unit.CharmURL() by returning
// an error instead of a bool, because it needs to make an API call.
func (u *Unit) CharmURL() (*charm.URL, error) {
	var results params.StringBoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("CharmURL", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	if result.Result != "" {
		curl, err := charm.ParseURL(result.Result)
		if err != nil {
			return nil, err
		}
		return curl, nil
	}
	return nil, ErrNoCharmURLSet
}

// SetCharmURL marks the unit as currently using the supplied charm URL.
// An error will be returned if the unit is dead, or the charm URL not known.
func (u *Unit) SetCharmURL(curl *charm.URL) error {
	if curl == nil {
		return fmt.Errorf("charm URL cannot be nil")
	}
	var result params.ErrorResults
	args := params.EntitiesCharmURL{
		Entities: []params.EntityCharmURL{
			{Tag: u.tag.String(), CharmURL: curl.String()},
		},
	}
	err := u.st.facade.FacadeCall("SetCharmURL", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// ClearResolved removes any resolved setting on the unit.
func (u *Unit) ClearResolved() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("ClearResolved", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// WatchConfigSettings returns a watcher for observing changes to the
// unit's service configuration settings. The unit must have a charm URL
// set before this method is called, and the returned watcher will be
// valid only while the unit's charm URL is not changed.
func (u *Unit) WatchConfigSettings() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("WatchConfigSettings", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(u.st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchAddresses returns a watcher for observing changes to the
// unit's addresses. The unit must be assigned to a machine before
// this method is called, and the returned watcher will be valid only
// while the unit's assigned machine is not changed.
func (u *Unit) WatchAddresses() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("WatchUnitAddresses", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(u.st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchActionNotifications returns a StringsWatcher for observing the
// ids of Actions added to the Unit. The initial event will contain the
// ids of any Actions pending at the time the Watcher is made.
func (u *Unit) WatchActionNotifications() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("WatchActionNotifications", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(u.st.facade.RawAPICaller(), result)
	return w, nil
}

// RequestReboot sets the reboot flag for its machine agent
func (u *Unit) RequestReboot() error {
	machineId, err := u.AssignedMachine()
	if err != nil {
		return err
	}
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: machineId.String()}},
	}
	err = u.st.facade.FacadeCall("RequestReboot", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// JoinedRelations returns the tags of the relations the unit has joined.
func (u *Unit) JoinedRelations() ([]names.RelationTag, error) {
	var results params.StringsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("JoinedRelations", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	var relTags []names.RelationTag
	for _, rel := range result.Result {
		tag, err := names.ParseRelationTag(rel)
		if err != nil {
			return nil, err
		}
		relTags = append(relTags, tag)
	}
	return relTags, nil
}

// MeterStatus returns the meter status of the unit.
func (u *Unit) MeterStatus() (statusCode, statusInfo string, rErr error) {
	var results params.MeterStatusResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("GetMeterStatus", args, &results)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return "", "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", "", errors.Trace(result.Error)
	}
	return result.Code, result.Info, nil
}

// WatchMeterStatus returns a watcher for observing changes to the
// unit's meter status.
func (u *Unit) WatchMeterStatus() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("WatchMeterStatus", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(u.st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchStorageInstances returns a watcher for observing changes to the
// unit's storage instances.
func (u *Unit) WatchStorageInstances() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("WatchStorageInstances", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(u.st.facade.RawAPICaller(), result)
	return w, nil
}
