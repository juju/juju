// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"encoding/json"
	"fmt"

	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/status"
)

type formattedStatus struct {
	Model        modelStatus                  `json:"model"`
	Machines     map[string]machineStatus     `json:"machines"`
	Applications map[string]applicationStatus `json:"applications"`
}

type formattedMachineStatus struct {
	Model    string                   `json:"model"`
	Machines map[string]machineStatus `json:"machines"`
}

type errorStatus struct {
	StatusError string `json:"status-error" yaml:"status-error"`
}

type modelStatus struct {
	Name             string `json:"name" yaml:"name"`
	Controller       string `json:"controller" yaml:"controller"`
	Cloud            string `json:"cloud" yaml:"cloud"`
	CloudRegion      string `json:"region,omitempty" yaml:"region,omitempty"`
	Version          string `json:"version" yaml:"version"`
	AvailableVersion string `json:"upgrade-available,omitempty" yaml:"upgrade-available,omitempty"`
	Migration        string `json:"migration,omitempty" yaml:"migration,omitempty"`
}

type machineStatus struct {
	Err           error                    `json:"-" yaml:",omitempty"`
	JujuStatus    statusInfoContents       `json:"juju-status,omitempty" yaml:"juju-status,omitempty"`
	DNSName       string                   `json:"dns-name,omitempty" yaml:"dns-name,omitempty"`
	InstanceId    instance.Id              `json:"instance-id,omitempty" yaml:"instance-id,omitempty"`
	MachineStatus statusInfoContents       `json:"machine-status,omitempty" yaml:"machine-status,omitempty"`
	Series        string                   `json:"series,omitempty" yaml:"series,omitempty"`
	Id            string                   `json:"-" yaml:"-"`
	Containers    map[string]machineStatus `json:"containers,omitempty" yaml:"containers,omitempty"`
	Hardware      string                   `json:"hardware,omitempty" yaml:"hardware,omitempty"`
	HAStatus      string                   `json:"controller-member-status,omitempty" yaml:"controller-member-status,omitempty"`
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

func (s machineStatus) MarshalYAML() (interface{}, error) {
	if s.Err != nil {
		return errorStatus{s.Err.Error()}, nil
	}
	return machineStatusNoMarshal(s), nil
}

type applicationStatus struct {
	Err           error                 `json:"-" yaml:",omitempty"`
	Charm         string                `json:"charm" yaml:"charm"`
	Series        string                `json:"series"`
	OS            string                `json:"os"`
	CharmOrigin   string                `json:"charm-origin" yaml:"charm-origin"`
	CharmName     string                `json:"charm-name" yaml:"charm-name"`
	CharmRev      int                   `json:"charm-rev" yaml:"charm-rev"`
	CanUpgradeTo  string                `json:"can-upgrade-to,omitempty" yaml:"can-upgrade-to,omitempty"`
	Exposed       bool                  `json:"exposed" yaml:"exposed"`
	Life          string                `json:"life,omitempty" yaml:"life,omitempty"`
	StatusInfo    statusInfoContents    `json:"application-status,omitempty" yaml:"application-status"`
	Relations     map[string][]string   `json:"relations,omitempty" yaml:"relations,omitempty"`
	SubordinateTo []string              `json:"subordinate-to,omitempty" yaml:"subordinate-to,omitempty"`
	Units         map[string]unitStatus `json:"units,omitempty" yaml:"units,omitempty"`
	Version       string                `json:"version,omitempty" yaml:"version,omitempty"`
}

type applicationStatusNoMarshal applicationStatus

func (s applicationStatus) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return json.Marshal(errorStatus{s.Err.Error()})
	}
	return json.Marshal(applicationStatusNoMarshal(s))
}

func (s applicationStatus) MarshalYAML() (interface{}, error) {
	if s.Err != nil {
		return errorStatus{s.Err.Error()}, nil
	}
	return applicationStatusNoMarshal(s), nil
}

type meterStatus struct {
	Color   string `json:"color,omitempty" yaml:"color,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

type unitStatus struct {
	// New Juju Health Status fields.
	WorkloadStatusInfo statusInfoContents `json:"workload-status,omitempty" yaml:"workload-status"`
	JujuStatusInfo     statusInfoContents `json:"juju-status,omitempty" yaml:"juju-status"`
	MeterStatus        *meterStatus       `json:"meter-status,omitempty" yaml:"meter-status,omitempty"`

	Charm         string                `json:"upgrading-from,omitempty" yaml:"upgrading-from,omitempty"`
	Machine       string                `json:"machine,omitempty" yaml:"machine,omitempty"`
	OpenedPorts   []string              `json:"open-ports,omitempty" yaml:"open-ports,omitempty"`
	PublicAddress string                `json:"public-address,omitempty" yaml:"public-address,omitempty"`
	Subordinates  map[string]unitStatus `json:"subordinates,omitempty" yaml:"subordinates,omitempty"`
}

func (s *formattedStatus) applicationScale(name string) string {
	// The current unit count are units that are either in Idle or Executing status.
	// In other words, units that are active and available.
	currentUnitCount := 0
	desiredUnitCount := 0

	app := s.Applications[name]
	match := func(u unitStatus) {
		desiredUnitCount += 1
		switch u.JujuStatusInfo.Current {
		case status.StatusExecuting, status.StatusIdle:
			currentUnitCount += 1
		}
	}
	// If the app is subordinate to other units, then this is a subordinate charm.
	if len(app.SubordinateTo) > 0 {
		for _, a := range s.Applications {
			for _, u := range a.Units {
				for sub, subStatus := range u.Subordinates {
					if subAppName, _ := names.UnitApplication(sub); subAppName == name {
						match(subStatus)
					}
				}
			}
		}
	} else {
		for _, u := range app.Units {
			match(u)
		}
	}
	return fmt.Sprintf("%d/%d", currentUnitCount, desiredUnitCount)
}

type statusInfoContents struct {
	Err     error         `json:"-" yaml:",omitempty"`
	Current status.Status `json:"current,omitempty" yaml:"current,omitempty"`
	Message string        `json:"message,omitempty" yaml:"message,omitempty"`
	Since   string        `json:"since,omitempty" yaml:"since,omitempty"`
	Version string        `json:"version,omitempty" yaml:"version,omitempty"`
	Life    string        `json:"life,omitempty" yaml:"life,omitempty"`
}

type statusInfoContentsNoMarshal statusInfoContents

func (s statusInfoContents) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return json.Marshal(errorStatus{s.Err.Error()})
	}
	return json.Marshal(statusInfoContentsNoMarshal(s))
}

func (s statusInfoContents) MarshalYAML() (interface{}, error) {
	if s.Err != nil {
		return errorStatus{s.Err.Error()}, nil
	}
	return statusInfoContentsNoMarshal(s), nil
}

type unitStatusNoMarshal unitStatus

func (s unitStatus) MarshalJSON() ([]byte, error) {
	if s.WorkloadStatusInfo.Err != nil {
		return json.Marshal(errorStatus{s.WorkloadStatusInfo.Err.Error()})
	}
	return json.Marshal(unitStatusNoMarshal(s))
}

func (s unitStatus) MarshalYAML() (interface{}, error) {
	if s.WorkloadStatusInfo.Err != nil {
		return errorStatus{s.WorkloadStatusInfo.Err.Error()}, nil
	}
	return unitStatusNoMarshal(s), nil
}
