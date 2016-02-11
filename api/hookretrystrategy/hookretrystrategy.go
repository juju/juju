// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package hookretrystrategy

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

// Client provides access to the hook retry strategy api
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a client for accessing the hook retry strategy api
func NewClient(apiCaller base.APICaller) *Client {
	return &Client{base.NewFacadeCaller(apiCaller, "HookRetryStrategy")}
}

// HookRetryStrategy returns the configuration for the agent specified by the agentTag.
func (c *Client) HookRetryStrategy(agentTag names.Tag) (params.HookRetryStrategy, error) {
	var results params.HookRetryStrategyResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag.String()}},
	}
	err := c.facade.FacadeCall("HookRetryStrategy", args, &results)
	if err != nil {
		return params.HookRetryStrategy{}, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return params.HookRetryStrategy{}, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.HookRetryStrategy{}, errors.Trace(result.Error)
	}
	return *result.Result, nil
}

// WatchHookRetryStrategy returns a notify watcher that looks for changes in the
// hook retry config for the agent specified by agentTag
// Right now only the boolean that decides whether we retry can be modified.
func (c *Client) WatchHookRetryStrategy(agentTag names.Tag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag.String()}},
	}
	err := c.facade.FacadeCall("WatchHookRetryStrategy", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
