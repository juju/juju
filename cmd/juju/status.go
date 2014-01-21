// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

type StatusCommand struct {
	cmd.EnvCommandBase
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
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

func (c *StatusCommand) Init(args []string) error {
	c.patterns = args
	return nil
}

var connectionError = `Unable to connect to environment "%s".
Please check your credentials or use 'juju bootstrap' to create a new environment.

Error details:
%v
`

func (c *StatusCommand) getStatus1dot16() (*api.Status, error) {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return nil, fmt.Errorf(connectionError, c.EnvName, err)
	}
	defer conn.Close()

	return statecmd.Status(conn, c.patterns)
}

func (c *StatusCommand) Run(ctx *cmd.Context) error {
	// Just verify the pattern validity client side, do not use the matcher
	_, err := statecmd.NewUnitMatcher(c.patterns)
	if err != nil {
		return err
	}
	apiclient, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return fmt.Errorf(connectionError, c.EnvName, err)
	}
	defer apiclient.Close()

	status, err := apiclient.Status(c.patterns)
	if params.IsCodeNotImplemented(err) {
		logger.Infof("Status not supported by the API server, " +
			"falling back to 1.16 compatibility mode " +
			"(direct DB access)")
		status, err = c.getStatus1dot16()
	}
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
	return out
}

func formatService(service api.ServiceStatus) serviceStatus {
	out := serviceStatus{
		Err:           service.Err,
		Charm:         service.Charm,
		Exposed:       service.Exposed,
		Life:          service.Life,
		Relations:     service.Relations,
		CanUpgradeTo:  service.CanUpgradeTo,
		SubordinateTo: service.SubordinateTo,
		Units:         make(map[string]unitStatus),
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
