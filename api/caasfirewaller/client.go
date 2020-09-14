// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/juju/api/common"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	charmscommon "github.com/juju/juju/api/common/charms"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
)

// Client allows access to the CAAS firewaller API endpoint.
type Client struct {
	facade base.FacadeCaller
}

// NewClientLegacy returns a client used to access the CAAS unit provisioner API.
func NewClientLegacy(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASFirewaller")
	return &Client{
		facade: facadeCaller,
	}
}

// ClientEmbedded allows access to the CAAS firewaller API endpoint for embedded applications.
type ClientEmbedded struct {
	*Client
	*charmscommon.CharmsClient
}

// NewClientEmbedded returns a client used to access the CAAS unit provisioner API.
func NewClientEmbedded(caller base.APICaller) *ClientEmbedded {
	// TODO: add OpenedPorts and ClosedPorts API for caasfirewallerembedded worker to fetch port mapping changes!!!!
	facadeCaller := base.NewFacadeCaller(caller, "CAASFirewallerEmbedded")
	charmsClient := charmscommon.NewCharmsClient(facadeCaller)
	return &ClientEmbedded{
		Client:       NewClientLegacy(caller),
		CharmsClient: charmsClient,
	}
}

// ModelTag returns the current model's tag.
func (c *ClientEmbedded) ModelTag() (names.ModelTag, bool) {
	return c.facade.RawAPICaller().ModelTag()
}

// WatchOpenedPorts returns a StringsWatcher that notifies of
// changes to the opened ports for the current model.
func (c *ClientEmbedded) WatchOpenedPorts() (watcher.StringsWatcher, error) {
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

// ApplicationCharmURL finds the CharmURL for an application.
func (c *ClientEmbedded) ApplicationCharmURL(appName string) (*charm.URL, error) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewApplicationTag(appName).String(),
	}}}
	var result params.StringResults
	if err := c.facade.FacadeCall("ApplicationCharmURLs", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(result.Results))
	}
	res := result.Results[0]
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "unable to fetch charm url for %s", appName)
	}
	url, err := charm.ParseURL(res.Result)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid charm url %q", res.Result)
	}
	return url, nil
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
