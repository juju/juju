// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"
	"github.com/lxc/lxd/shared/logger"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/downloader"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/tools"
)

// Client represents the client-accessible part of the state.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	conn   api.Connection
}

// NewClient returns an object that can be used to access client-specific
// functionality.
func NewClient(c api.Connection) *Client {
	frontend, backend := base.NewClientFacade(c, "Client")
	return &Client{ClientFacade: frontend, facade: backend, conn: c}
}

// Status returns the status of the juju model.
func (c *Client) Status(patterns []string) (*params.FullStatus, error) {
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
			logger.Errorf("history returned an unknown status kind %q", h.Kind)
		}
	}
	return history, nil
}

// Resolved clears errors on a unit.
func (c *Client) Resolved(unit string, retry bool) error {
	p := params.Resolved{
		UnitName: unit,
		Retry:    retry,
	}
	return c.facade.FacadeCall("Resolved", p, nil)
}

// ModelUUID returns the model UUID from the client connection
// and reports whether it is valued.
func (c *Client) ModelUUID() (string, bool) {
	tag, ok := c.conn.ModelTag()
	if !ok {
		return "", false
	}
	return tag.Id(), true
}

// ModelUserInfo returns information on all users in the model.
func (c *Client) ModelUserInfo() ([]params.ModelUserInfo, error) {
	var results params.ModelUserInfoResults
	err := c.facade.FacadeCall("ModelUserInfo", nil, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}

	info := []params.ModelUserInfo{}
	for i, result := range results.Results {
		if result.Result == nil {
			return nil, errors.Errorf("unexpected nil result at position %d", i)
		}
		info = append(info, *result.Result)
	}
	return info, nil
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

// FindTools returns a List containing all tools matching the specified parameters.
func (c *Client) FindTools(majorVersion, minorVersion int, osType, arch, agentStream string) (result params.FindToolsResult, err error) {
	args := params.FindToolsParams{
		MajorVersion: majorVersion,
		MinorVersion: minorVersion,
		Arch:         arch,
		OSType:       osType,
		AgentStream:  agentStream,
	}
	err = c.facade.FacadeCall("FindTools", args, &result)
	if err != nil {
		return result, errors.Trace(err)
	}
	if result.Error != nil {
		err = result.Error
		// We need to deal with older controllers.
		if strings.HasSuffix(result.Error.Message, "not valid") {
			err = errors.NewNotValid(result.Error, "finding tools")
		}
		if params.IsCodeNotFound(err) {
			err = errors.NewNotFound(err, "finding tools")
		}
	}
	return result, err
}

// OpenCharm streams out the identified charm from the controller via
// the API.
func (c *Client) OpenCharm(curl *charm.URL) (io.ReadCloser, error) {
	return c.OpenURI(openCharmArgs(curl))
}

// OpenCharm streams out the identified charm from the controller via
// the API.
func OpenCharm(apiCaller base.APICaller, curl *charm.URL) (io.ReadCloser, error) {
	uri, query := openCharmArgs(curl)
	return openURI(apiCaller, uri, query)
}

func openCharmArgs(curl *charm.URL) (string, url.Values) {
	query := make(url.Values)
	query.Add("url", curl.String())
	query.Add("file", "*")
	return "/charms", query
}

// OpenURI performs a GET on a Juju HTTP endpoint returning the
func (c *Client) OpenURI(uri string, query url.Values) (io.ReadCloser, error) {
	return openURI(c.conn, uri, query)
}

func openURI(apiCaller base.APICaller, uri string, query url.Values) (io.ReadCloser, error) {
	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := apiCaller.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	blob, err := openBlob(apiCaller.Context(), httpClient, uri, query)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return blob, nil
}

// NewCharmDownloader returns a new charm downloader that wraps the
// provided API caller.
func NewCharmDownloader(apiCaller base.APICaller) *downloader.Downloader {
	dlr := &downloader.Downloader{
		OpenBlob: func(url *url.URL) (io.ReadCloser, error) {
			curl, err := charm.ParseURL(url.String())
			if err != nil {
				return nil, errors.Annotate(err, "did not receive a valid charm URL")
			}
			reader, err := OpenCharm(apiCaller, curl)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return reader, nil
		},
	}
	return dlr
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

// APIHostPorts returns a slice of network.MachineHostPort for each API server.
func (c *Client) APIHostPorts() ([]network.MachineHostPorts, error) {
	var result params.APIHostPortsResult
	if err := c.facade.FacadeCall("APIHostPorts", nil, &result); err != nil {
		return nil, err
	}
	return result.MachineHostsPorts(), nil
}

// AgentVersion reports the version number of the api server.
func (c *Client) AgentVersion() (version.Number, error) {
	var result params.AgentVersionResult
	if err := c.facade.FacadeCall("AgentVersion", nil, &result); err != nil {
		return version.Number{}, err
	}
	return result.Version, nil
}

// WatchDebugLog returns a channel of structured Log Messages. Only log entries
// that match the filtering specified in the DebugLogParams are returned.
func (c *Client) WatchDebugLog(args common.DebugLogParams) (<-chan common.LogMessage, error) {
	return common.StreamDebugLog(context.TODO(), c.conn, args)
}
