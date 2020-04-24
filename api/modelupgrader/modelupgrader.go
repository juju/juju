// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

var logger = loggo.GetLogger("juju.api.modelupgrader")

// Client provides methods that the Juju client command uses to interact
// with models stored in the Juju Server.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(caller base.APICaller) *Client {
	return &Client{base.NewFacadeCaller(caller, "ModelUpgrader")}
}

// ModelEnvironVersion returns the current version of the environ corresponding
// to the specified model.
func (c *Client) ModelEnvironVersion(tag names.ModelTag) (int, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	var results params.IntResults
	err := c.facade.FacadeCall("ModelEnvironVersion", &args, &results)
	if err != nil {
		return -1, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return -1, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if err := results.Results[0].Error; err != nil {
		return -1, err
	}
	return results.Results[0].Result, nil
}

// ModelTargetEnvironVersion returns the target version of the environ
// corresponding to the specified model.
func (c *Client) ModelTargetEnvironVersion(tag names.ModelTag) (int, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	var results params.IntResults
	err := c.facade.FacadeCall("ModelTargetEnvironVersion", &args, &results)
	if err != nil {
		return -1, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return -1, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if err := results.Results[0].Error; err != nil {
		return -1, err
	}
	return results.Results[0].Result, nil
}

// SetModelEnvironVersion sets the current version of the environ corresponding
// to the specified model.
func (c *Client) SetModelEnvironVersion(tag names.ModelTag, v int) error {
	args := params.SetModelEnvironVersions{
		Models: []params.SetModelEnvironVersion{{
			ModelTag: tag.String(),
			Version:  v,
		}},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("SetModelEnvironVersion", &args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// WatchModelEnvironVersion starts a NotifyWatcher that notifies the caller upon
// changes to the environ version of the model with the specified tag.
func (c *Client) WatchModelEnvironVersion(tag names.ModelTag) (watcher.NotifyWatcher, error) {
	return common.Watch(c.facade, "WatchModelEnvironVersion", tag)
}

// SetModelStatus sets the status of a model.
func (c *Client) SetModelStatus(tag names.ModelTag, status status.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: tag.String(), Status: status.String(), Info: info, Data: data},
		},
	}
	if err := c.facade.FacadeCall("SetModelStatus", args, &result); err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}
