// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// Unit represents a juju unit as seen by a uniter worker.
type Unit struct {
	st   *State
	tag  string
	life params.Life
}

// Tag returns the unit's tag.
func (u *Unit) Tag() string {
	return u.tag
}

// Name returns the name of the unit.
func (u *Unit) Name() string {
	_, unitName, err := names.ParseTag(u.tag, names.UnitTagKind)
	if err != nil {
		panic(fmt.Sprintf("%q is not a valid unit tag", u.tag))
	}
	return unitName
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
	life, err := u.st.unitLife(u.tag)
	if err != nil {
		return err
	}
	u.life = life
	return nil
}

// SetStatus sets the status of the unit.
func (u *Unit) SetStatus(status params.Status, info string) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.SetEntityStatus{
			{Tag: u.tag, Status: status, Info: info},
		},
	}
	err := u.st.caller.Call("Uniter", "", "SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// EnsureDead sets the unit lifecycle to Dead if it is Alive or
// Dying. It does nothing otherwise.
func (u *Unit) EnsureDead() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag}},
	}
	err := u.st.caller.Call("Uniter", "", "EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Watch returns a watcher for observing changes to the unit.
func (u *Unit) Watch() (*watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag}},
	}
	err := u.st.caller.Call("Uniter", "", "Watch", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(u.st.caller, result)
	return w, nil
}

// Service returns the service.
func (u *Unit) Service() (*Service, error) {
	serviceTag := names.ServiceTag(u.ServiceName())
	service := &Service{
		st:  u.st,
		tag: serviceTag,
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
	// TODO: Call Uniter.ConfigSettings()
	panic("not implemented")
}

// ServiceName returns the service name.
func (u *Unit) ServiceName() string {
	return names.UnitService(u.Name())
}

// Destroy, when called on a Alive unit, advances its lifecycle as far as
// possible; it otherwise has no effect. In most situations, the unit's
// life is just set to Dying; but if a principal unit that is not assigned
// to a provisioned machine is Destroyed, it will be removed from state
// directly.
func (u *Unit) Destroy() (err error) {
	// TODO: Call Uniter.Destroy()
	panic("not implemented")
}

// Resolved returns the resolved mode for the unit.
//
// NOTE: This differs from state.Unit.Resolved() by returning an
// error as well, because it needs to make an API call
func (u *Unit) Resolved() (params.ResolvedMode, error) {
	// TODO: Call Uniter.Resolved()
	panic("not implemented")
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
//
// NOTE: This differs from state.Unit.IsPrincipal() by returning an
// error as well, because it needs to make an API call.
func (u *Unit) IsPrincipal() (bool, error) {
	// TODO: Call Uniter.GetPrincipal()
	panic("not implemented")
}

// SubordinateNames returns the names of any subordinate units.
//
// NOTE: This differs from state.Unit.SubordinateNames() by returning
// an error as well, because it needs to make an API call.
func (u *Unit) SubordinateNames() ([]string, error) {
	// TODO: Call Uniter.SubordinateNames()
	panic("not implemented")
}

// PublicAddress returns the public address of the unit and whether it
// is valid.
//
// NOTE: This differs from state.Unit.PublicAddres() by returning
// an error as well, because it needs to make an API call.
//
// TODO: We might be able to drop this, once we have machine
// addresses implemented fully.
func (u *Unit) PublicAddress() (string, bool, error) {
	// TODO: Call Uniter.PublicAddress()
	panic("not implemented")
}

// SetPublicAddress sets the public address of the unit.
//
// TODO: We might be able to drop this, once we have machine
// addresses implemented fully.
func (u *Unit) SetPublicAddress(address string) (err error) {
	// TODO: Call Uniter.SetPublicAddress()
	panic("not implemented")
}

// PrivateAddress returns the private address of the unit and whether
// it is valid.
//
// NOTE: This differs from state.Unit.PrivateAddress() by returning
// an error as well, because it needs to make an API call.
//
// TODO: We might be able to drop this, once we have machine
// addresses implemented fully.
func (u *Unit) PrivateAddress() (string, bool, error) {
	// TODO: Call Uniter.PrivateAddress()
	panic("not implemented")
}

// SetPrivateAddress sets the private address of the unit.
//
// TODO: We might be able to drop this, once we have machine
// addresses implemented fully.
func (u *Unit) SetPrivateAddress(address string) error {
	// TODO: Call Uniter.SetPrivateAddress()
	panic("not implemented")
}

// OpenPort sets the policy of the port with protocol and number to be
// opened.
//
// TODO: We should really be opening and closing ports on machines,
// rather than units.
func (u *Unit) OpenPort(protocol string, number int) (err error) {
	// TODO: Call Uniter.OpenPort()
	panic("not implemented")
}

// ClosePort sets the policy of the port with protocol and number to
// be closed.
//
// TODO: We should really be opening and closing ports on machines,
// rather than units.
func (u *Unit) ClosePort(protocol string, number int) (err error) {
	// TODO: Call Uniter.ClosePort()
	panic("not implemented")
}

// CharmURL returns the charm URL this unit is currently using.
//
// NOTE: This differs from state.Unit.CharmURL() by returning
// an error as well, because it needs to make an API call.
func (u *Unit) CharmURL() (*charm.URL, bool, error) {
	// TODO: Call Uniter.CharmURL()
	panic("not implemented")
}

// SetCharmURL marks the unit as currently using the supplied charm URL.
// An error will be returned if the unit is dead, or the charm URL not known.
func (u *Unit) SetCharmURL(curl *charm.URL) (err error) {
	// TODO: Call Uniter.SetCharmURL()
	panic("not implemented")
}

// ClearResolved removes any resolved setting on the unit.
func (u *Unit) ClearResolved() error {
	// TODO: Call Uniter.ClearResolved()
	panic("not implemented")
}

// WatchConfigSettings returns a watcher for observing changes to the
// unit's service configuration settings. The unit must have a charm URL
// set before this method is called, and the returned watcher will be
// valid only while the unit's charm URL is not changed.
func (u *Unit) WatchConfigSettings() (*watcher.NotifyWatcher, error) {
	// TODO: Call Uniter.WatchConfigSettings(), passing the unit tag
	// as argument, then start a client NotifyWatcher, like
	// uniter.Unit.Watch() does.
	panic("not implemented")
}
