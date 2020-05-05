// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
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
	unitTag  names.UnitTag
	appTag   names.ApplicationTag
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
			Relation: ru.relation.tag.String(),
			Unit:     ru.unitTag.String(),
		}},
	}
	err := ru.st.facade.FacadeCall("EnterScope", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(result.OneError())
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
			Relation: ru.relation.tag.String(),
			Unit:     ru.unitTag.String(),
		}},
	}
	err := ru.st.facade.FacadeCall("LeaveScope", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(result.OneError())
}

// Settings returns a Settings which allows access to the unit's settings
// within the relation.
func (ru *RelationUnit) Settings() (*Settings, error) {
	var results params.SettingsResults
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{{
			Relation: ru.relation.tag.String(),
			Unit:     ru.unitTag.String(),
		}},
	}
	err := ru.st.facade.FacadeCall("ReadSettings", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	return newSettings(ru.st, ru.relation.tag.String(), ru.unitTag.String(), result.Settings), nil
}

// ApplicationSettings returns a Settings which allows access to this unit's
// application settings within the relation. This can only be used from the
// leader unit. Calling it from a non-Leader generates a NotLeader error.
func (ru *RelationUnit) ApplicationSettings() (*Settings, error) {
	var result params.SettingsResult
	arg := params.RelationUnit{
		Relation: ru.relation.tag.String(),
		Unit:     ru.unitTag.String(),
	}
	if err := ru.st.facade.FacadeCall("ReadLocalApplicationSettings", arg, &result); err != nil {
		return nil, errors.Trace(err)
	} else if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	return newSettings(ru.st, ru.relation.tag.String(), ru.appTag.String(), result.Settings), nil
}

// ReadSettings returns a map holding the settings of the unit with the
// supplied name within this relation. An error will be returned if the
// relation no longer exists, or if the unit's application is not part of the
// relation, or the settings are invalid; but mere non-existence of the
// unit is not grounds for an error, because the unit settings are
// guaranteed to persist for the lifetime of the relation, regardless
// of the lifetime of the unit.
func (ru *RelationUnit) ReadSettings(name string) (params.Settings, error) {
	var tag names.Tag
	if names.IsValidUnit(name) {
		tag = names.NewUnitTag(name)
	} else if names.IsValidApplication(name) {
		tag = names.NewApplicationTag(name)
	} else {
		return nil, errors.Errorf("%q is not a valid unit or application", name)
	}
	var results params.SettingsResults
	args := params.RelationUnitPairs{
		RelationUnitPairs: []params.RelationUnitPair{{
			Relation:   ru.relation.tag.String(),
			LocalUnit:  ru.unitTag.String(),
			RemoteUnit: tag.String(),
		}},
	}
	err := ru.st.facade.FacadeCall("ReadRemoteSettings", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	return result.Settings, nil
}

// UpdateRelationSettings is used to record any changes to settings for this unit and application.
// It is only valid to update application settings if this unit is the leader, otherwise
// it is a NotLeader error. Note that either unit or application is allowed to be nil.
func (ru *RelationUnit) UpdateRelationSettings(unit, application params.Settings) error {
	var result params.ErrorResults
	args := params.RelationUnitsSettings{
		RelationUnits: []params.RelationUnitSettings{{
			Relation:            ru.relation.tag.String(),
			Unit:                ru.unitTag.String(),
			Settings:            unit,
			ApplicationSettings: application,
		}},
	}
	err := ru.st.facade.FacadeCall("UpdateSettings", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	err = result.OneError()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
