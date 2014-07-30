// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/client"
)

type StatusCommand struct {
	envcmd.EnvCommandBase
	out      cmd.Output
	patterns []string
}

var statusDoc = `
This command will report on the runtime state of various system entities.

Service or unit names may be specified to filter the status to only those
services and units that match, along with the related machines, services
and units. If a subordinate unit is matched, then its principal unit will
be displayed. If a principal unit is matched, then all of its subordinates
will be displayed.

Wildcards ('*') may be specified in service/unit names to match any sequence
of characters. For example, 'nova-*' will match any service whose name begins
with 'nova-': 'nova-compute', 'nova-volume', etc.
`

func (c *StatusCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status",
		Args:    "[pattern ...]",
		Purpose: "output status information about an environment",
		Doc:     statusDoc,
		Aliases: []string{"stat"},
	}
}

func (c *StatusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

func (c *StatusCommand) Init(args []string) error {
	c.patterns = args
	return nil
}

var connectionError = `Unable to connect to environment %q.
Please check your credentials or use 'juju bootstrap' to create a new environment.

Error details:
%v
`

type statusAPI interface {
	Status(patterns []string) (*api.Status, error)
	Close() error
}

var newApiClientForStatus = func(c *StatusCommand) (statusAPI, error) {
	return c.NewAPIClient()
}

func (c *StatusCommand) Run(ctx *cmd.Context) error {
	// Just verify the pattern validity client side, do not use the matcher
	_, err := client.NewUnitMatcher(c.patterns)
	if err != nil {
		return err
	}
	apiclient, err := newApiClientForStatus(c)
	if err != nil {
		return fmt.Errorf(connectionError, c.ConnectionName(), err)
	}
	defer apiclient.Close()

	status, err := apiclient.Status(c.patterns)
	if err != nil {
		if status == nil {
			// Status call completely failed, there is nothing to report
			return err
		}
		// Display any error, but continue to print status if some was returned
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	}
	result := newStatusFormatter(status).format()
	return c.out.Write(ctx, result)
}

type formattedStatus struct {
	Environment string                   `json:"environment"`
	Machines    map[string]machineStatus `json:"machines"`
	Services    map[string]serviceStatus `json:"services"`
	Networks    map[string]networkStatus `json:"networks,omitempty" yaml:",omitempty"`
}

type errorStatus struct {
	StatusError string `json:"status-error" yaml:"status-error"`
}

type machineStatus struct {
	Err            error                    `json:"-" yaml:",omitempty"`
	AgentState     params.Status            `json:"agent-state,omitempty" yaml:"agent-state,omitempty"`
	AgentStateInfo string                   `json:"agent-state-info,omitempty" yaml:"agent-state-info,omitempty"`
	AgentVersion   string                   `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
	DNSName        string                   `json:"dns-name,omitempty" yaml:"dns-name,omitempty"`
	InstanceId     instance.Id              `json:"instance-id,omitempty" yaml:"instance-id,omitempty"`
	InstanceState  string                   `json:"instance-state,omitempty" yaml:"instance-state,omitempty"`
	Life           string                   `json:"life,omitempty" yaml:"life,omitempty"`
	Series         string                   `json:"series,omitempty" yaml:"series,omitempty"`
	Id             string                   `json:"-" yaml:"-"`
	Containers     map[string]machineStatus `json:"containers,omitempty" yaml:"containers,omitempty"`
	Hardware       string                   `json:"hardware,omitempty" yaml:"hardware,omitempty"`
	HAStatus       string                   `json:"state-server-member-status,omitempty" yaml:"state-server-member-status,omitempty"`
}

// A goyaml bug means we can't declare these types
// locally to the GetYAML methods.
type machineStatusNoMarshal machineStatus

func (s machineStatus) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return json.Marshal(errorStatus{s.Err.Error()})
	}
	return json.Marshal(machineStatusNoMarshal(s))
}

func (s machineStatus) GetYAML() (tag string, value interface{}) {
	if s.Err != nil {
		return "", errorStatus{s.Err.Error()}
	}
	// TODO(rog) rename mNoMethods to noMethods (and also in
	// the other GetYAML methods) when people are using the non-buggy
	// goyaml version.
	type mNoMethods machineStatus
	return "", mNoMethods(s)
}

type serviceStatus struct {
	Err           error                 `json:"-" yaml:",omitempty"`
	Charm         string                `json:"charm" yaml:"charm"`
	CanUpgradeTo  string                `json:"can-upgrade-to,omitempty" yaml:"can-upgrade-to,omitempty"`
	Exposed       bool                  `json:"exposed" yaml:"exposed"`
	Life          string                `json:"life,omitempty" yaml:"life,omitempty"`
	Relations     map[string][]string   `json:"relations,omitempty" yaml:"relations,omitempty"`
	Networks      map[string][]string   `json:"networks,omitempty" yaml:"networks,omitempty"`
	SubordinateTo []string              `json:"subordinate-to,omitempty" yaml:"subordinate-to,omitempty"`
	Units         map[string]unitStatus `json:"units,omitempty" yaml:"units,omitempty"`
}

type serviceStatusNoMarshal serviceStatus

func (s serviceStatus) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return json.Marshal(errorStatus{s.Err.Error()})
	}
	type sNoMethods serviceStatus
	return json.Marshal(sNoMethods(s))
}

func (s serviceStatus) GetYAML() (tag string, value interface{}) {
	if s.Err != nil {
		return "", errorStatus{s.Err.Error()}
	}
	type sNoMethods serviceStatus
	return "", sNoMethods(s)
}

type unitStatus struct {
	Err            error                 `json:"-" yaml:",omitempty"`
	Charm          string                `json:"upgrading-from,omitempty" yaml:"upgrading-from,omitempty"`
	AgentState     params.Status         `json:"agent-state,omitempty" yaml:"agent-state,omitempty"`
	AgentStateInfo string                `json:"agent-state-info,omitempty" yaml:"agent-state-info,omitempty"`
	AgentVersion   string                `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
	Life           string                `json:"life,omitempty" yaml:"life,omitempty"`
	Machine        string                `json:"machine,omitempty" yaml:"machine,omitempty"`
	OpenedPorts    []string              `json:"open-ports,omitempty" yaml:"open-ports,omitempty"`
	PublicAddress  string                `json:"public-address,omitempty" yaml:"public-address,omitempty"`
	Subordinates   map[string]unitStatus `json:"subordinates,omitempty" yaml:"subordinates,omitempty"`
}

type unitStatusNoMarshal unitStatus

func (s unitStatus) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return json.Marshal(errorStatus{s.Err.Error()})
	}
	return json.Marshal(unitStatusNoMarshal(s))
}

func (s unitStatus) GetYAML() (tag string, value interface{}) {
	if s.Err != nil {
		return "", errorStatus{s.Err.Error()}
	}
	type uNoMethods unitStatus
	return "", unitStatusNoMarshal(s)
}

type networkStatus struct {
	Err        error      `json:"-" yaml:",omitempty"`
	ProviderId network.Id `json:"provider-id" yaml:"provider-id"`
	CIDR       string     `json:"cidr,omitempty" yaml:"cidr,omitempty"`
	VLANTag    int        `json:"vlan-tag,omitempty" yaml:"vlan-tag,omitempty"`
}

type networkStatusNoMarshal networkStatus

func (n networkStatus) MarshalJSON() ([]byte, error) {
	if n.Err != nil {
		return json.Marshal(errorStatus{n.Err.Error()})
	}
	type nNoMethods networkStatus
	return json.Marshal(nNoMethods(n))
}

func (n networkStatus) GetYAML() (tag string, value interface{}) {
	if n.Err != nil {
		return "", errorStatus{n.Err.Error()}
	}
	type nNoMethods networkStatus
	return "", nNoMethods(n)
}

type statusFormatter struct {
	status    *api.Status
	relations map[int]api.RelationStatus
}

func newStatusFormatter(status *api.Status) *statusFormatter {
	sf := statusFormatter{
		status:    status,
		relations: make(map[int]api.RelationStatus),
	}
	for _, relation := range status.Relations {
		sf.relations[relation.Id] = relation
	}
	return &sf
}

func (sf *statusFormatter) format() formattedStatus {
	if sf.status == nil {
		return formattedStatus{}
	}
	out := formattedStatus{
		Environment: sf.status.EnvironmentName,
		Machines:    make(map[string]machineStatus),
		Services:    make(map[string]serviceStatus),
	}
	for k, m := range sf.status.Machines {
		out.Machines[k] = sf.formatMachine(m)
	}
	for sn, s := range sf.status.Services {
		out.Services[sn] = sf.formatService(sn, s)
	}
	for k, n := range sf.status.Networks {
		if out.Networks == nil {
			out.Networks = make(map[string]networkStatus)
		}
		out.Networks[k] = sf.formatNetwork(n)
	}
	return out
}

func (sf *statusFormatter) formatMachine(machine api.MachineStatus) machineStatus {
	var out machineStatus

	if machine.Agent.Status == "" {
		// Older server
		// TODO: this will go away at some point (v1.21?).
		out = machineStatus{
			AgentState:     machine.AgentState,
			AgentStateInfo: machine.AgentStateInfo,
			AgentVersion:   machine.AgentVersion,
			Life:           machine.Life,
			Err:            machine.Err,
			DNSName:        machine.DNSName,
			InstanceId:     machine.InstanceId,
			InstanceState:  machine.InstanceState,
			Series:         machine.Series,
			Id:             machine.Id,
			Containers:     make(map[string]machineStatus),
			Hardware:       machine.Hardware,
		}
	} else {
		// New server
		agent := machine.Agent
		out = machineStatus{
			AgentState:     machine.AgentState,
			AgentStateInfo: adjustInfoIfAgentDown(machine.AgentState, agent.Status, agent.Info),
			AgentVersion:   agent.Version,
			Life:           agent.Life,
			Err:            agent.Err,
			DNSName:        machine.DNSName,
			InstanceId:     machine.InstanceId,
			InstanceState:  machine.InstanceState,
			Series:         machine.Series,
			Id:             machine.Id,
			Containers:     make(map[string]machineStatus),
			Hardware:       machine.Hardware,
		}
	}

	for k, m := range machine.Containers {
		out.Containers[k] = sf.formatMachine(m)
	}

	for _, job := range machine.Jobs {
		if job == params.JobManageEnviron {
			out.HAStatus = makeHAStatus(machine.HasVote, machine.WantsVote)
			break
		}
	}
	return out
}

func (sf *statusFormatter) formatService(name string, service api.ServiceStatus) serviceStatus {
	out := serviceStatus{
		Err:           service.Err,
		Charm:         service.Charm,
		Exposed:       service.Exposed,
		Life:          service.Life,
		Relations:     service.Relations,
		Networks:      make(map[string][]string),
		CanUpgradeTo:  service.CanUpgradeTo,
		SubordinateTo: service.SubordinateTo,
		Units:         make(map[string]unitStatus),
	}
	if len(service.Networks.Enabled) > 0 {
		out.Networks["enabled"] = service.Networks.Enabled
	}
	if len(service.Networks.Disabled) > 0 {
		out.Networks["disabled"] = service.Networks.Disabled
	}
	for k, m := range service.Units {
		out.Units[k] = sf.formatUnit(m, name)
	}
	return out
}

func (sf *statusFormatter) formatUnit(unit api.UnitStatus, serviceName string) unitStatus {
	out := unitStatus{
		Err:            unit.Err,
		AgentState:     unit.AgentState,
		AgentStateInfo: sf.getUnitStatusInfo(unit, serviceName),
		AgentVersion:   unit.AgentVersion,
		Life:           unit.Life,
		Machine:        unit.Machine,
		OpenedPorts:    unit.OpenedPorts,
		PublicAddress:  unit.PublicAddress,
		Charm:          unit.Charm,
		Subordinates:   make(map[string]unitStatus),
	}
	for k, m := range unit.Subordinates {
		out.Subordinates[k] = sf.formatUnit(m, serviceName)
	}
	return out
}

func (sf *statusFormatter) getUnitStatusInfo(unit api.UnitStatus, serviceName string) string {
	if unit.Agent.Status == "" {
		// Old server that doesn't support this field and others.
		// Just return the info string as-is.
		return unit.AgentStateInfo
	}
	statusInfo := unit.Agent.Info
	if unit.Agent.Status == params.StatusError {
		if relation, ok := sf.relations[getRelationIdFromData(unit)]; ok {
			// Append the details of the other endpoint on to the status info string.
			if ep, ok := findOtherEndpoint(relation.Endpoints, serviceName); ok {
				statusInfo = statusInfo + " for " + ep.String()
			}
		}
	}
	return adjustInfoIfAgentDown(unit.AgentState, unit.Agent.Status, statusInfo)
}

func (sf *statusFormatter) formatNetwork(network api.NetworkStatus) networkStatus {
	return networkStatus{
		Err:        network.Err,
		ProviderId: network.ProviderId,
		CIDR:       network.CIDR,
		VLANTag:    network.VLANTag,
	}
}

func makeHAStatus(hasVote, wantsVote bool) string {
	var s string
	switch {
	case hasVote && wantsVote:
		s = "has-vote"
	case hasVote && !wantsVote:
		s = "removing-vote"
	case !hasVote && wantsVote:
		s = "adding-vote"
	case !hasVote && !wantsVote:
		s = "no-vote"
	}
	return s
}

func getRelationIdFromData(unit api.UnitStatus) int {
	if relationId_, ok := unit.Agent.Data["relation-id"]; ok {
		if relationId, ok := relationId_.(float64); ok {
			return int(relationId)
		} else {
			logger.Infof("relation-id found status data but was unexpected "+
				"type: %q. Status output may be lacking some detail.", relationId_)
		}
	}
	return -1
}

// findOtherEndpoint searches the provided endpoints for an endpoint
// that *doesn't* match serviceName. The returned bool indicates if
// such an endpoint was found.
func findOtherEndpoint(endpoints []api.EndpointStatus, serviceName string) (api.EndpointStatus, bool) {
	for _, endpoint := range endpoints {
		if endpoint.ServiceName != serviceName {
			return endpoint, true
		}
	}
	return api.EndpointStatus{}, false
}

// adjustInfoIfAgentDown modifies the agent status info string if the
// agent is down. The original status and info is included in
// parentheses.
func adjustInfoIfAgentDown(status, origStatus params.Status, info string) string {
	if status == params.StatusDown {
		if info == "" {
			return fmt.Sprintf("(%s)", origStatus)
		}
		return fmt.Sprintf("(%s: %s)", origStatus, info)
	}
	return info
}
