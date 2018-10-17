// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"
	csparams "gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/downloader"
	"github.com/juju/juju/network"
	"github.com/juju/juju/tools"
)

// Client represents the client-accessible part of the state.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	st     *state
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
			Status:  status.Status(h.Status),
			Info:    h.Info,
			Data:    h.Data,
			Since:   h.Since,
			Kind:    status.HistoryKind(h.Kind),
			Version: h.Version,
			// TODO(perrito666) make sure these are still used.
			Life: h.Life,
			Err:  h.Err,
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

// RetryProvisioning updates the provisioning status of a machine allowing the
// provisioner to retry.
func (c *Client) RetryProvisioning(machines ...names.MachineTag) ([]params.ErrorResult, error) {
	p := params.Entities{}
	p.Entities = make([]params.Entity, len(machines))
	for i, machine := range machines {
		p.Entities[i] = params.Entity{Tag: machine.String()}
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("RetryProvisioning", p, &results)
	return results.Results, err
}

// PublicAddress returns the public address of the specified
// machine or unit. For a machine, target is an id not a tag.
func (c *Client) PublicAddress(target string) (string, error) {
	var results params.PublicAddressResults
	p := params.PublicAddress{Target: target}
	err := c.facade.FacadeCall("PublicAddress", p, &results)
	return results.PublicAddress, err
}

// PrivateAddress returns the private address of the specified
// machine or unit.
func (c *Client) PrivateAddress(target string) (string, error) {
	var results params.PrivateAddressResults
	p := params.PrivateAddress{Target: target}
	err := c.facade.FacadeCall("PrivateAddress", p, &results)
	return results.PrivateAddress, err
}

// AddMachines adds new machines with the supplied parameters.
func (c *Client) AddMachines(machineParams []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	args := params.AddMachines{
		MachineParams: machineParams,
	}
	results := new(params.AddMachinesResults)
	err := c.facade.FacadeCall("AddMachinesV2", args, results)
	return results.Machines, err
}

// ProvisioningScript returns a shell script that, when run,
// provisions a machine agent on the machine executing the script.
func (c *Client) ProvisioningScript(args params.ProvisioningScriptParams) (script string, err error) {
	var result params.ProvisioningScriptResult
	if err = c.facade.FacadeCall("ProvisioningScript", args, &result); err != nil {
		return "", err
	}
	return result.Script, nil
}

// DestroyMachines removes a given set of machines.
//
// NOTE(axw) this exists only for backwards compatibility, when MachineManager
// facade v3 is not available. The MachineManager.DestroyMachines method should
// be preferred.
//
// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (c *Client) DestroyMachines(machines ...string) error {
	params := params.DestroyMachines{MachineNames: machines}
	return c.facade.FacadeCall("DestroyMachines", params, nil)
}

// ForceDestroyMachines removes a given set of machines and all associated units.
//
// NOTE(axw) this exists only for backwards compatibility, when MachineManager
// facade v3 is not available. The MachineManager.ForceDestroyMachines method
// should be preferred.
//
// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (c *Client) ForceDestroyMachines(machines ...string) error {
	params := params.DestroyMachines{Force: true, MachineNames: machines}
	return c.facade.FacadeCall("DestroyMachines", params, nil)
}

// DestroyMachinesWithParams removes a given set of machines and all associated units.
//
// NOTE(wallyworld) this exists only for backwards compatibility, when MachineManager
// facade v4 is not available. The MachineManager.DestroyMachinesWithParams method
// should be preferred.
//
// TODO(wallyworld) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (c *Client) DestroyMachinesWithParams(force, keep bool, machines ...string) error {
	if keep {
		return errors.NotSupportedf("destroy machine with keep-instance=true")
	}
	return c.DestroyMachines(machines...)
}

// GetModelConstraints returns the constraints for the model.
func (c *Client) GetModelConstraints() (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.facade.FacadeCall("GetModelConstraints", nil, results)
	return results.Constraints, err
}

// SetModelConstraints specifies the constraints for the model.
func (c *Client) SetModelConstraints(constraints constraints.Value) error {
	params := params.SetConstraints{
		Constraints: constraints,
	}
	return c.facade.FacadeCall("SetModelConstraints", params, nil)
}

// ModelUUID returns the model UUID from the client connection
// and reports whether it is valued.
func (c *Client) ModelUUID() (string, bool) {
	tag, ok := c.st.ModelTag()
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
func (c *Client) WatchAll() (*AllWatcher, error) {
	var info params.AllWatcherId
	if err := c.facade.FacadeCall("WatchAll", nil, &info); err != nil {
		return nil, err
	}
	return NewAllWatcher(c.st, &info.AllWatcherId), nil
}

// Close closes the Client's underlying State connection
// Client is unique among the api.State facades in closing its own State
// connection, but it is conventional to use a Client object without any access
// to its underlying state connection.
func (c *Client) Close() error {
	return c.st.Close()
}

// SetModelAgentVersion sets the model agent-version setting
// to the given value.
func (c *Client) SetModelAgentVersion(version version.Number, ignoreAgentVersions bool) error {
	args := params.SetModelAgentVersion{Version: version, IgnoreAgentVersions: ignoreAgentVersions}
	return c.facade.FacadeCall("SetModelAgentVersion", args, nil)
}

// AbortCurrentUpgrade aborts and archives the current upgrade
// synchronisation record, if any.
func (c *Client) AbortCurrentUpgrade() error {
	return c.facade.FacadeCall("AbortCurrentUpgrade", nil, nil)
}

// FindTools returns a List containing all tools matching the specified parameters.
func (c *Client) FindTools(majorVersion, minorVersion int, series, arch, agentStream string) (result params.FindToolsResult, err error) {
	if c.facade.BestAPIVersion() == 1 && agentStream != "" {
		return params.FindToolsResult{}, errors.New(
			"passing agent-stream not supported by the controller")
	}
	args := params.FindToolsParams{
		MajorVersion: majorVersion,
		MinorVersion: minorVersion,
		Arch:         arch,
		Series:       series,
		AgentStream:  agentStream,
	}
	err = c.facade.FacadeCall("FindTools", args, &result)
	return result, err
}

// AddLocalCharm prepares the given charm with a local: schema in its
// URL, and uploads it via the API server, returning the assigned
// charm URL.
func (c *Client) AddLocalCharm(curl *charm.URL, ch charm.Charm, force bool) (*charm.URL, error) {
	if curl.Schema != "local" {
		return nil, errors.Errorf("expected charm URL with local: schema, got %q", curl.String())
	}

	if err := c.validateCharmVersion(ch); err != nil {
		return nil, errors.Trace(err)
	}
	if err := lxdprofile.ValidateCharmLXDProfile(ch); err != nil {
		if !force {
			return nil, errors.Trace(err)
		}
	}

	// Package the charm for uploading.
	var archive *os.File
	switch ch := ch.(type) {
	case *charm.CharmDir:
		var err error
		if archive, err = ioutil.TempFile("", "charm"); err != nil {
			return nil, errors.Annotate(err, "cannot create temp file")
		}
		defer os.Remove(archive.Name())
		defer archive.Close()
		if err := ch.ArchiveTo(archive); err != nil {
			return nil, errors.Annotate(err, "cannot repackage charm")
		}
		if _, err := archive.Seek(0, 0); err != nil {
			return nil, errors.Annotate(err, "cannot rewind packaged charm")
		}
	case *charm.CharmArchive:
		var err error
		if archive, err = os.Open(ch.Path); err != nil {
			return nil, errors.Annotate(err, "cannot read charm archive")
		}
		defer archive.Close()
	default:
		return nil, errors.Errorf("unknown charm type %T", ch)
	}

	anyHooks, err := hasHooks(archive.Name())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !anyHooks {
		return nil, errors.Errorf("invalid charm %q: has no hooks", curl.Name)
	}

	curl, err = c.UploadCharm(curl, archive)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return curl, nil
}

var hasHooks = hasHooksFolder

func hasHooksFolder(name string) (bool, error) {
	zipr, err := zip.OpenReader(name)
	if err != nil {
		return false, err
	}
	defer zipr.Close()
	count := 0
	// zip file spec 4.4.17.1 says that separators are always "/" even on Windows.
	hooksPath := "hooks/"
	for _, f := range zipr.File {
		if strings.Contains(f.Name, hooksPath) {
			count++
		}
		if count > 1 {
			// 1 is the magic number here.
			// Charm zip archive is expected to contain several files and folders.
			// All properly built charms will have a non-empty "hooks" folders.
			// File names in the archive will be of the form "hooks/" - for hooks folder; and
			// "hooks/*" for the actual charm hooks implementations.
			// For example, install hook may have a file with a name "hooks/install".
			// Once we know that there are, at least, 2 files that have names that start with "hooks/", we
			// know for sure that the charm has a non-empty hooks folder.
			return true, nil
		}
	}
	return false, nil
}

// UploadCharm sends the content to the API server using an HTTP post.
func (c *Client) UploadCharm(curl *charm.URL, content io.ReadSeeker) (*charm.URL, error) {
	args := url.Values{}
	args.Add("series", curl.Series)
	args.Add("schema", curl.Schema)
	args.Add("revision", strconv.Itoa(curl.Revision))
	apiURI := url.URL{Path: "/charms", RawQuery: args.Encode()}

	contentType := "application/zip"
	var resp params.CharmsResponse
	if err := c.httpPost(content, apiURI.String(), contentType, &resp); err != nil {
		return nil, errors.Trace(err)
	}

	curl, err := charm.ParseURL(resp.CharmURL)
	if err != nil {
		return nil, errors.Annotatef(err, "bad charm URL in response")
	}
	return curl, nil
}

type minJujuVersionErr struct {
	*errors.Err
}

func minVersionError(minver, jujuver version.Number) error {
	err := errors.NewErr("charm's min version (%s) is higher than this juju model's version (%s)",
		minver, jujuver)
	err.SetLocation(1)
	return minJujuVersionErr{&err}
}

func (c *Client) validateCharmVersion(ch charm.Charm) error {
	minver := ch.Meta().MinJujuVersion
	if minver != version.Zero {
		agentver, err := c.AgentVersion()
		if err != nil {
			return errors.Trace(err)
		}

		if minver.Compare(agentver) > 0 {
			return minVersionError(minver, agentver)
		}
	}
	return nil
}

// TODO(ericsnow) Use charmstore.CharmID for AddCharm() & AddCharmWithAuth().

// AddCharm adds the given charm URL (which must include revision) to
// the model, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm() in the
// client-side API.
//
// If the AddCharm API call fails because of an authorization error
// when retrieving the charm from the charm store, an error
// satisfying params.IsCodeUnauthorized will be returned.
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

// ResolveCharm resolves the best available charm URLs with series, for charm
// locations without a series specified.
func (c *Client) ResolveCharm(ref *charm.URL) (*charm.URL, error) {
	args := params.ResolveCharms{References: []string{ref.String()}}
	result := new(params.ResolveCharmResults)
	if err := c.facade.FacadeCall("ResolveCharms", args, result); err != nil {
		return nil, err
	}
	if len(result.URLs) == 0 {
		return nil, errors.New("unexpected empty response")
	}
	urlInfo := result.URLs[0]
	if urlInfo.Error != "" {
		return nil, errors.New(urlInfo.Error)
	}
	url, err := charm.ParseURL(urlInfo.URL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return url, nil
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
	return openURI(c.st, uri, query)
}

func openURI(apiCaller base.APICaller, uri string, query url.Values) (io.ReadCloser, error) {
	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := apiCaller.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	blob, err := openBlob(httpClient, uri, query)
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
	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		return errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", contentType)

	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := c.st.HTTPClient()
	if err != nil {
		return errors.Trace(err)
	}

	if err := httpClient.Do(req, content, response); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// APIHostPorts returns a slice of network.HostPort for each API server.
func (c *Client) APIHostPorts() ([][]network.HostPort, error) {
	var result params.APIHostPortsResult
	if err := c.facade.FacadeCall("APIHostPorts", nil, &result); err != nil {
		return nil, err
	}
	return result.NetworkHostsPorts(), nil
}

// AgentVersion reports the version number of the api server.
func (c *Client) AgentVersion() (version.Number, error) {
	var result params.AgentVersionResult
	if err := c.facade.FacadeCall("AgentVersion", nil, &result); err != nil {
		return version.Number{}, err
	}
	return result.Version, nil
}

// websocketDial is called instead of dialer.Dial so we can override it in
// tests.
var websocketDial = websocketDialWithErrors

// WebsocketDialer is something that can make a websocket connection. Enables
// testing the error unpacking in websocketDialWithErrors.
type WebsocketDialer interface {
	Dial(string, http.Header) (*websocket.Conn, *http.Response, error)
}

// websocketDialWithErrors dials the websocket and extracts any error
// from the response if there's a handshake error setting up the
// socket. Any other errors are returned normally.
func websocketDialWithErrors(dialer WebsocketDialer, urlStr string, requestHeader http.Header) (base.Stream, error) {
	c, resp, err := dialer.Dial(urlStr, requestHeader)
	if err != nil {
		if err == websocket.ErrBadHandshake {
			// If ErrBadHandshake is returned, a non-nil response
			// is returned so the client can react to auth errors
			// (for example).
			//
			// The problem here is that there is a response, but the response
			// body is truncated to 1024 bytes for debugging information, not
			// for a true response. While this may work for small bodies, it
			// isn't guaranteed to work for all messages.
			defer resp.Body.Close()
			body, readErr := ioutil.ReadAll(resp.Body)
			if readErr != nil {
				return nil, err
			}
			if resp.Header.Get("Content-Type") == "application/json" {
				var result params.ErrorResult
				jsonErr := json.Unmarshal(body, &result)
				if jsonErr != nil {
					return nil, errors.Annotate(jsonErr, "reading error response")
				}
				return nil, result.Error
			}

			err = errors.Errorf(
				"%s (%s)",
				strings.TrimSpace(string(body)),
				http.StatusText(resp.StatusCode),
			)
		}
		return nil, err
	}
	return c, nil
}

// WatchDebugLog returns a channel of structured Log Messages. Only log entries
// that match the filtering specified in the DebugLogParams are returned.
func (c *Client) WatchDebugLog(args common.DebugLogParams) (<-chan common.LogMessage, error) {
	return common.StreamDebugLog(c.st, args)
}
