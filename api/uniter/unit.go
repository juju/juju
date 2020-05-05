// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

// Unit represents a juju unit as seen by a uniter worker.
type Unit struct {
	st           *State
	tag          names.UnitTag
	life         life.Value
	resolvedMode params.ResolvedMode
	providerID   string
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
func (u *Unit) Resolved() params.ResolvedMode {
	return u.resolvedMode
}

// Refresh updates the cached local copy of the unit's data.
func (u *Unit) Refresh() error {
	var results params.UnitRefreshResults
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: u.tag.String()},
		},
	}
	err := u.st.facade.FacadeCall("Refresh", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return errors.Trace(result.Error)
	}

	u.life = result.Life
	u.resolvedMode = result.Resolved
	u.providerID = result.ProviderID
	return nil
}

// SetUnitStatus sets the status of the unit.
func (u *Unit) SetUnitStatus(unitStatus status.Status, info string, data map[string]interface{}) error {
	if u.st.facade.BestAPIVersion() < 2 {
		return errors.NotImplementedf("SetUnitStatus")
	}
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: u.tag.String(), Status: unitStatus.String(), Info: info, Data: data},
		},
	}
	err := u.st.facade.FacadeCall("SetUnitStatus", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}

// UnitStatus gets the status details of the unit.
func (u *Unit) UnitStatus() (params.StatusResult, error) {
	var results params.StatusResults
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: u.tag.String()},
		},
	}
	err := u.st.facade.FacadeCall("UnitStatus", args, &results)
	if err != nil {
		return params.StatusResult{}, errors.Trace(err)
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
func (u *Unit) SetAgentStatus(agentStatus status.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: u.tag.String(), Status: agentStatus.String(), Info: info, Data: data},
		},
	}
	setStatusFacadeCall := "SetAgentStatus"
	if u.st.facade.BestAPIVersion() < 2 {
		setStatusFacadeCall = "SetStatus"
	}
	err := u.st.facade.FacadeCall(setStatusFacadeCall, args, &result)
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

// AddMetricBatches makes an api call to the uniter requesting it to store metrics batches in state.
func (u *Unit) AddMetricBatches(batches []params.MetricBatch) (map[string]error, error) {
	p := params.MetricBatchParams{
		Batches: make([]params.MetricBatchParam, len(batches)),
	}

	batchResults := make(map[string]error, len(batches))

	for i, batch := range batches {
		p.Batches[i].Tag = u.tag.String()
		p.Batches[i].Batch = batch

		batchResults[batch.UUID] = nil
	}
	results := new(params.ErrorResults)
	err := u.st.facade.FacadeCall("AddMetricBatches", p, results)
	if err != nil {
		return nil, errors.Annotate(err, "failed to send metric batches")
	}
	for i, result := range results.Results {
		batchResults[batches[i].UUID] = result.Error
	}
	return batchResults, nil
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
	return common.Watch(u.st.facade, "Watch", u.tag)
}

// WatchRelations returns a StringsWatcher that notifies of changes to
// the lifecycles of relations involving u.
func (u *Unit) WatchRelations() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("WatchUnitRelations", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(u.st.facade.RawAPICaller(), result)
	return w, nil
}

// Application returns the unit's application.
func (u *Unit) Application() (*Application, error) {
	application := &Application{
		st:  u.st,
		tag: u.ApplicationTag(),
	}
	// Call Refresh() immediately to get the up-to-date
	// life and other needed locally cached fields.
	err := application.Refresh()
	if err != nil {
		return nil, err
	}
	return application, nil
}

// ConfigSettings returns the complete set of application charm config settings
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
func (u *Unit) PrincipalName() (string, bool, error) {
	var results params.StringBoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("GetPrincipal", args, &results)
	if err != nil {
		return "", false, err
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
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
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
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
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
		return errors.Errorf("charm URL cannot be nil")
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

// WatchConfigSettingsHash returns a watcher for observing changes to
// the unit's charm configuration settings (with a hash of the
// settings content so we can determine whether it has changed since
// it was last seen by the uniter). The unit must have a charm URL set
// before this method is called, and the returned watcher will be
// valid only while the unit's charm URL is not changed.
func (u *Unit) WatchConfigSettingsHash() (watcher.StringsWatcher, error) {
	return getHashWatcher(u, "WatchConfigSettingsHash")
}

// WatchTrustConfigSettingsHash returns a watcher for observing changes to
// the unit's application configuration settings (with a hash of the
// settings content so we can determine whether it has changed since
// it was last seen by the uniter).
func (u *Unit) WatchTrustConfigSettingsHash() (watcher.StringsWatcher, error) {
	return getHashWatcher(u, "WatchTrustConfigSettingsHash")
}

func getHashWatcher(u *Unit, methodName string) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall(methodName, args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(u.st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchAddressesHash returns a watcher for observing changes to the
// hash of the unit's addresses.
// For IAAS models, the unit must be assigned to a machine before
// this method is called, and the returned watcher will be valid
// only while the unit's assigned machine is not changed.
// For CAAS models, the watcher observes changes to the address
// of the pod associated with the unit.
func (u *Unit) WatchAddressesHash() (watcher.StringsWatcher, error) {
	return getHashWatcher(u, "WatchUnitAddressesHash")
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
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(u.st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchUpgradeSeriesNotifications returns a NotifyWatcher for observing the
// state of a series upgrade.
func (u *Unit) WatchUpgradeSeriesNotifications() (watcher.NotifyWatcher, error) {
	return u.st.WatchUpgradeSeriesNotifications()
}

// LogActionMessage logs a progress message for the specified action.
func (u *Unit) LogActionMessage(tag names.ActionTag, message string) error {
	// Just a safety check since controller is always ahead of unit agents.
	if u.st.facade.BestAPIVersion() < 12 {
		return errors.NotImplementedf("LogActionMessage() (need V12+)")
	}

	var result params.ErrorResults
	args := params.ActionMessageParams{
		Messages: []params.EntityString{{Tag: tag.String(), Value: message}},
	}
	err := u.st.facade.FacadeCall("LogActionsMessages", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// UpgradeSeriesStatus returns the upgrade series status of a unit from remote state
func (u *Unit) UpgradeSeriesStatus() (model.UpgradeSeriesStatus, error) {
	res, err := u.st.UpgradeSeriesUnitStatus()
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(res) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(res))
	}
	return res[0], nil
}

// SetUpgradeSeriesStatus sets the upgrade series status of the unit in the remote state
func (u *Unit) SetUpgradeSeriesStatus(status model.UpgradeSeriesStatus, reason string) error {
	return u.st.SetUpgradeSeriesUnitStatus(status, reason)
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
func (u *Unit) RelationsStatus() ([]RelationStatus, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	var results params.RelationUnitStatusResults
	err := u.st.facade.FacadeCall("RelationsStatus", args, &results)
	if err != nil {
		return nil, err
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
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(u.st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchStorage returns a watcher for observing changes to the
// unit's storage attachments.
func (u *Unit) WatchStorage() (watcher.StringsWatcher, error) {
	return u.st.WatchUnitStorageAttachments(u.tag)
}

// AddStorage adds desired storage instances to a unit.
func (u *Unit) AddStorage(constraints map[string][]params.StorageConstraints) error {
	if u.st.facade.BestAPIVersion() < 2 {
		return errors.NotImplementedf("AddStorage() (need V2+)")
	}

	all := make([]params.StorageAddParams, 0, len(constraints))
	for storage, cons := range constraints {
		for _, one := range cons {
			all = append(all, params.StorageAddParams{
				UnitTag:     u.Tag().String(),
				StorageName: storage,
				Constraints: one,
			})
		}
	}

	args := params.StoragesAddParams{Storages: all}
	var results params.ErrorResults
	err := u.st.facade.FacadeCall("AddUnitStorage", args, &results)
	if err != nil {
		return err
	}

	return results.Combine()
}

// NetworkInfo returns network interfaces/addresses for specified bindings.
func (u *Unit) NetworkInfo(bindings []string, relationId *int) (map[string]params.NetworkInfoResult, error) {
	var results params.NetworkInfoResults
	args := params.NetworkInfoParams{
		Unit:       u.tag.String(),
		Endpoints:  bindings,
		RelationId: relationId,
	}

	err := u.st.facade.FacadeCall("NetworkInfo", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return results.Results, nil
}

// UpdateNetworkInfo updates the network settings for the unit's bound
// endpoints.
func (u *Unit) UpdateNetworkInfo() error {
	args := params.Entities{
		Entities: []params.Entity{
			{
				Tag: u.tag.String(),
			},
		},
	}

	var results params.ErrorResults
	err := u.st.facade.FacadeCall("UpdateNetworkInfo", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// State returns the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *Unit) State() (params.UnitStateResult, error) {
	return u.st.State()
}

// SetState sets the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *Unit) SetState(unitState params.SetUnitStateArg) error {
	return u.st.SetState(unitState)
}

// CommitHookChanges batches together all required API calls for applying
// a set of changes after a hook successfully completes and executes them in a
// single transaction.
func (u *Unit) CommitHookChanges(req params.CommitHookChangesArgs) error {
	var results params.ErrorResults
	err := u.st.facade.FacadeCall("CommitHookChanges", req, &results)
	if err != nil {
		return err
	}
	// Make sure we correctly decode quota-related errors.
	return maybeRestoreQuotaLimitError(results.OneError())
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
func (b *CommitHookParamsBuilder) OpenPortRange(protocol string, fromPort, toPort int) {
	b.arg.OpenPorts = append(b.arg.OpenPorts, params.EntityPortRange{
		// The Tag is optional as the call uses the Tag from the
		// CommitHookChangesArg; it is included here for consistency.
		Tag:      b.arg.Tag,
		Protocol: protocol,
		FromPort: fromPort,
		ToPort:   toPort,
	})
}

// ClosePortRange records a request to close a particular port range.
func (b *CommitHookParamsBuilder) ClosePortRange(protocol string, fromPort, toPort int) {
	b.arg.ClosePorts = append(b.arg.ClosePorts, params.EntityPortRange{
		// The Tag is optional as the call uses the Tag from the
		// CommitHookChangesArg; it is included here for consistency.
		Tag:      b.arg.Tag,
		Protocol: protocol,
		FromPort: fromPort,
		ToPort:   toPort,
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
func (b *CommitHookParamsBuilder) AddStorage(constraints map[string][]params.StorageConstraints) {
	storageReqs := make([]params.StorageAddParams, 0, len(constraints))
	for storage, cons := range constraints {
		for _, one := range cons {
			storageReqs = append(storageReqs, params.StorageAddParams{
				UnitTag:     b.arg.Tag,
				StorageName: storage,
				Constraints: one,
			})
		}
	}

	b.arg.AddStorage = storageReqs
}

// SetPodSpec records a request to update the PodSpec for an application.
func (b *CommitHookParamsBuilder) SetPodSpec(appTag names.ApplicationTag, spec *string) {
	b.arg.SetPodSpec = &params.PodSpec{
		Tag:  appTag.String(),
		Spec: spec,
	}
}

// SetRawK8sSpec records a request to update the PodSpec for an application.
func (b *CommitHookParamsBuilder) SetRawK8sSpec(appTag names.ApplicationTag, spec *string) {
	b.arg.SetRawK8sSpec = &params.PodSpec{
		Tag:  appTag.String(),
		Spec: spec,
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
	if b.arg.SetPodSpec != nil {
		count++
	}
	if b.arg.SetRawK8sSpec != nil {
		count++
	}

	count += len(b.arg.RelationUnitSettings)
	count += len(b.arg.OpenPorts)
	count += len(b.arg.ClosePorts)
	count += len(b.arg.AddStorage)
	return count
}

// maybeRestoreQuotaLimitError checks if the server emitted a quota limit
// exceeded error and restores it back to a typed error from juju/errors.
// Ideally, we would use apiserver/common.RestoreError but apparently, that
// package imports worker/uniter/{operation, remotestate} causing an import
// cycle.
func maybeRestoreQuotaLimitError(err error) error {
	if params.IsCodeQuotaLimitExceeded(err) {
		return errors.NewQuotaLimitExceeded(nil, err.Error())
	}
	return err
}
