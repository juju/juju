// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/common/cloudspec"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
)

const firewallerFacade = "Firewaller"

// Client provides access to the Firewaller API facade.
type Client struct {
	facade base.FacadeCaller
	*common.ModelWatcher
	*cloudspec.CloudSpecAPI
}

// NewClient creates a new client-side Firewaller API facade.
func NewClient(caller base.APICaller) (*Client, error) {
	modelTag, isModel := caller.ModelTag()
	if !isModel {
		return nil, errors.New("expected model specific API connection")
	}
	facadeCaller := base.NewFacadeCaller(caller, firewallerFacade)
	return &Client{
		facade:       facadeCaller,
		ModelWatcher: common.NewModelWatcher(facadeCaller),
		CloudSpecAPI: cloudspec.NewCloudSpecAPI(facadeCaller, modelTag),
	}, nil
}

// BestAPIVersion returns the API version that we were able to
// determine is supported by both the client and the API Server.
func (c *Client) BestAPIVersion() int {
	return c.facade.BestAPIVersion()
}

// ModelTag returns the current model's tag.
func (c *Client) ModelTag() (names.ModelTag, bool) {
	return c.facade.RawAPICaller().ModelTag()
}

// life requests the life cycle of the given entity from the server.
func (c *Client) life(tag names.Tag) (life.Value, error) {
	return common.OneLife(c.facade, tag)
}

// Unit provides access to methods of a state.Unit through the facade.
func (c *Client) Unit(tag names.UnitTag) (*Unit, error) {
	life, err := c.life(tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:  tag,
		life: life,
		st:   c,
	}, nil
}

// Machine provides access to methods of a state.Machine through the
// facade.
func (c *Client) Machine(tag names.MachineTag) (*Machine, error) {
	life, err := c.life(tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:  tag,
		life: life,
		st:   c,
	}, nil
}

// WatchModelMachines returns a StringsWatcher that notifies of
// changes to the life cycles of the top level machines in the current
// model.
func (c *Client) WatchModelMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall("WatchModelMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchOpenedPorts returns a StringsWatcher that notifies of
// changes to the opened ports for the current model.
func (c *Client) WatchOpenedPorts() (watcher.StringsWatcher, error) {
	modelTag, ok := c.ModelTag()
	if !ok {
		return nil, errors.New("API connection is controller-only (should never happen)")
	}
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: modelTag.String()}},
	}
	if err := c.facade.FacadeCall("WatchOpenedPorts", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// Relation provides access to methods of a state.Relation through the
// facade.
func (c *Client) Relation(tag names.RelationTag) (*Relation, error) {
	life, err := c.life(tag)
	if err != nil {
		return nil, err
	}
	return &Relation{
		tag:  tag,
		life: life,
	}, nil
}

// WatchEgressAddressesForRelation returns a watcher that notifies when addresses,
// from which connections will originate to the provider side of the relation, change.
// Each event contains the entire set of addresses which the provider side is required
// to allow for access from the other side of the relation.
func (c *Client) WatchEgressAddressesForRelation(relationTag names.RelationTag) (watcher.StringsWatcher, error) {
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
// for ingress into this model from the other requirer side of the relation.
func (c *Client) WatchIngressAddressesForRelation(relationTag names.RelationTag) (watcher.StringsWatcher, error) {
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
func (c *Client) ControllerAPIInfoForModel(modelUUID string) (*api.Info, error) {
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

// MacaroonForRelation returns the macaroon to use when publishing changes for the relation.
func (c *Client) MacaroonForRelation(relationKey string) (*macaroon.Macaroon, error) {
	relationTag := names.NewRelationTag(relationKey)
	args := params.Entities{[]params.Entity{{Tag: relationTag.String()}}}
	var results params.MacaroonResults
	err := c.facade.FacadeCall("MacaroonForRelations", args, &results)
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
	return result.Result, nil
}

// SetRelationStatus sets the status for a given relation.
func (c *Client) SetRelationStatus(relationKey string, status relation.Status, message string) error {
	relationTag := names.NewRelationTag(relationKey)
	args := params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: relationTag.String(), Status: status.String(), Info: message},
	}}

	var results params.ErrorResults
	err := c.facade.FacadeCall("SetRelationsStatus", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// FirewallRules returns the firewall rules for the specified known service names.
func (c *Client) FirewallRules(knownServices ...string) ([]params.FirewallRule, error) {
	args := params.KnownServiceArgs{
		KnownServices: make([]params.KnownServiceValue, len(knownServices)),
	}
	for i, s := range knownServices {
		args.KnownServices[i] = params.KnownServiceValue(s)
	}

	var results params.ListFirewallRulesResults
	err := c.facade.FacadeCall("FirewallRules", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results.Rules, nil
}
