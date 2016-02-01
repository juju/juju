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
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"golang.org/x/net/websocket"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/network"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
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

// UnitStatusHistory retrieves the last <size> results of <kind:combined|agent|workload> status
// for <unitName> unit
func (c *Client) UnitStatusHistory(kind params.HistoryKind, unitName string, size int) (*params.UnitStatusHistory, error) {
	var results params.UnitStatusHistory
	args := params.StatusHistory{
		Kind: kind,
		Size: size,
		Name: unitName,
	}
	err := c.facade.FacadeCall("UnitStatusHistory", args, &results)
	if err != nil {
		return &params.UnitStatusHistory{}, errors.Trace(err)
	}
	return &results, nil
}

// LegacyStatus is a stub version of Status that 1.16 introduced. Should be
// removed along with structs when api versioning makes it safe to do so.
func (c *Client) LegacyStatus() (*params.LegacyStatus, error) {
	var result params.LegacyStatus
	if err := c.facade.FacadeCall("Status", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
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

// CharmInfo holds information about a charm.
type CharmInfo struct {
	Revision int
	URL      string
	Config   *charm.Config
	Meta     *charm.Meta
	Actions  *charm.Actions
}

// CharmInfo returns information about the requested charm.
func (c *Client) CharmInfo(charmURL string) (*CharmInfo, error) {
	args := params.CharmInfo{CharmURL: charmURL}
	info := new(CharmInfo)
	if err := c.facade.FacadeCall("CharmInfo", args, info); err != nil {
		return nil, err
	}
	return info, nil
}

// ModelInfo returns details about the Juju model.
func (c *Client) ModelInfo() (params.ModelInfo, error) {
	var info params.ModelInfo
	err := c.facade.FacadeCall("ModelInfo", nil, &info)
	return info, err
}

// ModelUUID returns the model UUID from the client connection.
func (c *Client) ModelUUID() string {
	tag, err := c.st.ModelTag()
	if err != nil {
		logger.Warningf("model tag not an model: %v", err)
		return ""
	}
	return tag.Id()
}

// ShareModel allows the given users access to the model.
func (c *Client) ShareModel(users ...names.UserTag) error {
	var args params.ModifyModelUsers
	for _, user := range users {
		if &user != nil {
			args.Changes = append(args.Changes, params.ModifyModelUser{
				UserTag: user.String(),
				Action:  params.AddModelUser,
			})
		}
	}

	var result params.ErrorResults
	err := c.facade.FacadeCall("ShareModel", args, &result)
	if err != nil {
		return errors.Trace(err)
	}

	for i, r := range result.Results {
		if r.Error != nil && r.Error.Code == params.CodeAlreadyExists {
			logger.Warningf("model is already shared with %s", users[i].Canonical())
			result.Results[i].Error = nil
		}
	}
	return result.Combine()
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

// UnshareModel removes access to the model for the given users.
func (c *Client) UnshareModel(users ...names.UserTag) error {
	var args params.ModifyModelUsers
	for _, user := range users {
		if &user != nil {
			args.Changes = append(args.Changes, params.ModifyModelUser{
				UserTag: user.String(),
				Action:  params.RemoveModelUser,
			})
		}
	}

	var result params.ErrorResults
	err := c.facade.FacadeCall("ShareModel", args, &result)
	if err != nil {
		return errors.Trace(err)
	}

	for i, r := range result.Results {
		if r.Error != nil && r.Error.Code == params.CodeNotFound {
			logger.Warningf("model was not previously shared with user %s", users[i].Canonical())
			result.Results[i].Error = nil
		}
	}
	return result.Combine()
}

// WatchAll holds the id of the newly-created AllWatcher/AllModelWatcher.
type WatchAll struct {
	AllWatcherId string
}

// WatchAll returns an AllWatcher, from which you can request the Next
// collection of Deltas.
func (c *Client) WatchAll() (*AllWatcher, error) {
	info := new(WatchAll)
	if err := c.facade.FacadeCall("WatchAll", nil, info); err != nil {
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

// ModelGet returns all model settings.
func (c *Client) ModelGet() (map[string]interface{}, error) {
	result := params.ModelConfigResults{}
	err := c.facade.FacadeCall("ModelGet", nil, &result)
	return result.Config, err
}

// ModelSet sets the given key-value pairs in the model.
func (c *Client) ModelSet(config map[string]interface{}) error {
	args := params.ModelSet{Config: config}
	return c.facade.FacadeCall("ModelSet", args, nil)
}

// ModelUnset sets the given key-value pairs in the model.
func (c *Client) ModelUnset(keys ...string) error {
	args := params.ModelUnset{Keys: keys}
	return c.facade.FacadeCall("ModelUnset", args, nil)
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

// RunOnAllMachines runs the command on all the machines with the specified
// timeout.
func (c *Client) RunOnAllMachines(commands string, timeout time.Duration) ([]params.RunResult, error) {
	var results params.RunResults
	args := params.RunParams{Commands: commands, Timeout: timeout}
	err := c.facade.FacadeCall("RunOnAllMachines", args, &results)
	return results.Results, err
}

// Run the Commands specified on the machines identified through the ids
// provided in the machines, services and units slices.
func (c *Client) Run(run params.RunParams) ([]params.RunResult, error) {
	var results params.RunResults
	err := c.facade.FacadeCall("Run", run, &results)
	return results.Results, err
}

// DestroyModel puts the model into a "dying" state,
// and removes all non-manager machine instances. DestroyModel
// will fail if there are any manually-provisioned non-manager machines
// in state.
func (c *Client) DestroyModel() error {
	return c.facade.FacadeCall("DestroyModel", nil, nil)
}

// AddLocalCharm prepares the given charm with a local: schema in its
// URL, and uploads it via the API server, returning the assigned
// charm URL.
func (c *Client) AddLocalCharm(curl *charm.URL, ch charm.Charm) (*charm.URL, error) {
	if curl.Schema != "local" {
		return nil, errors.Errorf("expected charm URL with local: schema, got %q", curl.String())
	}
	httpClient, err := c.st.HTTPClient()
	if err != nil {
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

	req, err := http.NewRequest("POST", "/charms?series="+curl.Series, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", "application/zip")

	var resp params.CharmsResponse
	if err := httpClient.Do(req, archive, &resp); err != nil {
		return nil, errors.Trace(err)
	}
	curl, err = charm.ParseURL(resp.CharmURL)
	if err != nil {
		return nil, errors.Annotatef(err, "bad charm URL in response")
	}
	return curl, nil
}

// AddCharm adds the given charm URL (which must include revision) to
// the model, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm() in the
// client-side API.
//
// If the AddCharm API call fails because of an authorization error
// when retrieving the charm from the charm store, an error
// satisfying params.IsCodeUnauthorized will be returned.
func (c *Client) AddCharm(curl *charm.URL) error {
	args := params.CharmURL{
		URL: curl.String(),
	}
	return c.facade.FacadeCall("AddCharm", args, nil)
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
func (c *Client) AddCharmWithAuthorization(curl *charm.URL, csMac *macaroon.Macaroon) error {
	args := params.AddCharmWithAuthorization{
		URL:                curl.String(),
		CharmStoreMacaroon: csMac,
	}
	return c.facade.FacadeCall("AddCharmWithAuthorization", args, nil)
}

// ResolveCharm resolves the best available charm URLs with series, for charm
// locations without a series specified.
func (c *Client) ResolveCharm(ref *charm.URL) (*charm.URL, error) {
	args := params.ResolveCharms{References: []charm.URL{*ref}}
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
	return urlInfo.URL, nil
}

// UploadTools uploads tools at the specified location to the API server over HTTPS.
func (c *Client) UploadTools(r io.ReadSeeker, vers version.Binary, additionalSeries ...string) (*tools.Tools, error) {
	endpoint := fmt.Sprintf("/tools?binaryVersion=%s&series=%s", vers, strings.Join(additionalSeries, ","))

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", "application/x-tar-gz")

	httpClient, err := c.st.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var resp params.ToolsResult
	err = httpClient.Do(req, r, &resp)
	if err != nil {
		msg := err.Error()
		if params.ErrCode(err) == "" && strings.Contains(msg, params.CodeOperationBlocked) {
			// We're probably talking to an old version of the API server
			// that doesn't provide error codes.
			// See https://bugs.launchpad.net/juju-core/+bug/1499277
			err = &params.Error{
				Code:    params.CodeOperationBlocked,
				Message: msg,
			}
		}
		return nil, errors.Trace(err)
	}
	return resp.Tools, nil
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

// WatchDebugLog returns a ReadCloser that the caller can read the log
// lines from. Only log lines that match the filtering specified in
// the DebugLogParams are returned. It returns an error that satisfies
// errors.IsNotImplemented when the API server does not support the
// end-point.
//
// TODO(dimitern) We already have errors.IsNotImplemented - why do we
// need to define a different error for this purpose here?
func (c *Client) WatchDebugLog(args DebugLogParams) (io.ReadCloser, error) {
	// The websocket connection just hangs if the server doesn't have the log
	// end point. So do a version check, as version was added at the same time
	// as the remote end point.
	_, err := c.AgentVersion()
	if err != nil {
		return nil, errors.NotSupportedf("WatchDebugLog")
	}
	// Prepare URL query attributes.
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

	connection, err := c.st.ConnectStream("/log", attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return connection, nil
}
