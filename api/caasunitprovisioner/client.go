// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/watcher"
)

// Client allows access to the CAAS unit provisioner API endpoint.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS unit provisioner API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASUnitProvisioner")
	return &Client{
		facade: facadeCaller,
	}
}

func applicationTag(application string) (names.ApplicationTag, error) {
	if !names.IsValidApplication(application) {
		return names.ApplicationTag{}, errors.NotValidf("application name %q", application)
	}
	return names.NewApplicationTag(application), nil
}

func unitTag(unit string) (names.UnitTag, error) {
	if !names.IsValidUnit(unit) {
		return names.UnitTag{}, errors.NotValidf("unit name %q", unit)
	}
	return names.NewUnitTag(unit), nil
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

// WatchContainerSpec returns a NotifyWatcher that notifies of
// changes to the container spec of the specified CAAS units in
// the current model.
func (c *Client) WatchContainerSpec(unit string) (watcher.NotifyWatcher, error) {
	unitTag, err := unitTag(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := entities(unitTag)

	var results params.NotifyWatchResults
	if err := c.facade.FacadeCall("WatchContainerSpec", args, &results); err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), results.Results[0])
	return w, nil
}

// ContainerSpec returns the container spec for the specified CAAS
// unit in the current model.
func (c *Client) ContainerSpec(unit string) (string, error) {
	unitTag, err := unitTag(unit)
	if err != nil {
		return "", errors.Trace(err)
	}
	args := entities(unitTag)

	var results params.StringResults
	if err := c.facade.FacadeCall("ContainerSpec", args, &results); err != nil {
		return "", err
	}
	if n := len(results.Results); n != 1 {
		return "", errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return "", maybeNotFound(err)
	}
	return results.Results[0].Result, nil
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

func maybeNotFound(err *params.Error) error {
	if !params.IsCodeNotFound(err) {
		return err
	}
	return errors.NewNotFound(err, "")
}
