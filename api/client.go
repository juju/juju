// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// Client represents the client-accessible part of the state.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	st     *State
}

// NetworksSpecification holds the enabled and disabled networks for a
// service.
// TODO(dimitern): Drop this in a follow-up.
type NetworksSpecification struct {
	Enabled  []string
	Disabled []string
}

// AgentStatus holds status info about a machine or unit agent.
type AgentStatus struct {
	Status  params.Status
	Info    string
	Data    map[string]interface{}
	Since   *time.Time
	Kind    params.HistoryKind
	Version string
	Life    string
	Err     error
}

// MachineStatus holds status info about a machine.
type MachineStatus struct {
	Agent AgentStatus

	// The following fields mirror fields in AgentStatus (introduced
	// in 1.19.x). The old fields below are being kept for
	// compatibility with old clients.
	// They can be removed once API versioning lands.
	AgentState     params.Status
	AgentStateInfo string
	AgentVersion   string
	Life           string
	Err            error

	DNSName       string
	InstanceId    instance.Id
	InstanceState string
	Series        string
	Id            string
	Containers    map[string]MachineStatus
	Hardware      string
	Jobs          []multiwatcher.MachineJob
	HasVote       bool
	WantsVote     bool
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
	MeterStatuses map[string]MeterStatus
	Status        AgentStatus
}

// UnitStatusHistory holds a slice of statuses.
type UnitStatusHistory struct {
	Statuses []AgentStatus
}

// MeterStatus represents the meter status of a unit.
type MeterStatus struct {
	Color   string
	Message string
}

// UnitStatus holds status info about a unit.
type UnitStatus struct {
	// UnitAgent holds the status for a unit's agent.
	UnitAgent AgentStatus

	// Workload holds the status for a unit's workload
	Workload AgentStatus

	// Until Juju 2.0, we need to continue to return legacy agent state values
	// as top level struct attributes when the "FullStatus" API is called.
	AgentState     params.Status
	AgentStateInfo string
	AgentVersion   string
	Life           string
	Err            error

	Machine       string
	OpenedPorts   []string
	PublicAddress string
	Charm         string
	Subordinates  map[string]UnitStatus
}

// RelationStatus holds status info about a relation.
type RelationStatus struct {
	Id        int
	Key       string
	Interface string
	Scope     charm.RelationScope
	Endpoints []EndpointStatus
}

// EndpointStatus holds status info about a single endpoint
type EndpointStatus struct {
	ServiceName string
	Name        string
	Role        charm.RelationRole
	Subordinate bool
}

func (epStatus *EndpointStatus) String() string {
	return epStatus.ServiceName + ":" + epStatus.Name
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
	Relations       []RelationStatus
}

// Status returns the status of the juju environment.
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
		if params.IsCodeNotImplemented(err) {
			return &params.UnitStatusHistory{}, errors.NotImplementedf("UnitStatusHistory")
		}
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

// ServiceSet sets configuration options on a service.
func (c *Client) ServiceSet(service string, options map[string]string) error {
	p := params.ServiceSet{
		ServiceName: service,
		Options:     options,
	}
	// TODO(Nate): Put this back to ServiceSet when the GUI stops expecting
	// ServiceSet to unset values set to an empty string.
	return c.facade.FacadeCall("NewServiceSetForClientAPI", p, nil)
}

// ServiceUnset resets configuration options on a service.
func (c *Client) ServiceUnset(service string, options []string) error {
	p := params.ServiceUnset{
		ServiceName: service,
		Options:     options,
	}
	return c.facade.FacadeCall("ServiceUnset", p, nil)
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

// ServiceSetYAML sets configuration options on a service
// given options in YAML format.
func (c *Client) ServiceSetYAML(service string, yaml string) error {
	p := params.ServiceSetYAML{
		ServiceName: service,
		Config:      yaml,
	}
	return c.facade.FacadeCall("ServiceSetYAML", p, nil)
}

// ServiceGet returns the configuration for the named service.
func (c *Client) ServiceGet(service string) (*params.ServiceGetResults, error) {
	var results params.ServiceGetResults
	params := params.ServiceGet{ServiceName: service}
	err := c.facade.FacadeCall("ServiceGet", params, &results)
	return &results, err
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(endpoints ...string) (*params.AddRelationResults, error) {
	var addRelRes params.AddRelationResults
	params := params.AddRelation{Endpoints: endpoints}
	err := c.facade.FacadeCall("AddRelation", params, &addRelRes)
	return &addRelRes, err
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *Client) DestroyRelation(endpoints ...string) error {
	params := params.DestroyRelation{Endpoints: endpoints}
	return c.facade.FacadeCall("DestroyRelation", params, nil)
}

// ServiceCharmRelations returns the service's charms relation names.
func (c *Client) ServiceCharmRelations(service string) ([]string, error) {
	var results params.ServiceCharmRelationsResults
	params := params.ServiceCharmRelations{ServiceName: service}
	err := c.facade.FacadeCall("ServiceCharmRelations", params, &results)
	return results.CharmRelations, err
}

// AddMachines1dot18 adds new machines with the supplied parameters.
//
// TODO(axw) 2014-04-11 #XXX
// This exists for backwards compatibility;
// We cannot remove this code while clients > 1.20 need to talk to 1.18
// servers (which is something we need for an undetermined amount of time).
func (c *Client) AddMachines1dot18(machineParams []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	args := params.AddMachines{
		MachineParams: machineParams,
	}
	results := new(params.AddMachinesResults)
	err := c.facade.FacadeCall("AddMachines", args, results)
	return results.Machines, err
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

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceExpose(service string) error {
	params := params.ServiceExpose{ServiceName: service}
	return c.facade.FacadeCall("ServiceExpose", params, nil)
}

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceUnexpose(service string) error {
	params := params.ServiceUnexpose{ServiceName: service}
	return c.facade.FacadeCall("ServiceUnexpose", params, nil)
}

// ServiceDeployWithNetworks works exactly like ServiceDeploy, but
// allows the specification of requested networks that must be present
// on the machines where the service is deployed. Another way to specify
// networks to include/exclude is using constraints.
func (c *Client) ServiceDeployWithNetworks(
	charmURL string,
	serviceName string,
	numUnits int,
	configYAML string,
	cons constraints.Value,
	toMachineSpec string,
	networks []string,
) error {
	params := params.ServiceDeploy{
		ServiceName:   serviceName,
		CharmUrl:      charmURL,
		NumUnits:      numUnits,
		ConfigYAML:    configYAML,
		Constraints:   cons,
		ToMachineSpec: toMachineSpec,
		Networks:      networks,
	}
	return c.facade.FacadeCall("ServiceDeployWithNetworks", params, nil)
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
	return c.facade.FacadeCall("ServiceDeploy", params, nil)
}

// ServiceUpdate updates the service attributes, including charm URL,
// minimum number of units, settings and constraints.
// TODO(frankban) deprecate redundant API calls that this supercedes.
func (c *Client) ServiceUpdate(args params.ServiceUpdate) error {
	return c.facade.FacadeCall("ServiceUpdate", args, nil)
}

// ServiceSetCharm sets the charm for a given service.
func (c *Client) ServiceSetCharm(serviceName string, charmUrl string, force bool) error {
	args := params.ServiceSetCharm{
		ServiceName: serviceName,
		CharmUrl:    charmUrl,
		Force:       force,
	}
	return c.facade.FacadeCall("ServiceSetCharm", args, nil)
}

// ServiceGetCharmURL returns the charm URL the given service is
// running at present.
func (c *Client) ServiceGetCharmURL(serviceName string) (*charm.URL, error) {
	result := new(params.StringResult)
	args := params.ServiceGet{ServiceName: serviceName}
	err := c.facade.FacadeCall("ServiceGetCharmURL", args, &result)
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
	err := c.facade.FacadeCall("AddServiceUnits", args, results)
	return results.Units, err
}

// AddServiceUnitsWithPlacement adds a given number of units to a service using the specified
// placement directives to assign units to machines.
func (c *Client) AddServiceUnitsWithPlacement(service string, numUnits int, placement []*instance.Placement) ([]string, error) {
	args := params.AddServiceUnits{
		ServiceName: service,
		NumUnits:    numUnits,
		Placement:   placement,
	}
	results := new(params.AddServiceUnitsResults)
	err := c.facade.FacadeCall("AddServiceUnitsWithPlacement", args, results)
	return results.Units, err
}

// DestroyServiceUnits decreases the number of units dedicated to a service.
func (c *Client) DestroyServiceUnits(unitNames ...string) error {
	params := params.DestroyServiceUnits{unitNames}
	return c.facade.FacadeCall("DestroyServiceUnits", params, nil)
}

// ServiceDestroy destroys a given service.
func (c *Client) ServiceDestroy(service string) error {
	params := params.ServiceDestroy{
		ServiceName: service,
	}
	return c.facade.FacadeCall("ServiceDestroy", params, nil)
}

// GetServiceConstraints returns the constraints for the given service.
func (c *Client) GetServiceConstraints(service string) (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.facade.FacadeCall("GetServiceConstraints", params.GetServiceConstraints{service}, results)
	return results.Constraints, err
}

// GetEnvironmentConstraints returns the constraints for the environment.
func (c *Client) GetEnvironmentConstraints() (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.facade.FacadeCall("GetEnvironmentConstraints", nil, results)
	return results.Constraints, err
}

// SetServiceConstraints specifies the constraints for the given service.
func (c *Client) SetServiceConstraints(service string, constraints constraints.Value) error {
	params := params.SetConstraints{
		ServiceName: service,
		Constraints: constraints,
	}
	return c.facade.FacadeCall("SetServiceConstraints", params, nil)
}

// SetEnvironmentConstraints specifies the constraints for the environment.
func (c *Client) SetEnvironmentConstraints(constraints constraints.Value) error {
	params := params.SetConstraints{
		Constraints: constraints,
	}
	return c.facade.FacadeCall("SetEnvironmentConstraints", params, nil)
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

// EnvironmentInfo holds information about the Juju environment.
type EnvironmentInfo struct {
	DefaultSeries string
	ProviderType  string
	Name          string
	UUID          string
	ServerUUID    string
}

// EnvironmentInfo returns details about the Juju environment.
func (c *Client) EnvironmentInfo() (*EnvironmentInfo, error) {
	info := new(EnvironmentInfo)
	err := c.facade.FacadeCall("EnvironmentInfo", nil, info)
	return info, err
}

// EnvironmentUUID returns the environment UUID from the client connection.
func (c *Client) EnvironmentUUID() string {
	tag, err := c.st.EnvironTag()
	if err != nil {
		logger.Warningf("environ tag not an environ: %v", err)
		return ""
	}
	return tag.Id()
}

// ShareEnvironment allows the given users access to the environment.
func (c *Client) ShareEnvironment(users ...names.UserTag) error {
	var args params.ModifyEnvironUsers
	for _, user := range users {
		if &user != nil {
			args.Changes = append(args.Changes, params.ModifyEnvironUser{
				UserTag: user.String(),
				Action:  params.AddEnvUser,
			})
		}
	}

	var result params.ErrorResults
	err := c.facade.FacadeCall("ShareEnvironment", args, &result)
	if err != nil {
		return errors.Trace(err)
	}

	for i, r := range result.Results {
		if r.Error != nil && r.Error.Code == params.CodeAlreadyExists {
			logger.Warningf("environment is already shared with %s", users[i].Username())
			result.Results[i].Error = nil
		}
	}
	return result.Combine()
}

// EnvironmentUserInfo returns information on all users in the environment.
func (c *Client) EnvironmentUserInfo() ([]params.EnvUserInfo, error) {
	var results params.EnvUserInfoResults
	err := c.facade.FacadeCall("EnvUserInfo", nil, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}

	info := []params.EnvUserInfo{}
	for i, result := range results.Results {
		if result.Result == nil {
			return nil, errors.Errorf("unexpected nil result at position %d", i)
		}
		info = append(info, *result.Result)
	}
	return info, nil
}

// UnshareEnvironment removes access to the environment for the given users.
func (c *Client) UnshareEnvironment(users ...names.UserTag) error {
	var args params.ModifyEnvironUsers
	for _, user := range users {
		if &user != nil {
			args.Changes = append(args.Changes, params.ModifyEnvironUser{
				UserTag: user.String(),
				Action:  params.RemoveEnvUser,
			})
		}
	}

	var result params.ErrorResults
	err := c.facade.FacadeCall("ShareEnvironment", args, &result)
	if err != nil {
		return errors.Trace(err)
	}

	for i, r := range result.Results {
		if r.Error != nil && r.Error.Code == params.CodeNotFound {
			logger.Warningf("environment was not previously shared with user %s", users[i].Username())
			result.Results[i].Error = nil
		}
	}
	return result.Combine()
}

// WatchAll holds the id of the newly-created AllWatcher/AllEnvWatcher.
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

// GetAnnotations returns annotations that have been set on the given entity.
// This API is now deprecated - "Annotations" client should be used instead.
// TODO(anastasiamac) remove for Juju 2.x
func (c *Client) GetAnnotations(tag string) (map[string]string, error) {
	args := params.GetAnnotations{tag}
	ann := new(params.GetAnnotationsResults)
	err := c.facade.FacadeCall("GetAnnotations", args, ann)
	return ann.Annotations, err
}

// SetAnnotations sets the annotation pairs on the given entity.
// Currently annotations are supported on machines, services,
// units and the environment itself.
// This API is now deprecated - "Annotations" client should be used instead.
// TODO(anastasiamac) remove for Juju 2.x
func (c *Client) SetAnnotations(tag string, pairs map[string]string) error {
	args := params.SetAnnotations{tag, pairs}
	return c.facade.FacadeCall("SetAnnotations", args, nil)
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
	result := params.EnvironmentConfigResults{}
	err := c.facade.FacadeCall("EnvironmentGet", nil, &result)
	return result.Config, err
}

// EnvironmentSet sets the given key-value pairs in the environment.
func (c *Client) EnvironmentSet(config map[string]interface{}) error {
	args := params.EnvironmentSet{Config: config}
	return c.facade.FacadeCall("EnvironmentSet", args, nil)
}

// EnvironmentUnset sets the given key-value pairs in the environment.
func (c *Client) EnvironmentUnset(keys ...string) error {
	args := params.EnvironmentUnset{Keys: keys}
	return c.facade.FacadeCall("EnvironmentUnset", args, nil)
}

// SetEnvironAgentVersion sets the environment agent-version setting
// to the given value.
func (c *Client) SetEnvironAgentVersion(version version.Number) error {
	args := params.SetEnvironAgentVersion{Version: version}
	return c.facade.FacadeCall("SetEnvironAgentVersion", args, nil)
}

// AbortCurrentUpgrade aborts and archives the current upgrade
// synchronisation record, if any.
func (c *Client) AbortCurrentUpgrade() error {
	return c.facade.FacadeCall("AbortCurrentUpgrade", nil, nil)
}

// FindTools returns a List containing all tools matching the specified parameters.
func (c *Client) FindTools(
	majorVersion, minorVersion int,
	series, arch string,
) (result params.FindToolsResult, err error) {
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

// DestroyEnvironment puts the environment into a "dying" state,
// and removes all non-manager machine instances. DestroyEnvironment
// will fail if there are any manually-provisioned non-manager machines
// in state.
func (c *Client) DestroyEnvironment() error {
	return c.facade.FacadeCall("DestroyEnvironment", nil, nil)
}

// AddLocalCharm prepares the given charm with a local: schema in its
// URL, and uploads it via the API server, returning the assigned
// charm URL. If the API server does not support charm uploads, an
// error satisfying params.IsCodeNotImplemented() is returned.
func (c *Client) AddLocalCharm(curl *charm.URL, ch charm.Charm) (*charm.URL, error) {
	if curl.Schema != "local" {
		return nil, errors.Errorf("expected charm URL with local: schema, got %q", curl.String())
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

	endPoint, err := c.apiEndpoint("charms", "series="+curl.Series)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// wrap archive in a noopCloser to prevent the underlying transport closing
	// the request body. This is neccessary to prevent a data race on the underlying
	// *os.File as the http transport _may_ issue Close once the body is sent, or it
	// may not if there is an error.
	noop := &noopCloser{archive}
	req, err := http.NewRequest("POST", endPoint, noop)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create upload request")
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
		return nil, errors.Annotate(err, "cannot upload charm")
	}
	defer resp.Body.Close()

	// Now parse the response & return.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Annotate(err, "cannot read charm upload response")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("charm upload failed: %v (%s)", resp.StatusCode, bytes.TrimSpace(body))
	}

	var jsonResponse params.CharmsResponse
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal upload response")
	}
	if jsonResponse.Error != "" {
		return nil, errors.Errorf("error uploading charm: %v", jsonResponse.Error)
	}
	return charm.MustParseURL(jsonResponse.CharmURL), nil
}

// noopCloser implements io.ReadCloser, but does not close the underlying io.ReadCloser.
// This is necessary to ensure the ownership of io.ReadCloser implementations that are
// passed to the net/http Transport which may (under some circumstances), call Close on
// the body passed to a request.
type noopCloser struct {
	io.ReadCloser
}

func (n *noopCloser) Close() error {

	// do not propogate the Close method to the underlying ReadCloser.
	return nil
}

func (c *Client) apiEndpoint(destination, query string) (string, error) {
	root, err := c.apiRoot()
	if err != nil {
		return "", errors.Trace(err)
	}

	upURL := url.URL{
		Scheme:   c.st.serverScheme,
		Host:     c.st.Addr(),
		Path:     path.Join(root, destination),
		RawQuery: query,
	}
	return upURL.String(), nil
}

func (c *Client) apiRoot() (string, error) {
	var apiRoot string
	if _, err := c.st.ServerTag(); err == nil {
		envTag, err := c.st.EnvironTag()
		if err != nil {
			return "", errors.Annotate(err, "cannot get API endpoint address")
		}

		apiRoot = fmt.Sprintf("/environment/%s/", envTag.Id())
	} else {
		// If the server tag is not set, then the agent version is < 1.23. We
		// use the old API endpoint for backwards compatibility.
		apiRoot = "/"
	}
	return apiRoot, nil
}

// AddCharm adds the given charm URL (which must include revision) to
// the environment, if it does not exist yet. Local charms are not
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
func (c *Client) ResolveCharm(ref *charm.Reference) (*charm.URL, error) {
	args := params.ResolveCharms{References: []charm.Reference{*ref}}
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
func (c *Client) UploadTools(r io.Reader, vers version.Binary, additionalSeries ...string) (*tools.Tools, error) {
	// Prepare the upload request.
	query := fmt.Sprintf("binaryVersion=%s&series=%s",
		vers,
		strings.Join(additionalSeries, ","),
	)

	endPoint, err := c.apiEndpoint("tools", query)
	if err != nil {
		return nil, errors.Trace(err)
	}

	req, err := http.NewRequest("POST", endPoint, r)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create upload request")
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
		return nil, errors.Annotate(err, "cannot upload tools")
	}
	defer resp.Body.Close()

	// Now parse the response & return.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Annotate(err, "cannot read tools upload response")
	}
	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("%s", bytes.TrimSpace(body))
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(message, params.CodeOperationBlocked) {
			// Operation Blocked errors must contain correct error code and message.
			return nil, &params.Error{Code: params.CodeOperationBlocked, Message: message}
		}
		return nil, errors.Errorf("tools upload failed: %v (%s)", resp.StatusCode, message)
	}

	var jsonResponse params.ToolsResult
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal upload response")
	}
	if err := jsonResponse.Error; err != nil {
		return nil, errors.Annotate(err, "error uploading tools")
	}
	return jsonResponse.Tools, nil
}

// APIHostPorts returns a slice of network.HostPort for each API server.
func (c *Client) APIHostPorts() ([][]network.HostPort, error) {
	var result params.APIHostPortsResult
	if err := c.facade.FacadeCall("APIHostPorts", nil, &result); err != nil {
		return nil, err
	}
	return result.NetworkHostsPorts(), nil
}

// EnsureAvailability ensures the availability of Juju state servers.
// DEPRECATED: remove when we stop supporting 1.20 and earlier servers.
// This API is now on the HighAvailability facade.
func (c *Client) EnsureAvailability(numStateServers int, cons constraints.Value, series string) (params.StateServersChanges, error) {
	var results params.StateServersChangeResults
	envTag, err := c.st.EnvironTag()
	if err != nil {
		return params.StateServersChanges{}, errors.Trace(err)
	}
	arg := params.StateServersSpecs{
		Specs: []params.StateServersSpec{{
			EnvironTag:      envTag.String(),
			NumStateServers: numStateServers,
			Constraints:     cons,
			Series:          series,
		}}}
	err = c.facade.FacadeCall("EnsureAvailability", arg, &results)
	if err != nil {
		return params.StateServersChanges{}, err
	}
	if len(results.Results) != 1 {
		return params.StateServersChanges{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.StateServersChanges{}, result.Error
	}
	return result.Result, nil
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

	path := "/log"
	if _, ok := c.st.ServerVersion(); ok {
		// If the server version is set, then we know the server is capable of
		// serving debug log at the environment path. We also fully expect
		// that the server has returned a valid environment tag.
		envTag, err := c.st.EnvironTag()
		if err != nil {
			return nil, errors.Annotate(err, "very unexpected")
		}
		path = fmt.Sprintf("/environment/%s/log", envTag.Id())
	}

	target := url.URL{
		Scheme:   "wss",
		Host:     c.st.addr,
		Path:     path,
		RawQuery: attrs.Encode(),
	}
	cfg, err := websocket.NewConfig(target.String(), "http://localhost/")
	cfg.Header = utils.BasicAuthHeader(c.st.tag, c.st.password)
	cfg.TlsConfig = &tls.Config{RootCAs: c.st.certPool, ServerName: "juju-apiserver"}
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
		return nil, errors.Annotate(err, "unable to read initial response")
	}
	line = line[0:n]

	logger.Debugf("initial line: %q", line)
	var errResult params.ErrorResult
	err = json.Unmarshal(line, &errResult)
	if err != nil {
		return nil, errors.Annotate(err, "unable to unmarshal initial response")
	}
	if errResult.Error != nil {
		return nil, errResult.Error
	}
	return connection, nil
}
