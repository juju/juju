// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"encoding/json"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

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

type meterStatus struct {
	Color   string `json:"color,omitempty" yaml:"color,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

type unitStatus struct {
	// New Juju Health Status fields.
	WorkloadStatusInfo statusInfoContents `json:"workload-status,omitempty" yaml:"workload-status,omitempty"`
	AgentStatusInfo    statusInfoContents `json:"agent-status,omitempty" yaml:"agent-status,omitempty"`
	MeterStatus        *meterStatus       `json:"meter-status,omitempty" yaml:"meter-status,omitempty"`

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
