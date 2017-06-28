// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/common/cloudspec"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

const firewallerFacade = "Firewaller"

// State provides access to the Firewaller API facade.
type State struct {
	facade base.FacadeCaller
	*common.ModelWatcher
	*cloudspec.CloudSpecAPI
}

// NewState creates a new client-side Firewaller API facade.
func NewState(caller base.APICaller) *State {
	facadeCaller := base.NewFacadeCaller(caller, firewallerFacade)
	return &State{
		facade:       facadeCaller,
		ModelWatcher: common.NewModelWatcher(facadeCaller),
		CloudSpecAPI: cloudspec.NewCloudSpecAPI(facadeCaller),
	}
}

// BestAPIVersion returns the API version that we were able to
// determine is supported by both the client and the API Server.
func (st *State) BestAPIVersion() int {
	return st.facade.BestAPIVersion()
}

// ModelTag returns the current model's tag.
func (st *State) ModelTag() (names.ModelTag, bool) {
	return st.facade.RawAPICaller().ModelTag()
}

// life requests the life cycle of the given entity from the server.
func (st *State) life(tag names.Tag) (params.Life, error) {
	return common.Life(st.facade, tag)
}

// Unit provides access to methods of a state.Unit through the facade.
func (st *State) Unit(tag names.UnitTag) (*Unit, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// Machine provides access to methods of a state.Machine through the
// facade.
func (st *State) Machine(tag names.MachineTag) (*Machine, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// WatchModelMachines returns a StringsWatcher that notifies of
// changes to the life cycles of the top level machines in the current
// model.
func (st *State) WatchModelMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := st.facade.FacadeCall("WatchModelMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchOpenedPorts returns a StringsWatcher that notifies of
// changes to the opened ports for the current model.
func (st *State) WatchOpenedPorts() (watcher.StringsWatcher, error) {
	modelTag, ok := st.ModelTag()
	if !ok {
		return nil, errors.New("API connection is controller-only (should never happen)")
	}
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: modelTag.String()}},
	}
	if err := st.facade.FacadeCall("WatchOpenedPorts", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// Relation provides access to methods of a state.Relation through the
// facade.
func (st *State) Relation(tag names.RelationTag) (*Relation, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Relation{
		tag:  tag,
		life: life,
	}, nil
}

// WatchEgressAddressesForRelation returns a watcher that notifies when addresses,
// from which connections will originate to the offering side of the relation, change.
// Each event contains the entire set of addresses which the offering side is required
// to allow for access to the other side of the relation.
func (c *State) WatchEgressAddressesForRelation(relationTag names.RelationTag) (watcher.StringsWatcher, error) {
	args := params.Entities{[]params.Entity{{Tag: relationTag.String()}}}
	var results params.StringsWatchResults
	err := c.facade.FacadeCall("WatchEgressAddressesForRelations", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchIngressAddressesForRelation returns a watcher that notifies when addresses,
// from which connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required
// for ingress into this model from the other (consuming) side of the relation.
func (c *State) WatchIngressAddressesForRelation(relationTag names.RelationTag) (watcher.StringsWatcher, error) {
	args := params.Entities{[]params.Entity{{Tag: relationTag.String()}}}
	var results params.StringsWatchResults
	err := c.facade.FacadeCall("WatchIngressAddressesForRelations", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// ControllerAPIInfoForModels returns the controller api connection details for the specified model.
func (c *State) ControllerAPIInfoForModel(modelUUID string) (*api.Info, error) {
	modelTag := names.NewModelTag(modelUUID)
	args := params.Entities{[]params.Entity{{Tag: modelTag.String()}}}
	var results params.ControllerAPIInfoResults
	err := c.facade.FacadeCall("ControllerAPIInfoForModels", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return &api.Info{
		Addrs:    result.Addresses,
		CACert:   result.CACert,
		ModelTag: modelTag,
	}, nil
}
