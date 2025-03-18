// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/common/cloudspec"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const firewallerFacade = "Firewaller"

// Client provides access to the Firewaller API facade.
type Client struct {
	facade base.FacadeCaller
	*common.ModelConfigWatcher
	*common.ControllerConfigAPI
	*cloudspec.CloudSpecAPI
}

// NewClient creates a new client-side Firewaller API facade.
func NewClient(caller base.APICaller, options ...Option) (*Client, error) {
	modelTag, isModel := caller.ModelTag()
	if !isModel {
		return nil, errors.New("expected model specific API connection")
	}
	facadeCaller := base.NewFacadeCaller(caller, firewallerFacade, options...)
	return &Client{
		facade:              facadeCaller,
		ModelConfigWatcher:  common.NewModelConfigWatcher(facadeCaller),
		ControllerConfigAPI: common.NewControllerConfig(facadeCaller),
		CloudSpecAPI:        cloudspec.NewCloudSpecAPI(facadeCaller, modelTag),
	}, nil
}

// ModelTag returns the current model's tag.
func (c *Client) ModelTag() (names.ModelTag, bool) {
	return c.facade.RawAPICaller().ModelTag()
}

// life requests the life cycle of the given entity from the server.
func (c *Client) life(ctx context.Context, tag names.Tag) (life.Value, error) {
	return common.OneLife(ctx, c.facade, tag)
}

// Unit provides access to methods of a state.Unit through the facade.
func (c *Client) Unit(ctx context.Context, tag names.UnitTag) (*Unit, error) {
	life, err := c.life(ctx, tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:    tag,
		life:   life,
		client: c,
	}, nil
}

// Machine provides access to methods of a state.Machine through the
// facade.
func (c *Client) Machine(ctx context.Context, tag names.MachineTag) (*Machine, error) {
	life, err := c.life(ctx, tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:    tag,
		life:   life,
		client: c,
	}, nil
}

// WatchModelMachines returns a StringsWatcher that notifies of
// changes to the life cycles of the top level machines in the current
// model.
func (c *Client) WatchModelMachines(ctx context.Context) (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall(ctx, "WatchModelMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// ModelFirewallRules returns the firewall rules that this model is
// configured to open
func (c *Client) ModelFirewallRules(ctx context.Context) (firewall.IngressRules, error) {
	var results params.IngressRulesResult
	if err := c.facade.FacadeCall(ctx, "ModelFirewallRules", nil, &results); err != nil {
		return nil, err
	}
	if results.Error != nil {
		return nil, results.Error
	}
	rules := make(firewall.IngressRules, len(results.Rules))
	for i, paramRule := range results.Rules {
		rules[i] = firewall.NewIngressRule(paramRule.PortRange.NetworkPortRange(), paramRule.SourceCIDRs...)
	}
	return rules, nil
}

// WatchModelFirewallRules returns a NotifyWatcher that notifies of
// potential changes to a model's configured firewall rules
func (c *Client) WatchModelFirewallRules(ctx context.Context) (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := c.facade.FacadeCall(ctx, "WatchModelFirewallRules", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// Relation provides access to methods of a state.Relation through the
// facade.
func (c *Client) Relation(ctx context.Context, tag names.RelationTag) (*Relation, error) {
	life, err := c.life(ctx, tag)
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
func (c *Client) WatchEgressAddressesForRelation(ctx context.Context, relationTag names.RelationTag) (watcher.StringsWatcher, error) {
	args := params.Entities{Entities: []params.Entity{{Tag: relationTag.String()}}}
	var results params.StringsWatchResults
	err := c.facade.FacadeCall(ctx, "WatchEgressAddressesForRelations", args, &results)
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
func (c *Client) WatchIngressAddressesForRelation(ctx context.Context, relationTag names.RelationTag) (watcher.StringsWatcher, error) {
	// todo(gfouillet): re-enable this watcher call whenever CMR will be fully
	//   implemented in the new domain. The facade implementation return a
	//   not implemented exception, so it is not necessary to try to fetch it.
	return common.NewDisabledWatcher(), nil
}

// ControllerAPIInfoForModels returns the controller api connection details for the specified model.
func (c *Client) ControllerAPIInfoForModel(ctx context.Context, modelUUID string) (*api.Info, error) {
	modelTag := names.NewModelTag(modelUUID)
	args := params.Entities{Entities: []params.Entity{{Tag: modelTag.String()}}}
	var results params.ControllerAPIInfoResults
	err := c.facade.FacadeCall(ctx, "ControllerAPIInfoForModels", args, &results)
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
func (c *Client) MacaroonForRelation(ctx context.Context, relationKey string) (*macaroon.Macaroon, error) {
	relationTag := names.NewRelationTag(relationKey)
	args := params.Entities{Entities: []params.Entity{{Tag: relationTag.String()}}}
	var results params.MacaroonResults
	err := c.facade.FacadeCall(ctx, "MacaroonForRelations", args, &results)
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
func (c *Client) SetRelationStatus(ctx context.Context, relationKey string, status relation.Status, message string) error {
	relationTag := names.NewRelationTag(relationKey)
	args := params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: relationTag.String(), Status: status.String(), Info: message},
	}}

	var results params.ErrorResults
	err := c.facade.FacadeCall(ctx, "SetRelationsStatus", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// AllSpaceInfos returns the details about the known spaces and their
// associated subnets.
func (c *Client) AllSpaceInfos(ctx context.Context) (network.SpaceInfos, error) {
	var result params.SpaceInfos
	err := c.facade.FacadeCall(ctx, "SpaceInfos", params.SpaceInfosParams{}, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return params.ToNetworkSpaceInfos(result), nil
}

// WatchSubnets returns a StringsWatcher that notifies of changes to the model
// subnets.
func (c *Client) WatchSubnets(ctx context.Context) (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall(ctx, "WatchSubnets", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
