// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	charmscommon "github.com/juju/juju/api/common/charms"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Client allows access to the CAAS firewaller API endpoint.
type Client struct {
	facade base.FacadeCaller
	*charmscommon.CharmInfoClient
	*charmscommon.ApplicationCharmInfoClient
}

// NewClientLegacy returns a client used to access the CAAS unit provisioner API.
func NewClientLegacy(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASFirewaller")
	charmInfoClient := charmscommon.NewCharmInfoClient(facadeCaller)
	appCharmInfoClient := charmscommon.NewApplicationCharmInfoClient(facadeCaller)
	return &Client{
		facade:                     facadeCaller,
		CharmInfoClient:            charmInfoClient,
		ApplicationCharmInfoClient: appCharmInfoClient,
	}
}

// ClientSidecar allows access to the CAAS firewaller API endpoint for sidecar applications.
type ClientSidecar struct {
	*Client
}

// NewClientSidecar returns a client used to access the CAAS unit provisioner API.
func NewClientSidecar(caller base.APICaller) *ClientSidecar {
	// TODO(sidecar): add OpenedPorts and ClosedPorts API for caasfirewallersidecar worker to fetch port mapping changes.
	// TODO(juju3): rename to CAASFirewallerSidecar
	facadeCaller := base.NewFacadeCaller(caller, "CAASFirewallerEmbedded")
	charmInfoClient := charmscommon.NewCharmInfoClient(facadeCaller)
	appCharmInfoClient := charmscommon.NewApplicationCharmInfoClient(facadeCaller)
	return &ClientSidecar{
		Client: &Client{
			facade:                     facadeCaller,
			CharmInfoClient:            charmInfoClient,
			ApplicationCharmInfoClient: appCharmInfoClient,
		},
	}
}

// modelTag returns the current model's tag.
func (c *ClientSidecar) modelTag() (names.ModelTag, bool) {
	return c.facade.RawAPICaller().ModelTag()
}

// WatchOpenedPorts returns a StringsWatcher that notifies of
// changes to the opened ports for the current model.
func (c *ClientSidecar) WatchOpenedPorts() (watcher.StringsWatcher, error) {
	modelTag, ok := c.modelTag()
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

func applicationTag(application string) (names.ApplicationTag, error) {
	if !names.IsValidApplication(application) {
		return names.ApplicationTag{}, errors.NotValidf("application name %q", application)
	}
	return names.NewApplicationTag(application), nil
}

func entities(tags ...names.Tag) params.Entities {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag.String()
	}
	return entities
}

// WatchApplications returns a StringsWatcher that notifies of
// changes to the lifecycles of CAAS applications in the current model.
func (c *Client) WatchApplications() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	if err := c.facade.FacadeCall("WatchApplications", nil, &result); err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchApplication returns a NotifyWatcher that notifies of
// changes to the application in the current model.
func (c *Client) WatchApplication(appName string) (watcher.NotifyWatcher, error) {
	appTag, err := applicationTag(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return common.Watch(c.facade, "Watch", appTag)
}

// Life returns the lifecycle state for the specified CAAS application
// in the current model.
func (c *Client) Life(appName string) (life.Value, error) {
	appTag, err := applicationTag(appName)
	if err != nil {
		return "", errors.Trace(err)
	}
	args := entities(appTag)

	var results params.LifeResults
	if err := c.facade.FacadeCall("Life", args, &results); err != nil {
		return "", err
	}
	if n := len(results.Results); n != 1 {
		return "", errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return "", maybeNotFound(err)
	}
	return results.Results[0].Life, nil
}

// ApplicationConfig returns the config for the specified application.
func (c *Client) ApplicationConfig(applicationName string) (application.ConfigAttributes, error) {
	var results params.ApplicationGetConfigResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewApplicationTag(applicationName).String()}},
	}
	err := c.facade.FacadeCall("ApplicationsConfig", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(args.Entities) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(args.Entities), len(results.Results))
	}
	return application.ConfigAttributes(results.Results[0].Config), nil
}

// IsExposed returns whether the specified CAAS application
// in the current model is exposed.
func (c *Client) IsExposed(appName string) (bool, error) {
	appTag, err := applicationTag(appName)
	if err != nil {
		return false, errors.Trace(err)
	}
	args := entities(appTag)

	var results params.BoolResults
	if err := c.facade.FacadeCall("IsExposed", args, &results); err != nil {
		return false, err
	}
	if n := len(results.Results); n != 1 {
		return false, errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return false, maybeNotFound(err)
	}
	return results.Results[0].Result, nil
}

// maybeNotFound returns an error satisfying errors.IsNotFound
// if the supplied error has a CodeNotFound error.
func maybeNotFound(err *params.Error) error {
	if err == nil || !params.IsCodeNotFound(err) {
		return err
	}
	return errors.NewNotFound(err, "")
}
