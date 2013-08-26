// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state/api/params"
)

// Client represents the client-accessible part of the state.
type Client struct {
	st *State
}

// MachineInfo holds information about a machine.
type MachineInfo struct {
	InstanceId string // blank if not set.
}

// Status holds information about the status of a juju environment.
type Status struct {
	Machines map[string]MachineInfo
	// TODO the rest
}

// Status returns the status of the juju environment.
func (c *Client) Status() (*Status, error) {
	var s Status
	if err := c.st.Call("Client", "", "Status", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ServiceSet sets configuration options on a service.
func (c *Client) ServiceSet(service string, options map[string]string) error {
	p := params.ServiceSet{
		ServiceName: service,
		Options:     options,
	}
	return c.st.Call("Client", "", "ServiceSet", p, nil)
}

// Resolved clears errors on a unit.
func (c *Client) Resolved(unit string, retry bool) error {
	p := params.Resolved{
		UnitName: unit,
		Retry:    retry,
	}
	return c.st.Call("Client", "", "Resolved", p, nil)
}

// ServiceSetYAML sets configuration options on a service
// given options in YAML format.
func (c *Client) ServiceSetYAML(service string, yaml string) error {
	p := params.ServiceSetYAML{
		ServiceName: service,
		Config:      yaml,
	}
	return c.st.Call("Client", "", "ServiceSetYAML", p, nil)
}

// ServiceGet returns the configuration for the named service.
func (c *Client) ServiceGet(service string) (*params.ServiceGetResults, error) {
	var results params.ServiceGetResults
	params := params.ServiceGet{ServiceName: service}
	err := c.st.Call("Client", "", "ServiceGet", params, &results)
	return &results, err
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(endpoints ...string) (*params.AddRelationResults, error) {
	var addRelRes params.AddRelationResults
	params := params.AddRelation{Endpoints: endpoints}
	err := c.st.Call("Client", "", "AddRelation", params, &addRelRes)
	return &addRelRes, err
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *Client) DestroyRelation(endpoints ...string) error {
	params := params.DestroyRelation{Endpoints: endpoints}
	return c.st.Call("Client", "", "DestroyRelation", params, nil)
}

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceExpose(service string) error {
	params := params.ServiceExpose{ServiceName: service}
	return c.st.Call("Client", "", "ServiceExpose", params, nil)
}

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceUnexpose(service string) error {
	params := params.ServiceUnexpose{ServiceName: service}
	return c.st.Call("Client", "", "ServiceUnexpose", params, nil)
}

// ServiceDeploy obtains the charm, either locally or from the charm store,
// and deploys it.
func (c *Client) ServiceDeploy(charmUrl string, serviceName string, numUnits int, configYAML string, cons constraints.Value) error {
	params := params.ServiceDeploy{
		ServiceName: serviceName,
		CharmUrl:    charmUrl,
		NumUnits:    numUnits,
		ConfigYAML:  configYAML,
		Constraints: cons,
	}
	return c.st.Call("Client", "", "ServiceDeploy", params, nil)
}

// ServiceUpdate updates the service attributes, including charm URL,
// minimum number of units, settings and constraints.
// TODO(frankban) deprecate redundant API calls that this supercedes.
func (c *Client) ServiceUpdate(args params.ServiceUpdate) error {
	return c.st.Call("Client", "", "ServiceUpdate", args, nil)
}

// ServiceSetCharm sets the charm for a given service.
func (c *Client) ServiceSetCharm(serviceName string, charmUrl string, force bool) error {
	args := params.ServiceSetCharm{
		ServiceName: serviceName,
		CharmUrl:    charmUrl,
		Force:       force,
	}
	return c.st.Call("Client", "", "ServiceSetCharm", args, nil)
}

// AddServiceUnits adds a given number of units to a service.
func (c *Client) AddServiceUnits(service string, numUnits int, machineSpec string) ([]string, error) {
	args := params.AddServiceUnits{
		ServiceName:   service,
		NumUnits:      numUnits,
		ToMachineSpec: machineSpec,
	}
	results := new(params.AddServiceUnitsResults)
	err := c.st.Call("Client", "", "AddServiceUnits", args, results)
	return results.Units, err
}

// DestroyServiceUnits decreases the number of units dedicated to a service.
func (c *Client) DestroyServiceUnits(unitNames []string) error {
	params := params.DestroyServiceUnits{unitNames}
	return c.st.Call("Client", "", "DestroyServiceUnits", params, nil)
}

// ServiceDestroy destroys a given service.
func (c *Client) ServiceDestroy(service string) error {
	params := params.ServiceDestroy{
		ServiceName: service,
	}
	return c.st.Call("Client", "", "ServiceDestroy", params, nil)
}

// GetServiceConstraints returns the constraints for the given service.
func (c *Client) GetServiceConstraints(service string) (constraints.Value, error) {
	results := new(params.GetServiceConstraintsResults)
	err := c.st.Call("Client", "", "GetServiceConstraints", params.GetServiceConstraints{service}, results)
	return results.Constraints, err
}

// SetServiceConstraints specifies the constraints for the given service.
func (c *Client) SetServiceConstraints(service string, constraints constraints.Value) error {
	params := params.SetServiceConstraints{
		ServiceName: service,
		Constraints: constraints,
	}
	return c.st.Call("Client", "", "SetServiceConstraints", params, nil)
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
	if err := c.st.Call("Client", "", "CharmInfo", args, info); err != nil {
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
	err := c.st.Call("Client", "", "EnvironmentInfo", nil, info)
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
	if err := c.st.Call("Client", "", "WatchAll", nil, info); err != nil {
		return nil, err
	}
	return newAllWatcher(c, &info.AllWatcherId), nil
}

// GetAnnotations returns annotations that have been set on the given entity.
func (c *Client) GetAnnotations(tag string) (map[string]string, error) {
	args := params.GetAnnotations{tag}
	ann := new(params.GetAnnotationsResults)
	err := c.st.Call("Client", "", "GetAnnotations", args, ann)
	return ann.Annotations, err
}

// SetAnnotations sets the annotation pairs on the given entity.
// Currently annotations are supported on machines, services,
// units and the environment itself.
func (c *Client) SetAnnotations(tag string, pairs map[string]string) error {
	args := params.SetAnnotations{tag, pairs}
	return c.st.Call("Client", "", "SetAnnotations", args, nil)
}

// Close closes the underlying State connection
// Client is unique amonge the api.State facades in closing its own State
// connection, but it is also the only object we want to expose to the CLI.
func (c *Client) Close() error {
	return c.st.Close()
}
