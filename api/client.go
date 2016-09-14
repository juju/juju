// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version"
	"golang.org/x/net/websocket"
	"gopkg.in/juju/charm.v6-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/downloader"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
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
			Size:  filter.Size,
			Date:  filter.Date,
			Delta: filter.Delta,
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
func (c *Client) DestroyMachines(machines ...string) error {
	params := params.DestroyMachines{MachineNames: machines}
	return c.facade.FacadeCall("DestroyMachines", params, nil)
}

// ForceDestroyMachines removes a given set of machines and all associated units.
func (c *Client) ForceDestroyMachines(machines ...string) error {
	params := params.DestroyMachines{Force: true, MachineNames: machines}
	return c.facade.FacadeCall("DestroyMachines", params, nil)
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
// and reports whether it is valud.
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
func (c *Client) SetModelAgentVersion(version version.Number) error {
	args := params.SetModelAgentVersion{Version: version}
	return c.facade.FacadeCall("SetModelAgentVersion", args, nil)
}

// AbortCurrentUpgrade aborts and archives the current upgrade
// synchronisation record, if any.
func (c *Client) AbortCurrentUpgrade() error {
	return c.facade.FacadeCall("AbortCurrentUpgrade", nil, nil)
}

// FindTools returns a List containing all tools matching the specified parameters.
func (c *Client) FindTools(majorVersion, minorVersion int, series, arch string) (result params.FindToolsResult, err error) {
	args := params.FindToolsParams{
		MajorVersion: majorVersion,
		MinorVersion: minorVersion,
		Arch:         arch,
		Series:       series,
	}
	err = c.facade.FacadeCall("FindTools", args, &result)
	return result, err
}

// AddLocalCharm prepares the given charm with a local: schema in its
// URL, and uploads it via the API server, returning the assigned
// charm URL.
func (c *Client) AddLocalCharm(curl *charm.URL, ch charm.Charm) (*charm.URL, error) {
	if curl.Schema != "local" {
		return nil, errors.Errorf("expected charm URL with local: schema, got %q", curl.String())
	}

	if err := c.validateCharmVersion(ch); err != nil {
		return nil, errors.Trace(err)
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

	curl, err := c.UploadCharm(curl, archive)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return curl, nil
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
	err := errors.NewErr("charm's min version (%s) is higher than this juju environment's version (%s)",
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
func (c *Client) AddCharm(curl *charm.URL, channel csparams.Channel) error {
	args := params.AddCharm{
		URL:     curl.String(),
		Channel: string(channel),
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
func (c *Client) AddCharmWithAuthorization(curl *charm.URL, channel csparams.Channel, csMac *macaroon.Macaroon) error {
	args := params.AddCharmWithAuthorization{
		URL:                curl.String(),
		Channel:            string(channel),
		CharmStoreMacaroon: csMac,
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
	query := make(url.Values)
	query.Add("url", curl.String())
	query.Add("file", "*")
	return c.OpenURI("/charms", query)
}

// OpenURI performs a GET on a Juju HTTP endpoint returning the
func (c *Client) OpenURI(uri string, query url.Values) (io.ReadCloser, error) {
	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := c.st.HTTPClient()
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
// provided API client.
func NewCharmDownloader(client *Client) *downloader.Downloader {
	dlr := &downloader.Downloader{
		OpenBlob: func(url *url.URL) (io.ReadCloser, error) {
			curl, err := charm.ParseURL(url.String())
			if err != nil {
				return nil, errors.Annotate(err, "did not receive a valid charm URL")
			}
			reader, err := client.OpenCharm(curl)
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

// websocketDialConfig is called instead of websocket.DialConfig so we can
// override it in tests.
var websocketDialConfig = func(config *websocket.Config) (base.Stream, error) {
	c, err := websocket.DialConfig(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return websocketStream{c}, nil
}

type websocketStream struct {
	*websocket.Conn
}

func (c websocketStream) ReadJSON(v interface{}) error {
	return websocket.JSON.Receive(c.Conn, v)
}

func (c websocketStream) WriteJSON(v interface{}) error {
	return websocket.JSON.Send(c.Conn, v)
}

// TODO(ericsnow) Fold DebugLogParams into params.LogStreamConfig.

// DebugLogParams holds parameters for WatchDebugLog that control the
// filtering of the log messages. If the structure is zero initialized, the
// entire log file is sent back starting from the end, and until the user
// closes the connection.
type DebugLogParams struct {
	// IncludeEntity lists entity tags to include in the response. Tags may
	// finish with a '*' to match a prefix e.g.: unit-mysql-*, machine-2. If
	// none are set, then all lines are considered included.
	IncludeEntity []string
	// IncludeModule lists logging modules to include in the response. If none
	// are set all modules are considered included.  If a module is specified,
	// all the submodules also match.
	IncludeModule []string
	// ExcludeEntity lists entity tags to exclude from the response. As with
	// IncludeEntity the values may finish with a '*'.
	ExcludeEntity []string
	// ExcludeModule lists logging modules to exclude from the resposne. If a
	// module is specified, all the submodules are also excluded.
	ExcludeModule []string
	// Limit defines the maximum number of lines to return. Once this many
	// have been sent, the socket is closed.  If zero, all filtered lines are
	// sent down the connection until the client closes the connection.
	Limit uint
	// Backlog tells the server to try to go back this many lines before
	// starting filtering. If backlog is zero and replay is false, then there
	// may be an initial delay until the next matching log message is written.
	Backlog uint
	// Level specifies the minimum logging level to be sent back in the response.
	Level loggo.Level
	// Replay tells the server to start at the start of the log file rather
	// than the end. If replay is true, backlog is ignored.
	Replay bool
	// NoTail tells the server to only return the logs it has now, and not
	// to wait for new logs to arrive.
	NoTail bool
}

func (args DebugLogParams) URLQuery() url.Values {
	attrs := url.Values{
		"includeEntity": args.IncludeEntity,
		"includeModule": args.IncludeModule,
		"excludeEntity": args.ExcludeEntity,
		"excludeModule": args.ExcludeModule,
	}
	if args.Replay {
		attrs.Set("replay", fmt.Sprint(args.Replay))
	}
	if args.NoTail {
		attrs.Set("noTail", fmt.Sprint(args.NoTail))
	}
	if args.Limit > 0 {
		attrs.Set("maxLines", fmt.Sprint(args.Limit))
	}
	if args.Backlog > 0 {
		attrs.Set("backlog", fmt.Sprint(args.Backlog))
	}
	if args.Level != loggo.UNSPECIFIED {
		attrs.Set("level", fmt.Sprint(args.Level))
	}
	return attrs
}

// LogMessage is a structured logging entry.
type LogMessage struct {
	Entity    string
	Timestamp time.Time
	Severity  string
	Module    string
	Location  string
	Message   string
}

// WatchDebugLog returns a channel of structured Log Messages. Only log entries
// that match the filtering specified in the DebugLogParams are returned.
func (c *Client) WatchDebugLog(args DebugLogParams) (<-chan LogMessage, error) {
	// Prepare URL query attributes.
	attrs := args.URLQuery()

	connection, err := c.st.ConnectStream("/log", attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	messages := make(chan LogMessage)
	go func() {
		defer close(messages)

		for {
			var msg params.LogMessage
			err := connection.ReadJSON(&msg)
			if err != nil {
				return
			}
			messages <- LogMessage{
				Entity:    msg.Entity,
				Timestamp: msg.Timestamp,
				Severity:  msg.Severity,
				Module:    msg.Module,
				Location:  msg.Location,
				Message:   msg.Message,
			}
		}
	}()

	return messages, nil
}
