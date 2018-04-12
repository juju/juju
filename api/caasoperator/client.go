// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
)

// Client allows access to the CAAS operator API endpoint.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS Operator API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASOperator")
	return &Client{
		facade: facadeCaller,
	}
}

func (c *Client) appTag(application string) (names.ApplicationTag, error) {
	if !names.IsValidApplication(application) {
		return names.ApplicationTag{}, errors.NotValidf("application name %q", application)
	}
	return names.NewApplicationTag(application), nil
}

// ModelName returns the name of the model.
func (c *Client) ModelName() (string, error) {
	var result params.StringResult
	err := c.facade.FacadeCall("ModelName", nil, &result)
	if err != nil {
		return "", errors.Trace(err)
	}
	if err := result.Error; err != nil {
		return "", err
	}
	return result.Result, nil
}

// SetStatus sets the status of the specified application.
func (c *Client) SetStatus(
	application string,
	status status.Status,
	info string,
	data map[string]interface{},
) error {
	tag, err := c.appTag(application)
	if err != nil {
		return errors.Trace(err)
	}
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    tag.String(),
			Status: status.String(),
			Info:   info,
			Data:   data,
		}},
	}
	if err := c.facade.FacadeCall("SetStatus", args, &result); err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}

// Charm returns information about the charm currently assigned
// to the application, including url, force upgrade and sha etc.
func (c *Client) Charm(application string) (_ *charm.URL, forceUpgrade bool, sha256 string, vers int, _ error) {
	tag, err := c.appTag(application)
	if err != nil {
		return nil, false, "", 0, errors.Trace(err)
	}
	var results params.ApplicationCharmResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	if err := c.facade.FacadeCall("Charm", args, &results); err != nil {
		return nil, false, "", 0, errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		return nil, false, "", 0, errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, false, "", 0, errors.Trace(err)
	}
	result := results.Results[0].Result
	curl, err := charm.ParseURL(result.URL)
	if err != nil {
		return nil, false, "", 0, errors.Trace(err)
	}
	return curl, result.ForceUpgrade, result.SHA256, result.CharmModifiedVersion, nil
}

// SetPodSpec sets the pod spec of the specified application.
func (c *Client) SetPodSpec(appName string, spec string) error {
	tag, err := applicationTag(appName)
	if err != nil {
		return errors.Trace(err)
	}
	var result params.ErrorResults
	args := params.SetPodSpecParams{
		Specs: []params.EntityString{{
			Tag:   tag.String(),
			Value: spec,
		}},
	}
	if err := c.facade.FacadeCall("SetPodSpec", args, &result); err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}

func applicationTag(application string) (names.ApplicationTag, error) {
	if !names.IsValidApplication(application) {
		return names.ApplicationTag{}, errors.NotValidf("application name %q", application)
	}
	return names.NewApplicationTag(application), nil
}

// Watch returns a watcher for observing changes to an application.
func (c *Client) Watch(application string) (watcher.NotifyWatcher, error) {
	tag, err := c.appTag(application)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return common.Watch(c.facade, "Watch", tag)
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

// WatchUnits returns a StringsWatcher that notifies of
// changes to the lifecycles of units of the specified
// CAAS application in the current model.
func (c *Client) WatchUnits(application string) (watcher.StringsWatcher, error) {
	applicationTag, err := applicationTag(application)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := entities(applicationTag)

	var results params.StringsWatchResults
	if err := c.facade.FacadeCall("WatchUnits", args, &results); err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), results.Results[0])
	return w, nil
}

// Life returns the lifecycle state for the specified CAAS application
// or unit in the current model.
func (c *Client) Life(entityName string) (life.Value, error) {
	var tag names.Tag
	switch {
	case names.IsValidApplication(entityName):
		tag = names.NewApplicationTag(entityName)
	case names.IsValidUnit(entityName):
		tag = names.NewUnitTag(entityName)
	default:
		return "", errors.NotValidf("application or unit name %q", entityName)
	}
	args := entities(tag)

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
	return life.Value(results.Results[0].Life), nil
}

// maybeNotFound returns an error satisfying errors.IsNotFound
// if the supplied error has a CodeNotFound error.
func maybeNotFound(err *params.Error) error {
	if err == nil || !params.IsCodeNotFound(err) {
		return err
	}
	return errors.NewNotFound(err, "")
}
