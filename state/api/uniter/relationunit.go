// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// This module implements a subset of the interface provided by
// state.RelationUnit, as needed by the uniter API.
// Most of this is pretty much a verbatim copy of the code in
// state/relationunit.go, except for a few API-specific changes.

// RelationUnit holds information about a single unit in a relation,
// and allows clients to conveniently access unit-specific
// functionality.
type RelationUnit struct {
	st       *State
	relation *Relation
	unit     *Unit
	endpoint Endpoint
	scope    string
}

// Relation returns the relation associated with the unit.
func (ru *RelationUnit) Relation() *Relation {
	return ru.relation
}

// Endpoint returns the relation endpoint that defines the unit's
// participation in the relation.
func (ru *RelationUnit) Endpoint() Endpoint {
	return ru.endpoint
}

// PrivateAddress returns the private address of the unit and whether
// it is valid.
//
// NOTE: This differs from state.RelationUnit.PrivateAddress() by
// returning an error instead of a bool, because it needs to make an
// API call.
func (ru *RelationUnit) PrivateAddress() (string, error) {
	return ru.unit.PrivateAddress()
}

// EnterScope ensures that the unit has entered its scope in the relation.
// When the unit has already entered its relation scope, EnterScope will report
// success but make no changes to state.
//
// Otherwise, assuming both the relation and the unit are alive, it will enter
// scope.
//
// If the unit is a principal and the relation has container scope, EnterScope
// will also create the required subordinate unit, if it does not already exist;
// this is because there's no point having a principal in scope if there is no
// corresponding subordinate to join it.
//
// Once a unit has entered a scope, it stays in scope without further
// intervention; the relation will not be able to become Dead until all units
// have departed its scopes.
//
// NOTE: Unlike state.RelatioUnit.EnterScope(), this method does not take
// settings, because uniter only uses this to supply the unit's private
// address, but this is not done at the server-side by the API.
func (ru *RelationUnit) EnterScope() error {
	var result params.ErrorResults
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{{
			Relation: ru.relation.tag,
			Unit:     ru.unit.tag,
		}},
	}
	err := ru.st.caller.Call("Uniter", "", "EnterScope", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// LeaveScope signals that the unit has left its scope in the relation.
// After the unit has left its relation scope, it is no longer a member
// of the relation; if the relation is dying when its last member unit
// leaves, it is removed immediately. It is not an error to leave a scope
// that the unit is not, or never was, a member of.
func (ru *RelationUnit) LeaveScope() error {
	var result params.ErrorResults
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{{
			Relation: ru.relation.tag,
			Unit:     ru.unit.tag,
		}},
	}
	err := ru.st.caller.Call("Uniter", "", "LeaveScope", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Settings returns a Settings which allows access to the unit's settings
// within the relation.
func (ru *RelationUnit) Settings() (*Settings, error) {
	var results params.RelationSettingsResults
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{{
			Relation: ru.relation.tag,
			Unit:     ru.unit.tag,
		}},
	}
	err := ru.st.caller.Call("Uniter", "", "ReadSettings", args, &results)
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
	return newSettings(ru.st, ru.relation.tag, ru.unit.tag, result.Settings), nil
}

// ReadSettings returns a map holding the settings of the unit with the
// supplied name within this relation. An error will be returned if the
// relation no longer exists, or if the unit's service is not part of the
// relation, or the settings are invalid; but mere non-existence of the
// unit is not grounds for an error, because the unit settings are
// guaranteed to persist for the lifetime of the relation, regardless
// of the lifetime of the unit.
func (ru *RelationUnit) ReadSettings(uname string) (params.RelationSettings, error) {
	tag := names.UnitTag(uname)
	var results params.RelationSettingsResults
	args := params.RelationUnitPairs{
		RelationUnitPairs: []params.RelationUnitPair{{
			Relation:   ru.relation.tag,
			LocalUnit:  ru.unit.tag,
			RemoteUnit: tag,
		}},
	}
	err := ru.st.caller.Call("Uniter", "", "ReadRemoteSettings", args, &results)
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
	return result.Settings, nil
}

// Watch returns a watcher that notifies of changes to counterpart
// units in the relation.
func (ru *RelationUnit) Watch() (watcher.RelationUnitsWatcher, error) {
	var results params.RelationUnitsWatchResults
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{{
			Relation: ru.relation.tag,
			Unit:     ru.unit.tag,
		}},
	}
	err := ru.st.caller.Call("Uniter", "", "WatchRelationUnits", args, &results)
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
	w := watcher.NewRelationUnitsWatcher(ru.st.caller, result)
	return w, nil
}
