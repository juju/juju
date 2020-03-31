// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

// Client allows access to the CAAS operator API endpoint.
type Client struct {
	facade base.FacadeCaller
	*common.APIAddresser
}

// NewClient returns a client used to access the CAAS Operator API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASOperator")
	return &Client{
		facade:       facadeCaller,
		APIAddresser: common.NewAPIAddresser(facadeCaller),
	}
}

func (c *Client) appTag(application string) (names.ApplicationTag, error) {
	if !names.IsValidApplication(application) {
		return names.ApplicationTag{}, errors.NotValidf("application name %q", application)
	}
	return names.NewApplicationTag(application), nil
}

// Model returns the model entity.
func (c *Client) Model() (*model.Model, error) {
	var result params.ModelResult
	err := c.facade.FacadeCall("CurrentModel", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	modelType := model.ModelType(result.Type)
	if modelType == "" {
		modelType = model.IAAS
	}
	return &model.Model{
		Name:      result.Name,
		UUID:      result.UUID,
		ModelType: modelType,
	}, nil
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

func convertNotFound(err error) error {
	if params.IsCodeNotFound(err) {
		return errors.NewNotFound(err, "")
	}
	return err
}

// CharmInfo holds info about the charm for an application.
type CharmInfo struct {
	// URL holds the URL of the charm assigned to the
	// application.
	URL *charm.URL

	// ForceUpgrade indicates whether or not application
	// units should upgrade to the charm even if they
	// are in an error state.
	ForceUpgrade bool

	// SHA256 holds the SHA256 hash of the charm archive.
	SHA256 string

	// CharmModifiedVersion increases when the charm changes in some way.
	CharmModifiedVersion int

	// DeploymentMode is either "operator" or "workload"
	DeploymentMode caas.DeploymentMode
}

// Charm returns information about the charm currently assigned
// to the application, including url, force upgrade and sha etc.
func (c *Client) Charm(application string) (*CharmInfo, error) {
	tag, err := c.appTag(application)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var results params.ApplicationCharmResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	if err := c.facade.FacadeCall("Charm", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Trace(err)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	result := results.Results[0].Result
	curl, err := charm.ParseURL(result.URL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Older controllers don't set DeploymentMode, all charms are workload charms.
	if result.DeploymentMode == "" {
		result.DeploymentMode = string(caas.ModeWorkload)
	}
	return &CharmInfo{
		URL:                  curl,
		ForceUpgrade:         result.ForceUpgrade,
		SHA256:               result.SHA256,
		CharmModifiedVersion: result.CharmModifiedVersion,
		DeploymentMode:       caas.DeploymentMode(result.DeploymentMode),
	}, nil
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

// RemoveUnit removes the specified unit from the current model.
func (c *Client) RemoveUnit(unitName string) error {
	if !names.IsValidUnit(unitName) {
		return errors.NotValidf("unit name %q", unitName)
	}
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewUnitTag(unitName).String()}},
	}
	err := c.facade.FacadeCall("Remove", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
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

// SetVersion sets the tools version associated with
// the given application.
func (c *Client) SetVersion(appName string, v version.Binary) error {
	if !names.IsValidApplication(appName) {
		return errors.NotValidf("application name %q", appName)
	}
	var results params.ErrorResults
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag:   names.NewApplicationTag(appName).String(),
			Tools: &params.Version{v},
		}},
	}
	err := c.facade.FacadeCall("SetTools", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// WatchContainerStart watchs for Unit starts via the CAAS provider.
func (c *Client) WatchContainerStart(application string, containerName string) (watcher.StringsWatcher, error) {
	applicationTag, err := applicationTag(application)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := params.WatchContainerStartArgs{
		Args: []params.WatchContainerStartArg{{
			Entity: params.Entity{
				Tag: applicationTag.String(),
			},
			Container: containerName,
		}},
	}
	var results params.StringsWatchResults
	if err := c.facade.FacadeCall("WatchContainerStart", args, &results); err != nil {
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
