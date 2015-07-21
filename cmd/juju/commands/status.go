// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
)

type StatusCommand struct {
	envcmd.EnvCommandBase
	out      cmd.Output
	patterns []string
	isoTime  bool
}

var statusDoc = `
This command will report on the runtime state of various system entities.

There are a number of ways to format the status output:

- {short|line|oneline}: List units and their subordinates. For each
           unit, the IP address and agent status are listed.
- summary: Displays the subnet(s) and port(s) the environment utilizes.
           Also displays aggregate information about:
           - MACHINES: total #, and # in each state.
           - UNITS: total #, and # in each state.
           - SERVICES: total #, and # exposed of each service.
- tabular: Displays information in a tabular format in these sections:
           - Machines: ID, STATE, VERSION, DNS, INS-ID, SERIES, HARDWARE
           - Services: NAME, EXPOSED, CHARM
           - Units: ID, STATE, VERSION, MACHINE, PORTS, PUBLIC-ADDRESS
             - Also displays subordinate units.
- yaml (DEFAULT): Displays information on machines, services, and units
                  in the yaml format.

Service or unit names may be specified to filter the status to only those
services and units that match, along with the related machines, services
and units. If a subordinate unit is matched, then its principal unit will
be displayed. If a principal unit is matched, then all of its subordinates
will be displayed.

Wildcards ('*') may be specified in service/unit names to match any sequence
of characters. For example, 'nova-*' will match any service whose name begins
with 'nova-': 'nova-compute', 'nova-volume', etc.
`

// RegisterUnitComponentFormatter registers a formatting function for the given component.  When status is returned from the API, unitstatus for the compoentn
func RegisterUnitComponentFormatter(component string, fn func(apiobj interface{}) interface{}) {
	unitComponentFormatters[component] = fn
}

var unitComponentFormatters = map[string]func(interface{}) interface{}{}

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
	f.BoolVar(&c.isoTime, "utc", false, "display time as UTC in RFC3339 format")

	defaultFormat := "yaml"
	if c.CompatVersion() > 1 {
		defaultFormat = "tabular"
	}
	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"short":   FormatOneline,
		"oneline": FormatOneline,
		"line":    FormatOneline,
		"tabular": FormatTabular,
		"summary": FormatSummary,
	})
}

func (c *StatusCommand) Init(args []string) error {
	c.patterns = args
	// If use of ISO time not specified on command line,
	// check env var.
	if !c.isoTime {
		var err error
		envVarValue := os.Getenv(osenv.JujuStatusIsoTimeEnvKey)
		if envVarValue != "" {
			if c.isoTime, err = strconv.ParseBool(envVarValue); err != nil {
				return errors.Annotatef(err, "invalid %s env var, expected true|false", osenv.JujuStatusIsoTimeEnvKey)
			}
		}
	}
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
	} else if status == nil {
		return errors.Errorf("unable to obtain the current status")
	}

	result := newStatusFormatter(status, c.CompatVersion(), c.isoTime).format()
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
	// goyaml version. // TODO(jw4) however verify that gccgo does not
	// complain about symbol already defined.
	type mNoMethods machineStatus
	return "", mNoMethods(s)
}

type serviceStatus struct {
	Err           error                 `json:"-" yaml:",omitempty"`
	Charm         string                `json:"charm" yaml:"charm"`
	CanUpgradeTo  string                `json:"can-upgrade-to,omitempty" yaml:"can-upgrade-to,omitempty"`
	Exposed       bool                  `json:"exposed" yaml:"exposed"`
	Life          string                `json:"life,omitempty" yaml:"life,omitempty"`
	StatusInfo    statusInfoContents    `json:"service-status,omitempty" yaml:"service-status,omitempty"`
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
	type ssNoMethods serviceStatus
	return json.Marshal(ssNoMethods(s))
}

func (s serviceStatus) GetYAML() (tag string, value interface{}) {
	if s.Err != nil {
		return "", errorStatus{s.Err.Error()}
	}
	type ssNoMethods serviceStatus
	return "", ssNoMethods(s)
}

type unitStatus struct {
	// New Juju Health Status fields.
	WorkloadStatusInfo statusInfoContents     `json:"workload-status,omitempty" yaml:"workload-status,omitempty"`
	AgentStatusInfo    statusInfoContents     `json:"agent-status,omitempty" yaml:"agent-status,omitempty"`
	Components         map[string]interface{} `json:"components,omitempty", yaml:"components,omitempty"`

	// Legacy status fields, to be removed in Juju 2.0
	AgentState     params.Status `json:"agent-state,omitempty" yaml:"agent-state,omitempty"`
	AgentStateInfo string        `json:"agent-state-info,omitempty" yaml:"agent-state-info,omitempty"`
	Err            error         `json:"-" yaml:",omitempty"`
	AgentVersion   string        `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
	Life           string        `json:"life,omitempty" yaml:"life,omitempty"`

	Charm         string                `json:"upgrading-from,omitempty" yaml:"upgrading-from,omitempty"`
	Machine       string                `json:"machine,omitempty" yaml:"machine,omitempty"`
	OpenedPorts   []string              `json:"open-ports,omitempty" yaml:"open-ports,omitempty"`
	PublicAddress string                `json:"public-address,omitempty" yaml:"public-address,omitempty"`
	Subordinates  map[string]unitStatus `json:"subordinates,omitempty" yaml:"subordinates,omitempty"`
}

type statusInfoContents struct {
	Err     error         `json:"-" yaml:",omitempty"`
	Current params.Status `json:"current,omitempty" yaml:"current,omitempty"`
	Message string        `json:"message,omitempty" yaml:"message,omitempty"`
	Since   string        `json:"since,omitempty" yaml:"since,omitempty"`
	Version string        `json:"version,omitempty" yaml:"version,omitempty"`
}

type statusInfoContentsNoMarshal statusInfoContents

func (s statusInfoContents) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return json.Marshal(errorStatus{s.Err.Error()})
	}
	return json.Marshal(statusInfoContentsNoMarshal(s))
}

func (s statusInfoContents) GetYAML() (tag string, value interface{}) {
	if s.Err != nil {
		return "", errorStatus{s.Err.Error()}
	}
	type sicNoMethods statusInfoContents
	return "", sicNoMethods(s)
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
	type usNoMethods unitStatus
	return "", usNoMethods(s)
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
	status        *api.Status
	relations     map[int]api.RelationStatus
	isoTime       bool
	compatVersion int
}

func newStatusFormatter(status *api.Status, compatVersion int, isoTime bool) *statusFormatter {
	sf := statusFormatter{
		status:        status,
		relations:     make(map[int]api.RelationStatus),
		compatVersion: compatVersion,
		isoTime:       isoTime,
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
			AgentStateInfo: adjustInfoIfMachineAgentDown(machine.AgentState, agent.Status, agent.Info),
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
		if job == multiwatcher.JobManageEnviron {
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
		StatusInfo:    sf.getServiceStatusInfo(service),
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

func (sf *statusFormatter) getServiceStatusInfo(service api.ServiceStatus) statusInfoContents {
	info := statusInfoContents{
		Err:     service.Status.Err,
		Current: service.Status.Status,
		Message: service.Status.Info,
		Version: service.Status.Version,
	}
	if service.Status.Since != nil {
		info.Since = formatStatusTime(service.Status.Since, sf.isoTime)
	}
	return info
}

func (sf *statusFormatter) formatUnit(unit api.UnitStatus, serviceName string) unitStatus {
	// TODO(Wallyworld) - this should be server side but we still need to support older servers.
	sf.updateUnitStatusInfo(&unit, serviceName)

	out := unitStatus{
		WorkloadStatusInfo: sf.getWorkloadStatusInfo(unit),
		AgentStatusInfo:    sf.getAgentStatusInfo(unit),
		Machine:            unit.Machine,
		OpenedPorts:        unit.OpenedPorts,
		PublicAddress:      unit.PublicAddress,
		Charm:              unit.Charm,
		Subordinates:       make(map[string]unitStatus),
		Components:         map[string]interface{}{},
	}

	for component, val := range unit.ComponentStatus {
		if fn, ok := unitComponentFormatters[component]; ok {
			out.Components[component] = fn(val)
		}
	}

	// These legacy fields will be dropped for Juju 2.0.
	if sf.compatVersion < 2 || out.AgentStatusInfo.Current == "" {
		out.Err = unit.Err
		out.AgentState = unit.AgentState
		out.AgentStateInfo = unit.AgentStateInfo
		out.Life = unit.Life
		out.AgentVersion = unit.AgentVersion
	}

	for k, m := range unit.Subordinates {
		out.Subordinates[k] = sf.formatUnit(m, serviceName)
	}
	return out
}

func (sf *statusFormatter) getWorkloadStatusInfo(unit api.UnitStatus) statusInfoContents {
	info := statusInfoContents{
		Err:     unit.Workload.Err,
		Current: unit.Workload.Status,
		Message: unit.Workload.Info,
		Version: unit.Workload.Version,
	}
	if unit.Workload.Since != nil {
		info.Since = formatStatusTime(unit.Workload.Since, sf.isoTime)
	}
	return info
}

func (sf *statusFormatter) getAgentStatusInfo(unit api.UnitStatus) statusInfoContents {
	info := statusInfoContents{
		Err:     unit.UnitAgent.Err,
		Current: unit.UnitAgent.Status,
		Message: unit.UnitAgent.Info,
		Version: unit.UnitAgent.Version,
	}
	if unit.UnitAgent.Since != nil {
		info.Since = formatStatusTime(unit.UnitAgent.Since, sf.isoTime)
	}
	return info
}

func (sf *statusFormatter) updateUnitStatusInfo(unit *api.UnitStatus, serviceName string) {
	// This logic has no business here but can't be moved until Juju 2.0.
	statusInfo := unit.Workload.Info
	if unit.Workload.Status == "" {
		// Old server that doesn't support this field and others.
		// Just use the info string as-is.
		statusInfo = unit.AgentStateInfo
	}
	if unit.Workload.Status == params.StatusError {
		if relation, ok := sf.relations[getRelationIdFromData(unit)]; ok {
			// Append the details of the other endpoint on to the status info string.
			if ep, ok := findOtherEndpoint(relation.Endpoints, serviceName); ok {
				unit.Workload.Info = statusInfo + " for " + ep.String()
				unit.AgentStateInfo = unit.Workload.Info
			}
		}
	}
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

func getRelationIdFromData(unit *api.UnitStatus) int {
	if relationId_, ok := unit.Workload.Data["relation-id"]; ok {
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

// adjustInfoIfMachineAgentDown modifies the agent status info string if the
// agent is down. The original status and info is included in
// parentheses.
func adjustInfoIfMachineAgentDown(status, origStatus params.Status, info string) string {
	if status == params.StatusDown {
		if info == "" {
			return fmt.Sprintf("(%s)", origStatus)
		}
		return fmt.Sprintf("(%s: %s)", origStatus, info)
	}
	return info
}
