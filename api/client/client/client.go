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
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/storage"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/tools"
)

// Logger is the interface used by the client to log errors.
type Logger interface {
	Errorf(string, ...interface{})
}

// Client represents the client-accessible part of the state.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	conn   api.Connection
	logger Logger
}

// NewClient returns an object that can be used to access client-specific
// functionality.
func NewClient(c api.Connection, logger Logger) *Client {
	frontend, backend := base.NewClientFacade(c, "Client")
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		conn:         c,
		logger:       logger,
	}
}

// Status returns the status of the juju model.
func (c *Client) Status(patterns []string, includeStorage bool) (*params.FullStatus, error) {
	if c.BestAPIVersion() <= 6 {
		return c.statusV6(patterns, includeStorage)
	}
	var result params.FullStatus
	p := params.StatusParams{Patterns: patterns, IncludeStorage: includeStorage}
	if err := c.facade.FacadeCall("FullStatus", p, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) statusV6(patterns []string, includeStorage bool) (*params.FullStatus, error) {
	var result params.FullStatus
	p := params.StatusParams{Patterns: patterns}
	if err := c.facade.FacadeCall("FullStatus", p, &result); err != nil {
		return nil, err
	}
	// Older servers don't fill out model type, but
	// we know a missing type is an "iaas" model.
	if result.Model.Type == "" {
		result.Model.Type = model.IAAS.String()
	}
	if includeStorage {
		storageClient := storage.NewClient(c.conn)
		storageDetails, err := storageClient.ListStorageDetails()
		if err != nil {
			return nil, errors.Annotatef(err, "cannot list storage details")
		}
		result.Storage = storageDetails

		filesystemResult, err := storageClient.ListFilesystems(nil)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot list filesystem details")
		}
		if len(filesystemResult) != 1 {
			return nil, errors.Errorf("cannot list filesystem details: expected one result got %d", len(filesystemResult))
		}
		result.Filesystems = filesystemResult[0].Result

		volumeResult, err := storageClient.ListVolumes(nil)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot list volume details")
		}
		if len(volumeResult) != 1 {
			return nil, errors.Errorf("cannot list volume details: expected one result got %d", len(volumeResult))
		}
		result.Volumes = volumeResult[0].Result
	}
	return &result, nil
}

// StatusHistory retrieves the last <size> results of
// <kind:combined|agent|workload|machine|machineinstance|container|containerinstance> status
// for <name> unit
func (c *Client) StatusHistory(kind status.HistoryKind, tag names.Tag, filter status.StatusHistoryFilter) (status.History, error) {
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
	err := c.facade.FacadeCall("StatusHistory", bulkArgs, &results)
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
			c.logger.Errorf("history returned an unknown status kind %q", h.Kind)
		}
	}
	return history, nil
}

// WatchAll returns an AllWatcher, from which you can request the Next
// collection of Deltas.
func (c *Client) WatchAll() (*api.AllWatcher, error) {
	var info params.AllWatcherId
	if err := c.facade.FacadeCall("WatchAll", nil, &info); err != nil {
		return nil, err
	}
	return api.NewAllWatcher(c.conn, &info.AllWatcherId), nil
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
func (c *Client) SetModelAgentVersion(version version.Number, stream string, ignoreAgentVersions bool) error {
	args := params.SetModelAgentVersion{
		Version:             version,
		AgentStream:         stream,
		IgnoreAgentVersions: ignoreAgentVersions,
	}
	return c.facade.FacadeCall("SetModelAgentVersion", args, nil)
}

// AbortCurrentUpgrade aborts and archives the current upgrade
// synchronisation record, if any.
func (c *Client) AbortCurrentUpgrade() error {
	return c.facade.FacadeCall("AbortCurrentUpgrade", nil, nil)
}

// UploadTools uploads tools at the specified location to the API server over HTTPS.
func (c *Client) UploadTools(r io.ReadSeeker, vers version.Binary, additionalSeries ...string) (tools.List, error) {
	endpoint := fmt.Sprintf("/tools?binaryVersion=%s&series=%s", vers, strings.Join(additionalSeries, ","))
	contentType := "application/x-tar-gz"
	var resp params.ToolsResult
	if err := c.httpPost(r, endpoint, contentType, &resp); err != nil {
		return nil, errors.Trace(err)
	}
	return resp.ToolsList, nil
}

func (c *Client) httpPost(content io.ReadSeeker, endpoint, contentType string, response interface{}) error {
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

	if err := httpClient.Do(c.facade.RawAPICaller().Context(), req, response); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// WatchDebugLog returns a channel of structured Log Messages. Only log entries
// that match the filtering specified in the DebugLogParams are returned.
func (c *Client) WatchDebugLog(args common.DebugLogParams) (<-chan common.LogMessage, error) {
	return common.StreamDebugLog(context.TODO(), c.conn, args)
}
