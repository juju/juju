// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/environs/network"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/client"
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

func (c *StatusCommand) Run(ctx *cmd.Context) error {
	// Just verify the pattern validity client side, do not use the matcher
	_, err := client.NewUnitMatcher(c.patterns)
	if err != nil {
		return err
	}
	apiclient, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return fmt.Errorf(connectionError, c.EnvName, err)
	}
	defer apiclient.Close()

	status, err := apiclient.Status(c.patterns)
	// Display any error, but continue to print status if some was returned
	if err != nil {
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	}
	result := formatStatus(status)
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

func formatStatus(status *api.Status) formattedStatus {
	if status == nil {
		return formattedStatus{}
	}
	out := formattedStatus{
		Environment: status.EnvironmentName,
		Machines:    make(map[string]machineStatus),
		Services:    make(map[string]serviceStatus),
	}
	for k, m := range status.Machines {
		out.Machines[k] = formatMachine(m)
	}
	for k, s := range status.Services {
		out.Services[k] = formatService(s)
	}
	for k, n := range status.Networks {
		if out.Networks == nil {
			out.Networks = make(map[string]networkStatus)
		}
		out.Networks[k] = formatNetwork(n)
	}
	return out
}

func formatMachine(machine api.MachineStatus) machineStatus {
	out := machineStatus{
		Err:            machine.Err,
		AgentState:     machine.AgentState,
		AgentStateInfo: machine.AgentStateInfo,
		AgentVersion:   machine.AgentVersion,
		DNSName:        machine.DNSName,
		InstanceId:     machine.InstanceId,
		InstanceState:  machine.InstanceState,
		Life:           machine.Life,
		Series:         machine.Series,
		Id:             machine.Id,
		Containers:     make(map[string]machineStatus),
		Hardware:       machine.Hardware,
	}
	for k, m := range machine.Containers {
		out.Containers[k] = formatMachine(m)
	}

	for _, job := range machine.Jobs {
		if job == params.JobManageEnviron {
			out.HAStatus = makeHAStatus(machine.HasVote, machine.WantsVote)
			break
		}
	}
	return out
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

func formatService(service api.ServiceStatus) serviceStatus {
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
		out.Units[k] = formatUnit(m)
	}
	return out
}

func formatUnit(unit api.UnitStatus) unitStatus {
	out := unitStatus{
		Err:            unit.Err,
		AgentState:     unit.AgentState,
		AgentStateInfo: unit.AgentStateInfo,
		AgentVersion:   unit.AgentVersion,
		Life:           unit.Life,
		Machine:        unit.Machine,
		OpenedPorts:    unit.OpenedPorts,
		PublicAddress:  unit.PublicAddress,
		Charm:          unit.Charm,
		Subordinates:   make(map[string]unitStatus),
	}
	for k, m := range unit.Subordinates {
		out.Subordinates[k] = formatUnit(m)
	}
	return out
}

func formatNetwork(network api.NetworkStatus) networkStatus {
	return networkStatus{
		Err:        network.Err,
		ProviderId: network.ProviderId,
		CIDR:       network.CIDR,
		VLANTag:    network.VLANTag,
	}
}
