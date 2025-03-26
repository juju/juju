// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"
	"fmt"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to an upgrader worker's view of the state.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a version of the api client that provides functionality
// required by the upgrader worker.
func NewClient(caller base.APICaller, options ...Option) *Client {
	return &Client{base.NewFacadeCaller(caller, "Upgrader", options...)}
}

// SetVersion sets the tools version associated with the entity with
// the given tag, which must be the tag of the entity that the
// upgrader is running on behalf of.
func (st *Client) SetVersion(ctx context.Context, tag string, v version.Binary) error {
	var results params.ErrorResults
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag:   tag,
			Tools: &params.Version{Version: v},
		}},
	}
	err := st.facade.FacadeCall(ctx, "SetTools", args, &results)
	if err != nil {
		return err
	}
	return results.OneError()
}

func (st *Client) DesiredVersion(ctx context.Context, tag string) (version.Number, error) {
	var results params.VersionResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.facade.FacadeCall(ctx, "DesiredVersion", args, &results)
	if err != nil {
		return version.Number{}, err
	}
	if len(results.Results) != 1 {
		return version.Number{}, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return version.Number{}, err
	}
	if result.Version == nil {
		return version.Number{}, fmt.Errorf("received no error, but got a nil Version")
	}
	return *result.Version, nil
}

// Tools returns the agent tools that should run on the given entity,
// along with a flag whether to disable SSL hostname verification.
func (st *Client) Tools(ctx context.Context, tag string) (tools.List, error) {
	var results params.ToolsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.facade.FacadeCall(ctx, "Tools", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, err
	}
	return result.ToolsList, nil
}

func (st *Client) WatchAPIVersion(ctx context.Context, agentTag string) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag}},
	}
	err := st.facade.FacadeCall(ctx, "WatchAPIVersion", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}
