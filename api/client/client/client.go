// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client represents the client-accessible part of the state.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	conn   api.Connection
	logger logger.Logger
}

// NewClient returns an object that can be used to access client-specific
// functionality.
func NewClient(c api.Connection, logger logger.Logger, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(c, "Client", options...)
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		conn:         c,
		logger:       logger,
	}
}

// StatusArgs holds the options for a call to Status.
type StatusArgs struct {
	// Patterns is used to filter the status response.
	Patterns []string

	// IncludeStorage can be set to true to return storage in the response.
	IncludeStorage bool
}

// Status returns the status of the juju model.
func (c *Client) Status(ctx context.Context, args *StatusArgs) (*params.FullStatus, error) {
	if args == nil {
		args = &StatusArgs{}
	}
	var result params.FullStatus
	p := params.StatusParams{Patterns: args.Patterns, IncludeStorage: args.IncludeStorage}
	if err := c.facade.FacadeCall(ctx, "FullStatus", p, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// StatusHistory retrieves the last <size> results of
// <kind:combined|agent|workload|machine|machineinstance|container|containerinstance> status
// for <name> unit
func (c *Client) StatusHistory(ctx context.Context, kind status.HistoryKind, tag names.Tag, filter status.StatusHistoryFilter) (status.History, error) {
	var results params.StatusHistoryResults
	args := params.StatusHistoryRequest{
		Kind: string(kind),
		Filter: params.StatusHistoryFilter{
			Size:    filter.Size,
			Date:    filter.FromDate,
			Delta:   filter.Delta,
			Exclude: filter.Exclude.Values(),
		},
		Tag: tag.String(),
	}
	bulkArgs := params.StatusHistoryRequests{Requests: []params.StatusHistoryRequest{args}}
	err := c.facade.FacadeCall(ctx, "StatusHistory", bulkArgs, &results)
	if err != nil {
		return status.History{}, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return status.History{}, errors.Errorf("expected 1 result got %d", len(results.Results))
	}
	if results.Results[0].Error != nil {
		return status.History{}, errors.Annotatef(results.Results[0].Error, "while processing the request")
	}
	history := make(status.History, len(results.Results[0].History.Statuses))
	if results.Results[0].History.Error != nil {
		return status.History{}, results.Results[0].History.Error
	}
	for i, h := range results.Results[0].History.Statuses {
		history[i] = status.DetailedStatus{
			Status: status.Status(h.Status),
			Info:   h.Info,
			Data:   h.Data,
			Since:  h.Since,
			Kind:   status.HistoryKind(h.Kind),
		}
		// TODO(perrito666) https://launchpad.net/bugs/1577589
		if !history[i].Kind.Valid() {
			c.logger.Errorf(context.TODO(), "history returned an unknown status kind %q", h.Kind)
		}
	}
	return history, nil
}

// Close closes the Client's underlying State connection
// Client is unique among the api.State facades in closing its own State
// connection, but it is conventional to use a Client object without any access
// to its underlying state connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// SetModelAgentVersion sets the model agent-version setting
// to the given value.
func (c *Client) SetModelAgentVersion(ctx context.Context, version semversion.Number, stream string, ignoreAgentVersions bool) error {
	args := params.SetModelAgentVersion{
		Version:             version,
		AgentStream:         stream,
		IgnoreAgentVersions: ignoreAgentVersions,
	}
	return c.facade.FacadeCall(ctx, "SetModelAgentVersion", args, nil)
}

// UploadTools uploads tools at the specified location to the API server over HTTPS.
func (c *Client) UploadTools(ctx context.Context, r io.ReadSeeker, vers semversion.Binary, additionalSeries ...string) (tools.List, error) {
	endpoint := fmt.Sprintf("/tools?binaryVersion=%s&series=%s", vers, strings.Join(additionalSeries, ","))
	contentType := "application/x-tar-gz"
	var resp params.ToolsResult
	if err := c.httpPost(ctx, r, endpoint, contentType, &resp); err != nil {
		return nil, errors.Trace(err)
	}
	return resp.ToolsList, nil
}

func (c *Client) httpPost(ctx context.Context, content io.ReadSeeker, endpoint, contentType string, response interface{}) error {
	req, err := http.NewRequest("POST", endpoint, content)
	if err != nil {
		return errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", contentType)

	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := c.conn.HTTPClient()
	if err != nil {
		return errors.Trace(err)
	}

	if err := httpClient.Do(ctx, req, response); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// WatchDebugLog returns a channel of structured Log Messages. Only log entries
// that match the filtering specified in the DebugLogParams are returned.
func (c *Client) WatchDebugLog(ctx context.Context, args common.DebugLogParams) (<-chan common.LogMessage, error) {
	return common.StreamDebugLog(ctx, c.conn, args)
}
