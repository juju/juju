// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/series"
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
	for id, m := range result.Machines {
		if m.Series == "" && m.Base.Name != "" {
			mSeries, err := series.GetSeriesFromChannel(m.Base.Name, m.Base.Channel)
			if err != nil {
				return nil, err
			}
			m.Series = mSeries
		}
		result.Machines[id] = m
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

// Resolved clears errors on a unit.
// TODO(juju3) - remove
func (c *Client) Resolved(unit string, retry bool) error {
	p := params.Resolved{
		UnitName: unit,
		Retry:    retry,
	}
	return c.facade.FacadeCall("Resolved", p, nil)
}

// RetryProvisioning updates the provisioning status of a machine allowing the
// provisioner to retry.
// TODO(juju3) - remove
func (c *Client) RetryProvisioning(all bool, machines ...names.MachineTag) ([]params.ErrorResult, error) {
	if all {
		return nil, errors.New(`retry provisioning "all" not supported by this version of Juju`)
	}
	p := params.Entities{}
	p.Entities = make([]params.Entity, len(machines))
	for i, machine := range machines {
		p.Entities[i] = params.Entity{Tag: machine.String()}
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("RetryProvisioning", p, &results)
	return results.Results, err
}

// AddMachines adds new machines with the supplied parameters.
// TODO(juju3) - remove
func (c *Client) AddMachines(machineParams []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	for i, m := range machineParams {
		if m.Base == nil || m.Base.Name != "centos" {
			continue
		}
		if c.BestAPIVersion() >= 6 {
			m.Base.Channel = series.FromLegacyCentosChannel(m.Base.Channel)
		} else {
			m.Base.Channel = series.ToLegacyCentosChannel(m.Base.Channel)
		}
		machineParams[i] = m
	}
	args := params.AddMachines{
		MachineParams: machineParams,
	}
	results := new(params.AddMachinesResults)
	err := c.facade.FacadeCall("AddMachinesV2", args, results)
	return results.Machines, err
}

// ProvisioningScript returns a shell script that, when run,
// provisions a machine agent on the machine executing the script.
// TODO(juju3) - remove
func (c *Client) ProvisioningScript(args params.ProvisioningScriptParams) (script string, err error) {
	var result params.ProvisioningScriptResult
	if err = c.facade.FacadeCall("ProvisioningScript", args, &result); err != nil {
		return "", err
	}
	return result.Script, nil
}

// DestroyMachinesWithParams removes a given set of machines and all associated units.
// TODO(juju3) - remove
func (c *Client) DestroyMachinesWithParams(force, keep bool, machines ...string) error {
	if keep {
		return errors.NotSupportedf("destroy machine with keep-instance=true")
	}
	p := params.DestroyMachines{MachineNames: machines}
	return c.facade.FacadeCall("DestroyMachines", p, nil)
}

// GetModelConstraints returns the constraints for the model.
// TODO(juju3) - remove
func (c *Client) GetModelConstraints() (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.facade.FacadeCall("GetModelConstraints", nil, results)
	return results.Constraints, err
}

// SetModelConstraints specifies the constraints for the model.
// TODO(juju3) - remove
func (c *Client) SetModelConstraints(constraints constraints.Value) error {
	params := params.SetConstraints{
		Constraints: constraints,
	}
	return c.facade.FacadeCall("SetModelConstraints", params, nil)
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
// TODO(juju3) - remove
func (c *Client) ModelUserInfo(modelUUID string) ([]params.ModelUserInfo, error) {
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
	if c.facade.BestAPIVersion() == 1 && agentStream != "" {
		return params.FindToolsResult{}, errors.New(
			"passing agent-stream not supported by the controller")
	}
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

// AddCharm adds the given charm URL (which must include revision) to
// the model, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm() in the
// client-side API.
//
// If the AddCharm API call fails because of an authorization error
// when retrieving the charm from the charm store, an error
// satisfying params.IsCodeUnauthorized will be returned.
// TODO(juju3) - remove
func (c *Client) AddCharm(curl *charm.URL, channel csparams.Channel, force bool) error {
	args := params.AddCharm{
		URL:     curl.String(),
		Channel: string(channel),
		Force:   force,
	}
	if err := c.facade.FacadeCall("AddCharm", args, nil); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// AddCharmWithAuthorization is like AddCharm except it also provides
// the given charmstore macaroon for the juju server to use when
// obtaining the charm from the charm store. The macaroon is
// conventionally obtained from the /delegatable-macaroon endpoint in
// the charm store.
//
// If the AddCharmWithAuthorization API call fails because of an
// authorization error when retrieving the charm from the charm store,
// an error satisfying params.IsCodeUnauthorized will be returned.
// Force is used to overload any validation errors that could occur during
// a deploy
// TODO(juju3) - remove
func (c *Client) AddCharmWithAuthorization(curl *charm.URL, channel csparams.Channel, csMac *macaroon.Macaroon, force bool) error {
	args := params.AddCharmWithAuthorization{
		URL:                curl.String(),
		Channel:            string(channel),
		CharmStoreMacaroon: csMac,
		Force:              force,
	}
	if err := c.facade.FacadeCall("AddCharmWithAuthorization", args, nil); err != nil {
		return errors.Trace(err)
	}
	return nil
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
