// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"code.google.com/p/go.net/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/network"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

// Client represents the client-accessible part of the state.
type Client struct {
	st *State
}

// NetworksSpecification holds the enabled and disabled networks for a
// service.
type NetworksSpecification struct {
	Enabled  []string
	Disabled []string
}

func (c *Client) call(method string, params, result interface{}) error {
	return c.st.Call("Client", "", method, params, result)
}

// MachineStatus holds status info about a machine.
type MachineStatus struct {
	Err            error
	AgentState     params.Status
	AgentStateInfo string
	AgentVersion   string
	DNSName        string
	InstanceId     instance.Id
	InstanceState  string
	Life           string
	Series         string
	Id             string
	Containers     map[string]MachineStatus
	Hardware       string
	Jobs           []params.MachineJob
	HasVote        bool
	WantsVote      bool
}

// ServiceStatus holds status info about a service.
type ServiceStatus struct {
	Err           error
	Charm         string
	Exposed       bool
	Life          string
	Relations     map[string][]string
	Networks      NetworksSpecification
	CanUpgradeTo  string
	SubordinateTo []string
	Units         map[string]UnitStatus
}

// UnitStatus holds status info about a unit.
type UnitStatus struct {
	Err            error
	AgentState     params.Status
	AgentStateInfo string
	AgentVersion   string
	Life           string
	Machine        string
	OpenedPorts    []string
	PublicAddress  string
	Charm          string
	Subordinates   map[string]UnitStatus
}

// NetworkStatus holds status info about a network.
type NetworkStatus struct {
	Err        error
	ProviderId network.Id
	CIDR       string
	VLANTag    int
}

// Status holds information about the status of a juju environment.
type Status struct {
	EnvironmentName string
	Machines        map[string]MachineStatus
	Services        map[string]ServiceStatus
	Networks        map[string]NetworkStatus
}

// Status returns the status of the juju environment.
func (c *Client) Status(patterns []string) (*Status, error) {
	var result Status
	p := params.StatusParams{Patterns: patterns}
	if err := c.call("FullStatus", p, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// LegacyMachineStatus holds just the instance-id of a machine.
type LegacyMachineStatus struct {
	InstanceId string // Not type instance.Id just to match original api.
}

// LegacyStatus holds minimal information on the status of a juju environment.
type LegacyStatus struct {
	Machines map[string]LegacyMachineStatus
}

// LegacyStatus is a stub version of Status that 1.16 introduced. Should be
// removed along with structs when api versioning makes it safe to do so.
func (c *Client) LegacyStatus() (*LegacyStatus, error) {
	var result LegacyStatus
	if err := c.call("Status", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ServiceSet sets configuration options on a service.
func (c *Client) ServiceSet(service string, options map[string]string) error {
	p := params.ServiceSet{
		ServiceName: service,
		Options:     options,
	}
	// TODO(Nate): Put this back to ServiceSet when the GUI stops expecting
	// ServiceSet to unset values set to an empty string.
	return c.call("NewServiceSetForClientAPI", p, nil)
}

// ServiceUnset resets configuration options on a service.
func (c *Client) ServiceUnset(service string, options []string) error {
	p := params.ServiceUnset{
		ServiceName: service,
		Options:     options,
	}
	return c.call("ServiceUnset", p, nil)
}

// Resolved clears errors on a unit.
func (c *Client) Resolved(unit string, retry bool) error {
	p := params.Resolved{
		UnitName: unit,
		Retry:    retry,
	}
	return c.call("Resolved", p, nil)
}

// RetryProvisioning updates the provisioning status of a machine allowing the
// provisioner to retry.
func (c *Client) RetryProvisioning(machines ...string) ([]params.ErrorResult, error) {
	p := params.Entities{}
	p.Entities = make([]params.Entity, len(machines))
	for i, machine := range machines {
		p.Entities[i] = params.Entity{Tag: machine}
	}
	var results params.ErrorResults
	err := c.st.Call("Client", "", "RetryProvisioning", p, &results)
	return results.Results, err
}

// PublicAddress returns the public address of the specified
// machine or unit.
func (c *Client) PublicAddress(target string) (string, error) {
	var results params.PublicAddressResults
	p := params.PublicAddress{Target: target}
	err := c.call("PublicAddress", p, &results)
	return results.PublicAddress, err
}

// PrivateAddress returns the private address of the specified
// machine or unit.
func (c *Client) PrivateAddress(target string) (string, error) {
	var results params.PrivateAddressResults
	p := params.PrivateAddress{Target: target}
	err := c.call("PrivateAddress", p, &results)
	return results.PrivateAddress, err
}

// ServiceSetYAML sets configuration options on a service
// given options in YAML format.
func (c *Client) ServiceSetYAML(service string, yaml string) error {
	p := params.ServiceSetYAML{
		ServiceName: service,
		Config:      yaml,
	}
	return c.call("ServiceSetYAML", p, nil)
}

// ServiceGet returns the configuration for the named service.
func (c *Client) ServiceGet(service string) (*params.ServiceGetResults, error) {
	var results params.ServiceGetResults
	params := params.ServiceGet{ServiceName: service}
	err := c.call("ServiceGet", params, &results)
	return &results, err
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(endpoints ...string) (*params.AddRelationResults, error) {
	var addRelRes params.AddRelationResults
	params := params.AddRelation{Endpoints: endpoints}
	err := c.call("AddRelation", params, &addRelRes)
	return &addRelRes, err
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *Client) DestroyRelation(endpoints ...string) error {
	params := params.DestroyRelation{Endpoints: endpoints}
	return c.call("DestroyRelation", params, nil)
}

// ServiceCharmRelations returns the service's charms relation names.
func (c *Client) ServiceCharmRelations(service string) ([]string, error) {
	var results params.ServiceCharmRelationsResults
	params := params.ServiceCharmRelations{ServiceName: service}
	err := c.call("ServiceCharmRelations", params, &results)
	return results.CharmRelations, err
}

// AddMachines1dot18 adds new machines with the supplied parameters.
//
// TODO(axw) 2014-04-11 #XXX
// This exists for backwards compatibility; remove in 1.21 (client only).
func (c *Client) AddMachines1dot18(machineParams []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	args := params.AddMachines{
		MachineParams: machineParams,
	}
	results := new(params.AddMachinesResults)
	err := c.call("AddMachines", args, results)
	return results.Machines, err
}

// AddMachines adds new machines with the supplied parameters.
func (c *Client) AddMachines(machineParams []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	args := params.AddMachines{
		MachineParams: machineParams,
	}
	results := new(params.AddMachinesResults)
	err := c.call("AddMachinesV2", args, results)
	return results.Machines, err
}

// ProvisioningScript returns a shell script that, when run,
// provisions a machine agent on the machine executing the script.
func (c *Client) ProvisioningScript(args params.ProvisioningScriptParams) (script string, err error) {
	var result params.ProvisioningScriptResult
	if err = c.call("ProvisioningScript", args, &result); err != nil {
		return "", err
	}
	return result.Script, nil
}

// DestroyMachines removes a given set of machines.
func (c *Client) DestroyMachines(machines ...string) error {
	params := params.DestroyMachines{MachineNames: machines}
	return c.call("DestroyMachines", params, nil)
}

// ForceDestroyMachines removes a given set of machines and all associated units.
func (c *Client) ForceDestroyMachines(machines ...string) error {
	params := params.DestroyMachines{Force: true, MachineNames: machines}
	return c.call("DestroyMachines", params, nil)
}

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceExpose(service string) error {
	params := params.ServiceExpose{ServiceName: service}
	return c.call("ServiceExpose", params, nil)
}

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceUnexpose(service string) error {
	params := params.ServiceUnexpose{ServiceName: service}
	return c.call("ServiceUnexpose", params, nil)
}

// ServiceDeployWithNetworks works exactly like ServiceDeploy, but
// allows the specification of networks that will be included or
// excluded on the machines where the service is deployed. Networks
// are specified with network tags (see names.NetworkTag).
func (c *Client) ServiceDeployWithNetworks(charmURL string, serviceName string, numUnits int, configYAML string, cons constraints.Value, toMachineSpec string, includeNetworks, excludeNetworks []string) error {
	params := params.ServiceDeploy{
		ServiceName:     serviceName,
		CharmUrl:        charmURL,
		NumUnits:        numUnits,
		ConfigYAML:      configYAML,
		Constraints:     cons,
		ToMachineSpec:   toMachineSpec,
		IncludeNetworks: includeNetworks,
		ExcludeNetworks: excludeNetworks,
	}
	return c.st.Call("Client", "", "ServiceDeployWithNetworks", params, nil)
}

// ServiceDeploy obtains the charm, either locally or from the charm store,
// and deploys it.
func (c *Client) ServiceDeploy(charmURL string, serviceName string, numUnits int, configYAML string, cons constraints.Value, toMachineSpec string) error {
	params := params.ServiceDeploy{
		ServiceName:   serviceName,
		CharmUrl:      charmURL,
		NumUnits:      numUnits,
		ConfigYAML:    configYAML,
		Constraints:   cons,
		ToMachineSpec: toMachineSpec,
	}
	return c.call("ServiceDeploy", params, nil)
}

// ServiceUpdate updates the service attributes, including charm URL,
// minimum number of units, settings and constraints.
// TODO(frankban) deprecate redundant API calls that this supercedes.
func (c *Client) ServiceUpdate(args params.ServiceUpdate) error {
	return c.call("ServiceUpdate", args, nil)
}

// ServiceSetCharm sets the charm for a given service.
func (c *Client) ServiceSetCharm(serviceName string, charmUrl string, force bool) error {
	args := params.ServiceSetCharm{
		ServiceName: serviceName,
		CharmUrl:    charmUrl,
		Force:       force,
	}
	return c.call("ServiceSetCharm", args, nil)
}

// ServiceGetCharmURL returns the charm URL the given service is
// running at present.
func (c *Client) ServiceGetCharmURL(serviceName string) (*charm.URL, error) {
	result := new(params.StringResult)
	args := params.ServiceGet{ServiceName: serviceName}
	err := c.call("ServiceGetCharmURL", args, &result)
	if err != nil {
		return nil, err
	}
	return charm.ParseURL(result.Result)
}

// AddServiceUnits adds a given number of units to a service.
func (c *Client) AddServiceUnits(service string, numUnits int, machineSpec string) ([]string, error) {
	args := params.AddServiceUnits{
		ServiceName:   service,
		NumUnits:      numUnits,
		ToMachineSpec: machineSpec,
	}
	results := new(params.AddServiceUnitsResults)
	err := c.call("AddServiceUnits", args, results)
	return results.Units, err
}

// DestroyServiceUnits decreases the number of units dedicated to a service.
func (c *Client) DestroyServiceUnits(unitNames ...string) error {
	params := params.DestroyServiceUnits{unitNames}
	return c.call("DestroyServiceUnits", params, nil)
}

// ServiceDestroy destroys a given service.
func (c *Client) ServiceDestroy(service string) error {
	params := params.ServiceDestroy{
		ServiceName: service,
	}
	return c.call("ServiceDestroy", params, nil)
}

// GetServiceConstraints returns the constraints for the given service.
func (c *Client) GetServiceConstraints(service string) (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.call("GetServiceConstraints", params.GetServiceConstraints{service}, results)
	return results.Constraints, err
}

// GetEnvironmentConstraints returns the constraints for the environment.
func (c *Client) GetEnvironmentConstraints() (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.call("GetEnvironmentConstraints", nil, results)
	return results.Constraints, err
}

// SetServiceConstraints specifies the constraints for the given service.
func (c *Client) SetServiceConstraints(service string, constraints constraints.Value) error {
	params := params.SetConstraints{
		ServiceName: service,
		Constraints: constraints,
	}
	return c.call("SetServiceConstraints", params, nil)
}

// SetEnvironmentConstraints specifies the constraints for the environment.
func (c *Client) SetEnvironmentConstraints(constraints constraints.Value) error {
	params := params.SetConstraints{
		Constraints: constraints,
	}
	return c.call("SetEnvironmentConstraints", params, nil)
}

// CharmInfo holds information about a charm.
type CharmInfo struct {
	Revision int
	URL      string
	Config   *charm.Config
	Meta     *charm.Meta
}

// CharmInfo returns information about the requested charm.
func (c *Client) CharmInfo(charmURL string) (*CharmInfo, error) {
	args := params.CharmInfo{CharmURL: charmURL}
	info := new(CharmInfo)
	if err := c.call("CharmInfo", args, info); err != nil {
		return nil, err
	}
	return info, nil
}

// EnvironmentInfo holds information about the Juju environment.
type EnvironmentInfo struct {
	DefaultSeries string
	ProviderType  string
	Name          string
	UUID          string
}

// EnvironmentInfo returns details about the Juju environment.
func (c *Client) EnvironmentInfo() (*EnvironmentInfo, error) {
	info := new(EnvironmentInfo)
	err := c.call("EnvironmentInfo", nil, info)
	return info, err
}

// WatchAll holds the id of the newly-created AllWatcher.
type WatchAll struct {
	AllWatcherId string
}

// WatchAll returns an AllWatcher, from which you can request the Next
// collection of Deltas.
func (c *Client) WatchAll() (*AllWatcher, error) {
	info := new(WatchAll)
	if err := c.call("WatchAll", nil, info); err != nil {
		return nil, err
	}
	return newAllWatcher(c, &info.AllWatcherId), nil
}

// GetAnnotations returns annotations that have been set on the given entity.
func (c *Client) GetAnnotations(tag string) (map[string]string, error) {
	args := params.GetAnnotations{tag}
	ann := new(params.GetAnnotationsResults)
	err := c.call("GetAnnotations", args, ann)
	return ann.Annotations, err
}

// SetAnnotations sets the annotation pairs on the given entity.
// Currently annotations are supported on machines, services,
// units and the environment itself.
func (c *Client) SetAnnotations(tag string, pairs map[string]string) error {
	args := params.SetAnnotations{tag, pairs}
	return c.call("SetAnnotations", args, nil)
}

// Close closes the Client's underlying State connection
// Client is unique among the api.State facades in closing its own State
// connection, but it is conventional to use a Client object without any access
// to its underlying state connection.
func (c *Client) Close() error {
	return c.st.Close()
}

// EnvironmentGet returns all environment settings.
func (c *Client) EnvironmentGet() (map[string]interface{}, error) {
	result := params.EnvironmentGetResults{}
	err := c.call("EnvironmentGet", nil, &result)
	return result.Config, err
}

// EnvironmentSet sets the given key-value pairs in the environment.
func (c *Client) EnvironmentSet(config map[string]interface{}) error {
	args := params.EnvironmentSet{Config: config}
	return c.call("EnvironmentSet", args, nil)
}

// EnvironmentUnset sets the given key-value pairs in the environment.
func (c *Client) EnvironmentUnset(keys ...string) error {
	args := params.EnvironmentUnset{Keys: keys}
	return c.call("EnvironmentUnset", args, nil)
}

// SetEnvironAgentVersion sets the environment agent-version setting
// to the given value.
func (c *Client) SetEnvironAgentVersion(version version.Number) error {
	args := params.SetEnvironAgentVersion{Version: version}
	return c.call("SetEnvironAgentVersion", args, nil)
}

// FindTools returns a List containing all tools matching the specified parameters.
func (c *Client) FindTools(majorVersion, minorVersion int,
	series, arch string) (result params.FindToolsResults, err error) {

	args := params.FindToolsParams{
		MajorVersion: majorVersion,
		MinorVersion: minorVersion,
		Arch:         arch,
		Series:       series,
	}
	err = c.call("FindTools", args, &result)
	return result, err
}

// RunOnAllMachines runs the command on all the machines with the specified
// timeout.
func (c *Client) RunOnAllMachines(commands string, timeout time.Duration) ([]params.RunResult, error) {
	var results params.RunResults
	args := params.RunParams{Commands: commands, Timeout: timeout}
	err := c.call("RunOnAllMachines", args, &results)
	return results.Results, err
}

// Run the Commands specified on the machines identified through the ids
// provided in the machines, services and units slices.
func (c *Client) Run(run params.RunParams) ([]params.RunResult, error) {
	var results params.RunResults
	err := c.call("Run", run, &results)
	return results.Results, err
}

// DestroyEnvironment puts the environment into a "dying" state,
// and removes all non-manager machine instances. DestroyEnvironment
// will fail if there are any manually-provisioned non-manager machines
// in state.
func (c *Client) DestroyEnvironment() error {
	return c.call("DestroyEnvironment", nil, nil)
}

// AddLocalCharm prepares the given charm with a local: schema in its
// URL, and uploads it via the API server, returning the assigned
// charm URL. If the API server does not support charm uploads, an
// error satisfying params.IsCodeNotImplemented() is returned.
func (c *Client) AddLocalCharm(curl *charm.URL, ch charm.Charm) (*charm.URL, error) {
	if curl.Schema != "local" {
		return nil, fmt.Errorf("expected charm URL with local: schema, got %q", curl.String())
	}
	// Package the charm for uploading.
	var archive *os.File
	switch ch := ch.(type) {
	case *charm.Dir:
		var err error
		if archive, err = ioutil.TempFile("", "charm"); err != nil {
			return nil, fmt.Errorf("cannot create temp file: %v", err)
		}
		defer os.Remove(archive.Name())
		defer archive.Close()
		if err := ch.BundleTo(archive); err != nil {
			return nil, fmt.Errorf("cannot repackage charm: %v", err)
		}
		if _, err := archive.Seek(0, 0); err != nil {
			return nil, fmt.Errorf("cannot rewind packaged charm: %v", err)
		}
	case *charm.Bundle:
		var err error
		if archive, err = os.Open(ch.Path); err != nil {
			return nil, fmt.Errorf("cannot read charm archive: %v", err)
		}
		defer archive.Close()
	default:
		return nil, fmt.Errorf("unknown charm type %T", ch)
	}

	// Prepare the upload request.
	url := fmt.Sprintf("%s/charms?series=%s", c.st.serverRoot, curl.Series)
	req, err := http.NewRequest("POST", url, archive)
	if err != nil {
		return nil, fmt.Errorf("cannot create upload request: %v", err)
	}
	req.SetBasicAuth(c.st.tag, c.st.password)
	req.Header.Set("Content-Type", "application/zip")

	// Send the request.

	// BUG(dimitern) 2013-12-17 bug #1261780
	// Due to issues with go 1.1.2, fixed later, we cannot use a
	// regular TLS client with the CACert here, because we get "x509:
	// cannot validate certificate for 127.0.0.1 because it doesn't
	// contain any IP SANs". Once we use a later go version, this
	// should be changed to connect to the API server with a regular
	// HTTP+TLS enabled client, using the CACert (possily cached, like
	// the tag and password) passed in api.Open()'s info argument.
	resp, err := utils.GetNonValidatingHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot upload charm: %v", err)
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		// API server is 1.16 or older, so charm upload
		// is not supported; notify the client.
		return nil, &params.Error{
			Message: "charm upload is not supported by the API server",
			Code:    params.CodeNotImplemented,
		}
	}

	// Now parse the response & return.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read charm upload response: %v", err)
	}
	defer resp.Body.Close()
	var jsonResponse params.CharmsResponse
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return nil, fmt.Errorf("cannot unmarshal upload response: %v", err)
	}
	if jsonResponse.Error != "" {
		return nil, fmt.Errorf("error uploading charm: %v", jsonResponse.Error)
	}
	return charm.MustParseURL(jsonResponse.CharmURL), nil
}

// AddCharm adds the given charm URL (which must include revision) to
// the environment, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm() in the
// client-side API.
func (c *Client) AddCharm(curl *charm.URL) error {
	args := params.CharmURL{URL: curl.String()}
	return c.call("AddCharm", args, nil)
}

// ResolveCharm resolves the best available charm URLs with series, for charm
// locations without a series specified.
func (c *Client) ResolveCharm(ref charm.Reference) (*charm.URL, error) {
	args := params.ResolveCharms{References: []charm.Reference{ref}}
	result := new(params.ResolveCharmResults)
	if err := c.st.Call("Client", "", "ResolveCharms", args, result); err != nil {
		return nil, err
	}
	if len(result.URLs) == 0 {
		return nil, fmt.Errorf("unexpected empty response")
	}
	urlInfo := result.URLs[0]
	if urlInfo.Error != "" {
		return nil, fmt.Errorf("%v", urlInfo.Error)
	}
	return urlInfo.URL, nil
}

func (c *Client) UploadTools(
	toolsFilename string, vers version.Binary, fakeSeries ...string,
) (
	tools *tools.Tools, err error,
) {
	toolsTarball, err := os.Open(toolsFilename)
	if err != nil {
		return nil, err
	}
	defer toolsTarball.Close()

	// Prepare the upload request.
	url := fmt.Sprintf("%s/tools?binaryVersion=%s&series=%s", c.st.serverRoot, vers, strings.Join(fakeSeries, ","))
	req, err := http.NewRequest("POST", url, toolsTarball)
	if err != nil {
		return nil, fmt.Errorf("cannot create upload request: %v", err)
	}
	req.SetBasicAuth(c.st.tag, c.st.password)
	req.Header.Set("Content-Type", "application/x-tar-gz")

	// Send the request.

	// BUG(dimitern) 2013-12-17 bug #1261780
	// Due to issues with go 1.1.2, fixed later, we cannot use a
	// regular TLS client with the CACert here, because we get "x509:
	// cannot validate certificate for 127.0.0.1 because it doesn't
	// contain any IP SANs". Once we use a later go version, this
	// should be changed to connect to the API server with a regular
	// HTTP+TLS enabled client, using the CACert (possily cached, like
	// the tag and password) passed in api.Open()'s info argument.
	resp, err := utils.GetNonValidatingHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot upload charm: %v", err)
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		// API server is older than 1.17.5, so tools upload
		// is not supported; notify the client.
		return nil, &params.Error{
			Message: "tools upload is not supported by the API server",
			Code:    params.CodeNotImplemented,
		}
	}

	// Now parse the response & return.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read tools upload response: %v", err)
	}
	defer resp.Body.Close()
	var jsonResponse params.ToolsResult
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return nil, fmt.Errorf("cannot unmarshal upload response: %v", err)
	}
	if err := jsonResponse.Error; err != nil {
		return nil, fmt.Errorf("error uploading tools: %v", err)
	}
	return jsonResponse.Tools, nil
}

// APIHostPorts returns a slice of instance.HostPort for each API server.
func (c *Client) APIHostPorts() ([][]instance.HostPort, error) {
	var result params.APIHostPortsResult
	if err := c.call("APIHostPorts", nil, &result); err != nil {
		return nil, err
	}
	return result.Servers, nil
}

// EnsureAvailability ensures the availability of Juju state servers.
func (c *Client) EnsureAvailability(numStateServers int, cons constraints.Value, series string) error {
	args := params.EnsureAvailability{
		NumStateServers: numStateServers,
		Constraints:     cons,
		Series:          series,
	}
	return c.call("EnsureAvailability", args, nil)
}

// AgentVersion reports the version number of the api server.
func (c *Client) AgentVersion() (version.Number, error) {
	var result params.AgentVersionResult
	if err := c.call("AgentVersion", nil, &result); err != nil {
		return version.Number{}, err
	}
	return result.Version, nil
}

// websocketDialConfig is called instead of websocket.DialConfig so we can
// override it in tests.
var websocketDialConfig = func(config *websocket.Config) (io.ReadCloser, error) {
	return websocket.DialConfig(config)
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
	// Prepare URL.
	attrs := url.Values{}
	if args.Replay {
		attrs.Set("replay", fmt.Sprint(args.Replay))
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
	attrs["includeEntity"] = args.IncludeEntity
	attrs["includeModule"] = args.IncludeModule
	attrs["excludeEntity"] = args.ExcludeEntity
	attrs["excludeModule"] = args.ExcludeModule

	target := url.URL{
		Scheme:   "wss",
		Host:     c.st.addr,
		Path:     "/log",
		RawQuery: attrs.Encode(),
	}
	cfg, err := websocket.NewConfig(target.String(), "http://localhost/")
	cfg.Header = utils.BasicAuthHeader(c.st.tag, c.st.password)
	cfg.TlsConfig = &tls.Config{RootCAs: c.st.certPool, ServerName: "anything"}
	connection, err := websocketDialConfig(cfg)
	if err != nil {
		return nil, err
	}
	// Read the initial error and translate to a real error.
	// Read up to the first new line character. We can't use bufio here as it
	// reads too much from the reader.
	line := make([]byte, 4096)
	n, err := connection.Read(line)
	if err != nil {
		return nil, fmt.Errorf("unable to read initial response: %v", err)
	}
	line = line[0:n]

	logger.Debugf("initial line: %q", line)
	var errResult params.ErrorResult
	err = json.Unmarshal(line, &errResult)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal initial response: %v", err)
	}
	if errResult.Error != nil {
		return nil, errResult.Error
	}
	return connection, nil
}
